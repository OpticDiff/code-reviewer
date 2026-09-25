package reviewer

import (
	"testing"
)

func TestDetectConfigPoisoning_Found(t *testing.T) {
	files := []string{"main.go", "config/.code-reviewer.yaml"}
	if !DetectConfigPoisoning(files) {
		t.Error("Expected to detect .code-reviewer.yaml")
	}

	files = []string{".code-reviewer.yml"}
	if !DetectConfigPoisoning(files) {
		t.Error("Expected to detect .code-reviewer.yml")
	}

	files = []string{"dir/.opticdiff.yaml"}
	if !DetectConfigPoisoning(files) {
		t.Error("Expected to detect .opticdiff.yaml")
	}
}

func TestDetectConfigPoisoning_NotFound(t *testing.T) {
	files := []string{"main.go", "config.yaml"}
	if DetectConfigPoisoning(files) {
		t.Error("Expected not to detect normal files")
	}
}

func TestDetectConfigPoisoning_EdgeCases(t *testing.T) {
	files := []string{"my.code-reviewer.yaml.bak", "foo-opticdiff.yaml"}
	if DetectConfigPoisoning(files) {
		t.Error("Expected not to detect similar but non-matching names")
	}
}
