package config

import (
	"strings"
	"testing"
)

func TestRule_Validate_Valid(t *testing.T) {
	r := Rule{
		Name:        "no-raw-sql",
		Description: "Flag raw SQL string concatenation",
		Category:    "security",
		Severity:    "high",
	}
	if err := r.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRule_Validate_CaseInsensitive(t *testing.T) {
	r := Rule{
		Name:        "test-rule",
		Description: "Some rule",
		Category:    "Security",
		Severity:    "HIGH",
	}
	if err := r.Validate(); err != nil {
		t.Errorf("unexpected error for mixed case: %v", err)
	}
	// Should be normalized to lowercase.
	if r.Category != "security" {
		t.Errorf("expected category normalized to 'security', got %q", r.Category)
	}
	if r.Severity != "high" {
		t.Errorf("expected severity normalized to 'high', got %q", r.Severity)
	}
}

func TestRule_Validate_MinimalFields(t *testing.T) {
	r := Rule{
		Name:        "check-errors",
		Description: "Always check error returns",
	}
	if err := r.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRule_Validate_MissingName(t *testing.T) {
	r := Rule{Description: "some rule"}
	err := r.Validate()
	if err == nil {
		t.Fatal("expected error for missing name")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("expected name error, got: %v", err)
	}
}

func TestRule_Validate_MissingDescription(t *testing.T) {
	r := Rule{Name: "my-rule"}
	err := r.Validate()
	if err == nil {
		t.Fatal("expected error for missing description")
	}
	if !strings.Contains(err.Error(), "description") {
		t.Errorf("expected description error, got: %v", err)
	}
}

func TestRule_Validate_InvalidCategory(t *testing.T) {
	r := Rule{Name: "r1", Description: "d", Category: "invalid"}
	err := r.Validate()
	if err == nil {
		t.Fatal("expected error for invalid category")
	}
	if !strings.Contains(err.Error(), "invalid category") {
		t.Errorf("expected category error, got: %v", err)
	}
}

func TestRule_Validate_InvalidSeverity(t *testing.T) {
	r := Rule{Name: "r1", Description: "d", Severity: "extreme"}
	err := r.Validate()
	if err == nil {
		t.Fatal("expected error for invalid severity")
	}
	if !strings.Contains(err.Error(), "invalid severity") {
		t.Errorf("expected severity error, got: %v", err)
	}
}

func TestRule_EffectiveCategory(t *testing.T) {
	r := Rule{Name: "r", Description: "d"}
	if r.EffectiveCategory() != "custom" {
		t.Errorf("expected default 'custom', got %q", r.EffectiveCategory())
	}
	r.Category = "security"
	if r.EffectiveCategory() != "security" {
		t.Errorf("expected 'security', got %q", r.EffectiveCategory())
	}
}

func TestRule_EffectiveSeverity(t *testing.T) {
	r := Rule{Name: "r", Description: "d"}
	if r.EffectiveSeverity() != "high" {
		t.Errorf("expected default 'high', got %q", r.EffectiveSeverity())
	}
	r.Severity = "critical"
	if r.EffectiveSeverity() != "critical" {
		t.Errorf("expected 'critical', got %q", r.EffectiveSeverity())
	}
}

func TestValidateRules_Valid(t *testing.T) {
	rules := []Rule{
		{Name: "r1", Description: "d1"},
		{Name: "r2", Description: "d2", Category: "bug", Severity: "low"},
	}
	if err := ValidateRules(rules); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateRules_DuplicateName(t *testing.T) {
	rules := []Rule{
		{Name: "r1", Description: "d1"},
		{Name: "r1", Description: "d2"},
	}
	err := ValidateRules(rules)
	if err == nil {
		t.Fatal("expected error for duplicate name")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("expected duplicate error, got: %v", err)
	}
}

func TestValidateRules_Empty(t *testing.T) {
	if err := ValidateRules(nil); err != nil {
		t.Errorf("unexpected error for nil rules: %v", err)
	}
	if err := ValidateRules([]Rule{}); err != nil {
		t.Errorf("unexpected error for empty rules: %v", err)
	}
}

func TestFormatRulesPrompt_Empty(t *testing.T) {
	if result := FormatRulesPrompt(nil); result != "" {
		t.Errorf("expected empty string for nil rules, got %q", result)
	}
}

func TestFormatRulesPrompt_SingleRule(t *testing.T) {
	rules := []Rule{
		{Name: "no-raw-sql", Description: "Flag raw SQL concatenation", Category: "security", Severity: "critical"},
	}
	result := FormatRulesPrompt(rules)

	if !strings.Contains(result, "## CUSTOM RULES") {
		t.Error("expected CUSTOM RULES header")
	}
	if !strings.Contains(result, "### Rule: no-raw-sql") {
		t.Error("expected rule name")
	}
	if !strings.Contains(result, "**Category**: security") {
		t.Error("expected category")
	}
	if !strings.Contains(result, "**Severity**: critical") {
		t.Error("expected severity")
	}
	if !strings.Contains(result, "Flag raw SQL concatenation") {
		t.Error("expected description")
	}
}

func TestFormatRulesPrompt_WithPaths(t *testing.T) {
	rules := []Rule{
		{Name: "react-hooks", Description: "Check hooks rules", Paths: []string{"*.tsx", "*.jsx"}},
	}
	result := FormatRulesPrompt(rules)
	if !strings.Contains(result, "**Applies to**: *.tsx, *.jsx") {
		t.Error("expected paths in output")
	}
}

func TestFormatRulesPrompt_DefaultCategoryAndSeverity(t *testing.T) {
	rules := []Rule{
		{Name: "my-rule", Description: "A custom check"},
	}
	result := FormatRulesPrompt(rules)
	if !strings.Contains(result, "**Category**: custom") {
		t.Error("expected default category 'custom'")
	}
	if !strings.Contains(result, "**Severity**: high") {
		t.Error("expected default severity 'high'")
	}
}

func TestFormatRulesPrompt_MultipleRules(t *testing.T) {
	rules := []Rule{
		{Name: "r1", Description: "d1"},
		{Name: "r2", Description: "d2"},
		{Name: "r3", Description: "d3"},
	}
	result := FormatRulesPrompt(rules)
	if strings.Count(result, "### Rule:") != 3 {
		t.Error("expected 3 rule sections")
	}
}

func TestApplyRepoConfig_Rules(t *testing.T) {
	yaml := `
rules:
  - name: no-raw-sql
    description: Flag raw SQL string concatenation
    category: security
    severity: critical
  - name: check-errors
    description: Always wrap errors with fmt.Errorf
`
	cfg := &Config{}
	if err := cfg.applyRepoConfig([]byte(yaml)); err != nil {
		t.Fatalf("applyRepoConfig failed: %v", err)
	}
	if len(cfg.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(cfg.Rules))
	}
	if cfg.Rules[0].Name != "no-raw-sql" {
		t.Errorf("expected rule name 'no-raw-sql', got %q", cfg.Rules[0].Name)
	}
	if cfg.Rules[0].Category != "security" {
		t.Errorf("expected category 'security', got %q", cfg.Rules[0].Category)
	}
	if cfg.Rules[1].Name != "check-errors" {
		t.Errorf("expected rule name 'check-errors', got %q", cfg.Rules[1].Name)
	}
}

func TestApplyRepoConfig_InvalidRule(t *testing.T) {
	yaml := `
rules:
  - name: ""
    description: Missing name
`
	cfg := &Config{}
	err := cfg.applyRepoConfig([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for invalid rule")
	}
}

func TestApplyRepoConfig_RulesWithPaths(t *testing.T) {
	yaml := `
rules:
  - name: react-hooks
    description: Enforce React hooks rules
    paths:
      - "*.tsx"
      - "*.jsx"
`
	cfg := &Config{}
	if err := cfg.applyRepoConfig([]byte(yaml)); err != nil {
		t.Fatalf("applyRepoConfig failed: %v", err)
	}
	if len(cfg.Rules[0].Paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(cfg.Rules[0].Paths))
	}
}
