package model

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/genai"
)

const reviewJSON = `{"summary":"ok","findings":[]}`

// genaiProviderReturning serves a canned generateContent response, so the
// usage metadata is read through the real genai client.
func genaiProviderReturning(t *testing.T, usageJSON string) *Provider {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body := map[string]any{
			"candidates": []any{map[string]any{"content": map[string]any{"role": "model", "parts": []any{map[string]any{"text": reviewJSON}}}}},
		}
		if usageJSON != "" {
			body["usageMetadata"] = json.RawMessage(usageJSON)
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(server.Close)
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		Backend:     genai.BackendGeminiAPI,
		APIKey:      "k",
		HTTPOptions: genai.HTTPOptions{BaseURL: server.URL},
	})
	if err != nil {
		t.Fatal(err)
	}
	return &Provider{client: client, modelName: "some-model"}
}

func TestProvider_ReviewReadsCachedAndThinkingTokens(t *testing.T) {
	tests := []struct {
		name  string
		usage string
		want  *TokenUsage
	}{
		{
			name:  "present",
			usage: `{"promptTokenCount":1000,"cachedContentTokenCount":800,"candidatesTokenCount":50,"thoughtsTokenCount":300,"totalTokenCount":1350}`,
			want:  &TokenUsage{InputTokens: 1000, CachedInputTokens: 800, OutputTokens: 50, ThinkingTokens: 300, TotalTokens: 1350},
		},
		{
			name:  "absent fields are zero",
			usage: `{"promptTokenCount":1000,"candidatesTokenCount":50,"totalTokenCount":1050}`,
			want:  &TokenUsage{InputTokens: 1000, OutputTokens: 50, TotalTokens: 1050},
		},
		{name: "no usage metadata", usage: "", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := genaiProviderReturning(t, tt.usage).Review(context.Background(), "sys", "user")
			if err != nil {
				t.Fatalf("Review: %v", err)
			}
			switch {
			case tt.want == nil && res.Usage != nil:
				t.Errorf("usage = %+v, want nil", res.Usage)
			case tt.want != nil && (res.Usage == nil || *res.Usage != *tt.want):
				t.Errorf("usage = %+v, want %+v", res.Usage, tt.want)
			}
		})
	}
}

func TestUsageFromMetadata_SummedAcrossCalls(t *testing.T) {
	var total TokenUsage
	for _, m := range []*genai.GenerateContentResponseUsageMetadata{
		{PromptTokenCount: 100, CachedContentTokenCount: 80, CandidatesTokenCount: 10, ThoughtsTokenCount: 5, TotalTokenCount: 115},
		nil,
		{PromptTokenCount: 120, CachedContentTokenCount: 100, CandidatesTokenCount: 12, ThoughtsTokenCount: 7, TotalTokenCount: 139},
	} {
		total.Add(UsageFromMetadata(m))
	}
	want := TokenUsage{InputTokens: 220, CachedInputTokens: 180, OutputTokens: 22, ThinkingTokens: 12, TotalTokens: 254}
	if total != want {
		t.Errorf("sum = %+v, want %+v", total, want)
	}
}

// chatServer answers every request with the given usage object (empty = none).
func chatServer(t *testing.T, usageJSON string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body := map[string]any{
			"choices": []any{map[string]any{"message": map[string]string{"content": reviewJSON}}},
		}
		if usageJSON != "" {
			body["usage"] = json.RawMessage(usageJSON)
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(server.Close)
	return server
}

var chatUsageCases = []struct {
	name  string
	usage string
	want  *TokenUsage
}{
	{
		name:  "present",
		usage: `{"prompt_tokens":1000,"completion_tokens":350,"total_tokens":1350,"prompt_tokens_details":{"cached_tokens":800},"completion_tokens_details":{"reasoning_tokens":300}}`,
		want:  &TokenUsage{InputTokens: 1000, CachedInputTokens: 800, OutputTokens: 350, ThinkingTokens: 300, TotalTokens: 1350},
	},
	{
		name:  "details absent",
		usage: `{"prompt_tokens":1000,"completion_tokens":50,"total_tokens":1050}`,
		want:  &TokenUsage{InputTokens: 1000, OutputTokens: 50, TotalTokens: 1050},
	},
	{
		name:  "details present without the fields",
		usage: `{"prompt_tokens":1000,"completion_tokens":50,"total_tokens":1050,"prompt_tokens_details":{"audio_tokens":0},"completion_tokens_details":{}}`,
		want:  &TokenUsage{InputTokens: 1000, OutputTokens: 50, TotalTokens: 1050},
	},
	{name: "no usage object", usage: "", want: nil},
}

func TestHTTPProvider_ReviewReadsCachedAndReasoningTokens(t *testing.T) {
	for _, tt := range chatUsageCases {
		t.Run(tt.name, func(t *testing.T) {
			server := chatServer(t, tt.usage)
			p, err := NewHTTPProvider(server.URL+"/v1", "k", "some-model")
			if err != nil {
				t.Fatal(err)
			}
			res, err := p.Review(context.Background(), "sys", "user")
			if err != nil {
				t.Fatalf("Review: %v", err)
			}
			switch {
			case tt.want == nil && res.Usage != nil:
				t.Errorf("usage = %+v, want nil", res.Usage)
			case tt.want != nil && (res.Usage == nil || *res.Usage != *tt.want):
				t.Errorf("usage = %+v, want %+v", res.Usage, tt.want)
			}
		})
	}
}

// TestHTTPChatter_CarriesCachedAndReasoningTokens checks the agent loop's
// transport: the counts travel in the genai usage metadata the loop reads.
func TestHTTPChatter_CarriesCachedAndReasoningTokens(t *testing.T) {
	for _, tt := range chatUsageCases {
		t.Run(tt.name, func(t *testing.T) {
			server := chatServer(t, tt.usage)
			p, err := NewHTTPProvider(server.URL+"/v1", "k", "some-model")
			if err != nil {
				t.Fatal(err)
			}
			resp, err := NewHTTPChatter(p).Chat(context.Background(),
				[]*genai.Content{genai.NewContentFromText("hi", genai.RoleUser)}, nil)
			if err != nil {
				t.Fatalf("Chat: %v", err)
			}
			got := UsageFromMetadata(resp.UsageMetadata)
			switch {
			case tt.want == nil && got != nil:
				t.Errorf("usage = %+v, want nil", got)
			case tt.want != nil && (got == nil || *got != *tt.want):
				t.Errorf("usage = %+v, want %+v", got, tt.want)
			}
		})
	}
}
