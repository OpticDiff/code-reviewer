package dataset

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCases(t *testing.T) {
	// Create a temporary directory structure for testing
	tmpDir := t.TempDir()
	categoryDir := filepath.Join(tmpDir, "security", "case1")
	if err := os.MkdirAll(categoryDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	meta := CaseMeta{
		Language:   "go",
		Difficulty: "medium",
		Tags:       []string{"sql-injection"},
	}
	metaBytes, _ := json.Marshal(meta)
	os.WriteFile(filepath.Join(categoryDir, "meta.json"), metaBytes, 0644)

	expected := ExpectedResult{
		Findings: []ExpectedFinding{
			{File: "main.go", Line: 10, Severity: "high", Category: "security", Title: "SQL Injection"},
		},
	}
	expectedBytes, _ := json.Marshal(expected)
	os.WriteFile(filepath.Join(categoryDir, "expected.json"), expectedBytes, 0644)

	os.WriteFile(filepath.Join(categoryDir, "diff.patch"), []byte("--- a/main.go\n+++ b/main.go\n"), 0644)

	cases, err := LoadCases(tmpDir)
	if err != nil {
		t.Fatalf("LoadCases failed: %v", err)
	}
	if len(cases) != 1 {
		t.Fatalf("Expected 1 case, got %d", len(cases))
	}
	c := cases[0]
	if c.Name != "case1" || c.Category != "security" || c.Language != "go" {
		t.Errorf("Unexpected case fields: %+v", c)
	}
}
