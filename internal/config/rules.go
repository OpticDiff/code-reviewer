package config

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/OpticDiff/code-reviewer/pkg/rules"
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

	// URL is an optional link to runbooks or documentation for this rule.
	URL string `yaml:"url,omitempty"`

	// Checklist is an optional path to a Markdown checklist file.
	Checklist string `yaml:"checklist,omitempty"`

	// AllowSuppression controls whether inline comment suppressions are honored.
	// Defaults to true if nil.
	AllowSuppression *bool `yaml:"allow_suppression,omitempty"`

	// Source indicates where the rule originated ("platform" or "repo").
	Source string `yaml:"-"`

	// SourceFile is the filename where the rule was defined (e.g. "hipaa-phi.yaml").
	SourceFile string `yaml:"-"`
}

// IsSuppressionAllowed returns true if inline suppression is allowed for this rule.
// For platform rules, inline suppression is denied by default unless explicitly allowed (allow_suppression: true).
// For repository/custom rules, inline suppression is allowed by default unless explicitly denied (allow_suppression: false).
func (r *Rule) IsSuppressionAllowed() bool {
	if r.AllowSuppression == nil {
		return r.Source != "platform"
	}
	return *r.AllowSuppression
}

// validRuleCategories are the allowed rule categories.
var validRuleCategories = map[string]bool{
	"bug":          true,
	"security":     true,
	"performance":  true,
	"style":        true,
	"docs":         true,
	"custom":       true,
	"architecture": true,
	"compliance":   true,
	"reliability":  true,
	"governance":   true,
	"quality":      true,
	"contract":     true,
	"guidance":     true,
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
		return fmt.Errorf("rule %q has invalid category %q (valid: bug, security, performance, style, docs, custom, architecture, compliance, reliability, governance, quality, contract, guidance)", r.Name, r.Category)
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
// If platform rules are present, they are rendered under a distinct high-priority section.
func FormatRulesPrompt(customRules []Rule, loader *rules.RuleLoader) string {
	if len(customRules) == 0 {
		return ""
	}

	var platformRules, repoRules []Rule
	for _, r := range customRules {
		if r.Source == "platform" {
			platformRules = append(platformRules, r)
		} else {
			repoRules = append(repoRules, r)
		}
	}

	var sb strings.Builder
	if len(platformRules) > 0 {
		sb.WriteString("## MANDATORY PLATFORM COMPLIANCE RULES\n\n")
		sb.WriteString("The following are organization-wide platform and security mandates. ")
		sb.WriteString("These rules are NON-NEGOTIABLE and take precedence over repository-level conventions. ")
		sb.WriteString("When any of these rules is violated, you MUST report it with the specified category, severity, and include the exact rule name in the \"rule_name\" field of the finding.\n\n")

		for _, r := range platformRules {
			formatRuleEntry(&sb, r, true, loader)
		}
	}

	if len(repoRules) > 0 {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString("## CUSTOM RULES\n\n")
		sb.WriteString("The following are team-defined review rules. ")
		sb.WriteString("When a rule is violated, use the specified category and severity, and include the rule name in the \"rule_name\" field.\n\n")

		for _, r := range repoRules {
			formatRuleEntry(&sb, r, false, loader)
		}
	}

	return sb.String()
}

func formatRuleEntry(sb *strings.Builder, r Rule, isPlatform bool, loader *rules.RuleLoader) {
	if isPlatform {
		fmt.Fprintf(sb, "### [Platform Mandate] %s\n", r.Name)
	} else {
		fmt.Fprintf(sb, "### Rule: %s\n", r.Name)
	}
	fmt.Fprintf(sb, "- **Category**: %s\n", r.EffectiveCategory())
	fmt.Fprintf(sb, "- **Severity**: %s\n", r.EffectiveSeverity())
	if len(r.Paths) > 0 {
		fmt.Fprintf(sb, "- **Applies to**: %s\n", strings.Join(r.Paths, ", "))
	}
	if r.URL != "" {
		fmt.Fprintf(sb, "- **Documentation**: %s\n", r.URL)
	}
	
	description := r.Description
	if r.Checklist != "" && loader != nil {
		if content, err := loader.Load(r.Checklist); err == nil {
			description += "\n\n" + content
		}
	}
	
	fmt.Fprintf(sb, "- **Rule**: %s\n\n", description)
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
		for _, pattern := range patterns {
			if matchPathPattern(pattern, fp) {
				return true
			}
		}
	}
	return false
}

func matchPathPattern(pattern, fp string) bool {
	base := filepath.Base(fp)
	// Standard filepath.Match against full path and basename
	if matched, _ := filepath.Match(pattern, fp); matched {
		return true
	}
	if matched, _ := filepath.Match(pattern, base); matched {
		return true
	}
	// Prefix **/ (e.g. **/*.kt matches Service.kt and path/to/Service.kt)
	if strings.HasPrefix(pattern, "**/") {
		subPattern := strings.TrimPrefix(pattern, "**/")
		if matched, _ := filepath.Match(subPattern, base); matched {
			return true
		}
		if matched, _ := filepath.Match(subPattern, fp); matched {
			return true
		}
	}
	// Globstar regex translation supporting **, *, ?, and character classes [...]
	if strings.Contains(pattern, "**") {
		if re, err := globstarToRegexp(pattern); err == nil {
			if re.MatchString(fp) || re.MatchString(base) {
				return true
			}
		}
	}
	return false
}

// globstarToRegexp translates a glob pattern supporting ** (globstar), *, ?, and [character classes] into a regex.
func globstarToRegexp(pattern string) (*regexp.Regexp, error) {
	var sb strings.Builder
	sb.WriteString("^")
	i := 0
	n := len(pattern)
	for i < n {
		// Middle or trailing /**/ matches / or /.../ (zero or more directories)
		if strings.HasPrefix(pattern[i:], "/**/") {
			sb.WriteString("(?:/.+)?/")
			i += 4
			continue
		}
		// Leading **/ matches zero or more directories at the beginning
		if strings.HasPrefix(pattern[i:], "**/") {
			sb.WriteString("(?:.*/)?")
			i += 3
			continue
		}
		// Trailing /** matches zero or more directories at the end
		if strings.HasPrefix(pattern[i:], "/**") && i+3 == n {
			sb.WriteString("(?:/.*)?")
			i += 3
			continue
		}
		// Standalone ** matches any path sequence
		if strings.HasPrefix(pattern[i:], "**") {
			sb.WriteString(".*")
			i += 2
			continue
		}

		ch := pattern[i]
		switch ch {
		case '*':
			sb.WriteString("[^/]*")
			i++
		case '?':
			sb.WriteString("[^/]")
			i++
		case '[':
			// Preserve character classes [0-9], [a-z], [!...]
			j := i + 1
			if j < n && (pattern[j] == '!' || pattern[j] == '^') {
				j++
			}
			if j < n && pattern[j] == ']' {
				j++
			}
			for j < n && pattern[j] != ']' {
				j++
			}
			if j < n && pattern[j] == ']' {
				classContent := pattern[i+1 : j]
				sb.WriteString("[")
				if strings.HasPrefix(classContent, "!") {
					sb.WriteString("^")
					classContent = classContent[1:]
				}
				sb.WriteString(classContent)
				sb.WriteString("]")
				i = j + 1
			} else {
				sb.WriteString(`\[`)
				i++
			}
		default:
			if strings.ContainsRune(".+()|{}^$\\-", rune(ch)) {
				sb.WriteByte('\\')
			}
			sb.WriteByte(ch)
			i++
		}
	}
	sb.WriteString("$")
	return regexp.Compile(sb.String())
}
