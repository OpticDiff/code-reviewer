// Package retry provides exponential backoff with jitter for transient failures.
package retry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/rand/v2"
	"time"
)

// Options configures retry behavior.
type Options struct {
	MaxAttempts int           // Maximum number of attempts (default: 3).
	BaseDelay   time.Duration // Initial delay between retries (default: 500ms).
	MaxDelay    time.Duration // Maximum delay between retries (default: 30s).
	RetryIf     func(error) bool // Predicate to determine if an error is retryable.
}

// DefaultOptions returns sensible defaults for API calls.
func DefaultOptions() Options {
	return Options{
		MaxAttempts: 3,
		BaseDelay:   500 * time.Millisecond,
		MaxDelay:    30 * time.Second,
		RetryIf:     func(error) bool { return true },
	}
}

// Do executes fn with exponential backoff and jitter.
// It retries on errors where opts.RetryIf returns true, up to opts.MaxAttempts times.
func Do(ctx context.Context, operation string, fn func() error, opts Options) error {
	if opts.MaxAttempts <= 0 {
		opts.MaxAttempts = 3
	}
	if opts.BaseDelay <= 0 {
		opts.BaseDelay = 500 * time.Millisecond
	}
	if opts.MaxDelay <= 0 {
		opts.MaxDelay = 30 * time.Second
	}
	if opts.RetryIf == nil {
		opts.RetryIf = func(error) bool { return true }
	}

	var lastErr error
	for attempt := 1; attempt <= opts.MaxAttempts; attempt++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		if !opts.RetryIf(lastErr) {
			return lastErr // Non-retryable error.
		}

		if attempt == opts.MaxAttempts {
			break // No more retries.
		}

		// Calculate delay with exponential backoff + jitter.
		delay := backoffDelay(attempt, opts.BaseDelay, opts.MaxDelay)

		slog.Warn(fmt.Sprintf("%s failed (attempt %d/%d), retrying in %s",
			operation, attempt, opts.MaxAttempts, delay),
			"error", lastErr,
		)

		select {
		case <-ctx.Done():
			return fmt.Errorf("%s cancelled: %w (last error: %w)", operation, ctx.Err(), lastErr)
		case <-time.After(delay):
		}
	}

	return fmt.Errorf("%s failed after %d attempts: %w", operation, opts.MaxAttempts, lastErr)
}

// backoffDelay computes the delay for the given attempt using exponential backoff with full jitter.
func backoffDelay(attempt int, baseDelay, maxDelay time.Duration) time.Duration {
	// Exponential: baseDelay * 2^(attempt-1)
	exp := math.Pow(2, float64(attempt-1))
	delay := time.Duration(float64(baseDelay) * exp)
	if delay > maxDelay {
		delay = maxDelay
	}
	// Full jitter: random value in [0, delay]
	jittered := time.Duration(rand.Int64N(int64(delay) + 1))
	return jittered
}

// IsRetryableHTTPStatus returns true for HTTP status codes that indicate transient failures.
func IsRetryableHTTPStatus(statusCode int) bool {
	switch statusCode {
	case 429: // Too Many Requests
		return true
	case 502: // Bad Gateway
		return true
	case 503: // Service Unavailable
		return true
	case 504: // Gateway Timeout
		return true
	default:
		return false
	}
}

// RetryableError wraps an error with a retryable flag.
type RetryableError struct {
	Err       error
	Retryable bool
}

func (e *RetryableError) Error() string { return e.Err.Error() }
func (e *RetryableError) Unwrap() error { return e.Err }

// IsRetryable checks if an error (or any error in its chain) is marked as retryable.
func IsRetryable(err error) bool {
	var re *RetryableError
	if errors.As(err, &re) {
		return re.Retryable
	}
	return false
}
