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
