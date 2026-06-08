package confluence

import (
	"context"
	"testing"
	"time"

	pkgconfluence "github.com/ekinolik/jax/pkg/confluence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSnapshotNeedsLiveRefresh(t *testing.T) {
	settings := testSettings(t)

	closedSnap := &pkgconfluence.ConfluenceSnapshot{MarketStatus: pkgconfluence.MarketStatusClosed}
	openSnap := &pkgconfluence.ConfluenceSnapshot{MarketStatus: pkgconfluence.MarketStatusOpen}

	loc, err := time.LoadLocation(settings.MarketHours.Timezone)
	require.NoError(t, err)

	preMarket := time.Date(2026, 6, 8, 9, 15, 0, 0, loc).UTC()
	rth := time.Date(2026, 6, 8, 10, 0, 0, 0, loc).UTC()
	afterClose := time.Date(2026, 6, 8, 16, 30, 0, 0, loc).UTC()

	assert.False(t, SnapshotNeedsLiveRefresh(settings, closedSnap, preMarket))
	assert.True(t, SnapshotNeedsLiveRefresh(settings, closedSnap, rth))
	assert.True(t, SnapshotNeedsLiveRefresh(settings, openSnap, rth))
	assert.False(t, SnapshotNeedsLiveRefresh(settings, closedSnap, afterClose))
	assert.True(t, SnapshotNeedsLiveRefresh(settings, openSnap, afterClose))
}

func TestRefreshSnapshotLiveAt_marketStatusTransition(t *testing.T) {
	settings := testSettings(t)
	registry := NewRegistry(settings.MaxActiveTickers)
	processor := NewProcessor(settings, testSectors(t), registry, nil, nil, nil)

	loc, err := time.LoadLocation(settings.MarketHours.Timezone)
	require.NoError(t, err)

	preMarket := time.Date(2026, 6, 8, 9, 15, 0, 0, loc).UTC()
	rth := time.Date(2026, 6, 8, 10, 0, 0, 0, loc).UTC()

	slices := []pkgconfluence.OptionSlice{{
		Ticker:       "NVDA",
		Expiration:   "2026-06-13",
		ExpiryWeight: 1.0,
		Strikes: []pkgconfluence.StrikeProfile{
			{Strike: 117, CallOI: 5000, CallGamma: 0.05, CallDelta: 0.45},
			{Strike: 118, CallOI: 3000, CallGamma: 0.03, CallDelta: 0.4},
			{Strike: 122, CallOI: 4000, CallGamma: 0.035, CallDelta: 0.35},
		},
	}}

	closedSnap, err := processor.RecomputeSnapshot("NVDA", pkgconfluence.ScoreInput{
		Spot:         119.0,
		Slices:       slices,
		OIStatus:     pkgconfluence.OIStatusReady,
		MarketStatus: pkgconfluence.MarketStatusClosed,
		RSI:          33,
		SPYOpen:      500,
		SPYSpot:      502,
		QQQOpen:      400,
		QQQSpot:      401,
		TargetOpen:   118,
		ETFOpen:      200,
		ETFSpot:      201,
		SectorETF:    "SMH",
		IntradayHigh: 121,
		IntradayLow:  117,
		Now:          preMarket,
	}, preMarket)
	require.NoError(t, err)
	require.Equal(t, pkgconfluence.MarketStatusClosed, closedSnap.MarketStatus)

	refreshed, err := processor.refreshSnapshotLiveAt(context.Background(), "NVDA", rth)
	require.NoError(t, err)
	require.NotNil(t, refreshed)
	assert.Equal(t, pkgconfluence.MarketStatusOpen, refreshed.MarketStatus)
	assert.False(t, SnapshotNeedsBootstrap(refreshed))
}

func TestRefreshSnapshotLiveAt_rsiUpdate(t *testing.T) {
	settings := testSettings(t)
	registry := NewRegistry(settings.MaxActiveTickers)
	processor := NewProcessor(settings, testSectors(t), registry, nil, nil, nil)

	loc, err := time.LoadLocation(settings.MarketHours.Timezone)
	require.NoError(t, err)
	rth := time.Date(2026, 6, 8, 10, 0, 0, 0, loc).UTC()

	slices := []pkgconfluence.OptionSlice{{
		Ticker:       "NVDA",
		Expiration:   "2026-06-13",
		ExpiryWeight: 1.0,
		Strikes: []pkgconfluence.StrikeProfile{
			{Strike: 117, CallOI: 5000, CallGamma: 0.05, CallDelta: 0.45},
			{Strike: 118, CallOI: 3000, CallGamma: 0.03, CallDelta: 0.4},
			{Strike: 122, CallOI: 4000, CallGamma: 0.035, CallDelta: 0.35},
		},
	}}

	initial, err := processor.RecomputeSnapshot("NVDA", pkgconfluence.ScoreInput{
		Spot:         119.0,
		Slices:       slices,
		OIStatus:     pkgconfluence.OIStatusReady,
		MarketStatus: pkgconfluence.MarketStatusOpen,
		RSI:          33,
		SPYOpen:      500,
		SPYSpot:      502,
		QQQOpen:      400,
		QQQSpot:      401,
		TargetOpen:   118,
		ETFOpen:      200,
		ETFSpot:      201,
		SectorETF:    "SMH",
		IntradayHigh: 121,
		IntradayLow:  117,
		Now:          rth,
	}, rth)
	require.NoError(t, err)
	require.Equal(t, 33.0, initial.RSI)

	in, changed, err := processor.buildLiveScoreInput(context.Background(), initial, rth, pkgconfluence.MarketStatusOpen)
	require.NoError(t, err)
	in.RSI = 48
	changed = true

	require.True(t, changed)
	refreshed, err := processor.RecomputeSnapshot("NVDA", in, rth)
	require.NoError(t, err)
	assert.Equal(t, 48.0, refreshed.RSI)
	assert.NotEqual(t, initial.Score, refreshed.Score)
}

func TestRefreshSnapshotLiveAt_skipsOutsideRTHWhenAlreadyClosed(t *testing.T) {
	settings := testSettings(t)
	registry := NewRegistry(settings.MaxActiveTickers)
	processor := NewProcessor(settings, testSectors(t), registry, nil, nil, nil)

	loc, err := time.LoadLocation(settings.MarketHours.Timezone)
	require.NoError(t, err)
	afterClose := time.Date(2026, 6, 8, 17, 0, 0, 0, loc).UTC()

	snap := &pkgconfluence.ConfluenceSnapshot{
		Ticker:       "NVDA",
		OIStatus:     pkgconfluence.OIStatusReady,
		MarketStatus: pkgconfluence.MarketStatusClosed,
		RSI:          33,
		UpdatedAt:    afterClose,
		Levels:       pkgconfluence.Levels{GammaFlip: 118.0},
	}
	processor.SetSnapshot("NVDA", snap)

	refreshed, err := processor.refreshSnapshotLiveAt(context.Background(), "NVDA", afterClose)
	require.NoError(t, err)
	assert.Same(t, snap, refreshed)
	assert.Equal(t, 33.0, refreshed.RSI)
}
