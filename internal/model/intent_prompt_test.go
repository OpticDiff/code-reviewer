package model

import (
	"strings"
	"testing"
)

func TestBuildIntentContext_Nil(t *testing.T) {
	result := BuildIntentContext(nil)
	if result != "" {
		t.Errorf("expected empty for nil, got %q", result)
	}
}

func TestBuildIntentContext_FeatClassification(t *testing.T) {
	s := &SummaryResult{
		Classification: "feat",
		Intent:         "Add OAuth2 authentication",
		RiskLevel:      "medium",
		ScopeAreas:     []string{"auth", "middleware"},
	}
	result := BuildIntentContext(s)
	if !strings.Contains(result, "Classification: feat") {
		t.Error("expected classification")
	}
	if !strings.Contains(result, "TEST COVERAGE") {
		t.Error("expected test coverage rule for feat")
	}
	if !strings.Contains(result, "SCOPE CREEP") {
		t.Error("expected scope creep rule")
	}
	if strings.Contains(result, "ROOT CAUSE") {
		t.Error("should not have fix-specific rules")
	}
}

func TestBuildIntentContext_FixClassification(t *testing.T) {
	s := &SummaryResult{
		Classification: "fix",
		Intent:         "Resolve race condition",
		RiskLevel:      "high",
		ScopeAreas:     []string{"session"},
	}
	result := BuildIntentContext(s)
	if !strings.Contains(result, "ROOT CAUSE") {
		t.Error("expected root cause rule for fix")
	}
	if !strings.Contains(result, "HIGH RISK") {
		t.Error("expected high risk rule")
	}
	if strings.Contains(result, "TEST COVERAGE") {
		t.Error("should not have feat-specific rules")
	}
}

func TestBuildIntentContext_RefactorClassification(t *testing.T) {
	s := &SummaryResult{
		Classification: "refactor",
		Intent:         "Extract service layer",
		RiskLevel:      "medium",
	}
	result := BuildIntentContext(s)
	if !strings.Contains(result, "BEHAVIORAL PRESERVATION") {
		t.Error("expected behavioral preservation rule for refactor")
	}
}

func TestBuildIntentContext_BreakingChanges(t *testing.T) {
	s := &SummaryResult{
		Classification:  "feat",
		Intent:          "Change API response format",
		RiskLevel:       "high",
		BreakingChanges: []string{"Response format changed from XML to JSON"},
	}
	result := BuildIntentContext(s)
	if !strings.Contains(result, "BREAKING CHANGES") {
		t.Error("expected breaking changes rule")
	}
	if !strings.Contains(result, "XML to JSON") {
		t.Error("expected breaking change detail")
	}
}

func TestBuildIntentContext_MinimalFields(t *testing.T) {
	s := &SummaryResult{
		Classification: "chore",
		Intent:         "Update dependencies",
		RiskLevel:      "low",
	}
	result := BuildIntentContext(s)
	// Should not have empty scope/breaking sections.
	if strings.Contains(result, "SCOPE CREEP") {
		t.Error("should not have scope creep rule without scope areas")
	}
	if strings.Contains(result, "BREAKING CHANGES:") {
		t.Error("should not have breaking changes rule")
	}
	// Should still have the header.
	if !strings.Contains(result, "DEVELOPER INTENT") {
		t.Error("expected intent header")
	}
}

func TestBuildIntentContext_ScopeCategory(t *testing.T) {
	s := &SummaryResult{
		Classification: "feat",
		Intent:         "test",
		RiskLevel:      "low",
		ScopeAreas:     []string{"api"},
	}
	result := BuildIntentContext(s)
	if !strings.Contains(result, `category "scope"`) {
		t.Error("expected scope category reference")
	}
}

func TestBuildPromptFull_WithIntentContext(t *testing.T) {
	intentCtx := "## DEVELOPER INTENT\nClassification: feat\n"
	result := BuildPromptFull("", "", []string{"bugs"}, "extra rule", intentCtx)

	// Intent context should appear in the prompt.
	if !strings.Contains(result, "DEVELOPER INTENT") {
		t.Error("expected intent context in prompt")
	}
	// Intent context should appear BEFORE extra rules.
	intentIdx := strings.Index(result, "DEVELOPER INTENT")
	extraIdx := strings.Index(result, "ADDITIONAL RULES")
	if intentIdx > extraIdx {
		t.Error("intent context should appear before extra rules")
	}
}
