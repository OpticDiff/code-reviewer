package reviewer

import (
	"fmt"
	"strings"

	"github.com/OpticDiff/code-reviewer/internal/model"
)

// formatIntentOneLiner renders a compact one-line intent summary for terminal output.
func formatIntentOneLiner(s *model.SummaryResult, useColor bool) string {
	if s == nil {
		return ""
	}
	risk := s.RiskLevel
	scope := strings.Join(s.ScopeAreas, ", ")
	if scope == "" {
		scope = "general"
	}
	if useColor {
		return fmt.Sprintf("\033[1m⚡ Intent:\033[0m %s · %s · Risk: %s · Scope: %s\n\n",
			s.Classification, s.Intent, risk, scope)
	}
	return fmt.Sprintf("⚡ Intent: %s · %s · Risk: %s · Scope: %s\n\n",
		s.Classification, s.Intent, risk, scope)
}

// formatIntentMarkdown renders an intent summary as a Markdown table for CI comments.
func formatIntentMarkdown(s *model.SummaryResult) string {
	if s == nil {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("### 🎯 Inferred Intent\n\n")
	sb.WriteString("| Field | Value |\n")
	sb.WriteString("|---|---|\n")
	sb.WriteString(fmt.Sprintf("| **Classification** | `%s` |\n", s.Classification))
	sb.WriteString(fmt.Sprintf("| **Intent** | %s |\n", s.Intent))
	sb.WriteString(fmt.Sprintf("| **Risk Level** | %s %s |\n", riskEmoji(s.RiskLevel), s.RiskLevel))
	if len(s.ScopeAreas) > 0 {
		areas := make([]string, len(s.ScopeAreas))
		for i, a := range s.ScopeAreas {
			areas[i] = "`" + a + "`"
		}
		sb.WriteString(fmt.Sprintf("| **Scope** | %s |\n", strings.Join(areas, ", ")))
	}
	if len(s.BreakingChanges) > 0 {
		sb.WriteString(fmt.Sprintf("| **Breaking Changes** | %s |\n", strings.Join(s.BreakingChanges, "; ")))
	}
	sb.WriteString("\n---\n\n")
	return sb.String()
}
