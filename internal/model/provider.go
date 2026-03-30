// Package model provides the AI model integration for code review via Vertex AI.
package model

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"google.golang.org/genai"
)

// ReviewResult is the structured output from the model.
type ReviewResult struct {
	Summary  string    `json:"summary"`
	Findings []Finding `json:"findings"`
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
func NewProvider(ctx context.Context, project, location, modelName string) (*Provider, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		Project:  project,
		Location: location,
		Backend:  genai.BackendVertexAI,
	})
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
	// Build the generation config.
	config := &genai.GenerateContentConfig{
		SystemInstruction: genai.NewContentFromText(systemPrompt, genai.RoleUser),
		Temperature:       genai.Ptr(float32(0.2)), // Low temperature for consistent, focused reviews.
	}

	// Gemini models support native JSON response schema.
	// For other models (Claude, Mistral), we rely on prompt-instructed JSON.
	if isGeminiModel(p.modelName) {
		config.ResponseMIMEType = "application/json"
	}

	result, err := p.client.Models.GenerateContent(ctx, p.modelName, []*genai.Content{genai.NewContentFromText(userPrompt, genai.RoleUser)}, config)
	if err != nil {
		return nil, fmt.Errorf("generating content: %w", err)
	}

	// Extract text from response.
	text := extractText(result)
	if text == "" {
		return nil, fmt.Errorf("empty response from model")
	}

	// Parse JSON response.
	review, err := parseReviewJSON(text)
	if err != nil {
		return nil, fmt.Errorf("parsing model response: %w (raw: %s)", err, truncate(text, 500))
	}

	return review, nil
}

// Close releases resources held by the provider.
func (p *Provider) Close() {
	// genai client doesn't have a Close method currently,
	// but we keep this for forward compatibility.
}

func isGeminiModel(model string) bool {
	return strings.HasPrefix(model, "gemini-")
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
	// Strip markdown code fences if present (models sometimes wrap JSON).
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```json") {
		text = strings.TrimPrefix(text, "```json")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	} else if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	}

	var result ReviewResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
