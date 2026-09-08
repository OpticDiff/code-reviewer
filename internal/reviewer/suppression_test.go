package reviewer

import (
	"testing"

	"github.com/OpticDiff/code-reviewer/internal/config"
	"github.com/OpticDiff/code-reviewer/internal/diff"
	"github.com/OpticDiff/code-reviewer/internal/model"
)

// TestEnforceRuleAttribution_PlatformRule verifies deterministic attribution and severity locking for platform rules.
func TestEnforceRuleAttribution_PlatformRule(t *testing.T) {
	allowFalse := false
	cfg := &config.Config{
		Rules: []config.Rule{
			{
				Name:             "platform-no-raw-sql",
				Description:      "Flag raw SQL concatenation",
				Category:         "security",
				Severity:         "critical",
				URL:              "https://docs.azra.internal/standards/sql",
				AllowSuppression: &allowFalse,
				Source:           "platform",
				SourceFile:       "security.yaml",
			},
			{
				Name:        "team-style-rule",
				Description: "Style guide",
				Category:    "style",
				Severity:    "low",
				Source:      "repo",
			},
		},
	}

	r := &Reviewer{cfg: cfg}

	findings := []model.Finding{
		{
			File:     "internal/db/query.go",
			Line:     42,
			RuleName: "platform-no-raw-sql",
			Severity: "LOW", // Model attempted to downplay severity
			Category: "bug", // Model attempted to misclassify
			Title:    "Concatenated SQL query",
		},
		{
			File:     "internal/api/handler.go",
			Line:     10,
			RuleName: "team-style-rule",
			Title:    "Format style",
		},
		{
			File:     "internal/api/handler.go",
			Line:     15,
			Title:    "General bug finding",
		},
	}

	diffs := []diff.FileDiff{
		{
			NewPath: "internal/db/query.go",
		},
		{
			NewPath: "internal/api/handler.go",
		},
	}

	attributed := r.enforceRuleAttributionAndSuppressions(findings, diffs)

	if len(attributed) != 3 {
		t.Fatalf("expected 3 findings, got %d", len(attributed))
	}

	// Platform rule: check deterministic attribution & locked severity
	f0 := attributed[0]
	if f0.RuleSource != "platform" {
		t.Errorf("expected RuleSource to be platform, got %s", f0.RuleSource)
	}
	if f0.RuleFile != "security.yaml" {
		t.Errorf("expected RuleFile to be security.yaml, got %s", f0.RuleFile)
	}
	if f0.RuleURL != "https://docs.azra.internal/standards/sql" {
		t.Errorf("expected RuleURL to be populated, got %s", f0.RuleURL)
	}
	if f0.Severity != "critical" {
		t.Errorf("expected platform severity critical to override model severity, got %s", f0.Severity)
	}
	if f0.Category != "security" {
		t.Errorf("expected platform category security to override model category, got %s", f0.Category)
	}

	// Repo rule: check repo attribution
	f1 := attributed[1]
	if f1.RuleSource != "repo" {
		t.Errorf("expected RuleSource to be repo, got %s", f1.RuleSource)
	}

	// Unattributed finding
	f2 := attributed[2]
	if f2.RuleSource != "" {
		t.Errorf("expected RuleSource to be empty, got %s", f2.RuleSource)
	}
}

// TestCheckInlineSuppression verifies inline comment suppression parsing across diff hunks.
func TestCheckInlineSuppression(t *testing.T) {
	fd := diff.FileDiff{
		NewPath: "internal/db/query.go",
		Hunks: []diff.Hunk{
			{
				Lines: []diff.DiffLine{
					{
						Type:      diff.LineAdded,
						NewLineNo: 41,
						Content:   "// opticdiff:ignore platform-no-raw-sql: internal tool script",
					},
					{
						Type:      diff.LineAdded,
						NewLineNo: 42,
						Content:   `query := "SELECT * FROM " + tableName`,
					},
				},
			},
		},
	}

	// Line 42 should match the suppression on line 41
	suppressed, reason := checkInlineSuppression(fd, 42, "platform-no-raw-sql")
	if !suppressed {
		t.Error("expected finding on line 42 to be suppressed by comment on line 41")
	}
	if reason != "internal tool script" {
		t.Errorf("expected reason 'internal tool script', got %q", reason)
	}

	// Different rule should not be suppressed
	diffRuleSuppressed, _ := checkInlineSuppression(fd, 42, "other-rule")
	if diffRuleSuppressed {
		t.Error("expected finding for other-rule NOT to be suppressed")
	}

	// Test opticdiff:ignore all
	fdAll := diff.FileDiff{
		NewPath: "internal/db/query.go",
		Hunks: []diff.Hunk{
			{
				Lines: []diff.DiffLine{
					{
						Type:      diff.LineAdded,
						NewLineNo: 10,
						Content:   "# opticdiff:ignore all: emergency hotfix approved by security lead",
					},
				},
			},
		},
	}
	allSuppressed, allReason := checkInlineSuppression(fdAll, 10, "any-rule-name")
	if !allSuppressed {
		t.Error("expected 'all' suppression to match")
	}
	if allReason != "emergency hotfix approved by security lead" {
		t.Errorf("expected allReason, got %q", allReason)
	}
}

