package model

import (
	"strings"
	"testing"
)

func TestBuildIntentContextWithChecks_DefaultsMatchLegacy(t *testing.T) {
	// Golden literals copied from the prompt text before intent_checks existed.
	tests := []struct {
		name string
		s    *SummaryResult
		want string
	}{
		{"feat", &SummaryResult{Classification: "feat", Intent: "x"},
			"* TEST COVERAGE: This is a feature change. If new behavior is introduced without corresponding test files or test functions, report as category \"scope\" with severity HIGH and title \"New feature missing test coverage\".\n"},
		{"refactor", &SummaryResult{Classification: "refactor", Intent: "x"},
			"* BEHAVIORAL PRESERVATION: This is a refactor. Verify no behavioral changes are introduced. If behavior changes, report as category \"scope\" with severity HIGH and title \"Refactor introduces behavioral change\".\n"},
		{"scope creep", &SummaryResult{Classification: "chore", Intent: "x", ScopeAreas: []string{"api", "db"}},
			"* SCOPE CREEP: Flag any file changes that fall OUTSIDE the stated scope areas (api, db). Report as category \"scope\" with severity MEDIUM.\n"},
		{"breaking", &SummaryResult{Classification: "chore", Intent: "x", BreakingChanges: []string{"b"}},
			"* BREAKING CHANGES: Breaking changes were detected. Verify each is documented in CHANGELOG, README, or migration guide. Report undocumented breaking changes as category \"scope\" with severity HIGH.\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, got := range []string{BuildIntentContext(tt.s), BuildIntentContextWithChecks(tt.s, IntentChecks{})} {
				if !strings.Contains(got, tt.want) {
					t.Errorf("default rule text changed; want line:\n%s\ngot:\n%s", tt.want, got)
				}
			}
		})
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
