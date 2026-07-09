package model

import (
	"testing"
)

func TestParseReviewJSON_Direct(t *testing.T) {
	input := `{"summary":"looks good","findings":[]}`
	result, err := parseReviewJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Summary != "looks good" {
		t.Errorf("got summary %q", result.Summary)
	}
	if len(result.Findings) != 0 {
		t.Errorf("expected 0 findings, got %d", len(result.Findings))
	}
}

func TestParseReviewJSON_MarkdownFence(t *testing.T) {
	input := "```json\n{\"summary\":\"fenced\",\"findings\":[]}\n```"
	result, err := parseReviewJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Summary != "fenced" {
		t.Errorf("got summary %q", result.Summary)
	}
}

func TestParseReviewJSON_MarkdownFenceUppercase(t *testing.T) {
	input := "```JSON\n{\"summary\":\"upper\",\"findings\":[]}\n```"
	result, err := parseReviewJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Summary != "upper" {
		t.Errorf("got summary %q", result.Summary)
	}
}

func TestParseReviewJSON_BareFence(t *testing.T) {
	input := "```\n{\"summary\":\"bare\",\"findings\":[]}\n```"
	result, err := parseReviewJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Summary != "bare" {
		t.Errorf("got summary %q", result.Summary)
	}
}

func TestParseReviewJSON_SurroundingText(t *testing.T) {
	input := "Here is my review:\n{\"summary\":\"surrounded\",\"findings\":[]}\nI hope this helps!"
	result, err := parseReviewJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Summary != "surrounded" {
		t.Errorf("got summary %q", result.Summary)
	}
}

func TestParseReviewJSON_WithFindings(t *testing.T) {
	input := `{
		"summary": "found issues",
		"findings": [
			{
				"file": "main.go",
				"line": 42,
				"severity": "HIGH",
				"category": "bug",
				"title": "nil deref",
				"body": "Pointer could be nil."
			}
		]
	}`
	result, err := parseReviewJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(result.Findings))
	}
	if result.Findings[0].Title != "nil deref" {
		t.Errorf("got title %q", result.Findings[0].Title)
	}
}

func TestParseReviewJSON_Malformed(t *testing.T) {
	_, err := parseReviewJSON("this is not json at all")
	if err == nil {
		t.Fatal("expected error for malformed input")
	}
}

func TestParseReviewJSON_EmptyString(t *testing.T) {
	_, err := parseReviewJSON("")
	if err == nil {
		t.Fatal("expected error for empty input")
	}
}

func TestParseReviewJSON_NestedBraces(t *testing.T) {
	// Model output with nested braces in suggestion field.
	input := `Some preamble text
{
  "summary": "nested",
  "findings": [
    {
      "file": "main.go",
      "line": 10,
      "severity": "HIGH",
      "category": "bug",
      "title": "test",
      "body": "use map[string]struct{} instead"
    }
  ]
}
Some trailing text`
	result, err := parseReviewJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Summary != "nested" {
		t.Errorf("got summary %q", result.Summary)
	}
}

func TestParseReviewJSON_WhitespaceWrapped(t *testing.T) {
	input := "\n  \n  {\"summary\":\"spaced\",\"findings\":[]}\n  \n"
	result, err := parseReviewJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Summary != "spaced" {
		t.Errorf("got summary %q", result.Summary)
	}
}

