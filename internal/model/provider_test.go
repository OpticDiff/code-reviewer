package model

import (
	"bytes"
	"log/slog"
	"strings"
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

func TestIsGemini3Model(t *testing.T) {
	tests := []struct {
		model string
		want  bool
	}{
		{"gemini-3.8-flash", true},
		{"gemini-3-pro-preview", true},
		{"gemini-3", true},
		{"gemini-2.5-flash", false},
		{"gemini-30-flash", false},
		{"claude-sonnet-4", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isGemini3Model(tt.model); got != tt.want {
			t.Errorf("isGemini3Model(%q) = %v, want %v", tt.model, got, tt.want)
		}
	}
}

func TestProvider_TemperatureConfig(t *testing.T) {
	zero := float32(0)
	tests := []struct {
		name     string
		model    string
		explicit *float32
		want     *float32 // nil = not sent.
	}{
		{"gemini 3 unset is not sent", "gemini-3.8-flash", nil, nil},
		{"gemini 2.5 unset keeps default", "gemini-2.5-flash", nil, genai.Ptr(float32(0.2))},
		{"claude unset keeps default", "claude-sonnet-4", nil, genai.Ptr(float32(0.2))},
		{"gemini 3 explicit zero is honoured", "gemini-3.8-flash", &zero, genai.Ptr(float32(0))},
		{"gemini 2.5 explicit zero is honoured", "gemini-2.5-flash", &zero, genai.Ptr(float32(0))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Provider{modelName: tt.model}
			if tt.explicit != nil {
				p.SetTemperature(*tt.explicit)
			}
			got := p.temperatureConfig()
			switch {
			case tt.want == nil && got != nil:
				t.Errorf("temperatureConfig() = %v, want nil", *got)
			case tt.want != nil && got == nil:
				t.Errorf("temperatureConfig() = nil, want %v", *tt.want)
			case tt.want != nil && *got != *tt.want:
				t.Errorf("temperatureConfig() = %v, want %v", *got, *tt.want)
			}
		})
	}
}

func TestProvider_SetTemperatureWarnsOnGemini3(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	(&Provider{modelName: "gemini-3.8-flash"}).SetTemperature(0)
	if !strings.Contains(buf.String(), "level=WARN") || !strings.Contains(buf.String(), "gemini-3.8-flash") {
		t.Errorf("expected a warning naming the model, got %q", buf.String())
	}

	buf.Reset()
	(&Provider{modelName: "gemini-2.5-flash"}).SetTemperature(0)
	if buf.Len() != 0 {
		t.Errorf("unexpected log for gemini-2.5: %q", buf.String())
	}
}
