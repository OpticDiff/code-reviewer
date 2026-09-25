package reviewer

import (
	"testing"
)

func TestDetectConfigPoisoning_Found(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		want  int
	}{
		{"yaml", []string{"main.go", ".code-reviewer.yaml"}, 1},
		{"yml", []string{".code-reviewer.yml", "utils.go"}, 1},
		{"opticdiff", []string{".opticdiff.yaml"}, 1},
		{"subdirectory", []string{"subdir/.code-reviewer.yaml"}, 1},
		{"multiple", []string{".code-reviewer.yaml", ".opticdiff.yaml"}, 2},
		{"both old and new", []string{".code-reviewer.yaml", ".code-reviewer.yml"}, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectConfigPoisoning(tt.files)
			if len(got) != tt.want {
				t.Errorf("DetectConfigPoisoning(%v) = %v (len %d), want len %d", tt.files, got, len(got), tt.want)
			}
		})
	}
}

func TestDetectConfigPoisoning_NotFound(t *testing.T) {
	files := []string{"main.go", "config.yaml", "reviewer.go", "code-reviewer.yaml"}
	got := DetectConfigPoisoning(files)
	if len(got) != 0 {
		t.Errorf("DetectConfigPoisoning(%v) = %v, want empty", files, got)
	}
}

func TestDetectConfigPoisoning_EdgeCases(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		want  int
	}{
		{"empty list", nil, 0},
		{"empty strings", []string{""}, 0},
		{"similar name", []string{"not-.code-reviewer.yaml"}, 0},
		{"no leading dot", []string{"code-reviewer.yaml"}, 0},
		{"deeply nested", []string{"a/b/c/.code-reviewer.yaml"}, 1},
		{"dedup", []string{".code-reviewer.yaml", ".code-reviewer.yaml"}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectConfigPoisoning(tt.files)
			if len(got) != tt.want {
				t.Errorf("DetectConfigPoisoning(%v) = %v (len %d), want len %d", tt.files, got, len(got), tt.want)
			}
		})
	}
}

func TestConfigPoisoningFinding(t *testing.T) {
	f := ConfigPoisoningFinding("subdir/.opticdiff.yaml", 42)
	if f.File != "subdir/.opticdiff.yaml" {
		t.Errorf("File = %q, want %q", f.File, "subdir/.opticdiff.yaml")
	}
	if f.Line != 42 {
		t.Errorf("Line = %d, want 42", f.Line)
	}
	if f.Severity != "CRITICAL" {
		t.Errorf("Severity = %q, want CRITICAL", f.Severity)
	}
}

func TestConfigPoisoningFinding_ZeroLine(t *testing.T) {
	f := ConfigPoisoningFinding(".code-reviewer.yaml", 0)
	if f.Line != 1 {
		t.Errorf("Line = %d, want 1 (should clamp to 1)", f.Line)
	}
}
