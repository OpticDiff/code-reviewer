package main

import (
	"errors"
	"strings"
	"testing"
)

func TestWrapProviderError_AuthErrors(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantAuth bool
	}{
		{"credentials keyword", errors.New("missing credentials for project"), true},
		{"oauth2 keyword", errors.New("oauth2: token expired"), true},
		{"authentication keyword", errors.New("authentication failed: 401"), true},
		{"mixed case", errors.New("OAuth2 token invalid"), true},
		{"generic error", errors.New("connection timeout"), false},
		{"empty error", errors.New(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped := wrapProviderError(tt.err)
			msg := wrapped.Error()

			if tt.wantAuth {
				if !strings.Contains(msg, "vertex AI authentication failed") {
					t.Errorf("expected auth error message, got: %s", msg)
				}
				if !strings.Contains(msg, "gcloud auth application-default login") {
					t.Errorf("expected ADC remediation guidance, got: %s", msg)
				}
			} else {
				if strings.Contains(msg, "authentication failed") {
					t.Errorf("should not contain auth message for non-auth error, got: %s", msg)
				}
				if !strings.Contains(msg, "initializing model provider") {
					t.Errorf("expected generic provider error, got: %s", msg)
				}
			}

			// Original error should be wrapped.
			if !errors.Is(wrapped, tt.err) {
				t.Error("original error should be wrapped")
			}
		})
	}
}
