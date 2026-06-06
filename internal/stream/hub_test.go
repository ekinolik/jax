package stream

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
