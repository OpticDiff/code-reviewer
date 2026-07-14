// Package reviewer orchestrates the code review pipeline:
// input (diffs) → model (AI analysis) → output (terminal or GitLab).
package reviewer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/OpticDiff/code-reviewer/internal/config"
	ctxpkg "github.com/OpticDiff/code-reviewer/internal/context"
	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/model"
	"github.com/OpticDiff/code-reviewer/internal/vcs"
)

// DiffSource provides diffs for review. Extracted for testability.
type DiffSource interface {
	GetDiffs(ctx context.Context) ([]diff.FileDiff, string, string, error)
}

// Reviewer orchestrates the full review pipeline.
type Reviewer struct {
	cfg             *config.Config
	provider        ModelReviewer
	glClient        VCSClient
	diffSource      DiffSource
	contextProvider ctxpkg.Provider
}

// New creates a new Reviewer.
func New(cfg *config.Config, provider ModelReviewer, glClient VCSClient) *Reviewer {
	return &Reviewer{
		cfg:      cfg,
		provider: provider,
		glClient: glClient,
	}
}

// NewWithContext creates a Reviewer with a context provider for repo-aware reviews.
func NewWithContext(cfg *config.Config, provider ModelReviewer, glClient VCSClient, cp ctxpkg.Provider) *Reviewer {
	return &Reviewer{
		cfg:             cfg,
		provider:        provider,
		glClient:        glClient,
		contextProvider: cp,
	}
}

// NewWithDiffSource creates a Reviewer with a custom DiffSource (useful for testing).
func NewWithDiffSource(cfg *config.Config, provider ModelReviewer, glClient VCSClient, ds DiffSource) *Reviewer {
	return &Reviewer{
		cfg:        cfg,
		provider:   provider,
		glClient:   glClient,
		diffSource: ds,
	}
}

