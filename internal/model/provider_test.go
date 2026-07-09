package model

import (
	"testing"

	"google.golang.org/genai"
)

// ---------------------------------------------------------------------------
// parseReviewJSON
// ---------------------------------------------------------------------------

func TestParseReviewJSON_ValidJSON(t *testing.T) {
	input := `{"summary":"looks good","findings":[{"file":"a.go","line":1,"severity":"LOW","category":"style","title":"nit","body":"minor"}]}`
	result, err := parseReviewJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Summary != "looks good" {
		t.Errorf("summary = %q, want %q", result.Summary, "looks good")
	}
	if len(result.Findings) != 1 {
		t.Fatalf("findings count = %d, want 1", len(result.Findings))
	}
	if result.Findings[0].File != "a.go" {
		t.Errorf("file = %q, want %q", result.Findings[0].File, "a.go")
	}
}

func TestParseReviewJSON_FencedJSON(t *testing.T) {
	input := "```json\n{\"summary\":\"ok\",\"findings\":[]}\n```"
	result, err := parseReviewJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Summary != "ok" {
		t.Errorf("summary = %q, want %q", result.Summary, "ok")
	}
	if len(result.Findings) != 0 {
		t.Errorf("findings count = %d, want 0", len(result.Findings))
	}
}

func TestParseReviewJSON_FencedUppercase(t *testing.T) {
	input := "```JSON\n{\"summary\":\"uppercase fence\",\"findings\":[]}\n```"
	result, err := parseReviewJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Summary != "uppercase fence" {
		t.Errorf("summary = %q, want %q", result.Summary, "uppercase fence")
	}
}

func TestParseReviewJSON_BraceExtraction(t *testing.T) {
	input := `Here is my review:

{"summary":"extracted","findings":[{"file":"b.go","line":10,"severity":"HIGH","category":"bug","title":"oops","body":"details"}]}

Hope this helps!`

	result, err := parseReviewJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Summary != "extracted" {
		t.Errorf("summary = %q, want %q", result.Summary, "extracted")
	}
	if len(result.Findings) != 1 {
		t.Fatalf("findings count = %d, want 1", len(result.Findings))
	}
}

func TestParseReviewJSON_InvalidJSON(t *testing.T) {
	input := "this is not json at all"
	_, err := parseReviewJSON(input)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestParseReviewJSON_EmptyFindings(t *testing.T) {
	input := `{"summary":"all clear","findings":[]}`
	result, err := parseReviewJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Summary != "all clear" {
		t.Errorf("summary = %q, want %q", result.Summary, "all clear")
	}
	if len(result.Findings) != 0 {
		t.Errorf("findings count = %d, want 0", len(result.Findings))
	}
}

func TestParseReviewJSON_TrailingFence(t *testing.T) {
	input := "```json\n{\"summary\":\"trailing\",\"findings\":[]}\n```\nSome extra text after the fence."
	result, err := parseReviewJSON(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Summary != "trailing" {
		t.Errorf("summary = %q, want %q", result.Summary, "trailing")
	}
}

// ---------------------------------------------------------------------------
// extractText
// ---------------------------------------------------------------------------

func TestExtractText_NilResponse(t *testing.T) {
	got := extractText(nil)
	if got != "" {
		t.Errorf("extractText(nil) = %q, want empty", got)
	}
}

func TestExtractText_NoCandidates(t *testing.T) {
	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{},
	}
	got := extractText(resp)
	if got != "" {
		t.Errorf("extractText(empty candidates) = %q, want empty", got)
	}
}

func TestExtractText_MultiPart(t *testing.T) {
	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content: &genai.Content{
				Parts: []*genai.Part{
					{Text: "hello "},
					{Text: "world"},
				},
			},
		}},
	}
	got := extractText(resp)
	want := "hello world"
	if got != want {
		t.Errorf("extractText = %q, want %q", got, want)
	}
}

// ---------------------------------------------------------------------------
// isGeminiModel
// ---------------------------------------------------------------------------

func TestIsGeminiModel(t *testing.T) {
	tests := []struct {
		name  string
		model string
		want  bool
	}{
		{"gemini flash", "gemini-2.5-flash", true},
		{"claude sonnet", "claude-sonnet-4", false},
		{"mistral medium", "mistral-medium-3", false},
		{"gemini pro", "gemini-2.5-pro", true},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isGeminiModel(tt.model)
			if got != tt.want {
				t.Errorf("isGeminiModel(%q) = %v, want %v", tt.model, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// truncate
// ---------------------------------------------------------------------------

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"short unchanged", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"long truncated", "hello world", 5, "hello..."},
		{"empty string", "", 5, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.input, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, got, tt.want)
			}
		})
	}
}
