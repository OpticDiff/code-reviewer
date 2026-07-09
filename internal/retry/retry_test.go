package retry

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDo_Success(t *testing.T) {
	calls := 0
	err := Do(context.Background(), "test", func() error {
		calls++
		return nil
	}, DefaultOptions())

	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 call, got %d", calls)
	}
}

func TestDo_RetryThenSuccess(t *testing.T) {
	calls := 0
	err := Do(context.Background(), "test", func() error {
		calls++
		if calls < 3 {
			return errors.New("transient")
		}
		return nil
	}, Options{
		MaxAttempts: 3,
		BaseDelay:   1 * time.Millisecond,
		MaxDelay:    10 * time.Millisecond,
		RetryIf:     func(error) bool { return true },
	})

	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestDo_MaxAttemptsExhausted(t *testing.T) {
	calls := 0
	err := Do(context.Background(), "test", func() error {
		calls++
		return errors.New("persistent")
	}, Options{
		MaxAttempts: 3,
		BaseDelay:   1 * time.Millisecond,
		MaxDelay:    10 * time.Millisecond,
		RetryIf:     func(error) bool { return true },
	})

	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestDo_NonRetryable(t *testing.T) {
	calls := 0
	err := Do(context.Background(), "test", func() error {
		calls++
		return errors.New("non-retryable")
	}, Options{
		MaxAttempts: 3,
		BaseDelay:   1 * time.Millisecond,
		MaxDelay:    10 * time.Millisecond,
		RetryIf:     func(error) bool { return false },
	})

	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("expected 1 call (no retry), got %d", calls)
	}
}

func TestDo_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	err := Do(ctx, "test", func() error {
		calls++
		cancel() // Cancel after first attempt.
		return errors.New("fail")
	}, Options{
		MaxAttempts: 5,
		BaseDelay:   100 * time.Millisecond,
		MaxDelay:    1 * time.Second,
		RetryIf:     func(error) bool { return true },
	})

	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("expected 1 call before cancel, got %d", calls)
	}
}

func TestIsRetryableHTTPStatus(t *testing.T) {
	tests := []struct {
		code int
		want bool
	}{
		{200, false},
		{400, false},
		{401, false},
		{403, false},
		{404, false},
		{429, true},
		{500, false},
		{502, true},
		{503, true},
		{504, true},
	}
	for _, tt := range tests {
		got := IsRetryableHTTPStatus(tt.code)
		if got != tt.want {
			t.Errorf("IsRetryableHTTPStatus(%d) = %v, want %v", tt.code, got, tt.want)
		}
	}
}

func TestBackoffDelay(t *testing.T) {
	// Just verify it doesn't exceed maxDelay.
	for attempt := 1; attempt <= 10; attempt++ {
		delay := backoffDelay(attempt, 100*time.Millisecond, 5*time.Second)
		if delay > 5*time.Second {
			t.Errorf("attempt %d: delay %s exceeds max 5s", attempt, delay)
		}
		if delay < 0 {
			t.Errorf("attempt %d: delay %s is negative", attempt, delay)
		}
	}
}