// TestCheckInlineSuppression_SkipsRemovedLines verifies that deleted lines (NewLineNo == 0)
// do not accidentally suppress findings on lines 1 or 2.
func TestCheckInlineSuppression_SkipsRemovedLines(t *testing.T) {
	fd := diff.FileDiff{
		NewPath: "internal/init.go",
		Hunks: []diff.Hunk{
			{
				Lines: []diff.DiffLine{
					{
						Type:      diff.LineRemoved,
						NewLineNo: 0,
						OldLineNo: 1,
						Content:   "// opticdiff:ignore all: old deleted comment",
					},
					{
						Type:      diff.LineAdded,
						NewLineNo: 1,
						Content:   "package main",
					},
					{
						Type:      diff.LineAdded,
						NewLineNo: 2,
						Content:   "import \"os\"",
					},
				},
			},
		},
	}

	suppressed, _ := checkInlineSuppression(fd, 1, "any-rule")
	if suppressed {
		t.Error("deleted line should NOT suppress finding on line 1")
	}

	suppressed2, _ := checkInlineSuppression(fd, 2, "any-rule")
	if suppressed2 {
		t.Error("deleted line should NOT suppress finding on line 2")
	}
}

// TestEnforceRuleAttribution_PlatformDefaultSuppressionDenied verifies that platform rules
// deny inline suppression by default (AllowSuppression == nil), unless explicitly opted in.
func TestEnforceRuleAttribution_PlatformDefaultSuppressionDenied(t *testing.T) {
	allowTrue := true
	cfg := &config.Config{
		Rules: []config.Rule{
			{
				Name:        "platform-default-strict",
				Description: "Platform rule with nil AllowSuppression (default deny)",
				Category:    "security",
				Severity:    "high",
				Source:      "platform",
			},
			{
				Name:             "platform-opt-in",
				Description:      "Platform rule with explicit AllowSuppression: true",
				Category:         "security",
				Severity:         "medium",
				AllowSuppression: &allowTrue,
				Source:           "platform",
			},
			{
				Name:        "repo-rule",
				Description: "Repo rule with nil AllowSuppression (default allow)",
				Category:    "style",
				Severity:    "low",
				Source:      "repo",
			},
		},
	}

	r := &Reviewer{cfg: cfg}

	diffs := []diff.FileDiff{
		{
			NewPath: "main.go",
			Hunks: []diff.Hunk{
				{
					Lines: []diff.DiffLine{
						{
							Type:      diff.LineAdded,
							NewLineNo: 5,
							Content:   "// opticdiff:ignore platform-default-strict: tried to suppress",
						},
						{
							Type:      diff.LineAdded,
							NewLineNo: 6,
							Content:   "// opticdiff:ignore platform-opt-in: allowed suppression",
						},
						{
							Type:      diff.LineAdded,
							NewLineNo: 7,
							Content:   "// opticdiff:ignore repo-rule: allowed suppression",
						},
						{
							Type:      diff.LineAdded,
							NewLineNo: 8,
							Content:   "code := 123",
						},
					},
				},
			},
		},
	}

	findings := []model.Finding{
		{
			File:     "main.go",
			Line:     8,
			RuleName: "platform-default-strict",
			Title:    "Strict platform finding",
		},
		{
			File:     "main.go",
			Line:     8,
			RuleName: "platform-opt-in",
			Title:    "Opt-in platform finding",
		},
		{
			File:     "main.go",
			Line:     8,
			RuleName: "repo-rule",
			Title:    "Repo finding",
		},
	}

	result := r.enforceRuleAttributionAndSuppressions(findings, diffs)

	// platform-default-strict must NOT be suppressed (should be in result)
	// platform-opt-in MUST be suppressed (not in result)
	// repo-rule MUST be suppressed (not in result)
	if len(result) != 1 {
		t.Fatalf("expected exactly 1 finding remaining (strict platform rule), got %d: %+v", len(result), result)
	}
	if result[0].RuleName != "platform-default-strict" {
		t.Errorf("expected platform-default-strict finding, got %s", result[0].RuleName)
	}
}
