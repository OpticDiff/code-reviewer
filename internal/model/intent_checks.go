package model

import (
	"fmt"
	"strings"
)

// IntentCheckOff disables an intent check when used as its severity.
const IntentCheckOff = "off"

// IntentCheck configures one intent-aware review rule.
type IntentCheck struct {
	// Severity is off, low, medium, high or critical. Empty keeps the built-in default.
	Severity string `yaml:"severity"`
	// DocumentedIn lists where breaking changes count as documented (breaking_changes only).
	// The literal MR_DESCRIPTION stands for the merge request description.
	DocumentedIn []string `yaml:"documented_in"`
}

// IntentChecks configures the severities of the intent-aware review rules.
// The zero value reproduces the built-in behaviour.
type IntentChecks struct {
	MissingTests    IntentCheck `yaml:"missing_tests"`
	BehaviourChange IntentCheck `yaml:"behaviour_change"`
	BreakingChanges IntentCheck `yaml:"breaking_changes"`
	ScopeCreep      IntentCheck `yaml:"scope_creep"`
	// StackAware downgrades missing_tests to low when the change targets a
	// non-default branch (a stacked merge request).
	StackAware bool `yaml:"stack_aware"`

	// Stacked reports that the change targets a non-default branch. It is
	// detected from the CI environment, not read from YAML.
	Stacked bool `yaml:"-"`
}

// Validate reports an error when any configured severity is unknown.
func (c IntentChecks) Validate() error {
	for name, chk := range map[string]IntentCheck{
		"missing_tests":    c.MissingTests,
		"behaviour_change": c.BehaviourChange,
		"breaking_changes": c.BreakingChanges,
		"scope_creep":      c.ScopeCreep,
	} {
		if _, err := chk.level("MEDIUM"); err != nil {
			return fmt.Errorf("intent_checks.%s: %w", name, err)
		}
	}
	return nil
}

// level returns the upper-case severity, falling back to def, or "" when off.
func (c IntentCheck) level(def string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(c.Severity))
	switch s {
	case "":
		return def, nil
	case IntentCheckOff:
		return "", nil
	case "low", "medium", "high", "critical":
		return strings.ToUpper(s), nil
	}
	return "", fmt.Errorf("unknown severity %q (valid: off, low, medium, high, critical)", c.Severity)
}

// mustLevel is level for configs already checked by Validate; invalid values use def.
func (c IntentCheck) mustLevel(def string) string {
	l, err := c.level(def)
	if err != nil {
		return def
	}
	return l
}

// documentationLocations renders DocumentedIn for the prompt, or "" when unset.
func (c IntentCheck) documentationLocations() string {
	if len(c.DocumentedIn) == 0 {
		return ""
	}
	locs := make([]string, len(c.DocumentedIn))
	for i, l := range c.DocumentedIn {
		if l == "MR_DESCRIPTION" {
			l = "the merge request description"
		}
		locs[i] = l
	}
	return strings.Join(locs, ", ")
}
