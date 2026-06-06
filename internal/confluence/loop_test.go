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

func testSettings(t *testing.T) *pkgconfluence.Settings {
	t.Helper()
	settings, err := pkgconfluence.LoadSettings("../../confluence-configs/settings.yaml")
	require.NoError(t, err)
	return settings
}

func testSectors(t *testing.T) *pkgconfluence.SICSectors {
	t.Helper()
	sectors, err := pkgconfluence.LoadSICSectors("../../confluence-configs/sic_sectors.yaml")
	require.NoError(t, err)
	return sectors
}

func TestSnapshotFingerprint_changesOnSignalStatus(t *testing.T) {
	base := &pkgconfluence.ConfluenceSnapshot{
		Score: 55,
		Signals: []pkgconfluence.Signal{
			{Name: "rsi", Status: pkgconfluence.SignalAligned},
		},
	}
	other := &pkgconfluence.ConfluenceSnapshot{
		Score: 55,
		Signals: []pkgconfluence.Signal{
			{Name: "rsi", Status: pkgconfluence.SignalAgainst},
		},
	}
	assert.NotEqual(t, snapshotFingerprint(base), snapshotFingerprint(other))
	assert.Equal(t, snapshotFingerprint(base), snapshotFingerprint(base))
}

func TestShouldPrefetchNow_weekdayAtConfiguredTime(t *testing.T) {
	settings := testSettings(t)
	p := NewProcessor(settings, testSectors(t), NewRegistry(5), nil, nil, nil)

	loc, err := time.LoadLocation(settings.MarketHours.Timezone)
	require.NoError(t, err)
	atPrefetch := time.Date(2024, 6, 7, 8, 0, 0, 0, loc)
	assert.True(t, p.shouldPrefetchNow(atPrefetch))

	atOther := time.Date(2024, 6, 7, 9, 0, 0, 0, loc)
	assert.False(t, p.shouldPrefetchNow(atOther))

	saturday := time.Date(2024, 6, 8, 8, 0, 0, 0, loc)
	assert.False(t, p.shouldPrefetchNow(saturday))
}

func TestWatchSubscribeAndSnapshot(t *testing.T) {
	settings := testSettings(t)
	registry := NewRegistry(settings.MaxActiveTickers)

	p := NewProcessor(settings, testSectors(t), registry, nil, nil, nil)
	ctx := context.Background()
	require.NoError(t, p.Start(ctx))
	defer p.Stop()

	ch, unsub, err := p.Watch(ctx, "NVDA")
	require.NoError(t, err)
	defer unsub()

	// Without a polygon client bootstrap cannot score; unscored placeholders are not pushed.
	select {
	case snap := <-ch:
		assert.Equal(t, "NVDA", snap.Ticker)
		assert.False(t, SnapshotNeedsBootstrap(snap))
	case <-time.After(200 * time.Millisecond):
	}

	got, ok := p.GetSnapshot("NVDA")
	require.True(t, ok)
	assert.Equal(t, "NVDA", got.Ticker)
}

func TestOnSubscribe_RTHSnapshot(t *testing.T) {
	settings := testSettings(t)
	p := NewProcessor(settings, testSectors(t), NewRegistry(5), nil, nil, nil)

	now := time.Date(2024, 6, 7, 15, 0, 0, 0, time.UTC) // 11:00 ET, RTH
	require.NoError(t, p.OnSubscribe(context.Background(), "NVDA", now))

	got, ok := p.GetSnapshot("NVDA")
	require.True(t, ok)
	assert.Equal(t, pkgconfluence.MarketStatusOpen, got.MarketStatus)
}

func TestRegistryIdleTriggersDeactivate(t *testing.T) {
	settings := testSettings(t)
	registry := NewRegistry(settings.MaxActiveTickers)
	p := NewProcessor(settings, testSectors(t), registry, nil, nil, nil)

	_, err := registry.Subscribe("SPY")
	require.NoError(t, err)
	registry.Unsubscribe("SPY")

	entry, ok := registry.Get("SPY")
	require.True(t, ok)
	entry.IdleSince = time.Now().UTC().Add(-idleGracePeriod - time.Second)

	idle := registry.IdleTickers(idleGracePeriod)
	require.Contains(t, idle, "SPY")

	p.deactivateTicker("SPY")
	assert.False(t, registry.HasEntries())
	_, ok = p.GetSnapshot("SPY")
	assert.False(t, ok)
}

func TestMaybePushUpdate_debouncesDuplicates(t *testing.T) {
	settings := testSettings(t)
	p := NewProcessor(settings, testSectors(t), NewRegistry(5), nil, nil, nil)
	p.tickers["NVDA"] = &tickerRuntime{
		watchers: make(map[int]chan *pkgconfluence.ConfluenceSnapshot),
	}

	snap := &pkgconfluence.ConfluenceSnapshot{
		Score: 60,
		Signals: []pkgconfluence.Signal{
			{Name: "market", Status: pkgconfluence.SignalAligned},
		},
	}

	p.maybePushUpdate("NVDA", snap)
	fp := snapshotFingerprint(snap)

	p.tickerMu.Lock()
	rt := p.tickers["NVDA"]
	rt.lastPushFP = fp
	rt.lastPushAt = time.Now().UTC()
	p.tickerMu.Unlock()

	// Same fingerprint within debounce window should not update lastPushAt.
	before := time.Now().UTC()
	p.maybePushUpdate("NVDA", snap)
	p.tickerMu.Lock()
	assert.True(t, rt.lastPushAt.Before(before.Add(time.Millisecond)))
	p.tickerMu.Unlock()
}

func TestOnSpotTick_schedulesBenchmarkRecompute(t *testing.T) {
	settings := testSettings(t)
	p := NewProcessor(settings, testSectors(t), NewRegistry(5), nil, nil, nil)

	now := time.Date(2024, 6, 7, 14, 0, 0, 0, time.UTC)
	p.tickers["NVDA"] = &tickerRuntime{
		sectorETF: "SMH",
		watchers:  make(map[int]chan *pkgconfluence.ConfluenceSnapshot),
	}

	p.onSpotTick("SPY", stream.SpotTick{Price: 501, Timestamp: now})
	p.tickerMu.Lock()
	rt := p.tickers["NVDA"]
	assert.NotNil(t, rt.debounceTimer)
	p.tickerMu.Unlock()
}
