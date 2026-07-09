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

