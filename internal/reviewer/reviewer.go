// Package reviewer orchestrates the code review pipeline:
// input (diffs) → model (AI analysis) → output (terminal or GitLab).
package reviewer

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"

	"github.com/OpticDiff/code-reviewer/internal/config"
	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/gitlab"
	"github.com/OpticDiff/code-reviewer/internal/model"
)

// Reviewer orchestrates the full review pipeline.
type Reviewer struct {
	cfg      *config.Config
	provider *model.Provider
	glClient *gitlab.Client
}

// New creates a new Reviewer.
func New(cfg *config.Config, provider *model.Provider, glClient *gitlab.Client) *Reviewer {
	return &Reviewer{
		cfg:      cfg,
		provider: provider,
		glClient: glClient,
	}
}

// Run executes the full review pipeline and returns the number of findings.
func (r *Reviewer) Run(ctx context.Context) (int, error) {
	// Step 1: Get diffs.
	slog.Info("fetching diffs", "mode", r.cfg.Mode())
	diffs, mrTitle, mrDesc, err := r.getDiffs(ctx)
	if err != nil {
		return 0, fmt.Errorf("getting diffs: %w", err)
	}
	slog.Info(fmt.Sprintf("found %d file(s) in diff", len(diffs)))

	// Step 2: Filter excluded files.
	diffs = diff.Filter(diffs, r.cfg.ExcludedPatterns)
	slog.Info(fmt.Sprintf("%d file(s) after filtering", len(diffs)))

	if len(diffs) == 0 {
		slog.Info("no files to review after filtering")
		fmt.Println("✅ No reviewable files in diff.")
		return 0, nil
	}

	// Step 3: Check context window / chunk.
	tokenLimit := diff.TokenLimitForModel(r.cfg.Model)
	chunker, err := diff.NewChunkStrategy(string(r.cfg.ChunkStrategy))
	if err != nil {
		return 0, err
	}

	chunks, err := chunker.Chunk(diffs, tokenLimit)
	if err != nil {
		return 0, err
	}
	slog.Info(fmt.Sprintf("review split into %d chunk(s)", len(chunks)))

	// Step 4: Build prompt and call model for each chunk.
	systemPrompt := model.BuildPrompt(r.cfg.Focus, r.cfg.ExtraRules)
	var allFindings []model.Finding
	var summary string

	for i, chunk := range chunks {
		slog.Info(fmt.Sprintf("reviewing chunk %d/%d (%d files, ~%d tokens)",
			i+1, len(chunks), len(chunk), diff.EstimateTokens(chunk)))

		numberedDiff := buildNumberedDiff(chunk)
		userPrompt := model.BuildUserPrompt(mrTitle, mrDesc, numberedDiff)

		result, err := r.provider.Review(ctx, systemPrompt, userPrompt)
		if err != nil {
			return 0, fmt.Errorf("model review (chunk %d): %w", i+1, err)
		}

		if summary == "" {
			summary = result.Summary
		}
		allFindings = append(allFindings, result.Findings...)
	}

	// Step 5: Validate line references.
	allFindings = ValidateFindings(allFindings, diffs)

	// Step 6: Filter by severity.
	allFindings = filterBySeverity(allFindings, r.cfg.MinSeverity)
	slog.Info(fmt.Sprintf("%d finding(s) at or above %s severity", len(allFindings), r.cfg.MinSeverity))

	result := &model.ReviewResult{
		Summary:  summary,
		Findings: allFindings,
	}

	// Step 7: Output.
	if r.cfg.DryRun || !r.cfg.CIMode {
		// Terminal output.
		fmt.Print(TerminalOutput(result))
	} else {
		// Post to GitLab.
		var version *gitlab.DiffVersion
		if r.cfg.CommentMode == config.CommentModeDiscussions {
			versions, err := r.glClient.GetMRVersions(ctx, r.cfg.CIProjectID, r.cfg.CIMergeRequestID)
			if err != nil {
				slog.Warn("could not fetch MR versions, inline comments may fail", "error", err)
			} else if len(versions) > 0 {
				version = &versions[0]
			}
		}

		if err := PostToGitLab(ctx, r.cfg, r.glClient, result, version); err != nil {
			return len(allFindings), fmt.Errorf("posting to GitLab: %w", err)
		}
	}

	return len(allFindings), nil
}

