package reviewer

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/OpticDiff/code-reviewer/internal/config"
	"github.com/OpticDiff/code-reviewer/internal/gitlab"
	"github.com/OpticDiff/code-reviewer/internal/model"
)

// TerminalOutput formats findings as colored markdown for terminal display.
func TerminalOutput(result *model.ReviewResult) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "# Review Summary\n%s\n\n", result.Summary)

	if len(result.Findings) == 0 {
		sb.WriteString("✅ No issues found. Code looks clean and ready to merge.\n")
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

	return sb.String()
}

// PostToGitLab posts review results to a GitLab merge request.
func PostToGitLab(ctx context.Context, cfg *config.Config, client *gitlab.Client, result *model.ReviewResult, version *gitlab.DiffVersion) error {
	projectID := cfg.CIProjectID
	mrIID := cfg.CIMergeRequestID

	// Clean previous bot comments.
	deleted, err := client.CleanPreviousReviews(ctx, projectID, mrIID)
	if err != nil {
		slog.Warn("failed to clean previous reviews", "error", err)
	} else if deleted > 0 {
		slog.Info(fmt.Sprintf("cleaned %d previous bot comment(s)", deleted))
	}

	// Post summary note.
	summary := formatSummaryNote(result)
	if _, err := client.PostNote(ctx, projectID, mrIID, summary); err != nil {
		return fmt.Errorf("posting summary: %w", err)
	}
	slog.Info("posted review summary")

	// In discussions mode, also post inline comments.
	if cfg.CommentMode == config.CommentModeDiscussions && version != nil {
		posted := 0
		for _, f := range result.Findings {
			newLine := f.Line
			req := gitlab.CreateDiscussionRequest{
				Body: formatInlineComment(f),
				Position: &gitlab.DiscussionPosition{
					PositionType: "text",
					BaseSHA:      version.BaseSHA,
					HeadSHA:      version.HeadSHA,
					StartSHA:     version.StartSHA,
					NewPath:      f.File,
					OldPath:      f.File,
					NewLine:      &newLine,
				},
			}

			if err := client.CreateDiscussion(ctx, projectID, mrIID, req); err != nil {
				slog.Warn("failed to create inline discussion, posting as note instead",
					"file", f.File,
					"line", f.Line,
					"error", err,
				)
				// Fallback: post as a regular note.
				noteBody := fmt.Sprintf("**%s:%d** — %s", f.File, f.Line, formatInlineComment(f))
				if _, err := client.PostNote(ctx, projectID, mrIID, noteBody); err != nil {
					slog.Error("failed to post fallback note", "error", err)
				}
			} else {
				posted++
			}

			time.Sleep(100 * time.Millisecond) // Rate limit.
		}
		slog.Info(fmt.Sprintf("posted %d inline discussion(s)", posted))
	}

	return nil
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
