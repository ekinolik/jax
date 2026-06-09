package polygon

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/massive-com/client-go/v2/rest/models"
)

// ErrRateLimited indicates Massive returned HTTP 429.
var ErrRateLimited = errors.New("massive API rate limited")

// RetryConfig controls exponential backoff for Massive REST calls used by confluence.
type RetryConfig struct {
	MaxRetries  int
	BaseDelayMs int
}

// DefaultRetryConfig returns defaults tuned for t3.nano shared with the node frontend.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:  5,
		BaseDelayMs: 500,
	}
}

func (c RetryConfig) normalized() RetryConfig {
	if c.MaxRetries <= 0 {
		c.MaxRetries = DefaultRetryConfig().MaxRetries
	}
	if c.BaseDelayMs <= 0 {
		c.BaseDelayMs = DefaultRetryConfig().BaseDelayMs
	}
	return c
}

// IsRateLimitError reports whether err is a Massive HTTP 429.
func IsRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrRateLimited) {
		return true
	}
	var apiErr *models.ErrorResponse
	if errors.As(err, &apiErr) && apiErr.StatusCode == 429 {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "429") || strings.Contains(msg, "too many requests")
}

// IsRetryableAPIError reports whether err is a transient Massive REST failure (429 or 5xx).
// Context cancellation, deadlines, and client timeouts are not retried — fail fast so handlers
// and shutdown can unwind instead of multiplying wait time via backoff.
func IsRetryableAPIError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var apiErr *models.ErrorResponse
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 429 || apiErr.StatusCode >= 500
	}

	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "context canceled") || strings.Contains(msg, "context deadline exceeded") {
		return false
	}
	if strings.Contains(msg, "429") || strings.Contains(msg, "too many requests") {
		return true
	}
	if strings.Contains(msg, "502") || strings.Contains(msg, "503") || strings.Contains(msg, "504") {
		return true
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return false
	}
	return errors.As(err, &netErr) && netErr.Temporary()
}

// WithRetry executes fn with exponential backoff on retryable API errors.
func WithRetry(ctx context.Context, cfg RetryConfig, fn func(context.Context) error) error {
	cfg = cfg.normalized()
	var lastErr error
	delay := time.Duration(cfg.BaseDelayMs) * time.Millisecond

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		lastErr = fn(ctx)
		if lastErr == nil {
			return nil
		}
		if !IsRetryableAPIError(lastErr) {
			return lastErr
		}
		if attempt == cfg.MaxRetries {
			break
		}
		if IsRateLimitError(lastErr) {
			delay = maxDuration(delay, 2*time.Second)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
	}
	if IsRateLimitError(lastErr) {
		return fmt.Errorf("%w: retry exhausted after %d attempts", ErrRateLimited, cfg.MaxRetries+1)
	}
	return fmt.Errorf("retry exhausted after %d attempts: %w", cfg.MaxRetries+1, lastErr)
}

func maxDuration(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}
