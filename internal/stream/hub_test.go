package stream

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReconnectBackoffCapsAtMax(t *testing.T) {
	delay := reconnectBaseDelay
	for i := 0; i < 10; i++ {
		if delay < reconnectMaxDelay {
			delay *= 2
			if delay > reconnectMaxDelay {
				delay = reconnectMaxDelay
			}
		}
	}
	assert.Equal(t, reconnectMaxDelay, delay)
}

func TestHubInjectSpotInvokesHandler(t *testing.T) {
	h := &Hub{
		spots: make(map[string]SpotTick),
		subs:  make(map[string]int),
	}
	now := time.Now().UTC()
	got := make(chan SpotTick, 1)
	h.OnSpotUpdate(func(_ string, tick SpotTick) {
		got <- tick
	})
	h.InjectSpot("NVDA", SpotTick{Price: 120.5, Timestamp: now})

	select {
	case tick := <-got:
		assert.Equal(t, 120.5, tick.Price)
	case <-time.After(time.Second):
		t.Fatal("handler not invoked")
	}
}

func TestHubSupervisorExitsOnCancel(t *testing.T) {
	h := &Hub{apiKey: "test"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	h.runSupervisor(ctx)
}

func TestConnectWithContextRespectsCancellation(t *testing.T) {
	h, err := NewHub("test-key")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = h.connectWithContext(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestHubStopReturnsQuickly(t *testing.T) {
	h, err := NewHub("test-key")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, h.Start(ctx))

	done := make(chan struct{})
	go func() {
		h.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("Stop blocked")
	}
	cancel()
}
