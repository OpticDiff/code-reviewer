package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/OpticDiff/code-reviewer/internal/retry"
)

// maxResponseBytes caps the response body read to prevent OOM from malicious servers.
const maxResponseBytes = 10 * 1024 * 1024 // 10 MB

// httpStatusError captures the HTTP status code for reliable retry classification.
type httpStatusError struct {
	StatusCode int
	Body       string
	Endpoint   string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("HTTP %d from %s: %s", e.StatusCode, e.Endpoint, e.Body)
}

// HTTPProvider implements ModelReviewer using the OpenAI-compatible
// Chat Completions API (/v1/chat/completions). Works with any server
// that speaks this format: vLLM, Ollama, Candela, TGI, llama.cpp,
// Cloud Run + Gemma, Vertex AI OpenAI-compat, etc.
type HTTPProvider struct {
	baseURL    string
	apiKey     string
	modelName  string
	httpClient *http.Client
}

// NewHTTPProvider creates a provider that talks to any OpenAI-compatible endpoint.
// baseURL should be the root URL (e.g., "http://localhost:11434/v1" or
// "https://gemma-xyz.run.app/v1"). The /chat/completions path is appended automatically.
func NewHTTPProvider(baseURL, apiKey, modelName string) (*HTTPProvider, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("--api-url is required for HTTP provider")
	}
	if modelName == "" {
		return nil, fmt.Errorf("--model is required for HTTP provider")
	}

	// Normalize: strip trailing slash, ensure no /chat/completions suffix.
	baseURL = strings.TrimRight(baseURL, "/")
	baseURL = strings.TrimSuffix(baseURL, "/chat/completions")

	return &HTTPProvider{
		baseURL:   baseURL,
		apiKey:    apiKey,
		modelName: modelName,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute, // LLM calls can be slow on large diffs.
		},
	}, nil
}

// chatRequest is the OpenAI Chat Completions request format.
type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float32       `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatResponse is the OpenAI Chat Completions response format.
type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
		TotalTokens      int64 `json:"total_tokens"`
	} `json:"usage"`
}

// Review sends a diff to the model for review and returns structured findings.
func (p *HTTPProvider) Review(ctx context.Context, systemPrompt, userPrompt string) (*ReviewResult, error) {
	reqBody := chatRequest{
		Model: p.modelName,
		Messages: []chatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		Temperature: 0.2,
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	endpoint := p.baseURL + "/chat/completions"
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
		// Fallback: network-level errors.
		errStr := strings.ToLower(err.Error())
		return strings.Contains(errStr, "unavailable") ||
			strings.Contains(errStr, "overloaded") ||
			strings.Contains(errStr, "temporarily")
	}

	if err := retry.Do(ctx, "http model review", func() error {
		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
		if reqErr != nil {
			return fmt.Errorf("creating request: %w", reqErr)
		}

		req.Header.Set("Content-Type", "application/json")
		if p.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+p.apiKey)
		}

		resp, doErr := p.httpClient.Do(req)
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
		return nil, fmt.Errorf("generating content: %w", err)
	}

	// Parse the OpenAI-format response.
	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, fmt.Errorf("parsing response JSON: %w (raw: %s)", err, truncateBytes(respBody, 500))
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("empty response from model (no choices)")
	}

	text := chatResp.Choices[0].Message.Content
	if text == "" {
		return nil, fmt.Errorf("empty content in model response")
	}

	slog.Debug("raw model response", "length", len(text))

	// Parse the review JSON from the content.
	review, err := parseReviewJSON(text)
	if err != nil {
		return nil, fmt.Errorf("parsing model response: %w (raw: %s)", err, truncate(text, 500))
	}

	// Capture token usage.
	if chatResp.Usage != nil {
		review.Usage = &TokenUsage{
			InputTokens:  chatResp.Usage.PromptTokens,
			OutputTokens: chatResp.Usage.CompletionTokens,
			TotalTokens:  chatResp.Usage.TotalTokens,
		}
	}

	return review, nil
}

// Close is a no-op for the HTTP provider.
func (p *HTTPProvider) Close() {}

func truncateBytes(b []byte, max int) string {
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "..."
}
