package reviewer

import (
	"strings"
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/model"
)

func testDiffs() []diff.FileDiff {
	return []diff.FileDiff{
		{
			NewPath: "internal/handler.go",
			Hunks: []diff.Hunk{{
				NewStart: 10,
				NewCount: 5,
				Lines: []diff.DiffLine{
					{Type: diff.LineContext, NewLineNo: 10, Content: "ctx := r.Context()"},
					{Type: diff.LineAdded, NewLineNo: 11, Content: "if id == \"\" {"},
					{Type: diff.LineAdded, NewLineNo: 12, Content: "    return"},
					{Type: diff.LineAdded, NewLineNo: 13, Content: "}"},
					{Type: diff.LineContext, NewLineNo: 14, Content: "result, err := fetch()"},
				},
			}},
		},
	}
}

func TestValidateFindings_ValidLines(t *testing.T) {
	findings := []model.Finding{
		{File: "internal/handler.go", Line: 11, Severity: "HIGH", Title: "valid finding"},
		{File: "internal/handler.go", Line: 12, Severity: "MEDIUM", Title: "valid finding 2"},
	}
	result := ValidateFindings(findings, testDiffs())
	if len(result) != 2 {
		t.Errorf("expected 2 valid findings, got %d", len(result))
	}
}

func TestValidateFindings_InvalidLine(t *testing.T) {
	findings := []model.Finding{
		{File: "internal/handler.go", Line: 999, Severity: "HIGH", Title: "hallucinated line"},
	}
	result := ValidateFindings(findings, testDiffs())
	if len(result) != 0 {
		t.Errorf("expected 0 findings (hallucinated line dropped), got %d", len(result))
	}
}

func TestValidateFindings_InvalidFile(t *testing.T) {
	findings := []model.Finding{
		{File: "nonexistent.go", Line: 1, Severity: "HIGH", Title: "wrong file"},
	}
	result := ValidateFindings(findings, testDiffs())
	if len(result) != 0 {
		t.Errorf("expected 0 findings (wrong file dropped), got %d", len(result))
	}
}

func TestValidateFindings_ContextLine(t *testing.T) {
	// Line 10 is a context line (not added/removed) but is in hunk range.
	findings := []model.Finding{
		{File: "internal/handler.go", Line: 10, Severity: "LOW", Title: "context line"},
	}
	result := ValidateFindings(findings, testDiffs())
	// Context lines in hunk range are kept as notes.
	if len(result) != 1 {
		t.Errorf("expected 1 finding (context line in hunk kept), got %d", len(result))
	}
}

func TestValidateFindings_EmptyInput(t *testing.T) {
	result := ValidateFindings(nil, testDiffs())
	if len(result) != 0 {
		t.Errorf("expected 0 findings for nil input, got %d", len(result))
	}
}

