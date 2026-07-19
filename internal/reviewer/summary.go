package reviewer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/model"
)

// RunSummary executes the summarization pipeline: fetch diffs, ask the model
// for a structured summary, and render the result. Unlike Run, it never fails
// on findings — it always returns 0.
func (r *Reviewer) RunSummary(ctx context.Context) (int, error) {
	// Step 1: Get diffs.
	slog.Info("fetching diffs for summary", "mode", r.cfg.Mode())
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
		slog.Info("no files to summarize after filtering")
		fmt.Println("✅ No reviewable files in diff.")
		return 0, nil
	}

	// Step 3: Build numbered diff and prompts.
	numberedDiff := buildNumberedDiff(diffs)
	systemPrompt := model.BuildSummaryPrompt()
	userPrompt := model.BuildSummaryUserPrompt(mrTitle, mrDesc, numberedDiff)

	// Step 4: Call the model.
	sp, ok := r.provider.(model.SummarizeProvider)
	if !ok {
		return 0, fmt.Errorf("provider does not support summarize mode")
	}

	slog.Info("requesting summary from model")
	result, err := sp.Summarize(ctx, systemPrompt, userPrompt)
	if err != nil {
		return 0, fmt.Errorf("model summarize: %w", err)
	}

	if result.Usage != nil && result.Usage.TotalTokens > 0 {
		slog.Info("token usage",
			"input", result.Usage.InputTokens,
			"output", result.Usage.OutputTokens,
			"total", result.Usage.TotalTokens,
		)
	}

	// Step 5: Output.
	if r.cfg.DryRun || !r.cfg.CIMode {
		if r.cfg.OutputJSON {
			jsonOut, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return 0, fmt.Errorf("marshaling JSON output: %w", err)
			}
			fmt.Println(string(jsonOut))
		} else {
			useColor := !r.cfg.NoColor && isTTY()
			fmt.Print(formatSummaryTerminal(result, useColor))
		}
	} else {
		// Post to GitLab as an MR comment.
		note := formatSummaryMarkdown(result)
		if _, err := r.glClient.PostNote(ctx, r.cfg.CIProjectID, r.cfg.CIMergeRequestID, note); err != nil {
			return 0, fmt.Errorf("posting summary note: %w", err)
		}
		slog.Info("posted MR summary to GitLab")
	}

	return 0, nil
}

