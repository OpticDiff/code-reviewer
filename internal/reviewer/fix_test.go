package reviewer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/model"
)

func TestApplyFixes_SingleLine(t *testing.T) {
	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "main.go")
	content := "package main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	findings := []model.Finding{{
		File:       file,
		Line:       4,
		Severity:   "medium",
		Title:      "Use log instead of fmt",
		Suggestion: "\tlog.Println(\"hello\")",
	}}

	fixes := ApplyFixes(findings, "")
	if len(fixes) != 1 {
		t.Fatalf("expected 1 fix, got %d", len(fixes))
	}
	if !fixes[0].Applied {
		t.Errorf("expected fix to be applied, reason: %s", fixes[0].Reason)
	}

	result, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result), "log.Println") {
		t.Errorf("expected log.Println in result, got:\n%s", string(result))
	}
	if strings.Contains(string(result), "fmt.Println") {
		t.Errorf("expected fmt.Println removed, got:\n%s", string(result))
	}
}

func TestApplyFixes_MultiLineSuggestion(t *testing.T) {
	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "handler.go")
	content := "package handler\n\nfunc Handle() {\n\treturn nil\n}\n"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	findings := []model.Finding{{
		File:       file,
		Line:       4,
		Severity:   "high",
		Title:      "Add error check",
		Suggestion: "\tif err != nil {\n\t\treturn err\n\t}\n\treturn nil",
	}}

	fixes := ApplyFixes(findings, "")
	if len(fixes) != 1 || !fixes[0].Applied {
		t.Fatalf("expected 1 applied fix, got %v", fixes)
	}

	result, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(result), "if err != nil") {
		t.Error("expected error check in result")
	}
	lines := strings.Split(string(result), "\n")
	// Original has 6 entries after split (5 lines + trailing empty from final \n).
	// Replacing 1 line with 4 gives 6 - 1 + 4 = 9.
	if len(lines) != 9 {
		t.Errorf("expected 9 lines, got %d:\n%s", len(lines), string(result))
	}
}

func TestApplyFixes_NoSuggestion(t *testing.T) {
	findings := []model.Finding{{
		File:     "test.go",
		Line:     1,
		Severity: "low",
		Title:    "Style nit",
		Body:     "Consider renaming",
	}}

	fixes := ApplyFixes(findings, "")
	if len(fixes) != 0 {
		t.Errorf("expected 0 fixes for findings without suggestions, got %d", len(fixes))
	}
}

func TestApplyFixes_FileNotFound(t *testing.T) {
	findings := []model.Finding{{
		File:       "/nonexistent/path/file.go",
		Line:       1,
		Severity:   "medium",
		Title:      "Fix this",
		Suggestion: "fixed",
	}}

	fixes := ApplyFixes(findings, "")
	if len(fixes) != 1 {
		t.Fatalf("expected 1 fix, got %d", len(fixes))
	}
	if fixes[0].Applied {
		t.Error("expected fix to be skipped for missing file")
	}
	if !strings.Contains(fixes[0].Reason, "cannot read file") {
		t.Errorf("unexpected reason: %s", fixes[0].Reason)
	}
}

func TestApplyFixes_LineOutOfRange(t *testing.T) {
	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "small.go")
	content := "package small\n"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	findings := []model.Finding{{
		File:       file,
		Line:       999,
		Severity:   "medium",
		Title:      "Far away",
		Suggestion: "fixed",
	}}

	fixes := ApplyFixes(findings, "")
	if len(fixes) != 1 || fixes[0].Applied {
		t.Errorf("expected fix skipped for out-of-range line")
	}
	if !strings.Contains(fixes[0].Reason, "out of range") {
		t.Errorf("unexpected reason: %s", fixes[0].Reason)
	}
}

func TestApplyFixes_MultipleFixesSameFile(t *testing.T) {
	tmpDir := t.TempDir()
	file := filepath.Join(tmpDir, "multi.go")
	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	findings := []model.Finding{
		{File: file, Line: 2, Severity: "medium", Title: "Fix line 2", Suggestion: "FIXED2"},
		{File: file, Line: 4, Severity: "medium", Title: "Fix line 4", Suggestion: "FIXED4"},
	}

	fixes := ApplyFixes(findings, "")
	applied := 0
	for _, f := range fixes {
		if f.Applied {
			applied++
		}
	}
	if applied != 2 {
		t.Errorf("expected 2 applied fixes, got %d", applied)
	}

	result, _ := os.ReadFile(file)
	if !strings.Contains(string(result), "FIXED2") || !strings.Contains(string(result), "FIXED4") {
		t.Errorf("expected both fixes applied, got:\n%s", string(result))
	}
	if strings.Contains(string(result), "line2") || strings.Contains(string(result), "line4") {
		t.Error("expected original lines replaced")
	}
}

func TestApplyFixes_WithRepoRoot(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(subDir, "app.go")
	content := "package app\n\nfunc Init() {}\n"
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	findings := []model.Finding{{
		File:       "src/app.go",
		Line:       3,
		Severity:   "medium",
		Title:      "Rename",
		Suggestion: "func Initialize() {}",
	}}

	fixes := ApplyFixes(findings, tmpDir)
	if len(fixes) != 1 || !fixes[0].Applied {
		t.Fatalf("expected applied fix with repoRoot, got %v", fixes)
	}

	result, _ := os.ReadFile(file)
	if !strings.Contains(string(result), "Initialize") {
		t.Error("expected renamed function")
	}
}

func TestFormatFixSummary_Empty(t *testing.T) {
	result := FormatFixSummary(nil, false)
	if !strings.Contains(result, "No suggestions") {
		t.Error("expected empty message")
	}
}

func TestFormatFixSummary_Mixed(t *testing.T) {
	fixes := []ApplyFix{
		{File: "a.go", Line: 10, Title: "Fix A", Applied: true},
		{File: "b.go", Line: 20, Title: "Fix B", Reason: "out of range"},
	}
	result := FormatFixSummary(fixes, false)
	if !strings.Contains(result, "✅ a.go:10") {
		t.Error("expected applied fix in summary")
	}
	if !strings.Contains(result, "⏭️") {
		t.Error("expected skipped fix in summary")
	}
	if !strings.Contains(result, "Applied: 1") {
		t.Error("expected applied count")
	}
	if !strings.Contains(result, "Skipped: 1") {
		t.Error("expected skipped count")
	}
}
