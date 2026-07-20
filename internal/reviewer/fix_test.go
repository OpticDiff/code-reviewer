package reviewer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/model"
)

func TestApplyFixes(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(t *testing.T) (findings []model.Finding, repoRoot string)
		wantApplied int
		wantSkipped int
		wantReason  string   // If non-empty, at least one fix must contain this reason.
		wantIn      []string // Strings that must appear in the modified file.
		wantNotIn   []string // Strings that must NOT appear in the modified file.
		checkFile   string   // If set, read this file for wantIn/wantNotIn checks.
	}{
		{
			name: "single line replacement",
			setup: func(t *testing.T) ([]model.Finding, string) {
				t.Helper()
				dir := t.TempDir()
				file := filepath.Join(dir, "main.go")
				writeFile(t, file, "package main\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n")
				return []model.Finding{{
					File: file, Line: 4, Severity: "medium",
					Title: "Use log", Suggestion: "\tlog.Println(\"hello\")",
				}}, ""
			},
			wantApplied: 1,
			wantIn:      []string{"log.Println"},
			wantNotIn:   []string{"fmt.Println"},
		},
		{
			name: "multi-line suggestion expands one line",
			setup: func(t *testing.T) ([]model.Finding, string) {
				t.Helper()
				dir := t.TempDir()
				file := filepath.Join(dir, "handler.go")
				writeFile(t, file, "package handler\n\nfunc Handle() {\n\treturn nil\n}\n")
				return []model.Finding{{
					File: file, Line: 4, Severity: "high",
					Title: "Add error check", Suggestion: "\tif err != nil {\n\t\treturn err\n\t}\n\treturn nil",
				}}, ""
			},
			wantApplied: 1,
			wantIn:      []string{"if err != nil"},
		},
		{
			name: "no suggestion is a no-op",
			setup: func(t *testing.T) ([]model.Finding, string) {
				t.Helper()
				return []model.Finding{{
					File: "test.go", Line: 1, Severity: "low",
					Title: "Style nit", Body: "Consider renaming",
				}}, ""
			},
			wantApplied: 0,
			wantSkipped: 0, // No fixes collected at all.
		},
		{
			name: "file not found",
			setup: func(t *testing.T) ([]model.Finding, string) {
				t.Helper()
				return []model.Finding{{
					File: "/nonexistent/path/file.go", Line: 1, Severity: "medium",
					Title: "Fix this", Suggestion: "fixed",
				}}, ""
			},
			wantSkipped: 1,
			wantReason:  "cannot read file",
		},
		{
			name: "line out of range",
			setup: func(t *testing.T) ([]model.Finding, string) {
				t.Helper()
				dir := t.TempDir()
				file := filepath.Join(dir, "small.go")
				writeFile(t, file, "package small\n")
				return []model.Finding{{
					File: file, Line: 999, Severity: "medium",
					Title: "Far away", Suggestion: "fixed",
				}}, ""
			},
			wantSkipped: 1,
			wantReason:  "out of range",
		},
		{
			name: "multiple fixes same file applied bottom-up",
			setup: func(t *testing.T) ([]model.Finding, string) {
				t.Helper()
				dir := t.TempDir()
				file := filepath.Join(dir, "multi.go")
				writeFile(t, file, "line1\nline2\nline3\nline4\nline5\n")
				return []model.Finding{
					{File: file, Line: 2, Severity: "medium", Title: "Fix 2", Suggestion: "FIXED2"},
					{File: file, Line: 4, Severity: "medium", Title: "Fix 4", Suggestion: "FIXED4"},
				}, ""
			},
			wantApplied: 2,
			wantIn:      []string{"FIXED2", "FIXED4"},
			wantNotIn:   []string{"line2", "line4"},
		},
		{
			name: "with repoRoot path resolution",
			setup: func(t *testing.T) ([]model.Finding, string) {
				t.Helper()
				dir := t.TempDir()
				subDir := filepath.Join(dir, "src")
				if err := os.MkdirAll(subDir, 0o755); err != nil {
					t.Fatal(err)
				}
				writeFile(t, filepath.Join(subDir, "app.go"), "package app\n\nfunc Init() {}\n")
				return []model.Finding{{
					File: "src/app.go", Line: 3, Severity: "medium",
					Title: "Rename", Suggestion: "func Initialize() {}",
				}}, dir
			},
			wantApplied: 1,
			wantIn:      []string{"Initialize"},
		},
		{
			name: "path traversal via ../sibling blocked",
			setup: func(t *testing.T) ([]model.Finding, string) {
				t.Helper()
				dir := t.TempDir()
				repoRoot := filepath.Join(dir, "repo")
				sibling := filepath.Join(dir, "sibling")
				if err := os.MkdirAll(repoRoot, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(sibling, 0o755); err != nil {
					t.Fatal(err)
				}
				writeFile(t, filepath.Join(sibling, "secret.txt"), "secret\n")
				return []model.Finding{{
					File: "../sibling/secret.txt", Line: 1, Severity: "high",
					Title: "Escape", Suggestion: "pwned",
				}}, repoRoot
			},
			wantSkipped: 1,
			wantReason:  "escapes repository root",
		},
		{
			name: "../../etc/passwd traversal blocked",
			setup: func(t *testing.T) ([]model.Finding, string) {
				t.Helper()
				dir := t.TempDir()
				writeFile(t, filepath.Join(dir, "safe.go"), "package safe\n")
				return []model.Finding{{
					File: "../../etc/passwd", Line: 1, Severity: "high",
					Title: "Malicious", Suggestion: "pwned",
				}}, dir
			},
			wantSkipped: 1,
			wantReason:  "escapes repository root",
		},
		{
			name: "duplicate same-line keeps first only",
			setup: func(t *testing.T) ([]model.Finding, string) {
				t.Helper()
				dir := t.TempDir()
				file := filepath.Join(dir, "dup.go")
				writeFile(t, file, "line1\nline2\nline3\n")
				return []model.Finding{
					{File: file, Line: 2, Severity: "medium", Title: "First", Suggestion: "FIRST"},
					{File: file, Line: 2, Severity: "low", Title: "Second", Suggestion: "SECOND"},
				}, ""
			},
			wantApplied: 1,
			wantSkipped: 1,
			wantReason:  "duplicate fix",
			wantIn:      []string{"FIRST"},
			wantNotIn:   []string{"SECOND"},
		},
		{
			name: "no repoRoot with absolute path fails gracefully",
			setup: func(t *testing.T) ([]model.Finding, string) {
				t.Helper()
				return []model.Finding{{
					File: "/nonexistent/absolute/path.go", Line: 1, Severity: "high",
					Title: "Abs path", Suggestion: "fixed",
				}}, ""
			},
			wantSkipped: 1,
			wantReason:  "cannot read file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings, repoRoot := tt.setup(t)
			fixes := ApplyFixes(findings, repoRoot)

			applied, skipped := 0, 0
			for _, f := range fixes {
				if f.Applied {
					applied++
				} else if f.Reason != "" {
					skipped++
				}
			}

			if tt.wantApplied > 0 && applied != tt.wantApplied {
				t.Errorf("applied = %d, want %d", applied, tt.wantApplied)
			}
			if tt.wantSkipped > 0 && skipped != tt.wantSkipped {
				t.Errorf("skipped = %d, want %d", skipped, tt.wantSkipped)
			}

			if tt.wantReason != "" {
				found := false
				for _, f := range fixes {
					if strings.Contains(f.Reason, tt.wantReason) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("no fix has reason containing %q", tt.wantReason)
				}
			}

			// File content checks — find the first fixable file to read.
			if len(tt.wantIn) > 0 || len(tt.wantNotIn) > 0 {
				checkPath := tt.checkFile
				if checkPath == "" && len(findings) > 0 {
					f := findings[0].File
					if repoRoot != "" && !filepath.IsAbs(f) {
						f = filepath.Join(repoRoot, f)
					}
					checkPath = f
				}
				if checkPath != "" {
					content, err := os.ReadFile(checkPath)
					if err == nil {
						s := string(content)
						for _, want := range tt.wantIn {
							if !strings.Contains(s, want) {
								t.Errorf("file should contain %q, got:\n%s", want, s)
							}
						}
						for _, notWant := range tt.wantNotIn {
							if strings.Contains(s, notWant) {
								t.Errorf("file should NOT contain %q, got:\n%s", notWant, s)
							}
						}
					}
				}
			}
		})
	}
}

func TestFormatFixSummary(t *testing.T) {
	tests := []struct {
		name    string
		fixes   []ApplyFix
		wantIn  []string
	}{
		{
			name:   "nil returns no suggestions message",
			fixes:  nil,
			wantIn: []string{"No suggestions"},
		},
		{
			name: "mixed applied and skipped",
			fixes: []ApplyFix{
				{File: "a.go", Line: 10, Title: "Fix A", Applied: true},
				{File: "b.go", Line: 20, Title: "Fix B", Reason: "out of range"},
			},
			wantIn: []string{"✅ a.go:10", "⏭️", "Applied: 1", "Skipped: 1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatFixSummary(tt.fixes, false)
			for _, want := range tt.wantIn {
				if !strings.Contains(result, want) {
					t.Errorf("output should contain %q, got:\n%s", want, result)
				}
			}
		})
	}
}

// writeFile is a test helper that writes content to a file, failing the test on error.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}