// Run executes the full review pipeline and returns the number of findings.
func (r *Reviewer) Run(ctx context.Context) (int, error) {
	// Step 1: Get diffs.
	slog.Info("fetching diffs", "mode", r.cfg.Mode())
	var diffs []diff.FileDiff
	var mrTitle, mrDesc string
	var err error
	var cachedVersions []vcs.DiffVersion
	if r.diffSource != nil {
		diffs, mrTitle, mrDesc, err = r.diffSource.GetDiffs(ctx)
	} else {
		diffs, mrTitle, mrDesc, err = r.getDiffs(ctx)
	}
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

	// Step 2b: Incremental review — filter to only files changed in latest push.
	if r.cfg.Incremental && r.cfg.CIMode && r.glClient != nil {
		versions, verr := r.glClient.GetMRVersions(ctx, r.cfg.CIProjectID, r.cfg.CIMergeRequestID)
		if verr == nil {
			cachedVersions = versions
		}
		if verr != nil {
			slog.Warn("failed to get MR versions for incremental review, falling back to full review", "error", verr)
		} else if len(versions) > 1 {
			// Compare previous version's head to current version's head.
			prevHead := versions[1].HeadSHA
			currHead := versions[0].HeadSHA
			changedFiles, cerr := r.glClient.CompareCommits(ctx, r.cfg.CIProjectID, prevHead, currHead)
			if cerr != nil {
				slog.Warn("failed to compare commits for incremental review, falling back to full review", "error", cerr)
			} else {
				before := len(diffs)
				diffs = filterByFiles(diffs, changedFiles)
				slog.Info("incremental review",
					"total_files", before,
					"changed_files", len(changedFiles),
					"reviewing", len(diffs),
				)
			}
		} else {
			slog.Info("first push to MR, performing full review")
		}
	}

	if len(diffs) == 0 {
		slog.Info("no files to review after incremental filtering")
		fmt.Println("✅ No reviewable files changed in latest push.")
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

	// Step 3b: Discover related code context.
	var contextSnippets []model.ContextSnippet
	if r.contextProvider != nil {
		repoRoot := findRepoRoot()
		if repoRoot != "" {
			raw, cerr := r.contextProvider.FindRelatedCode(ctx, repoRoot, diffs)
			if cerr != nil {
				slog.Warn("context discovery failed, continuing without context", "error", cerr)
			} else if len(raw) > 0 {
				for _, s := range raw {
					contextSnippets = append(contextSnippets, model.ContextSnippet{
						File:    s.File,
						Line:    s.Line,
						Content: s.Content,
						Symbol:  s.Symbol,
					})
				}
				slog.Info(fmt.Sprintf("found %d related code snippet(s)", len(contextSnippets)))
			}
		}
	}

	// Step 4: Build prompt and call model for each chunk.
	systemPrompt := model.BuildPromptWithCustom(r.cfg.CustomPrompt, r.cfg.Focus, r.cfg.ExtraRules)
	var allFindings []model.Finding
	var summary string
	var totalUsage model.TokenUsage

	for i, chunk := range chunks {
		slog.Info(fmt.Sprintf("reviewing chunk %d/%d (%d files, ~%d tokens)",
			i+1, len(chunks), len(chunk), diff.EstimateTokens(chunk)))

		numberedDiff := buildNumberedDiff(chunk)
		userPrompt := model.BuildUserPromptWithContext(mrTitle, mrDesc, numberedDiff, contextSnippets)

		result, err := r.provider.Review(ctx, systemPrompt, userPrompt)
		if err != nil {
			return 0, fmt.Errorf("model review (chunk %d): %w", i+1, err)
		}

		if summary == "" {
			summary = result.Summary
		}
		allFindings = append(allFindings, result.Findings...)
		if result.Usage != nil {
			totalUsage.InputTokens += result.Usage.InputTokens
			totalUsage.OutputTokens += result.Usage.OutputTokens
			totalUsage.TotalTokens += result.Usage.TotalTokens
		}
	}

	if totalUsage.TotalTokens > 0 {
		slog.Info("token usage",
			"input", totalUsage.InputTokens,
			"output", totalUsage.OutputTokens,
			"total", totalUsage.TotalTokens,
		)
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
	if totalUsage.TotalTokens > 0 {
		result.Usage = &totalUsage
	}

	// Step 7a: Write SARIF if requested (before posting to GitLab so it's not
	// skipped when PostToGitLab fails).
	if r.cfg.SARIFOutput != "" {
		if err := WriteSARIF(r.cfg.SARIFOutput, result); err != nil {
			return len(allFindings), fmt.Errorf("writing SARIF: %w", err)
		}
		slog.Info("SARIF output written", "path", r.cfg.SARIFOutput)
	}

	// Step 7b: Output.
	if r.cfg.DryRun || !r.cfg.CIMode {
		if r.cfg.OutputJSON {
			jsonOut, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return 0, fmt.Errorf("marshaling JSON output: %w", err)
			}
			fmt.Println(string(jsonOut))
		} else {
			useColor := !r.cfg.NoColor && isTTY()
			fmt.Print(ColorTerminalOutput(result, useColor))
		}
	} else {
		// Post to GitLab.
		var version *vcs.DiffVersion
		if r.cfg.CommentMode == config.CommentModeDiscussions {
			if len(cachedVersions) > 0 {
				version = &cachedVersions[0]
			} else {
				versions, err := r.glClient.GetMRVersions(ctx, r.cfg.CIProjectID, r.cfg.CIMergeRequestID)
				if err != nil {
					slog.Warn("could not fetch MR versions, inline comments may fail", "error", err)
				} else if len(versions) > 0 {
					version = &versions[0]
				}
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

	// Prevent command injection: reject refs that look like flags.
	if strings.HasPrefix(ref, "-") {
		return nil, "", "", fmt.Errorf("invalid diff ref %q: must not start with '-'", ref)
	}

	// Use '--' to terminate flag parsing, preventing ref from being interpreted as a git option.
	cmd := exec.Command("git", "diff", "-U5", "--merge-base", ref, "--")
	output, err := cmd.Output()
	if err != nil {
		// Fallback: try without --merge-base.
		cmd = exec.Command("git", "diff", "-U5", ref, "--")
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
	// Validate file paths: reject any that look like flags.
	for _, f := range r.cfg.Files {
		if strings.HasPrefix(f, "-") {
			return nil, "", "", fmt.Errorf("invalid file path %q: must not start with '-'", f)
		}
	}

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

// filterByFiles returns only diffs whose file path matches one of the changed files.
func filterByFiles(diffs []diff.FileDiff, changedFiles []string) []diff.FileDiff {
	changed := make(map[string]bool, len(changedFiles))
	for _, f := range changedFiles {
		changed[f] = true
	}
	var filtered []diff.FileDiff
	for _, d := range diffs {
		path := d.NewPath
		if path == "" {
			path = d.OldPath
		}
		if changed[path] {
			filtered = append(filtered, d)
		}
	}
	return filtered
}

// findRepoRoot walks up from cwd to find the git repository root.
func findRepoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
