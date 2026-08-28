package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
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

// TestRun_MissingGCPProject verifies that run() returns a config error when
// GOOGLE_CLOUD_PROJECT is not set.
func TestRun_MissingGCPProject(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"code-reviewer", "--diff"}

	t.Setenv("GOOGLE_CLOUD_PROJECT", "")

	ctx := context.Background()
	initCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := run(ctx, initCtx)
	if err == nil {
		t.Fatal("run() expected error for missing GOOGLE_CLOUD_PROJECT, got nil")
	}
	if !strings.Contains(err.Error(), "GOOGLE_CLOUD_PROJECT") {
		t.Errorf("error = %q, want substring %q", err.Error(), "GOOGLE_CLOUD_PROJECT")
	}
}

// TestRun_NoInputMode verifies that run() returns a config error when no
// input mode (--ci, --diff, --files) is specified.
func TestRun_NoInputMode(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"code-reviewer"}

	t.Setenv("GOOGLE_CLOUD_PROJECT", "test-project")

	ctx := context.Background()
	initCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := run(ctx, initCtx)
	if err == nil {
		t.Fatal("run() expected error for no input mode, got nil")
	}
	if !strings.Contains(err.Error(), "must specify one of") {
		t.Errorf("error = %q, want substring %q", err.Error(), "must specify one of")
	}
}

func TestRunCache_NoArgs(t *testing.T) {
	err := runCache(nil)
	if err == nil {
		t.Fatal("expected error for no args")
	}
	if !strings.Contains(err.Error(), "usage:") {
		t.Errorf("expected usage message, got: %v", err)
	}
}

func TestRunCache_UnknownSubcommand(t *testing.T) {
	dir := t.TempDir()
	err := runCache([]string{"--cache-dir", dir, "nope"})
	if err == nil {
		t.Fatal("expected error for unknown subcommand")
	}
	if !strings.Contains(err.Error(), "unknown cache command") {
		t.Errorf("expected 'unknown cache command', got: %v", err)
	}
}

func TestRunCache_Stats_Empty(t *testing.T) {
	dir := t.TempDir()
	err := runCache([]string{"stats", "--cache-dir", dir})
	if err != nil {
		t.Fatalf("stats on empty cache should succeed, got: %v", err)
	}
}

func TestRunCache_Clear_Empty(t *testing.T) {
	dir := t.TempDir()
	err := runCache([]string{"clear", "--cache-dir", dir})
	if err != nil {
		t.Fatalf("clear on empty cache should succeed, got: %v", err)
	}
}

func TestRunCache_CacheDirEquals(t *testing.T) {
	dir := t.TempDir()
	err := runCache([]string{"stats", "--cache-dir=" + dir})
	if err != nil {
		t.Fatalf("--cache-dir=path should work, got: %v", err)
	}
}

func TestRunCache_CacheDirMissingValue(t *testing.T) {
	err := runCache([]string{"--cache-dir"})
	if err == nil {
		t.Fatal("expected error for --cache-dir without value")
	}
	if !strings.Contains(err.Error(), "requires a value") {
		t.Errorf("expected 'requires a value', got: %v", err)
	}
}

func TestRunCache_UnknownFlag(t *testing.T) {
	err := runCache([]string{"clear", "--verbose"})
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
	if !strings.Contains(err.Error(), "unknown cache flag") {
		t.Errorf("expected 'unknown cache flag', got: %v", err)
	}
}

func TestRunCache_ExtraArgs(t *testing.T) {
	dir := t.TempDir()
	err := runCache([]string{"clear", "--cache-dir", dir, "extra"})
	if err == nil {
		t.Fatal("expected error for extra argument")
	}
	if !strings.Contains(err.Error(), "unexpected cache argument") {
		t.Errorf("expected 'unexpected cache argument', got: %v", err)
	}
}