func TestPathMatch(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"internal/handler.go", "internal/handler.go", true},
		{"a/internal/handler.go", "internal/handler.go", true},
		{"internal/handler.go", "a/internal/handler.go", true},
		{"handler.go", "service.go", false},
	}
	for _, tt := range tests {
		got := pathMatch(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("pathMatch(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestIsInHunkRange(t *testing.T) {
	diffs := []diff.FileDiff{
		{
			NewPath: "main.go",
			Hunks: []diff.Hunk{
				{NewStart: 10, NewCount: 5},
				{NewStart: 30, NewCount: 3},
			},
		},
	}

	tests := []struct {
		name     string
		file     string
		line     int
		expected bool
	}{
		{"start of hunk 1", "main.go", 10, true},
		{"within hunk 1", "main.go", 14, true},
		{"past hunk 1", "main.go", 15, false},
		{"start of hunk 2", "main.go", 30, true},
		{"within hunk 2", "main.go", 32, true},
		{"past hunk 2", "main.go", 33, false},
		{"before any hunk", "main.go", 1, false},
		{"wrong file", "other.go", 10, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isInHunkRange(tt.file, tt.line, diffs)
			if got != tt.expected {
				t.Errorf("isInHunkRange(%q, %d) = %v, want %v", tt.file, tt.line, got, tt.expected)
			}
		})
	}
}

func TestValidateFindings_NegativeLine(t *testing.T) {
	findings := []model.Finding{
		{File: "internal/handler.go", Line: -1, Severity: "LOW", Title: "bad line"},
	}
	result := ValidateFindings(findings, testDiffs())
	if len(result) != 0 {
		t.Errorf("expected 0 findings (negative line dropped), got %d", len(result))
	}
}

func TestValidateFindings_PartialPathMatch(t *testing.T) {
	findings := []model.Finding{
		{File: "handler.go", Line: 11, Severity: "HIGH", Title: "partial path"},
	}
	result := ValidateFindings(findings, testDiffs())
	// "handler.go" should match "internal/handler.go" via suffix matching.
	if len(result) != 1 {
		t.Errorf("expected 1 finding (partial path matched), got %d", len(result))
	}
	if len(result) == 1 && result[0].File != "internal/handler.go" {
		t.Errorf("expected file to be normalized to 'internal/handler.go', got %q", result[0].File)
	}
}

func TestSanitizeSuggestions_StripCodeFences(t *testing.T) {
	findings := []model.Finding{
		{Suggestion: "```go\nreturn nil, err\n```"},
		{Suggestion: "```\nfmt.Println(x)\n```"},
	}
	result := sanitizeSuggestions(findings)
	if result[0].Suggestion != "return nil, err" {
		t.Errorf("expected fences stripped, got %q", result[0].Suggestion)
	}
	if result[1].Suggestion != "fmt.Println(x)" {
		t.Errorf("expected fences stripped, got %q", result[1].Suggestion)
	}
}

func TestSanitizeSuggestions_StripDiffMarkers(t *testing.T) {
	findings := []model.Finding{
		{Suggestion: "+ return nil, err\n+ if x > 0 {"},
	}
	result := sanitizeSuggestions(findings)
	if !strings.Contains(result[0].Suggestion, "return nil, err") {
		t.Errorf("expected code preserved, got %q", result[0].Suggestion)
	}
	if strings.HasPrefix(strings.TrimSpace(result[0].Suggestion), "+ ") {
		t.Errorf("expected diff markers stripped, got %q", result[0].Suggestion)
	}
}

func TestSanitizeSuggestions_PreserveNegativeNumbers(t *testing.T) {
	findings := []model.Finding{
		{Suggestion: "-1"},
		{Suggestion: "x := -42"},
		{Suggestion: "++i"},
	}
	result := sanitizeSuggestions(findings)
	if result[0].Suggestion != "-1" {
		t.Errorf("negative number corrupted, got %q", result[0].Suggestion)
	}
	if result[1].Suggestion != "x := -42" {
		t.Errorf("negative assignment corrupted, got %q", result[1].Suggestion)
	}
	if result[2].Suggestion != "++i" {
		t.Errorf("increment corrupted, got %q", result[2].Suggestion)
	}
}

func TestSanitizeSuggestions_StripExplanatoryPrefix(t *testing.T) {
	findings := []model.Finding{
		{Suggestion: "Fix: return nil, err"},
		{Suggestion: "suggestion: x := 1"},
	}
	result := sanitizeSuggestions(findings)
	if result[0].Suggestion != "return nil, err" {
		t.Errorf("expected prefix stripped, got %q", result[0].Suggestion)
	}
	if result[1].Suggestion != "x := 1" {
		t.Errorf("expected prefix stripped, got %q", result[1].Suggestion)
	}
}

func TestSanitizeSuggestions_ClearProse(t *testing.T) {
	findings := []model.Finding{
		{File: "test.go", Title: "test", Suggestion: "Use parameterized queries instead of string concatenation."},
		{File: "test.go", Title: "test", Suggestion: "Consider using a mutex instead of a channel for this case."},
	}
	result := sanitizeSuggestions(findings)
	if result[0].Suggestion != "" {
		t.Errorf("expected prose cleared, got %q", result[0].Suggestion)
	}
	if result[1].Suggestion != "" {
		t.Errorf("expected prose cleared, got %q", result[1].Suggestion)
	}
}

func TestSanitizeSuggestions_PreserveValidCode(t *testing.T) {
	findings := []model.Finding{
		{Suggestion: "db.Query(\"SELECT * FROM t WHERE id = ?\", id)"},
		{Suggestion: "if err != nil {\n\treturn nil, fmt.Errorf(\"failed: %w\", err)\n}"},
		{Suggestion: "x := 1"},
	}
	result := sanitizeSuggestions(findings)
	if result[0].Suggestion != "db.Query(\"SELECT * FROM t WHERE id = ?\", id)" {
		t.Errorf("expected valid code preserved, got %q", result[0].Suggestion)
	}
	if !strings.Contains(result[1].Suggestion, "return nil, fmt.Errorf") {
		t.Errorf("expected multiline code preserved, got %q", result[1].Suggestion)
	}
	if result[2].Suggestion != "x := 1" {
		t.Errorf("expected short code preserved, got %q", result[2].Suggestion)
	}
}

func TestStripCodeFences(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"with language", "```go\ncode\n```", "code"},
		{"without language", "```\ncode\n```", "code"},
		{"no fences", "code", "code"},
		{"single line", "```code```", "```code```"},
		{"multiline content", "```py\nline1\nline2\n```", "line1\nline2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripCodeFences(tt.input)
			if got != tt.want {
				t.Errorf("stripCodeFences(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestLooksLikeProse(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		// Should be detected as prose (2+ prose words, no code chars).
		{"Use a mutex instead of a channel for this.", true},
		{"Consider refactoring the handler to use middleware instead.", true},
		// Should NOT be detected as prose.
		{"return nil, err", false},
		{"db.Query(\"SELECT 1\")", false},
		{"x := 1", false},
		{"short", false},                            // too short
		{"if err != nil {", false},                   // has code chars
		{"delete(theMap, key)", false},               // has code chars despite 'the'
		{"reuse the cached version", false},          // only 1 prose word match
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := looksLikeProse(tt.input)
			if got != tt.want {
				t.Errorf("looksLikeProse(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeSuggestions_EmptyAndWhitespace(t *testing.T) {
	findings := []model.Finding{
		{Suggestion: ""},
		{Suggestion: "   "},
		{Suggestion: "```go\n\n```"},
	}
	result := sanitizeSuggestions(findings)
	if result[0].Suggestion != "" {
		t.Errorf("expected empty to stay empty, got %q", result[0].Suggestion)
	}
	if result[1].Suggestion != "" {
		t.Errorf("expected whitespace-only to be trimmed, got %q", result[1].Suggestion)
	}
	if result[2].Suggestion != "" {
		t.Errorf("expected empty fenced block to be cleared, got %q", result[2].Suggestion)
	}
}

func TestValidateFindings_DropsMojibakeSuggestion(t *testing.T) {
	const src = "const LIGATURES = \"ﬀﬁﬂﬃﬄﬅﬆ\"; // soft\u00adhyphen 🚀 日本語"
	diffs := []diff.FileDiff{{
		NewPath: "lib/text.go",
		Hunks: []diff.Hunk{{
			NewStart: 1, NewCount: 2,
			Lines: []diff.DiffLine{
				{Type: diff.LineAdded, NewLineNo: 1, Content: src},
				{Type: diff.LineAdded, NewLineNo: 2, Content: "var x = 1"},
			},
		}},
	}}
	mojibake := latin1Mojibake(src)
	if mojibake == src {
		t.Fatal("fixture did not produce mojibake")
	}

	tests := []struct {
		name       string
		suggestion string
		want       string
	}{
		{"intact text is kept", src, src},
		{"mojibake of source is dropped", mojibake, ""},
		{"new non-ASCII text is kept", `const GREETING = "café 日本語"`, `const GREETING = "café 日本語"`},
		{"ASCII is kept", "var y = 2", "var y = 2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateFindings([]model.Finding{{File: "lib/text.go", Line: 1, Suggestion: tt.suggestion}}, diffs)
			if len(got) != 1 {
				t.Fatalf("finding must survive, got %d findings", len(got))
			}
			if got[0].Suggestion != tt.want {
				t.Errorf("suggestion = %q, want %q", got[0].Suggestion, tt.want)
			}
		})
	}
}

// latin1Mojibake re-encodes s the way a byte-wise Latin-1 decode would.
func latin1Mojibake(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		sb.WriteRune(rune(s[i]))
	}
	return sb.String()
}

func removedLineDiffs() []diff.FileDiff {
	return []diff.FileDiff{
		{
			OldPath: "internal/auth.go",
			NewPath: "internal/auth.go",
			Hunks: []diff.Hunk{{
				OldStart: 20,
				OldCount: 4,
				NewStart: 20,
				NewCount: 2,
				Lines: []diff.DiffLine{
					{Type: diff.LineContext, OldLineNo: 20, NewLineNo: 20, Content: "func check(u User) error {"},
					{Type: diff.LineRemoved, OldLineNo: 21, Content: "if !u.Admin {"},
					{Type: diff.LineRemoved, OldLineNo: 22, Content: "    return ErrForbidden"},
					{Type: diff.LineRemoved, OldLineNo: 23, Content: "}"},
					{Type: diff.LineContext, OldLineNo: 24, NewLineNo: 21, Content: "return nil"},
				},
			}},
		},
	}
}

func TestValidateFindings_RemovedLineKept(t *testing.T) {
	findings := []model.Finding{
		{File: "internal/auth.go", OldLine: 21, Severity: "HIGH", Title: "removed guard"},
	}
	result := ValidateFindings(findings, removedLineDiffs())
	if len(result) != 1 {
		t.Fatalf("expected removed-line finding kept, got %d", len(result))
	}
	if result[0].OldLine != 21 || result[0].Line != 0 {
		t.Errorf("got line=%d old_line=%d, want line=0 old_line=21", result[0].Line, result[0].OldLine)
	}
}

func TestValidateFindings_RemovedLineClearsNewLine(t *testing.T) {
	// A model that sets both fields for a removed line gets anchored on the old side only.
	findings := []model.Finding{
		{File: "internal/auth.go", Line: 21, OldLine: 22, Severity: "HIGH", Title: "removed guard"},
	}
	result := ValidateFindings(findings, removedLineDiffs())
	if len(result) != 1 || result[0].Line != 0 || result[0].OldLine != 22 {
		t.Fatalf("got %+v, want one finding with line=0 old_line=22", result)
	}
}

func TestValidateFindings_OldLineNotRemovedDropped(t *testing.T) {
	// old_line 20 is an unchanged line, 999 is outside the diff: neither is a removed line.
	for _, old := range []int{20, 999} {
		findings := []model.Finding{
			{File: "internal/auth.go", OldLine: old, Severity: "HIGH", Title: "bad old line"},
		}
		if result := ValidateFindings(findings, removedLineDiffs()); len(result) != 0 {
			t.Errorf("old_line %d: expected finding dropped, got %d", old, len(result))
		}
	}
}

func TestValidateFindings_OldLineIgnoredWhenNewLineValid(t *testing.T) {
	// Added line 11 is valid; a stray old_line must not turn it into a removed-line finding.
	findings := []model.Finding{
		{File: "internal/handler.go", Line: 11, OldLine: 5, Severity: "HIGH", Title: "added"},
	}
	result := ValidateFindings(findings, testDiffs())
	if len(result) != 1 || result[0].Line != 11 || result[0].OldLine != 0 {
		t.Fatalf("got %+v, want line=11 old_line=0", result)
	}
}
