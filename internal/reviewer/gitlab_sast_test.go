package reviewer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/model"
)

func TestWriteGitLabSAST(t *testing.T) {
	tempDir := t.TempDir()
	outPath := filepath.Join(tempDir, "gl-sast-report.json")

	result := &model.ReviewResult{
		Findings: []model.Finding{
			{
				File:       "main.go",
				Line:       10,
				EndLine:    12,
				Category:   "Security",
				Title:      "SQL Injection",
				Body:       "Found potential SQL injection.",
				Suggestion: "Use parameterized queries.",
				Severity:   "CRITICAL",
			},
			{
				File:       "utils.go",
				Line:       20,
				EndLine:    20,
				Category:   "Bug",
				Title:      "Null pointer dereference",
				Body:       "May panic.",
				Suggestion: "",
				Severity:   "HIGH",
			},
			{
				File:       "config.go",
				Line:       30,
				Category:   "Style",
				Title:      "Unused variable",
				Body:       "Variable is not used.",
				Severity:   "LOW",
			},
			{
				File:       "app.go",
				Line:       40,
				Category:   "Info",
				Title:      "Missing docstring",
				Body:       "Add documentation.",
				Severity:   "UNKNOWN",
			},
		},
	}

	err := WriteGitLabSAST(outPath, "dev", result)
	if err != nil {
		t.Fatalf("WriteGitLabSAST failed: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("Failed to read output: %v", err)
	}

	var report gitlabSASTReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if report.Version != "15.0.0" {
		t.Errorf("Expected version 15.0.0, got %s", report.Version)
	}
	if report.Scan.Type != "sast" {
		t.Errorf("Expected scan type sast, got %s", report.Scan.Type)
	}

	if len(report.Vulnerabilities) != 4 {
		t.Fatalf("Expected 4 vulnerabilities, got %d", len(report.Vulnerabilities))
	}

	vuln1 := report.Vulnerabilities[0]
	if vuln1.Severity != "Critical" {
		t.Errorf("Expected Critical severity, got %s", vuln1.Severity)
	}
	if vuln1.Description != "Found potential SQL injection.\n\nSuggestion:\nUse parameterized queries." {
		t.Errorf("Expected suggestion in description, got %s", vuln1.Description)
	}
	if vuln1.Location.StartLine != 10 || vuln1.Location.EndLine != 12 {
		t.Errorf("Expected line 10-12, got %d-%d", vuln1.Location.StartLine, vuln1.Location.EndLine)
	}

	vuln2 := report.Vulnerabilities[1]
	if vuln2.Severity != "High" {
		t.Errorf("Expected High severity, got %s", vuln2.Severity)
	}
	if vuln2.Location.StartLine != 20 || vuln2.Location.EndLine != 20 {
		t.Errorf("Expected line 20-20, got %d-%d", vuln2.Location.StartLine, vuln2.Location.EndLine)
	}

	vuln3 := report.Vulnerabilities[2]
	if vuln3.Severity != "Low" {
		t.Errorf("Expected Low severity, got %s", vuln3.Severity)
	}

	vuln4 := report.Vulnerabilities[3]
	if vuln4.Severity != "Info" {
		t.Errorf("Expected Info severity, got %s", vuln4.Severity)
	}

	// Deterministic ID check
	id1 := vuln1.ID
	id2 := vuln2.ID
	if len(id1) != 32 {
		t.Errorf("Expected ID length 32, got %d", len(id1))
	}
	if id1 == id2 {
		t.Errorf("Expected unique IDs, got same %s", id1)
	}
}

func TestWriteGitLabSAST_Empty(t *testing.T) {
	tempDir := t.TempDir()
	outPath := filepath.Join(tempDir, "empty.json")

	result := &model.ReviewResult{
		Findings: []model.Finding{},
	}

	err := WriteGitLabSAST(outPath, "dev", result)
	if err != nil {
		t.Fatalf("WriteGitLabSAST failed: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("Failed to read output: %v", err)
	}

	var report gitlabSASTReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if len(report.Vulnerabilities) != 0 {
		t.Fatalf("Expected 0 vulnerabilities, got %d", len(report.Vulnerabilities))
	}
}
