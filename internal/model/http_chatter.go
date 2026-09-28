package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/OpticDiff/code-reviewer/internal/retry"
	"google.golang.org/genai"
)

// HTTPChatter implements the reviewer.Chatter interface using the same
// OpenAI-compatible endpoint as HTTPProvider. This enables the agent loop
// for --api-url users (OpenRouter, Ollama, vLLM, etc.).
type HTTPChatter struct {
	provider *HTTPProvider
}

// NewHTTPChatter creates a Chatter backed by the given HTTPProvider.
func NewHTTPChatter(p *HTTPProvider) *HTTPChatter {
	return &HTTPChatter{provider: p}
}

// Chat translates genai.Content history to OpenAI messages and calls the endpoint.
func (c *HTTPChatter) Chat(ctx context.Context, history []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	// Build OpenAI messages from genai history.
	messages := make([]chatMessage, 0, len(history)+1)

	if config != nil && config.SystemInstruction != nil {
		sysText := contentText(config.SystemInstruction)
		if sysText != "" {
			messages = append(messages, chatMessage{Role: "system", Content: sysText})
		}
	}

	for _, content := range history {
		role := "user"
		if content.Role == "model" {
			role = "assistant"
		}
		text := contentText(content)
		if text != "" {
			messages = append(messages, chatMessage{Role: role, Content: text})
		}
	}

	temp := float32(0.2)
	if config != nil && config.Temperature != nil {
		temp = *config.Temperature
	}

	reqBody := chatRequest{
		Model:       c.provider.modelName,
		Messages:    messages,
		Temperature: temp,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling chat request: %w", err)
	}

	endpoint := c.provider.baseURL + "/chat/completions"
	var respBody []byte

	retryOpts := retry.DefaultOptions()
	retryOpts.RetryIf = func(err error) bool {
		var statusErr *httpStatusError
		if errors.As(err, &statusErr) {
			switch statusErr.StatusCode {
			case 429, 502, 503, 504:
				return true
			}
		}
		errStr := strings.ToLower(err.Error())
		return strings.Contains(errStr, "unavailable") ||
			strings.Contains(errStr, "overloaded")
	}

	if err := retry.Do(ctx, "http agent chat", func() error {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
		if reqErr != nil {
			return fmt.Errorf("creating request: %w", reqErr)
		}

		req.Header.Set("Content-Type", "application/json")
		if c.provider.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+c.provider.apiKey)
		} else if c.provider.tokenSource != nil {
			tok, tokErr := c.provider.tokenSource.Token()
			if tokErr != nil {
				return fmt.Errorf("obtaining GCP access token: %w", tokErr)
			}
			req.Header.Set("Authorization", "Bearer "+tok.AccessToken)
		}

		resp, doErr := c.provider.httpClient.Do(req)
		if doErr != nil {
			return fmt.Errorf("HTTP request failed: %w", doErr)
		}
		defer func() { _ = resp.Body.Close() }()

		respBody, err = io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		if err != nil {
			return fmt.Errorf("reading response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return &httpStatusError{
				StatusCode: resp.StatusCode,
				Body:       truncateBytes(respBody, 500),
				Endpoint:   endpoint,
			}
		}

		return nil
	}, retryOpts); err != nil {
		return nil, fmt.Errorf("agent chat: %w", err)
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("parsing response: %w (raw: %s)", err, truncateBytes(respBody, 500))
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("empty response from model (no choices)")
	}

	text := chatResp.Choices[0].Message.Content

	genaiResp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: genai.NewContentFromText(text, genai.RoleModel),
			},
		},
	}
	if chatResp.Usage != nil {
		genaiResp.UsageMetadata = &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:     int32(chatResp.Usage.PromptTokens),
			CandidatesTokenCount: int32(chatResp.Usage.CompletionTokens),
			TotalTokenCount:      int32(chatResp.Usage.TotalTokens),
		}
	}

	return genaiResp, nil
}

// contentText pulls all text parts from a genai.Content.
func contentText(content *genai.Content) string {
	if content == nil || len(content.Parts) == 0 {
		return ""
	}
	var sb strings.Builder
	for _, part := range content.Parts {
		if part.Text != "" {
			sb.WriteString(part.Text)
		}
	}
	return sb.String()
}
