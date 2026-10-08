package model

import (
	"strings"
	"testing"
)

func TestBuildIntentContextWithChecks_DefaultsMatchLegacy(t *testing.T) {
	s := &SummaryResult{
		Classification:  "feat",
		Intent:          "Add API",
		RiskLevel:       "high",
		ScopeAreas:      []string{"api"},
		BreakingChanges: []string{"removed field"},
	}
	if got, want := BuildIntentContextWithChecks(s, IntentChecks{}), BuildIntentContext(s); got != want {
		t.Errorf("zero IntentChecks must match defaults:\n%s\nvs\n%s", got, want)
	}
}

func TestBuildIntentContextWithChecks_MissingTestsSeverity(t *testing.T) {
	s := &SummaryResult{Classification: "feat", Intent: "x"}
	got := BuildIntentContextWithChecks(s, IntentChecks{MissingTests: IntentCheck{Severity: "medium"}})
	if !strings.Contains(got, `severity MEDIUM and title "New feature missing test coverage"`) {
		t.Errorf("expected MEDIUM missing-tests severity, got:\n%s", got)
	}
}

func TestBuildIntentContextWithChecks_Off(t *testing.T) {
	s := &SummaryResult{
		Classification:  "refactor",
		Intent:          "x",
		ScopeAreas:      []string{"api"},
		BreakingChanges: []string{"b"},
	}
	got := BuildIntentContextWithChecks(s, IntentChecks{
		BehaviourChange: IntentCheck{Severity: "off"},
		BreakingChanges: IntentCheck{Severity: "off"},
		ScopeCreep:      IntentCheck{Severity: "off"},
	})
	for _, banned := range []string{"BEHAVIORAL PRESERVATION", "BREAKING CHANGES: Breaking", "SCOPE CREEP"} {
		if strings.Contains(got, banned) {
			t.Errorf("rule %q should be disabled, got:\n%s", banned, got)
		}
	}
	if !strings.Contains(got, "Breaking Changes: b") {
		t.Error("intent summary must still be emitted")
	}

	feat := &SummaryResult{Classification: "feat", Intent: "x"}
	got = BuildIntentContextWithChecks(feat, IntentChecks{MissingTests: IntentCheck{Severity: "off"}})
	if strings.Contains(got, "TEST COVERAGE") {
		t.Error("missing_tests off should drop the rule")
	}
}

func TestBuildIntentContextWithChecks_DocumentedIn(t *testing.T) {
	s := &SummaryResult{Classification: "feat", Intent: "x", BreakingChanges: []string{"b"}}
	got := BuildIntentContextWithChecks(s, IntentChecks{BreakingChanges: IntentCheck{
		Severity:     "medium",
		DocumentedIn: []string{"CHANGELOG.md", "proto/**", "MR_DESCRIPTION"},
	}})
	if !strings.Contains(got, "CHANGELOG.md, proto/**, the merge request description") {
		t.Errorf("expected documentation locations, got:\n%s", got)
	}
	if !strings.Contains(got, "undocumented breaking changes as category \"scope\" with severity MEDIUM") {
		t.Errorf("expected MEDIUM breaking severity, got:\n%s", got)
	}
}

func TestBuildIntentContextWithChecks_StackedDowngradesMissingTests(t *testing.T) {
	s := &SummaryResult{Classification: "feat", Intent: "x"}
	got := BuildIntentContextWithChecks(s, IntentChecks{StackAware: true, Stacked: true})
	if !strings.Contains(got, `severity LOW and title "New feature missing test coverage"`) {
		t.Errorf("stacked MR should downgrade to LOW, got:\n%s", got)
	}
	got = BuildIntentContextWithChecks(s, IntentChecks{Stacked: true})
	if !strings.Contains(got, `severity HIGH and title "New feature missing test coverage"`) {
		t.Errorf("without stack_aware the default must stay HIGH, got:\n%s", got)
	}
}

func TestIntentChecks_Validate(t *testing.T) {
	if err := (IntentChecks{MissingTests: IntentCheck{Severity: "Medium"}}).Validate(); err != nil {
		t.Errorf("valid severity rejected: %v", err)
	}
	if err := (IntentChecks{ScopeCreep: IntentCheck{Severity: "urgent"}}).Validate(); err == nil {
		t.Error("expected error for invalid severity")
	}
}
