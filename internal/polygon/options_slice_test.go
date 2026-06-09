package polygon_test

import (
	"context"
	"testing"
	"time"

	"github.com/ekinolik/jax/internal/polygon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithRetry_cancelDuringPaginationLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		err := polygon.WithRetry(ctx, polygon.RetryConfig{MaxRetries: 0, BaseDelayMs: 1}, func(callCtx context.Context) error {
			for {
				if callCtx.Err() != nil {
					return callCtx.Err()
				}
				time.Sleep(5 * time.Millisecond)
			}
		})
		require.ErrorIs(t, err, context.Canceled)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("pagination loop did not exit after context cancel")
	}
}

func TestWithRetry_deadlineDuringPaginationLoop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	start := time.Now()
	err := polygon.WithRetry(ctx, polygon.RetryConfig{MaxRetries: 0, BaseDelayMs: 1}, func(callCtx context.Context) error {
		for {
			if callCtx.Err() != nil {
				return callCtx.Err()
			}
			time.Sleep(5 * time.Millisecond)
		}
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), time.Second)
}
