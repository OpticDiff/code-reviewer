package reviewer

import (
	"context"
	"fmt"
	"strings"

	"github.com/OpticDiff/code-reviewer/internal/config"
	"github.com/OpticDiff/code-reviewer/internal/model"
	"github.com/OpticDiff/code-reviewer/internal/vcs"
)

// TerminalOutput formats findings as colored markdown for terminal display.
func TerminalOutput(result *model.ReviewResult) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "# Review Summary\n%s\n\n", result.Summary)

	if len(result.Findings) == 0 {
		sb.WriteString("✅ No issues found. Code looks clean and ready to merge.\n")
		if result.Usage != nil && result.Usage.TotalTokens > 0 {
			fmt.Fprintf(&sb, "---\nTokens: %d in / %d out / %d total\n",
				result.Usage.InputTokens, result.Usage.OutputTokens, result.Usage.TotalTokens)
		}
		return sb.String()
	}

	fmt.Fprintf(&sb, "Found **%d** issue(s):\n\n", len(result.Findings))

	// Group by file.
	byFile := make(map[string][]model.Finding)
	var fileOrder []string
	for _, f := range result.Findings {
		if _, seen := byFile[f.File]; !seen {
			fileOrder = append(fileOrder, f.File)
		}
		byFile[f.File] = append(byFile[f.File], f)
	}

	for _, file := range fileOrder {
		fmt.Fprintf(&sb, "## File: %s\n", file)
		for _, f := range byFile[file] {
			fmt.Fprintf(&sb, "### L%d: [%s] %s\n", f.Line, f.Severity, f.Title)
			sb.WriteString(f.Body + "\n")
			if f.Suggestion != "" {
				fmt.Fprintf(&sb, "\n```suggestion\n%s\n```\n", f.Suggestion)
			}
			sb.WriteString("\n")
		}
	}

	if result.Usage != nil && result.Usage.TotalTokens > 0 {
		fmt.Fprintf(&sb, "---\nTokens: %d in / %d out / %d total\n",
			result.Usage.InputTokens, result.Usage.OutputTokens, result.Usage.TotalTokens)
	}

	return sb.String()
}

// PostReview posts review results to a GitLab merge request.
func PostReview(ctx context.Context, cfg *config.Config, client VCSClient, result *model.ReviewResult, version *vcs.DiffVersion) error {
	req := vcs.SubmitReviewRequest{
		Summary:     formatSummaryNote(result),
		Version:     version,
		CleanupMode: string(cfg.CleanupMode),
	}

	if cfg.CommentMode == config.CommentModeDiscussions && version != nil {
		for _, f := range result.Findings {
			req.Comments = append(req.Comments, vcs.ReviewComment{
				Path: f.File,
				Line: f.Line,
				Body: formatInlineComment(f),
			})
		}
	}

	return client.SubmitReview(ctx, cfg.CIProjectID, cfg.CIMergeRequestID, req)
}

func formatSummaryNote(result *model.ReviewResult) string {
	var sb strings.Builder

	sb.WriteString("## 📋 Code Review Summary\n\n")
	sb.WriteString(result.Summary)
	sb.WriteString("\n\n")

	if len(result.Findings) == 0 {
		sb.WriteString("✅ No issues found.\n")
		return sb.String()
	}

	// Count by severity.
	counts := make(map[string]int)
	for _, f := range result.Findings {
		counts[f.Severity]++
	}

	sb.WriteString("### Findings\n\n")
	sb.WriteString("| Severity | Count |\n|---|---|\n")
	for _, sev := range []string{"CRITICAL", "HIGH", "MEDIUM", "LOW"} {
		if c, ok := counts[sev]; ok {
			fmt.Fprintf(&sb, "| %s %s | %d |\n", severityEmoji(sev), sev, c)
		}
	}
	sb.WriteString("\n")

	// List findings.
	for _, f := range result.Findings {
		fmt.Fprintf(&sb, "- %s **[%s]** `%s:%d` — %s\n", severityEmoji(f.Severity), f.Severity, f.File, f.Line, f.Title)
	}

	return sb.String()
}

func formatInlineComment(f model.Finding) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s **[%s]** %s\n\n", severityEmoji(f.Severity), f.Severity, f.Title)
	sb.WriteString(f.Body)
	if f.Suggestion != "" {
		fmt.Fprintf(&sb, "\n\n```suggestion\n%s\n```", f.Suggestion)
	}
	return sb.String()
}

func severityEmoji(severity string) string {
	switch strings.ToUpper(severity) {
	case "CRITICAL":
		return "🔴"
	case "HIGH":
		return "🟠"
	case "MEDIUM":
		return "🟡"
	case "LOW":
		return "🔵"
	default:
		return "⚪"
	}
}
