package reviewer

import (
	"strings"
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/model"
)

// ---------------------------------------------------------------------------
// ColorTerminalOutput tests (new colored output)
// ---------------------------------------------------------------------------

func TestColorTerminalOutput_NoColor_FallsBackToPlain(t *testing.T) {
	result := &model.ReviewResult{
		Summary: "test",
		Findings: []model.Finding{
			{File: "main.go", Line: 1, Severity: "HIGH", Title: "Bug", Body: "desc"},
		},
	}
	// useColor=false should produce output identical to TerminalOutput.
	out := ColorTerminalOutput(result, false)
	plain := TerminalOutput(result)
	if out != plain {
		t.Error("ColorTerminalOutput(false) should equal TerminalOutput")
	}
}

func TestColorTerminalOutput_WithColor_HasANSI(t *testing.T) {
	result := &model.ReviewResult{
		Summary: "Found issues.",
		Findings: []model.Finding{
			{File: "main.go", Line: 42, Severity: "CRITICAL", Title: "SQL Injection", Body: "User input in query.", Suggestion: "use parameterized queries"},
			{File: "main.go", Line: 55, Severity: "HIGH", Title: "Resource leak", Body: "Unclosed body."},
			{File: "config.go", Line: 10, Severity: "MEDIUM", Title: "Missing validation", Body: "No input check."},
			{File: "util.go", Line: 5, Severity: "LOW", Title: "Naming", Body: "Use better name."},
		},
	}
	out := ColorTerminalOutput(result, true)

	// Verify ANSI codes are present.
	if !strings.Contains(out, "\033[") {
		t.Error("expected ANSI escape codes in colored output")
	}

	// Verify box drawing.
	if !strings.Contains(out, "┌") {
		t.Error("expected box-drawing top border")
	}
	if !strings.Contains(out, "└") {
		t.Error("expected box-drawing bottom border")
	}

	// Verify severity badges.
	if !strings.Contains(out, "CRITICAL") {
		t.Error("expected CRITICAL severity in output")
	}
	if !strings.Contains(out, "🔴") {
		t.Error("expected 🔴 in severity counts")
	}

	// Verify file separators.
	if !strings.Contains(out, "── main.go") {
		t.Error("expected file separator for main.go")
	}

	// Verify suggestion block.
	if !strings.Contains(out, "Suggestion:") {
		t.Error("expected 'Suggestion:' label")
	}

	// Verify summary counts.
	if !strings.Contains(out, "1 critical") {
		t.Error("expected '1 critical' in summary")
	}
	if !strings.Contains(out, "1 high") {
		t.Error("expected '1 high' in summary")
	}
}

func TestColorTerminalOutput_NoFindings_ShowsClean(t *testing.T) {
	result := &model.ReviewResult{
		Summary:  "Clean code.",
		Findings: nil,
	}
	out := ColorTerminalOutput(result, true)
	if !strings.Contains(out, "No issues found") {
		t.Error("expected 'No issues found' in colored empty output")
	}
	if !strings.Contains(out, "┌") {
		t.Error("expected box drawing even with no findings")
	}
}

// ---------------------------------------------------------------------------
// formatSummaryNote tests (GitLab posting)
// ---------------------------------------------------------------------------

func TestFormatSummaryNote_WithFindings(t *testing.T) {
	result := &model.ReviewResult{
		Summary: "Mixed issues.",
		Findings: []model.Finding{
			{File: "a.go", Line: 1, Severity: "CRITICAL", Title: "critical", Body: "c"},
			{File: "a.go", Line: 2, Severity: "HIGH", Title: "high", Body: "h"},
			{File: "a.go", Line: 3, Severity: "HIGH", Title: "high2", Body: "h2"},
			{File: "a.go", Line: 4, Severity: "LOW", Title: "low", Body: "l"},
		},
	}
	out := formatSummaryNote(result)

	if !strings.Contains(out, "📋 Code Review Summary") {
		t.Error("expected summary header")
	}
	if !strings.Contains(out, "CRITICAL") {
		t.Error("expected CRITICAL in table")
	}
	// HIGH should show count 2.
	if !strings.Contains(out, "2") {
		t.Error("expected count of 2 for HIGH")
	}
}