func (r *Reviewer) getDiffs(ctx context.Context) ([]diff.FileDiff, string, string, error) {
	switch {
	case r.cfg.CIMode:
		return r.getCIDiffs(ctx)
	case r.cfg.DiffMode:
		return r.getLocalDiffs()
	case len(r.cfg.Files) > 0:
		return r.getFileDiffs()
	default:
		return nil, "", "", fmt.Errorf("no input mode specified")
	}
}

func (r *Reviewer) getCIDiffs(ctx context.Context) ([]diff.FileDiff, string, string, error) {
	mr, err := r.glClient.GetMRChanges(ctx, r.cfg.CIProjectID, r.cfg.CIMergeRequestID)
	if err != nil {
		return nil, "", "", err
	}

	// Check if draft and should skip.
	if r.cfg.SkipDraftMRs && mr.Draft {
		return nil, "", "", fmt.Errorf("skipping draft MR")
	}

	// Parse each file's diff.
	var diffs []diff.FileDiff
	for _, change := range mr.Changes {
		parsed, err := diff.Parse(strings.NewReader("diff --git a/" + change.OldPath + " b/" + change.NewPath + "\n" + change.Diff))
		if err != nil {
			slog.Warn("failed to parse diff for file", "file", change.NewPath, "error", err)
			continue
		}
		diffs = append(diffs, parsed...)
	}

	return diffs, mr.Title, mr.Description, nil
}

func (r *Reviewer) getLocalDiffs() ([]diff.FileDiff, string, string, error) {
	ref := r.cfg.DiffRef
	if ref == "" {
		ref = "origin/HEAD"
	}

	cmd := exec.Command("git", "diff", "-U5", "--merge-base", ref)
	output, err := cmd.Output()
	if err != nil {
		// Fallback: try without --merge-base.
		cmd = exec.Command("git", "diff", "-U5", ref)
		output, err = cmd.Output()
		if err != nil {
			return nil, "", "", fmt.Errorf("running git diff: %w", err)
		}
	}

	diffs, err := diff.Parse(strings.NewReader(string(output)))
	if err != nil {
		return nil, "", "", fmt.Errorf("parsing git diff: %w", err)
	}

	return diffs, "", "", nil
}

func (r *Reviewer) getFileDiffs() ([]diff.FileDiff, string, string, error) {
	args := append([]string{"diff", "-U5", "HEAD", "--"}, r.cfg.Files...)
	cmd := exec.Command("git", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, "", "", fmt.Errorf("running git diff for files: %w", err)
	}

	diffs, err := diff.Parse(strings.NewReader(string(output)))
	if err != nil {
		return nil, "", "", fmt.Errorf("parsing file diffs: %w", err)
	}

	return diffs, "", "", nil
}

// buildNumberedDiff creates a numbered diff representation for the model prompt.
// Line numbers help the model reference specific lines accurately.
func buildNumberedDiff(diffs []diff.FileDiff) string {
	var sb strings.Builder

	for _, d := range diffs {
		path := d.NewPath
		if path == "" {
			path = d.OldPath
		}
		fmt.Fprintf(&sb, "=== File: %s ===\n", path)

		for _, h := range d.Hunks {
			sb.WriteString(h.Header + "\n")
			for _, l := range h.Lines {
				prefix := " "
				lineNo := l.NewLineNo
				switch l.Type {
				case diff.LineAdded:
					prefix = "+"
					lineNo = l.NewLineNo
				case diff.LineRemoved:
					prefix = "-"
					lineNo = l.OldLineNo
				case diff.LineContext:
					lineNo = l.NewLineNo
				}
				fmt.Fprintf(&sb, "%4d %s %s\n", lineNo, prefix, l.Content)
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

func filterBySeverity(findings []model.Finding, minSeverity config.Severity) []model.Finding {
	if minSeverity == config.SeverityLow {
		return findings // No filtering needed.
	}

	var filtered []model.Finding
	for _, f := range findings {
		sev, err := config.ParseSeverity(f.Severity)
		if err != nil {
			// Include findings with unknown severity.
			filtered = append(filtered, f)
			continue
		}
		if sev >= minSeverity {
			filtered = append(filtered, f)
		}
	}
	return filtered
}