// formatSummaryTerminal renders a SummaryResult for terminal display with
// optional ANSI color.
func formatSummaryTerminal(result *model.SummaryResult, useColor bool) string {
	var sb strings.Builder

	// Header.
	if useColor {
		sb.WriteString("\n" + ansiBold + ansiCyan)
	}
	sb.WriteString("📋 MR Summary\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	if useColor {
		sb.WriteString(ansiReset)
	}
	sb.WriteString("\n")

	// Type + Confidence.
	typeStr := result.Classification
	confidence := result.Confidence
	if useColor {
		sb.WriteString(ansiDim + "  Type: " + ansiReset + ansiBold)
	} else {
		sb.WriteString("  Type: ")
	}
	sb.WriteString(typeStr)
	if useColor {
		sb.WriteString(ansiReset + ansiDim)
	}
	fmt.Fprintf(&sb, " (%d%% confidence)", int(confidence*100))
	if useColor {
		sb.WriteString(ansiReset)
	}
	sb.WriteString("\n")

	// Risk.
	riskColor := riskLevelColor(result.RiskLevel)
	if useColor {
		sb.WriteString(ansiDim + "  Risk: " + ansiReset + riskColor + ansiBold)
	} else {
		sb.WriteString("  Risk: ")
	}
	sb.WriteString(result.RiskLevel)
	if useColor {
		sb.WriteString(ansiReset)
	}
	sb.WriteString("\n")

	// Scope.
	if len(result.ScopeAreas) > 0 {
		if useColor {
			sb.WriteString(ansiDim + "  Scope: " + ansiReset)
		} else {
			sb.WriteString("  Scope: ")
		}
		sb.WriteString(strings.Join(result.ScopeAreas, ", "))
		sb.WriteString("\n")
	}

	sb.WriteString("\n")

	// Title.
	if useColor {
		sb.WriteString("  " + ansiBold + ansiWhite)
	} else {
		sb.WriteString("  Title: ")
	}
	sb.WriteString(result.Title)
	if useColor {
		sb.WriteString(ansiReset)
	}
	sb.WriteString("\n")

	// Description.
	if result.Description != "" {
		sb.WriteString("\n")
		if useColor {
			sb.WriteString(ansiDim + "  Description:" + ansiReset + "\n")
		} else {
			sb.WriteString("  Description:\n")
		}
		for _, line := range strings.Split(result.Description, "\n") {
			sb.WriteString("  " + line + "\n")
		}
	}

	// Intent.
	if result.Intent != "" {
		sb.WriteString("\n")
		if useColor {
			sb.WriteString(ansiDim + "  Intent: " + ansiReset)
		} else {
			sb.WriteString("  Intent: ")
		}
		sb.WriteString(result.Intent)
		sb.WriteString("\n")
	}

	// Breaking changes.
	sb.WriteString("\n")
	if len(result.BreakingChanges) > 0 {
		if useColor {
			sb.WriteString("  " + ansiYellow + "⚠️  Breaking Changes:" + ansiReset + "\n")
		} else {
			sb.WriteString("  ⚠️ Breaking Changes:\n")
		}
		for _, bc := range result.BreakingChanges {
			sb.WriteString("  • " + bc + "\n")
		}
	} else {
		if useColor {
			sb.WriteString("  " + ansiGreen + "⚠️  Breaking Changes: None" + ansiReset + "\n")
		} else {
			sb.WriteString("  ⚠️ Breaking Changes: None\n")
		}
	}

	sb.WriteString("\n")
	return sb.String()
}

// riskLevelColor returns the ANSI color for a risk level.
func riskLevelColor(risk string) string {
	switch strings.ToLower(risk) {
	case "critical":
		return ansiRed
	case "high":
		return ansiOrange
	case "medium":
		return ansiYellow
	case "low":
		return ansiGreen
	default:
		return ansiWhite
	}
}

// formatSummaryMarkdown renders a SummaryResult as a GitLab-flavored Markdown
// comment suitable for posting as an MR note.
func formatSummaryMarkdown(result *model.SummaryResult) string {
	var sb strings.Builder

	sb.WriteString("## 📋 MR Summary\n\n")

	// Metadata table.
	sb.WriteString("| | |\n")
	sb.WriteString("|---|---|\n")
	fmt.Fprintf(&sb, "| **Type** | `%s` (%d%% confidence) |\n",
		result.Classification, int(result.Confidence*100))
	fmt.Fprintf(&sb, "| **Risk** | %s %s |\n",
		riskEmoji(result.RiskLevel), result.RiskLevel)
	if len(result.ScopeAreas) > 0 {
		fmt.Fprintf(&sb, "| **Scope** | %s |\n",
			strings.Join(result.ScopeAreas, ", "))
	}
	sb.WriteString("\n")

	// Title.
	fmt.Fprintf(&sb, "### %s\n\n", result.Title)

	// Description.
	if result.Description != "" {
		sb.WriteString(result.Description)
		sb.WriteString("\n\n")
	}

	// Intent.
	if result.Intent != "" {
		fmt.Fprintf(&sb, "**Intent:** %s\n\n", result.Intent)
	}

	// Breaking changes.
	if len(result.BreakingChanges) > 0 {
		sb.WriteString("### ⚠️ Breaking Changes\n\n")
		for _, bc := range result.BreakingChanges {
			sb.WriteString("- " + bc + "\n")
		}
	} else {
		sb.WriteString("**Breaking Changes:** None\n")
	}

	return sb.String()
}

// riskEmoji returns a colored emoji for a risk level.
func riskEmoji(risk string) string {
	switch strings.ToLower(risk) {
	case "critical":
		return "🔴"
	case "high":
		return "🟠"
	case "medium":
		return "🟡"
	case "low":
		return "🟢"
	default:
		return "⚪"
	}
}
