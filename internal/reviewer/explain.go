package reviewer

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"unicode"

	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/model"
)

// RunExplain executes the explain pipeline: fetch diffs, ask the model to
// explain what the changes do, and render the result. Always returns 0.
func (r *Reviewer) RunExplain(ctx context.Context) (int, error) {
	// Step 1: Get diffs.
	slog.Info("fetching diffs for explain", "mode", r.cfg.Mode())
	var diffs []diff.FileDiff
	var mrTitle, mrDesc string
	var err error
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
		slog.Info("no files to explain after filtering")
		fmt.Println("✅ No reviewable files in diff.")
		return 0, nil
	}

	// Step 3: Build numbered diff and prompts.
	numberedDiff := buildNumberedDiff(diffs)
	systemPrompt := model.BuildExplainPrompt()
	userPrompt := model.BuildExplainUserPrompt(mrTitle, mrDesc, numberedDiff)

	// Step 4: Call the model.
	ep, ok := r.provider.(model.ExplainProvider)
	if !ok {
		return 0, fmt.Errorf("provider does not support explain mode")
	}

	slog.Info("requesting explanation from model")
	explanation, usage, err := ep.Explain(ctx, systemPrompt, userPrompt)
	if err != nil {
		return 0, fmt.Errorf("model explain: %w", err)
	}

	if usage != nil && usage.TotalTokens > 0 {
		slog.Info("token usage",
			"input", usage.InputTokens,
			"output", usage.OutputTokens,
			"total", usage.TotalTokens,
		)
	}

	// Step 5: Output.
	if r.cfg.DryRun || !r.cfg.CIMode {
		useColor := !r.cfg.NoColor && isTTY()
		fmt.Print(formatExplainTerminal(explanation, useColor))
	} else {
		// Post to GitLab as an MR comment.
		note := formatExplainMarkdown(explanation)
		if _, err := r.glClient.PostNote(ctx, r.cfg.CIProjectID, r.cfg.CIMergeRequestID, note); err != nil {
			return 0, fmt.Errorf("posting explain note: %w", err)
		}
		slog.Info("posted explanation to GitLab")
	}

	return 0, nil
}

// formatExplainTerminal renders an explanation for terminal display.
func formatExplainTerminal(explanation string, useColor bool) string {
	var sb strings.Builder

	// Header.
	if useColor {
		sb.WriteString("\n" + ansiBold + ansiCyan)
	}
	sb.WriteString("🔍 Explanation\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	if useColor {
		sb.WriteString(ansiReset)
	}
	sb.WriteString("\n\n")

	sb.WriteString(stripControlChars(strings.TrimSpace(explanation)))
	sb.WriteString("\n\n")

	return sb.String()
}

// formatExplainMarkdown renders an explanation as a GitLab-flavored Markdown
// comment suitable for posting as an MR note.
func formatExplainMarkdown(explanation string) string {
	var sb strings.Builder
	sb.WriteString("## 🔍 Diff Explanation\n\n")
	sb.WriteString(strings.TrimSpace(explanation))
	sb.WriteString("\n")
	return sb.String()
}

// stripControlChars removes terminal control characters (OSC, CSI, etc.)
// from untrusted model output while preserving newlines and tabs.
func stripControlChars(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r == '\r' {
			return r
		}
		if unicode.IsControl(r) {
			return -1 // Drop.
		}
		return r
	}, s)
}
