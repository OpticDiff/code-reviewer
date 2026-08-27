package reviewer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/model"
)

func TestBuildSARIF(t *testing.T) {
	result := &model.ReviewResult{
		Summary: "Test review.",
		Findings: []model.Finding{
			{File: "main.go", Line: 10, EndLine: 12, Severity: "CRITICAL", Category: "security", Title: "SQL injection", Body: "Use parameterized queries.", Suggestion: "db.Query(\"SELECT * FROM users WHERE id = ?\", id)"},
			{File: "utils.go", Line: 25, Severity: "LOW", Category: "style", Title: "Unused var", Body: "Remove x."},
			{File: "api.go", Line: 0, Severity: "MEDIUM", Category: "security", Title: "Missing auth", Body: "Add auth check."},
		},
	}

	report := buildSARIF(result, "1.2.3")

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
	if run.Tool.Driver.Version != "1.2.3" {
		t.Errorf("expected tool version 1.2.3, got %s", run.Tool.Driver.Version)
	}

	// 2 unique categories: security, style
	if len(run.Tool.Driver.Rules) != 2 {
		t.Errorf("expected 2 rules, got %d", len(run.Tool.Driver.Rules))
	}

	if len(run.Results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(run.Results))
	}

	// Check security-severity property
	rulesMap := make(map[string]sarifRule)
	for _, r := range run.Tool.Driver.Rules {
		rulesMap[r.ID] = r
	}
	if secRule, ok := rulesMap["security"]; ok {
		if secRule.Properties["security-severity"] != "9.5" {
			t.Errorf("expected security-severity 9.5 for CRITICAL, got %v", secRule.Properties["security-severity"])
		}
		tags, ok := secRule.Properties["tags"].([]string)
		if !ok || len(tags) == 0 || tags[0] != "security" {
			t.Errorf("expected tags [security], got %v", secRule.Properties["tags"])
		}
	} else {
		t.Errorf("expected security rule")
	}

	// Check suggestion in message text
	if !strings.Contains(run.Results[0].Message.Text, "**Suggested fix:**") {
		t.Errorf("expected message to contain suggestion markdown, got: %s", run.Results[0].Message.Text)
	}
	if !strings.Contains(run.Results[0].Message.Markdown, "**Suggested fix:**") {
		t.Errorf("expected markdown to contain suggestion markdown")
	}

	// Check EndLine
	region := run.Results[0].Locations[0].PhysicalLocation.Region
	if region.StartLine != 10 {
		t.Errorf("expected StartLine 10, got %d", region.StartLine)
	}
	if region.EndLine != 12 {
		t.Errorf("expected EndLine 12, got %d", region.EndLine)
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

	// Check ruleIndex
	if run.Results[0].RuleIndex != 0 {
		t.Errorf("expected ruleIndex 0 for security finding 1, got %d", run.Results[0].RuleIndex)
	}
	if run.Results[1].RuleIndex != 1 {
		t.Errorf("expected ruleIndex 1 for style finding, got %d", run.Results[1].RuleIndex)
	}
	if run.Results[2].RuleIndex != 0 {
		t.Errorf("expected ruleIndex 0 for security finding 2, got %d", run.Results[2].RuleIndex)
	}

	// Check partialFingerprints
	if fp, ok := run.Results[0].PartialFingerprints["primaryLocationLineHash"]; ok {
		if len(fp) != 16 {
			t.Errorf("expected 16 chars for fingerprint, got %s", fp)
		}
	} else {
		t.Errorf("expected primaryLocationLineHash fingerprint")
	}
}

func TestBuildSARIF_EmptyFindings(t *testing.T) {
	result := &model.ReviewResult{Summary: "Clean.", Findings: nil}
	report := buildSARIF(result, "dev")

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
	if err := WriteSARIF(path, result, "dev"); err != nil {
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
	report := buildSARIF(result, "dev")
	if report.Runs[0].Results[0].RuleID != "general" {
		t.Errorf("expected 'general' for empty category, got %s", report.Runs[0].Results[0].RuleID)
	}
}
