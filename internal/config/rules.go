package config

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Rule represents a single custom review rule defined in .code-reviewer.yaml.
// Rules are injected into the AI prompt to enforce team-specific conventions.
type Rule struct {
	// Name is a short identifier for the rule (e.g., "no-raw-sql").
	Name string `yaml:"name"`

	// Description is the instruction given to the AI reviewer.
	// This is what the model sees in the system prompt.
	Description string `yaml:"description"`

	// Category is the finding category to use when this rule fires.
	// Defaults to "custom" if not specified.
	// Valid: bug, security, performance, style, docs, custom.
	Category string `yaml:"category,omitempty"`

	// Severity is the severity to assign when this rule fires.
	// Defaults to "high" if not specified.
	// Valid: low, medium, high, critical.
	Severity string `yaml:"severity,omitempty"`

	// Paths limits the rule to files matching these glob patterns.
	// If empty, the rule applies to all files.
	Paths []string `yaml:"paths,omitempty"`
}

// validRuleCategories are the allowed rule categories.
var validRuleCategories = map[string]bool{
	"bug":         true,
	"security":    true,
	"performance": true,
	"style":       true,
	"docs":        true,
	"custom":      true,
}

// validRuleSeverities are the allowed rule severities.
var validRuleSeverities = map[string]bool{
	"low":      true,
	"medium":   true,
	"high":     true,
	"critical": true,
}

// Validate checks that all required fields are set and values are valid.
func (r *Rule) Validate() error {
	if r.Name == "" {
		return fmt.Errorf("rule missing required field: name")
	}
	if r.Description == "" {
		return fmt.Errorf("rule %q missing required field: description", r.Name)
	}
	if r.Category != "" && !validRuleCategories[strings.ToLower(r.Category)] {
		return fmt.Errorf("rule %q has invalid category %q (valid: bug, security, performance, style, docs, custom)", r.Name, r.Category)
	}
	if r.Severity != "" && !validRuleSeverities[strings.ToLower(r.Severity)] {
		return fmt.Errorf("rule %q has invalid severity %q (valid: low, medium, high, critical)", r.Name, r.Severity)
	}
	// Normalize to lowercase for consistent prompt formatting.
	r.Category = strings.ToLower(r.Category)
	r.Severity = strings.ToLower(r.Severity)
	return nil
}

// EffectiveCategory returns the rule's category, defaulting to "custom".
func (r *Rule) EffectiveCategory() string {
	if r.Category == "" {
		return "custom"
	}
	return r.Category
}

// EffectiveSeverity returns the rule's severity, defaulting to "high".
func (r *Rule) EffectiveSeverity() string {
	if r.Severity == "" {
		return "high"
	}
	return r.Severity
}

// ValidateRules validates a slice of rules and returns the first error found.
func ValidateRules(rules []Rule) error {
	seen := make(map[string]bool)
	for i := range rules {
		if err := rules[i].Validate(); err != nil {
			return fmt.Errorf("rules[%d]: %w", i, err)
		}
		if seen[rules[i].Name] {
			return fmt.Errorf("rules[%d]: duplicate rule name %q", i, rules[i].Name)
		}
		seen[rules[i].Name] = true
	}
	return nil
}

// FormatRulesPrompt formats custom rules into a prompt section for the AI model.
func FormatRulesPrompt(rules []Rule) string {
	if len(rules) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## CUSTOM RULES\n\n")
	sb.WriteString("The following are team-defined review rules. ")
	sb.WriteString("When a rule is violated, use the specified category and severity.\n\n")

	for _, r := range rules {
		fmt.Fprintf(&sb, "### Rule: %s\n", r.Name)
		fmt.Fprintf(&sb, "- **Category**: %s\n", r.EffectiveCategory())
		fmt.Fprintf(&sb, "- **Severity**: %s\n", r.EffectiveSeverity())
		if len(r.Paths) > 0 {
			fmt.Fprintf(&sb, "- **Applies to**: %s\n", strings.Join(r.Paths, ", "))
		}
		fmt.Fprintf(&sb, "- **Rule**: %s\n\n", r.Description)
	}

	return sb.String()
}

// FilterRulesByPaths returns only rules that apply to the given file paths.
// Rules with empty Paths match all files. Rules with Paths match if any
// file path matches any of the rule's glob patterns.
func FilterRulesByPaths(rules []Rule, filePaths []string) []Rule {
	if len(rules) == 0 {
		return nil
	}

	var result []Rule
	for _, r := range rules {
		if len(r.Paths) == 0 {
			// No path restriction — applies to all files.
			result = append(result, r)
			continue
		}
		if ruleMatchesAnyFile(r.Paths, filePaths) {
			result = append(result, r)
		}
	}
	return result
}

// ruleMatchesAnyFile checks if any file path matches any of the glob patterns.
func ruleMatchesAnyFile(patterns, filePaths []string) bool {
	for _, fp := range filePaths {
		base := filepath.Base(fp)
		for _, pattern := range patterns {
			// Match against both full path and basename to support
			// patterns like "*.go" (basename) and "internal/*.go" (path).
			if matched, _ := filepath.Match(pattern, fp); matched {
				return true
			}
			if matched, _ := filepath.Match(pattern, base); matched {
				return true
			}
		}
	}
	return false
}
