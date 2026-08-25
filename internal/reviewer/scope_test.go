package reviewer

import (
	"strings"
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/diff"
)

func TestCheckScope_UnderThreshold(t *testing.T) {
	diffs := make([]diff.FileDiff, 5)
	assessment := CheckScope(diffs, 10)
	if assessment.IsOversized {
		t.Errorf("Expected IsOversized to be false for 5 files (max 10)")
	}
}

func TestCheckScope_OverThreshold(t *testing.T) {
	diffs := make([]diff.FileDiff, 15)
	assessment := CheckScope(diffs, 10)
	if !assessment.IsOversized {
		t.Errorf("Expected IsOversized to be true for 15 files (max 10)")
	}
}

func TestCheckScope_AtThreshold(t *testing.T) {
	diffs := make([]diff.FileDiff, 10)
	assessment := CheckScope(diffs, 10)
	if assessment.IsOversized {
		t.Errorf("Expected IsOversized to be false for 10 files (max 10)")
	}
}

func TestCheckScope_Disabled(t *testing.T) {
	diffs := make([]diff.FileDiff, 100)
	assessment := CheckScope(diffs, 0)
	if assessment.IsOversized {
		t.Errorf("Expected IsOversized to be false when threshold is 0")
	}
}

func TestFormatScopeWarning(t *testing.T) {
	assessment := &ScopeAssessment{FileCount: 15, Threshold: 10, IsOversized: true}
	warning := FormatScopeWarning(assessment)
	if !strings.Contains(warning, "15 files") {
		t.Errorf("Warning missing file count: %s", warning)
	}
	if !strings.Contains(warning, "threshold: 10") {
		t.Errorf("Warning missing threshold: %s", warning)
	}
}

func TestFormatScopeMarkdown(t *testing.T) {
	assessment := &ScopeAssessment{FileCount: 15, Threshold: 10, IsOversized: true}
	md := FormatScopeMarkdown(assessment)
	if !strings.Contains(md, "> ⚠️ **Scope Warning**") {
		t.Errorf("Markdown missing warning header: %s", md)
	}
	if !strings.Contains(md, "15 files") || !strings.Contains(md, "threshold: 10") {
		t.Errorf("Markdown missing counts: %s", md)
	}
}
