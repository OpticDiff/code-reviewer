package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildPrompt_AllFocus(t *testing.T) {
	prompt := BuildPrompt([]string{"all"}, "")
	if !strings.Contains(prompt, "PERSONA") {
		t.Error("prompt missing base PERSONA section")
	}
	if !strings.Contains(prompt, "Bug Detection") {
		t.Error("prompt missing bugs focus")
	}
	if !strings.Contains(prompt, "Security Review") {
		t.Error("prompt missing security focus")
	}
	if !strings.Contains(prompt, "Performance Review") {
		t.Error("prompt missing performance focus")
	}
}

func TestBuildPrompt_SingleFocus(t *testing.T) {
	prompt := BuildPrompt([]string{"security"}, "")
	if !strings.Contains(prompt, "Security Review") {
		t.Error("prompt missing security focus")
	}
	if strings.Contains(prompt, "Bug Detection") {
		t.Error("prompt should not contain bugs focus when only security is selected")
	}
}

func TestBuildPrompt_MultipleFocus(t *testing.T) {
	prompt := BuildPrompt([]string{"bugs", "performance"}, "")
	if !strings.Contains(prompt, "Bug Detection") {
		t.Error("prompt missing bugs focus")
	}
	if !strings.Contains(prompt, "Performance Review") {
		t.Error("prompt missing performance focus")
	}
	if strings.Contains(prompt, "Security Review") {
		t.Error("prompt should not contain security focus")
	}
}

func TestBuildPrompt_ExtraRules(t *testing.T) {
	rules := "Always flag raw SQL queries."
	prompt := BuildPrompt([]string{"all"}, rules)
	if !strings.Contains(prompt, "ADDITIONAL RULES") {
		t.Error("prompt missing ADDITIONAL RULES section")
	}
	if !strings.Contains(prompt, rules) {
		t.Error("prompt missing custom rules content")
	}
}

func TestBuildPrompt_NoExtraRules(t *testing.T) {
	prompt := BuildPrompt([]string{"all"}, "")
	if strings.Contains(prompt, "ADDITIONAL RULES") {
		t.Error("prompt should not contain ADDITIONAL RULES when none provided")
	}
}

func TestBuildPrompt_EmptyFocus(t *testing.T) {
	// Empty focus should default to all.
	prompt := BuildPrompt(nil, "")
	if !strings.Contains(prompt, "Bug Detection") {
		t.Error("empty focus should include all overlays")
	}
}

func TestBuildUserPrompt(t *testing.T) {
	prompt := BuildUserPrompt("Fix nil deref", "Handles nil response", "+ fixed line")
	if !strings.Contains(prompt, "Fix nil deref") {
		t.Error("user prompt missing MR title")
	}
	if !strings.Contains(prompt, "Handles nil response") {
		t.Error("user prompt missing MR description")
	}
	if !strings.Contains(prompt, "```diff") {
		t.Error("user prompt missing diff block")
	}
}

func TestBuildUserPrompt_NoMetadata(t *testing.T) {
	prompt := BuildUserPrompt("", "", "+ some code")
	if strings.Contains(prompt, "Merge Request:") {
		t.Error("should not include MR title header when empty")
	}
	if !strings.Contains(prompt, "```diff") {
		t.Error("user prompt missing diff block")
	}
}

func TestBuildPromptWithCustom_ValidFile(t *testing.T) {
	dir := t.TempDir()
	promptFile := filepath.Join(dir, "custom.md")
	if err := os.WriteFile(promptFile, []byte("You are a Go specialist."), 0o644); err != nil {
		t.Fatal(err)
	}

	prompt := BuildPromptWithCustom(promptFile, []string{"security"}, "Flag raw SQL.")
	if !strings.Contains(prompt, "You are a Go specialist.") {
		t.Error("prompt should contain custom prompt content")
	}
	if strings.Contains(prompt, "Principal Software Engineer") {
		t.Error("prompt should NOT contain built-in base prompt when custom is provided")
	}
	if !strings.Contains(prompt, "Security Review") {
		t.Error("focus overlays should still be appended to custom prompt")
	}
	if !strings.Contains(prompt, "Flag raw SQL.") {
		t.Error("extra rules should still be appended to custom prompt")
	}
}

func TestBuildPromptWithCustom_MissingFile(t *testing.T) {
	prompt := BuildPromptWithCustom("/nonexistent/path/prompt.md", []string{"all"}, "")
	// Should fall back to built-in prompt.
	if !strings.Contains(prompt, "Principal Software Engineer") {
		t.Error("missing custom prompt file should fall back to built-in base prompt")
	}
}

func TestBuildPromptWithCustom_EmptyPath(t *testing.T) {
	prompt := BuildPromptWithCustom("", []string{"bugs"}, "")
	// Empty path = use built-in prompt (same as BuildPrompt).
	if !strings.Contains(prompt, "Principal Software Engineer") {
		t.Error("empty custom prompt path should use built-in base prompt")
	}
	if !strings.Contains(prompt, "Bug Detection") {
		t.Error("focus overlays should be applied")
	}
}

