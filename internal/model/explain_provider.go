package model

import "context"

// ExplainProvider is implemented by providers that support explain mode.
type ExplainProvider interface {
	Explain(ctx context.Context, systemPrompt, userPrompt string) (string, *TokenUsage, error)
}

// Explain sends a diff to the model and returns a free-form explanation.
func (p *Provider) Explain(ctx context.Context, systemPrompt, userPrompt string) (string, *TokenUsage, error) {
	return p.generateRaw(ctx, systemPrompt, userPrompt)
}

// Explain sends a diff to the model via the OpenAI-compatible API and returns a free-form explanation.
func (p *HTTPProvider) Explain(ctx context.Context, systemPrompt, userPrompt string) (string, *TokenUsage, error) {
	return p.generateRaw(ctx, systemPrompt, userPrompt)
}
