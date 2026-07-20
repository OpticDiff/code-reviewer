package model

import (
	"context"
	"encoding/json"
	"fmt"
)

// SummarizeProvider is implemented by providers that support the summarize mode.
// Both Provider (Vertex AI) and HTTPProvider implement this interface.
type SummarizeProvider interface {
	Summarize(ctx context.Context, systemPrompt, userPrompt string) (*SummaryResult, error)
}

// Summarize sends a diff to the model for summarization and returns a structured summary.
// It reuses the same model call infrastructure as Review but parses the response
// as SummaryResult instead of ReviewResult.
func (p *Provider) Summarize(ctx context.Context, systemPrompt, userPrompt string) (*SummaryResult, error) {
	result, usage, err := p.generateRaw(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, err
	}

	summary, err := parseSummaryJSON(result)
	if err != nil {
		return nil, fmt.Errorf("parsing summary response: %w (raw: %s)", err, truncate(result, 500))
	}

	summary.Usage = usage
	return summary, nil
}

// Summarize sends a diff to the model for summarization via the OpenAI-compatible API.
func (p *HTTPProvider) Summarize(ctx context.Context, systemPrompt, userPrompt string) (*SummaryResult, error) {
	text, usage, err := p.generateRaw(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, err
	}

	summary, err := parseSummaryJSON(text)
	if err != nil {
		return nil, fmt.Errorf("parsing summary response: %w (raw: %s)", err, truncate(text, 500))
	}

	summary.Usage = usage
	return summary, nil
}

// parseSummaryJSON parses model output as a SummaryResult using the same
// three-tier strategy as parseReviewJSON: direct parse, strip fences, extract braces.
func parseSummaryJSON(text string) (*SummaryResult, error) {
	cleaned := cleanJSONText(text)

	var result SummaryResult
	if err := json.Unmarshal([]byte(cleaned), &result); err != nil {
		return nil, fmt.Errorf("could not parse summary response as JSON: %w", err)
	}
	if err := validateSummary(&result); err != nil {
		return nil, err
	}
	return &result, nil
}

// validateSummary checks that the model returned required fields.
func validateSummary(s *SummaryResult) error {
	if s.Title == "" {
		return fmt.Errorf("summary response missing required field: title")
	}
	if s.Classification == "" {
		return fmt.Errorf("summary response missing required field: classification")
	}
	if s.RiskLevel == "" {
		return fmt.Errorf("summary response missing required field: risk_level")
	}
	return nil
}
