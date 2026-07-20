package model

import (
	"strings"
	"testing"
)

func TestBuildExplainPrompt_NotEmpty(t *testing.T) {
	result := BuildExplainPrompt()
	if result == "" {
		t.Error("expected non-empty explain prompt")
	}
	if !strings.Contains(result, "explaining code changes") {
		t.Error("expected explain persona in prompt")
	}
}

func TestBuildExplainPrompt_AdversarialWarning(t *testing.T) {
	result := BuildExplainPrompt()
	// Must explicitly call out ALL untrusted inputs.
	if !strings.Contains(result, "MR title") {
		t.Error("adversarial warning must mention MR title as untrusted")
	}
	if !strings.Contains(result, "MR description") {
		t.Error("adversarial warning must mention MR description as untrusted")
	}
	if !strings.Contains(result, "diff") {
		t.Error("adversarial warning must mention diff as untrusted")
	}
	if !strings.Contains(result, "untrusted data") {
		t.Error("adversarial warning must label inputs as untrusted data")
	}
}

func TestBuildExplainUserPrompt_WithTitle(t *testing.T) {
	result := BuildExplainUserPrompt("Fix login bug", "Resolves race condition", "+ fixed line")
	if !strings.Contains(result, "Fix login bug") {
		t.Error("expected MR title in user prompt")
	}
	if !strings.Contains(result, "Resolves race condition") {
		t.Error("expected MR description in user prompt")
	}
	if !strings.Contains(result, "+ fixed line") {
		t.Error("expected diff content in user prompt")
	}
}

func TestBuildExplainUserPrompt_NoTitle(t *testing.T) {
	result := BuildExplainUserPrompt("", "", "+ added line")
	if strings.Contains(result, "MR Title") {
		t.Error("should not contain MR Title header when empty")
	}
	if !strings.Contains(result, "+ added line") {
		t.Error("expected diff content in user prompt")
	}
}
