package reviewer

import (
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
