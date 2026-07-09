package reviewer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/model"
)

func TestBuildSARIF(t *testing.T) {
	result := &model.ReviewResult{
		Summary: "Test review.",
		Findings: []model.Finding{
			{File: "main.go", Line: 10, Severity: "CRITICAL", Category: "security", Title: "SQL injection", Body: "Use parameterized queries."},
			{File: "utils.go", Line: 25, Severity: "LOW", Category: "style", Title: "Unused var", Body: "Remove x."},
			{File: "api.go", Line: 0, Severity: "MEDIUM", Category: "security", Title: "Missing auth", Body: "Add auth check."},
		},
	}

	report := buildSARIF(result)

	if report.Version != "2.1.0" {
		t.Errorf("expected version 2.1.0, got %s", report.Version)
	}
	if len(report.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(report.Runs))
	}

	run := report.Runs[0]
	if run.Tool.Driver.Name != "code-reviewer" {
		t.Errorf("expected tool name code-reviewer, got %s", run.Tool.Driver.Name)
	}

	// 2 unique categories: security, style
	if len(run.Tool.Driver.Rules) != 2 {
		t.Errorf("expected 2 rules, got %d", len(run.Tool.Driver.Rules))
	}

	if len(run.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(run.Results))
	}

	// Check severity mapping.
	tests := []struct {
		index int
		level string
	}{
		{0, "error"},   // CRITICAL
		{1, "note"},    // LOW
		{2, "warning"}, // MEDIUM
	}
	for _, tt := range tests {
		if run.Results[tt.index].Level != tt.level {
			t.Errorf("result[%d] level = %s, want %s", tt.index, run.Results[tt.index].Level, tt.level)
		}
	}

	// Check line 0 → 1 clamping.
	if run.Results[2].Locations[0].PhysicalLocation.Region.StartLine != 1 {
		t.Error("expected line 0 to be clamped to 1")
	}
}

func TestBuildSARIF_EmptyFindings(t *testing.T) {
	result := &model.ReviewResult{Summary: "Clean.", Findings: nil}
	report := buildSARIF(result)

	if len(report.Runs[0].Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(report.Runs[0].Results))
	}
}

func TestWriteSARIF(t *testing.T) {
	result := &model.ReviewResult{
		Findings: []model.Finding{
			{File: "a.go", Line: 5, Severity: "HIGH", Category: "bug", Title: "NPE", Body: "Null check."},
		},
	}

	path := filepath.Join(t.TempDir(), "results.sarif")
	if err := WriteSARIF(path, result); err != nil {
		t.Fatalf("WriteSARIF error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading SARIF file: %v", err)
	}

	// Validate it's valid JSON.
	var report sarifReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if report.Version != "2.1.0" {
		t.Errorf("expected version 2.1.0, got %s", report.Version)
	}
	if len(report.Runs[0].Results) != 1 {
		t.Errorf("expected 1 result, got %d", len(report.Runs[0].Results))
	}
}

func TestBuildSARIF_EmptyCategory(t *testing.T) {
	result := &model.ReviewResult{
		Findings: []model.Finding{
			{File: "a.go", Line: 1, Severity: "LOW", Category: "", Title: "test", Body: "body"},
		},
	}
	report := buildSARIF(result)
	if report.Runs[0].Results[0].RuleID != "general" {
		t.Errorf("expected 'general' for empty category, got %s", report.Runs[0].Results[0].RuleID)
	}
}
