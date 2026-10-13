package model

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.org/x/oauth2"
	"google.golang.org/genai"
)

func TestHTTPProvider_Review(t *testing.T) {
	// Mock server that returns a valid review response.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request format.
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("expected /v1/chat/completions, got %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json content-type")
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %s", r.Header.Get("Authorization"))
		}

		// Verify request body.
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decoding request: %v", err)
		}
		if req.Model != "gemma-3-27b" {
			t.Errorf("expected model gemma-3-27b, got %s", req.Model)
		}
		if len(req.Messages) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(req.Messages))
		}
		if req.Messages[0].Role != "system" {
			t.Errorf("expected system role, got %s", req.Messages[0].Role)
		}

		// Return a valid review response.
		review := ReviewResult{
			Summary: "LGTM",
			Findings: []Finding{
				{File: "main.go", Line: 10, Severity: "HIGH", Category: "bug", Title: "nil check", Body: "Missing nil check"},
			},
		}
		reviewJSON, _ := json.Marshal(review)

		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": string(reviewJSON)}},
			},
			"usage": map[string]int64{
				"prompt_tokens":     100,
				"completion_tokens": 50,
				"total_tokens":      150,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, err := NewHTTPProvider(server.URL+"/v1", "test-key", "gemma-3-27b")
	if err != nil {
		t.Fatalf("creating provider: %v", err)
	}

	result, err := provider.Review(context.Background(), "Review this code", "diff content here")
	if err != nil {
		t.Fatalf("Review() error: %v", err)
	}

	if result.Summary != "LGTM" {
		t.Errorf("summary = %q, want LGTM", result.Summary)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(result.Findings))
	}
	if result.Findings[0].File != "main.go" {
		t.Errorf("finding file = %q, want main.go", result.Findings[0].File)
	}
	if result.Usage == nil {
		t.Fatal("expected token usage")
	}
	if result.Usage.TotalTokens != 150 {
		t.Errorf("total tokens = %d, want 150", result.Usage.TotalTokens)
	}
}

func TestHTTPProvider_Review_NoAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify no Authorization header when key is empty.
		if r.Header.Get("Authorization") != "" {
			t.Errorf("expected no Authorization header, got %q", r.Header.Get("Authorization"))
		}

		review := ReviewResult{Summary: "OK", Findings: []Finding{}}
		reviewJSON, _ := json.Marshal(review)
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": string(reviewJSON)}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, err := NewHTTPProvider(server.URL+"/v1", "", "gemma-3-27b")
	if err != nil {
		t.Fatalf("creating provider: %v", err)
	}

	result, err := provider.Review(context.Background(), "system", "user")
	if err != nil {
		t.Fatalf("Review() error: %v", err)
	}
	if result.Summary != "OK" {
		t.Errorf("summary = %q, want OK", result.Summary)
	}
}

func TestHTTPProvider_Review_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "internal server error"}`))
	}))
	defer server.Close()

	provider, err := NewHTTPProvider(server.URL+"/v1", "", "test-model")
	if err != nil {
		t.Fatalf("creating provider: %v", err)
	}

	_, err = provider.Review(context.Background(), "system", "user")
	if err == nil {
		t.Fatal("expected error on 500 response")
	}
}

func TestHTTPProvider_Review_EmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]interface{}{"choices": []interface{}{}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	provider, err := NewHTTPProvider(server.URL+"/v1", "", "test-model")
	if err != nil {
		t.Fatalf("creating provider: %v", err)
	}

	_, err = provider.Review(context.Background(), "system", "user")
	if err == nil {
		t.Fatal("expected error on empty choices")
	}
}

func TestNewHTTPProvider_Validation(t *testing.T) {
	_, err := NewHTTPProvider("", "key", "model")
	if err == nil {
		t.Fatal("expected error with empty baseURL")
	}

	_, err = NewHTTPProvider("http://localhost", "key", "")
	if err == nil {
		t.Fatal("expected error with empty model")
	}
}

func TestNewHTTPProvider_URLNormalization(t *testing.T) {
	p, err := NewHTTPProvider("http://localhost:8080/v1/", "key", "model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.baseURL != "http://localhost:8080/v1" {
		t.Errorf("baseURL = %q, expected trailing slash stripped", p.baseURL)
	}

	p, err = NewHTTPProvider("http://localhost:8080/v1/chat/completions", "key", "model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.baseURL != "http://localhost:8080/v1" {
		t.Errorf("baseURL = %q, expected /chat/completions stripped", p.baseURL)
	}
}

func TestHTTPProvider_Review_ADCTokenSource(t *testing.T) {
	// Mock server that captures the Authorization header.
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")

		review := ReviewResult{Summary: "OK", Findings: []Finding{}}
		reviewJSON, _ := json.Marshal(review)
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": string(reviewJSON)}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create provider with no API key, then inject a mock token source.
	provider, err := NewHTTPProvider(server.URL+"/v1", "", "test-model")
	if err != nil {
		t.Fatalf("creating provider: %v", err)
	}
	provider.tokenSource = &staticTokenSource{token: "adc-access-token-123"}

	_, err = provider.Review(context.Background(), "system", "user")
	if err != nil {
		t.Fatalf("Review() error: %v", err)
	}

	if gotAuth != "Bearer adc-access-token-123" {
		t.Errorf("Authorization = %q, want Bearer adc-access-token-123", gotAuth)
	}
}

func TestHTTPProvider_Review_APIKeyTakesPriorityOverADC(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")

		review := ReviewResult{Summary: "OK", Findings: []Finding{}}
		reviewJSON, _ := json.Marshal(review)
		resp := map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": string(reviewJSON)}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create provider WITH API key, AND inject a token source.
	// API key should win.
	provider, err := NewHTTPProvider(server.URL+"/v1", "my-api-key", "test-model")
	if err != nil {
		t.Fatalf("creating provider: %v", err)
	}
	provider.tokenSource = &staticTokenSource{token: "should-not-be-used"}

	_, err = provider.Review(context.Background(), "system", "user")
	if err != nil {
		t.Fatalf("Review() error: %v", err)
	}

	if gotAuth != "Bearer my-api-key" {
		t.Errorf("Authorization = %q, want Bearer my-api-key (API key should take priority)", gotAuth)
	}
}

func TestHTTPProvider_Review_ADCTokenRefreshError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should not be called when token refresh fails")
	}))
	defer server.Close()

	provider, err := NewHTTPProvider(server.URL+"/v1", "", "test-model")
	if err != nil {
		t.Fatalf("creating provider: %v", err)
	}
	provider.tokenSource = &errorTokenSource{err: fmt.Errorf("token expired and refresh failed")}

	_, err = provider.Review(context.Background(), "system", "user")
	if err == nil {
		t.Fatal("expected error when token source fails")
	}
	if !strings.Contains(err.Error(), "GCP access token") {
		t.Errorf("error = %q, want mention of GCP access token", err.Error())
	}
}

// staticTokenSource returns the same token every time (test helper).
type staticTokenSource struct {
	token string
}

func (s *staticTokenSource) Token() (*oauth2.Token, error) {
	return &oauth2.Token{AccessToken: s.token}, nil
}

// errorTokenSource always returns an error (test helper).
type errorTokenSource struct {
	err error
}

func (e *errorTokenSource) Token() (*oauth2.Token, error) {
	return nil, e.err
}

// chatRequestBody captures the raw JSON body of the first request a handler
// receives, so tests can assert which fields are present on the wire.
func chatRequestBody(t *testing.T, body *map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(body); err != nil {
			t.Errorf("decoding request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"summary\":\"ok\",\"findings\":[]}"}}]}`))
	}))
}

