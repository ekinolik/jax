package confluence

import (
	"context"
	"testing"
	"time"

	"github.com/ekinolik/jax/internal/stream"
	pkgconfluence "github.com/ekinolik/jax/pkg/confluence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackgroundWarmup_doesNotBlockOtherTickers(t *testing.T) {
	block := make(chan struct{})
	testWarmupBlock.Store("SPY", block)
	defer testWarmupBlock.Delete("SPY")

	settings := testSettings(t)
	registry := NewRegistry(settings.MaxActiveTickers)
	p := NewProcessor(settings, testSectors(t), registry, nil, nil, nil)
	require.NoError(t, p.Start(context.Background()))
	defer func() {
		close(block)
		p.Stop()
	}()

	now := time.Now().UTC()
	require.NoError(t, p.OnSubscribe(context.Background(), "SPY", now))
	time.Sleep(20 * time.Millisecond)

	start := time.Now()
	require.NoError(t, p.OnSubscribe(context.Background(), "NVDA", now))
	assert.Less(t, time.Since(start), 500*time.Millisecond)
}

func TestStopCancelsBackgroundWarmup(t *testing.T) {
	block := make(chan struct{})
	testWarmupBlock.Store("SPY", block)
	defer func() {
		close(block)
		testWarmupBlock.Delete("SPY")
	}()

	settings := testSettings(t)
	registry := NewRegistry(settings.MaxActiveTickers)
	p := NewProcessor(settings, testSectors(t), registry, nil, nil, nil)
	require.NoError(t, p.Start(context.Background()))

	require.NoError(t, p.OnSubscribe(context.Background(), "SPY", time.Now().UTC()))
	time.Sleep(20 * time.Millisecond)

	done := make(chan struct{})
	go func() {
		p.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("processor Stop blocked while background warmup was in flight")
	}
}

func TestStartBackgroundWarmup_dedup(t *testing.T) {
	settings := testSettings(t)
	registry := NewRegistry(settings.MaxActiveTickers)
	p := NewProcessor(settings, testSectors(t), registry, nil, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.ctx = ctx
	p.cancel = cancel

	require.NoError(t, p.OnSubscribe(context.Background(), "NVDA", time.Now().UTC()))
	p.startBackgroundWarmup("NVDA")
	p.startBackgroundWarmup("NVDA")

	p.tickerMu.Lock()
	rt := p.tickers["NVDA"]
	require.NotNil(t, rt)
	assert.True(t, rt.warmupRunning)
	p.tickerMu.Unlock()
}

func TestRecomputeSnapshotNow_withoutSubscribers(t *testing.T) {
	settings := testSettings(t)
	registry := NewRegistry(settings.MaxActiveTickers)
	hub, err := stream.NewHub("test-api-key")
	require.NoError(t, err)
	p := NewProcessor(settings, testSectors(t), registry, nil, nil, hub)

	now := time.Date(2024, 6, 7, 15, 0, 0, 0, time.UTC)
	prevNow := timeNowUTC
	timeNowUTC = func() time.Time { return now }
	defer func() { timeNowUTC = prevNow }()

	_, err = registry.Subscribe("SNDK")
	require.NoError(t, err)
	registry.Unsubscribe("SNDK")

	entry, ok := registry.Get("SNDK")
	require.True(t, ok)
	assert.Equal(t, 0, entry.SubscriberCount)

	registry.SetOIState("SNDK", pkgconfluence.OIStatusReady)
	rt := p.ensureRuntime("SNDK")
	rt.slices = []pkgconfluence.OptionSlice{{
		Ticker:       "SNDK",
		Expiration:   "2024-06-14",
		ExpiryWeight: 1.0,
		Strikes: []pkgconfluence.StrikeProfile{
			{Strike: 1640, CallOI: 5000, CallGamma: 0.05, CallDelta: 0.45},
			{Strike: 1650, CallOI: 3000, CallGamma: 0.03, CallDelta: 0.4},
			{Strike: 1680, CallOI: 4000, CallGamma: 0.035, CallDelta: 0.35},
		},
	}}
	rt.sectorETF = "SMH"
	rt.greeksSpot = 1667.43

	marketDate, err := MarketCalendarDate(settings, now)
	require.NoError(t, err)
	dateKey := marketDate.Format("2006-01-02")
	p.dayStatsMu.Lock()
	p.dayStats["SNDK_"+dateKey] = DayStats{Open: 1600, High: 1680, Low: 1590, Volume: 1e6}
	p.dayStats["SPY_"+dateKey] = DayStats{Open: 500, High: 505, Low: 498}
	p.dayStats["QQQ_"+dateKey] = DayStats{Open: 400, High: 402, Low: 398}
	p.dayStats["SMH_"+dateKey] = DayStats{Open: 200, High: 202, Low: 198}
	p.dayStatsMu.Unlock()

	for _, sym := range []string{"SNDK", "SPY", "QQQ", "SMH"} {
		hub.InjectSpot(sym, stream.SpotTick{Price: spotForSymbol(sym), Timestamp: now})
	}

	p.SetSnapshot("SNDK", &pkgconfluence.ConfluenceSnapshot{
		Ticker:       "SNDK",
		Spot:         1667.43,
		OIStatus:     pkgconfluence.OIStatusLoading,
		MarketStatus: pkgconfluence.MarketStatusOpen,
	})

	p.recomputeSnapshotNow(context.Background(), "SNDK")

	snap, ok := p.GetSnapshot("SNDK")
	require.True(t, ok)
	assert.Equal(t, pkgconfluence.OIStatusReady, snap.OIStatus)
	assert.False(t, SnapshotNeedsBootstrap(snap))
	assert.True(t, snap.Levels.GammaFlip > 0 || len(snap.Levels.Support) > 0)
}

func spotForSymbol(sym string) float64 {
	switch sym {
	case "SNDK":
		return 1667.43
	case "SPY":
		return 502
	case "QQQ":
		return 401
	default:
		return 201
	}
}

func TestWarmupTicker_closedMarketUsesBootstrapPath(t *testing.T) {
	settings := testSettings(t)
	registry := NewRegistry(settings.MaxActiveTickers)
	p := NewProcessor(settings, testSectors(t), registry, nil, nil, nil)

	closed := time.Date(2026, 6, 6, 22, 0, 0, 0, time.UTC)
	prevNow := timeNowUTC
	timeNowUTC = func() time.Time { return closed }
	defer func() { timeNowUTC = prevNow }()

	p.SetSnapshot("NVDA", &pkgconfluence.ConfluenceSnapshot{
		Ticker:       "NVDA",
		OIStatus:     pkgconfluence.OIStatusLoading,
		MarketStatus: pkgconfluence.MarketStatusClosed,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p.warmupTicker(ctx, "NVDA")
}
