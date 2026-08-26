package reviewer

import (
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/model"
)

func TestDedup_ExactMatch(t *testing.T) {
	findings := []model.Finding{
		{File: "auth.go", Line: 10, Category: "bug", Severity: "HIGH", Body: "short"},
		{File: "auth.go", Line: 10, Category: "bug", Severity: "HIGH", Body: "longer explanation here"},
	}
	result := DeduplicateFindings(findings)
	if len(result) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(result))
	}
	if result[0].Body != "longer explanation here" {
		t.Errorf("expected longer body to be kept, got %q", result[0].Body)
	}
}

func TestDedup_LineProximity(t *testing.T) {
	findings := []model.Finding{
		{File: "auth.go", Line: 10, Category: "bug", Severity: "HIGH", Body: "a"},
		{File: "auth.go", Line: 13, Category: "bug", Severity: "HIGH", Body: "b"},  // within 3
		{File: "auth.go", Line: 14, Category: "bug", Severity: "HIGH", Body: "cc"}, // outside 3 from anchor (10)
	}
	result := DeduplicateFindings(findings)
	if len(result) != 2 {
		t.Fatalf("expected 2 findings (10+13 merged, 14 separate), got %d", len(result))
	}
}

func TestDedup_DifferentCategory(t *testing.T) {
	findings := []model.Finding{
		{File: "auth.go", Line: 10, Category: "bug", Severity: "HIGH", Body: "a"},
		{File: "auth.go", Line: 10, Category: "security", Severity: "HIGH", Body: "b"},
	}
	result := DeduplicateFindings(findings)
	if len(result) != 2 {
		t.Fatalf("expected 2 findings (different categories), got %d", len(result))
	}
}

func TestDedup_DifferentFile(t *testing.T) {
	findings := []model.Finding{
		{File: "auth.go", Line: 10, Category: "bug", Severity: "HIGH", Body: "a"},
		{File: "handler.go", Line: 10, Category: "bug", Severity: "HIGH", Body: "b"},
	}
	result := DeduplicateFindings(findings)
	if len(result) != 2 {
		t.Fatalf("expected 2 findings (different files), got %d", len(result))
	}
}

func TestDedup_KeepsHigherSeverity(t *testing.T) {
	findings := []model.Finding{
		{File: "auth.go", Line: 10, Category: "bug", Severity: "MEDIUM", Body: "same length!!"},
		{File: "auth.go", Line: 10, Category: "bug", Severity: "HIGH", Body: "same length!!"},
	}
	result := DeduplicateFindings(findings)
	if len(result) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(result))
	}
	if result[0].Severity != "HIGH" {
		t.Errorf("expected HIGH severity to be kept, got %q", result[0].Severity)
	}
}

func TestDedup_KeepsLongerBody(t *testing.T) {
	findings := []model.Finding{
		{File: "auth.go", Line: 10, Category: "bug", Severity: "HIGH", Body: "short"},
		{File: "auth.go", Line: 11, Category: "bug", Severity: "HIGH", Body: "much longer detailed explanation"},
	}
	result := DeduplicateFindings(findings)
	if len(result) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(result))
	}
	if result[0].Body != "much longer detailed explanation" {
		t.Errorf("expected longer body, got %q", result[0].Body)
	}
}

func TestDedup_PreservesSuggestion(t *testing.T) {
	findings := []model.Finding{
		{File: "auth.go", Line: 10, Category: "bug", Severity: "HIGH", Body: "x", Suggestion: "fix()"},
		{File: "auth.go", Line: 10, Category: "bug", Severity: "HIGH", Body: "x"},
	}
	result := DeduplicateFindings(findings)
	if len(result) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(result))
	}
	if result[0].Suggestion != "fix()" {
		t.Errorf("expected suggestion to be preserved, got %q", result[0].Suggestion)
	}
}

func TestDedup_NoFindings(t *testing.T) {
	result := DeduplicateFindings(nil)
	if len(result) != 0 {
		t.Fatalf("expected 0 findings, got %d", len(result))
	}

	result = DeduplicateFindings([]model.Finding{})
	if len(result) != 0 {
		t.Fatalf("expected 0 findings for empty slice, got %d", len(result))
	}
}

func TestDedup_SingleFinding(t *testing.T) {
	findings := []model.Finding{
		{File: "auth.go", Line: 10, Category: "bug", Severity: "HIGH", Body: "only one"},
	}
	result := DeduplicateFindings(findings)
	if len(result) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(result))
	}
}

func TestSeverityRank(t *testing.T) {
	tests := []struct {
		severity string
		want     int
	}{
		{"CRITICAL", 4},
		{"critical", 4},
		{"HIGH", 3},
		{"MEDIUM", 2},
		{"LOW", 1},
		{"unknown", 0},
		{"", 0},
	}
	for _, tt := range tests {
		got := severityRank(tt.severity)
		if got != tt.want {
			t.Errorf("severityRank(%q) = %d, want %d", tt.severity, got, tt.want)
		}
	}
}

func TestIsBetterFinding(t *testing.T) {
	base := model.Finding{File: "a.go", Line: 1, Severity: "MEDIUM", Body: "short"}

	// Higher severity wins.
	high := model.Finding{File: "a.go", Line: 1, Severity: "HIGH", Body: "short"}
	if !isBetterFinding(high, base) {
		t.Error("expected HIGH to be better than MEDIUM")
	}
	if isBetterFinding(base, high) {
		t.Error("expected MEDIUM to not be better than HIGH")
	}

	// Same severity, longer body wins.
	longer := model.Finding{File: "a.go", Line: 1, Severity: "MEDIUM", Body: "much longer body"}
	if !isBetterFinding(longer, base) {
		t.Error("expected longer body to be better")
	}

	// Same severity+body, suggestion wins.
	withSugg := model.Finding{File: "a.go", Line: 1, Severity: "MEDIUM", Body: "short", Suggestion: "fix"}
	if !isBetterFinding(withSugg, base) {
		t.Error("expected finding with suggestion to be better")
	}
}
