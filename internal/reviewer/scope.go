package reviewer

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/OpticDiff/code-reviewer/internal/diff"
)

// ScopeAssessment holds the result of a pre-flight scope check.
type ScopeAssessment struct {
	FileCount   int
	Threshold   int
	IsOversized bool
}

// CheckScope evaluates whether the diff exceeds configured size thresholds.
func CheckScope(diffs []diff.FileDiff, maxFiles int) *ScopeAssessment {
	assessment := &ScopeAssessment{
		FileCount: len(diffs),
		Threshold: maxFiles,
	}
	if maxFiles > 0 && len(diffs) > maxFiles {
		assessment.IsOversized = true
	}
	return assessment
}

// FormatScopeWarning returns a formatted warning string for oversized MRs.
func FormatScopeWarning(a *ScopeAssessment) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "⚠️  Scope warning: %d files changed (threshold: %d).\n", a.FileCount, a.Threshold)
	sb.WriteString("    Review quality degrades significantly beyond this threshold.\n")
	sb.WriteString("    Consider splitting this MR into smaller, focused changes.\n")
	return sb.String()
}

// FormatScopeMarkdown returns a markdown-formatted scope warning for CI comments.
func FormatScopeMarkdown(a *ScopeAssessment) string {
	return fmt.Sprintf(
		"> ⚠️ **Scope Warning**: This MR changes %d files (threshold: %d). "+
			"Review quality degrades significantly beyond this threshold. "+
			"Consider splitting into smaller, focused changes.\n\n",
		a.FileCount, a.Threshold,
	)
}

// LogScopeStatus logs the scope assessment result.
func LogScopeStatus(a *ScopeAssessment) {
	if a.IsOversized {
		slog.Warn("MR exceeds scope threshold",
			"files", a.FileCount,
			"threshold", a.Threshold,
		)
	} else {
		slog.Info("scope check passed",
			"files", a.FileCount,
			"threshold", a.Threshold,
		)
	}
}