func TestFormatSummaryNote_NoFindings(t *testing.T) {
	result := &model.ReviewResult{
		Summary:  "All clean.",
		Findings: nil,
	}
	out := formatSummaryNote(result)
	if !strings.Contains(out, "No issues found") {
		t.Error("expected 'No issues found' in summary note")
	}
}

// ---------------------------------------------------------------------------
// Helper function tests
// ---------------------------------------------------------------------------

func TestSeverityEmoji_AllCases(t *testing.T) {
	tests := []struct {
		severity string
		emoji    string
	}{
		{"CRITICAL", "🔴"},
		{"critical", "🔴"},
		{"HIGH", "🟠"},
		{"high", "🟠"},
		{"MEDIUM", "🟡"},
		{"LOW", "🔵"},
		{"UNKNOWN", "⚪"},
		{"", "⚪"},
	}
	for _, tt := range tests {
		t.Run(tt.severity, func(t *testing.T) {
			got := severityEmoji(tt.severity)
			if got != tt.emoji {
				t.Errorf("severityEmoji(%q) = %q, want %q", tt.severity, got, tt.emoji)
			}
		})
	}
}

func TestSeverityColor(t *testing.T) {
	tests := []struct {
		severity string
		color    string
	}{
		{"CRITICAL", ansiRed},
		{"HIGH", ansiOrange},
		{"MEDIUM", ansiYellow},
		{"LOW", ansiBlue},
		{"UNKNOWN", ansiWhite},
	}
	for _, tt := range tests {
		t.Run(tt.severity, func(t *testing.T) {
			got := severityColor(tt.severity)
			if got != tt.color {
				t.Errorf("severityColor(%q) = %q, want %q", tt.severity, got, tt.color)
			}
		})
	}
}

func TestFormatInlineComment_WithoutSuggestion(t *testing.T) {
	f := model.Finding{
		Severity: "HIGH",
		Title:    "Resource leak",
		Body:     "File handle not closed.",
	}
	out := formatInlineComment(f)
	if !strings.Contains(out, "🟠") {
		t.Error("expected HIGH emoji")
	}
	if !strings.Contains(out, "Resource leak") {
		t.Error("expected title")
	}
	if strings.Contains(out, "```suggestion") {
		t.Error("should not have suggestion block when suggestion is empty")
	}
}

func TestFormatInlineComment_WithSuggestion(t *testing.T) {
	f := model.Finding{
		Severity:   "CRITICAL",
		Title:      "SQL injection",
		Body:       "Raw concat.",
		Suggestion: "db.Query(\"SELECT * FROM t WHERE id = ?\", id)",
	}
	out := formatInlineComment(f)
	if !strings.Contains(out, "```suggestion") {
		t.Error("expected suggestion code block")
	}
	if !strings.Contains(out, "db.Query") {
		t.Error("expected suggestion content")
	}
}

func TestColorTerminalOutput_TokenUsage(t *testing.T) {
	result := &model.ReviewResult{
		Summary:  "Review done.",
		Findings: []model.Finding{{File: "a.go", Line: 1, Severity: "LOW", Category: "style", Title: "test", Body: "body"}},
		Usage:    model.TokenUsage{InputTokens: 1500, OutputTokens: 200, TotalTokens: 1700},
	}
	out := ColorTerminalOutput(result, true)
	if !strings.Contains(out, "1500") {
		t.Error("expected input token count in output")
	}
	if !strings.Contains(out, "200") {
		t.Error("expected output token count in output")
	}
	if !strings.Contains(out, "1700") {
		t.Error("expected total token count in output")
	}
}

func TestColorTerminalOutput_NoTokenUsage(t *testing.T) {
	result := &model.ReviewResult{
		Summary:  "Review done.",
		Findings: nil,
	}
	out := ColorTerminalOutput(result, true)
	if strings.Contains(out, "Tokens:") {
		t.Error("should not show token line when usage is zero")
	}
}
