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

// IsRetryableAPIError reports whether err is a transient Massive REST failure (429 or 5xx).
func IsRetryableAPIError(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *models.ErrorResponse
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 429 || apiErr.StatusCode >= 500
	}

	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "429") || strings.Contains(msg, "too many requests") {
		return true
	}
	if strings.Contains(msg, "502") || strings.Contains(msg, "503") || strings.Contains(msg, "504") {
		return true
	}

	var netErr net.Error
	return errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary())
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

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
	}
	return fmt.Errorf("retry exhausted after %d attempts: %w", cfg.MaxRetries+1, lastErr)
}
