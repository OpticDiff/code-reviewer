// Package model provides the AI model integration for code review via Vertex AI.
package model

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/OpticDiff/code-reviewer/internal/retry"
	"google.golang.org/genai"
)

// TokenUsage tracks input/output token counts for cost visibility.
type TokenUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	TotalTokens  int64 `json:"total_tokens"`
}

// ReviewResult is the structured output from the model.
type ReviewResult struct {
	Summary  string      `json:"summary"`
	Findings []Finding   `json:"findings"`
	Usage    *TokenUsage `json:"usage,omitempty"`
}

// Finding is a single review comment from the model.
type Finding struct {
	File       string `json:"file"`
	Line       int    `json:"line"`
	Severity   string `json:"severity"`
	Category   string `json:"category"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	Suggestion string `json:"suggestion,omitempty"`
}

// Provider wraps the Vertex AI genai client for code review.
type Provider struct {
	client    *genai.Client
	modelName string
}

// NewProvider creates a new model provider using Vertex AI with ADC.
// If proxyURL is non-empty, model calls are routed through that URL
// (e.g., a Candela observability proxy).
func NewProvider(ctx context.Context, project, location, modelName, proxyURL string) (*Provider, error) {
	clientCfg := &genai.ClientConfig{
		Project:  project,
		Location: location,
		Backend:  genai.BackendVertexAI,
	}
	if proxyURL != "" {
		clientCfg.HTTPOptions = genai.HTTPOptions{BaseURL: proxyURL}
	}

	client, err := genai.NewClient(ctx, clientCfg)
	if err != nil {
		return nil, fmt.Errorf("creating genai client: %w", err)
	}

	return &Provider{
		client:    client,
		modelName: modelName,
	}, nil
}

// Review sends a diff to the model for review and returns structured findings.
func (p *Provider) Review(ctx context.Context, systemPrompt, userPrompt string) (*ReviewResult, error) {
	text, usage, err := p.generateRaw(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, err
	}

	// Parse JSON response.
	review, err := parseReviewJSON(text)
	if err != nil {
		return nil, fmt.Errorf("parsing model response: %w (raw: %s)", err, truncate(text, 500))
	}

	review.Usage = usage
	return review, nil
}

// generateRaw sends a prompt to the model and returns the raw text response
// along with token usage. This is the shared core used by both Review and Summarize.
func (p *Provider) generateRaw(ctx context.Context, systemPrompt, userPrompt string) (string, *TokenUsage, error) {
	config := &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText(systemPrompt, genai.RoleUser),
		Temperature:       genai.Ptr(float32(0.2)),
	}

	// Gemini models support native JSON mode (without schema constraint for flexibility).
	if isGeminiModel(p.modelName) {
		config.ResponseMIMEType = "application/json"
	}

	var result *genai.GenerateContentResponse
	var genErr error

	retryOpts := retry.DefaultOptions()
	retryOpts.RetryIf = func(err error) bool {
		errStr := strings.ToLower(err.Error())
		return strings.Contains(errStr, "429") ||
			strings.Contains(errStr, "503") ||
			strings.Contains(errStr, "502") ||
			strings.Contains(errStr, "504") ||
			strings.Contains(errStr, "rate") ||
			strings.Contains(errStr, "unavailable") ||
			strings.Contains(errStr, "overloaded") ||
			strings.Contains(errStr, "temporarily")
	}

	if err := retry.Do(ctx, "model call", func() error {
		result, genErr = p.client.Models.GenerateContent(ctx, p.modelName, []*genai.Content{genai.NewContentFromText(userPrompt, genai.RoleUser)}, config)
		return genErr
	}, retryOpts); err != nil {
		return "", nil, fmt.Errorf("generating content: %w", err)
	}

	text := extractText(result)
	if text == "" {
		return "", nil, fmt.Errorf("empty response from model")
	}

	var usage *TokenUsage
	if result.UsageMetadata != nil {
		usage = &TokenUsage{
			InputTokens:  int64(result.UsageMetadata.PromptTokenCount),
			OutputTokens: int64(result.UsageMetadata.CandidatesTokenCount),
			TotalTokens:  int64(result.UsageMetadata.TotalTokenCount),
		}
	}

	return text, usage, nil
}

// Close releases resources held by the provider.
func (p *Provider) Close() {
	// genai client doesn't have a Close method currently,
	// but we keep this for forward compatibility.
}

func isGeminiModel(model string) bool {
	return strings.HasPrefix(model, "gemini-")
}

// reviewResultSchema returns the JSON schema for ReviewResult, used to constrain Gemini output.
func reviewResultSchema() *genai.Schema {
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"summary": {
				Type:        genai.TypeString,
				Description: "Brief summary of the overall change and review.",
			},
			"findings": {
				Type:        genai.TypeArray,
				Description: "List of review findings.",
				Items: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"file":       {Type: genai.TypeString, Description: "File path."},
						"line":       {Type: genai.TypeInteger, Description: "Line number in the new file."},
						"severity":   {Type: genai.TypeString, Description: "CRITICAL, HIGH, MEDIUM, or LOW.", Enum: []string{"CRITICAL", "HIGH", "MEDIUM", "LOW"}},
						"category":   {Type: genai.TypeString, Description: "Finding category.", Enum: []string{"bug", "security", "performance", "style", "docs"}},
						"title":      {Type: genai.TypeString, Description: "Single sentence summary."},
						"body":       {Type: genai.TypeString, Description: "Detailed explanation."},
						"suggestion": {Type: genai.TypeString, Description: "Optional corrected code."},
					},
					Required: []string{"file", "line", "severity", "category", "title", "body"},
				},
			},
		},
		Required: []string{"summary", "findings"},
	}
}

func extractText(result *genai.GenerateContentResponse) string {
	if result == nil || len(result.Candidates) == 0 {
		return ""
	}
	candidate := result.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			sb.WriteString(part.Text)
		}
	}
	return sb.String()
}

func parseReviewJSON(text string) (*ReviewResult, error) {
	cleaned := cleanJSONText(text)

	var result ReviewResult
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, fmt.Errorf("could not parse model response as JSON: %w", err)
	}
	return &result, nil
}

// cleanJSONText strips markdown code fences and extracts the JSON object
// from model output. Used by both parseReviewJSON and parseSummaryJSON.
func cleanJSONText(text string) string {
	text = strings.TrimSpace(text)

	// Try as-is first.
	if json.Valid([]byte(text)) {
		return text
	}

	// Strip markdown code fences (case-insensitive).
	lower := strings.ToLower(text)
	if strings.HasPrefix(lower, "```json") {
		text = text[7:] // len("```json") == 7
	} else if strings.HasPrefix(lower, "```") {
		text = text[3:]
	}
	if idx := strings.LastIndex(text, "```"); idx >= 0 {
		text = text[:idx]
	}
	text = strings.TrimSpace(text)

	if json.Valid([]byte(text)) {
		return text
	}

	// Fallback: extract JSON object by finding first '{' and last '}'.
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		candidate := text[start : end+1]
		if json.Valid([]byte(candidate)) {
			return candidate
		}
	}

	return text
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