func TestBuildPromptFull_ReviewMD(t *testing.T) {
	reviewMD := "## Always check\n- New API routes have integration tests\n- No PII in logs"
	prompt := BuildPromptFull("", reviewMD, []string{"bugs"}, "")

	if !strings.Contains(prompt, "REVIEW INSTRUCTIONS (HIGHEST PRIORITY)") {
		t.Error("prompt should contain REVIEW INSTRUCTIONS header")
	}
	if !strings.Contains(prompt, "New API routes have integration tests") {
		t.Error("prompt should contain REVIEW.md content")
	}
	if !strings.Contains(prompt, "No PII in logs") {
		t.Error("prompt should contain all REVIEW.md content")
	}
	// Verify REVIEW.md comes after base prompt and focus overlays.
	baseIdx := strings.Index(prompt, "Principal Software Engineer")
	reviewIdx := strings.Index(prompt, "REVIEW INSTRUCTIONS")
	if reviewIdx <= baseIdx {
		t.Error("REVIEW.md should appear after base prompt (recency = highest priority)")
	}
	// Verify immutable guardrails come after REVIEW.md.
	guardrailIdx := strings.Index(prompt, "IMMUTABLE OUTPUT CONSTRAINTS")
	if guardrailIdx < 0 {
		t.Fatal("prompt should contain IMMUTABLE OUTPUT CONSTRAINTS")
	}
	if guardrailIdx <= reviewIdx {
		t.Error("immutable guardrails should appear after REVIEW.md")
	}
}

func TestBuildPromptFull_ReviewMD_AfterExtraRules(t *testing.T) {
	reviewMD := "Only report CRITICAL severity."
	prompt := BuildPromptFull("", reviewMD, []string{"all"}, "Flag raw SQL.")

	rulesIdx := strings.Index(prompt, "ADDITIONAL RULES")
	reviewIdx := strings.Index(prompt, "REVIEW INSTRUCTIONS")
	if rulesIdx < 0 || reviewIdx < 0 {
		t.Fatal("both ADDITIONAL RULES and REVIEW INSTRUCTIONS should be present")
	}
	if reviewIdx <= rulesIdx {
		t.Error("REVIEW.md should appear after extra rules (highest priority = last)")
	}
}

func TestBuildPromptFull_EmptyReviewMD(t *testing.T) {
	prompt := BuildPromptFull("", "", []string{"bugs"}, "")
	if strings.Contains(prompt, "REVIEW INSTRUCTIONS") {
		t.Error("empty reviewMD should not inject REVIEW INSTRUCTIONS section")
	}
}

func TestBuildPromptFull_WithCustomPromptAndReviewMD(t *testing.T) {
	dir := t.TempDir()
	promptFile := filepath.Join(dir, "custom.md")
	if err := os.WriteFile(promptFile, []byte("You are a security auditor."), 0o644); err != nil {
		t.Fatal(err)
	}

	reviewMD := "Focus on SQL injection only."
	prompt := BuildPromptFull(promptFile, reviewMD, []string{"security"}, "")

	if !strings.Contains(prompt, "You are a security auditor.") {
		t.Error("custom prompt should be used as base")
	}
	if !strings.Contains(prompt, "Focus on SQL injection only.") {
		t.Error("REVIEW.md should be appended")
	}
	if strings.Contains(prompt, "Principal Software Engineer") {
		t.Error("built-in base should not be present when custom prompt is used")
	}
}

func TestBuildUserPromptWithContext_WithSnippets(t *testing.T) {
	snippets := []ContextSnippet{
		{File: "handler.go", Line: 42, Content: "auth.ValidateSession(token)", Symbol: "ValidateSession"},
		{File: "middleware.go", Line: 18, Content: "auth.ValidateSession(req.Token)", Symbol: "ValidateSession"},
	}

	prompt := BuildUserPromptWithContext("Fix auth", "Updated session validation", "diff content", snippets)

	if !strings.Contains(prompt, "Related Unchanged Code") {
		t.Error("should contain Related Unchanged Code section")
	}
	if !strings.Contains(prompt, "handler.go:42") {
		t.Error("should contain handler.go:42 reference")
	}
	if !strings.Contains(prompt, "middleware.go:18") {
		t.Error("should contain middleware.go:18 reference")
	}
	if !strings.Contains(prompt, "`ValidateSession`") {
		t.Error("should contain symbol name")
	}
	if !strings.Contains(prompt, "auth.ValidateSession(token)") {
		t.Error("should contain snippet content")
	}
}

func TestBuildUserPromptWithContext_NoSnippets(t *testing.T) {
	prompt := BuildUserPromptWithContext("Fix auth", "desc", "diff content", nil)
	if strings.Contains(prompt, "Related Unchanged Code") {
		t.Error("should not contain Related Unchanged Code when no snippets")
	}
	// Should still contain the normal diff.
	if !strings.Contains(prompt, "diff content") {
		t.Error("should contain diff content")
	}
}

func TestBuildUserPromptWithContext_EmptySnippets(t *testing.T) {
	prompt := BuildUserPromptWithContext("Fix auth", "desc", "diff content", []ContextSnippet{})
	if strings.Contains(prompt, "Related Unchanged Code") {
		t.Error("empty snippets should not inject the Related Unchanged Code section")
	}
}
