package confluence

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithAPITimeout_respectsParentDeadline(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	ctx, cancel := withAPITimeout(parent)
	defer cancel()

	<-ctx.Done()
	elapsed := time.Since(start)
	require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
	assert.Less(t, elapsed, 200*time.Millisecond)
}

func TestWithAPITimeout_honorsCancel(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	ctx, ctxCancel := withAPITimeout(parent)
	defer ctxCancel()

	cancel()
	require.Eventually(t, func() bool {
		return ctx.Err() != nil
	}, time.Second, 10*time.Millisecond)
	assert.ErrorIs(t, ctx.Err(), context.Canceled)
}