func TestHTTPProvider_Temperature(t *testing.T) {
	tests := []struct {
		name string
		set  *float32
		omit bool
		want any // nil = field absent from the JSON body.
	}{
		{name: "default keeps 0.2", want: 0.2},
		{name: "override to zero", set: float32Ptr(0), want: float64(0)},
		{name: "omitted", omit: true},
		{name: "omit wins over override", set: float32Ptr(0.7), omit: true},
	}
	calls := map[string]func(*HTTPProvider) error{
		"review": func(p *HTTPProvider) error {
			_, err := p.Review(context.Background(), "sys", "user")
			return err
		},
		"summarize": func(p *HTTPProvider) error {
			_, _ = p.Summarize(context.Background(), "sys", "user") // Only the request is under test, not the parsed summary.
			return nil
		},
		"explain": func(p *HTTPProvider) error {
			_, _, err := p.Explain(context.Background(), "sys", "user")
			return err
		},
	}
	for _, tc := range tests {
		for callName, call := range calls {
			t.Run(tc.name+"/"+callName, func(t *testing.T) {
				var body map[string]any
				server := chatRequestBody(t, &body)
				defer server.Close()

				p, err := NewHTTPProvider(server.URL+"/v1", "k", "m")
				if err != nil {
					t.Fatal(err)
				}
				if tc.set != nil {
					p.SetTemperature(*tc.set)
				}
				if tc.omit {
					p.OmitTemperature()
				}
				if err := call(p); err != nil {
					t.Fatalf("call: %v", err)
				}
				got, present := body["temperature"]
				if tc.want == nil {
					if present {
						t.Errorf("temperature = %v, want the field absent", got)
					}
					return
				}
				if got != tc.want {
					t.Errorf("temperature = %v, want %v", got, tc.want)
				}
			})
		}
	}
}

func TestHTTPChatter_Temperature(t *testing.T) {
	tests := []struct {
		name   string
		config *genai.GenerateContentConfig
		omit   bool
		want   any // nil = field absent from the JSON body.
	}{
		{name: "nil config falls back to 0.2", config: nil, want: 0.2},
		{name: "config without temperature falls back to 0.2", config: &genai.GenerateContentConfig{}, want: 0.2},
		{name: "config temperature is sent", config: &genai.GenerateContentConfig{Temperature: float32Ptr(0)}, want: float64(0)},
		{name: "omitted with nil config", config: nil, omit: true},
		{name: "omitted with config temperature", config: &genai.GenerateContentConfig{Temperature: float32Ptr(0.5)}, omit: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]any
			server := chatRequestBody(t, &body)
			defer server.Close()

			p, err := NewHTTPProvider(server.URL+"/v1", "k", "m")
			if err != nil {
				t.Fatal(err)
			}
			if tc.omit {
				p.OmitTemperature()
			}
			history := []*genai.Content{genai.NewContentFromText("hi", genai.RoleUser)}
			if _, err := NewHTTPChatter(p).Chat(context.Background(), history, tc.config); err != nil {
				t.Fatalf("Chat: %v", err)
			}
			got, present := body["temperature"]
			if tc.want == nil {
				if present {
					t.Errorf("temperature = %v, want the field absent", got)
				}
				return
			}
			if got != tc.want {
				t.Errorf("temperature = %v, want %v", got, tc.want)
			}
		})
	}
}

func float32Ptr(v float32) *float32 { return &v }
