package confluence_test

import (
	"context"
	"testing"
	"time"

	intconfluence "github.com/ekinolik/jax/internal/confluence"
	pkgconfluence "github.com/ekinolik/jax/pkg/confluence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSnapshotNeedsBootstrap(t *testing.T) {
	tests := []struct {
		name string
		snap *pkgconfluence.ConfluenceSnapshot
		want bool
	}{
		{name: "nil", snap: nil, want: true},
		{
			name: "loading",
			snap: &pkgconfluence.ConfluenceSnapshot{OIStatus: pkgconfluence.OIStatusLoading, MarketStatus: pkgconfluence.MarketStatusClosed},
			want: true,
		},
		{
			name: "ready without levels",
			snap: &pkgconfluence.ConfluenceSnapshot{OIStatus: pkgconfluence.OIStatusReady, MarketStatus: pkgconfluence.MarketStatusClosed},
			want: true,
		},
		{
			name: "ready with gamma flip",
			snap: &pkgconfluence.ConfluenceSnapshot{
				OIStatus:     pkgconfluence.OIStatusReady,
				MarketStatus: pkgconfluence.MarketStatusClosed,
				Levels:       pkgconfluence.Levels{GammaFlip: 118.5},
			},
			want: false,
		},
		{
			name: "error with levels",
			snap: &pkgconfluence.ConfluenceSnapshot{
				OIStatus:     pkgconfluence.OIStatusError,
				MarketStatus: pkgconfluence.MarketStatusClosed,
				Levels:       pkgconfluence.Levels{GammaFlip: 118.5},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, intconfluence.SnapshotNeedsBootstrap(tt.snap))
		})
	}
}

func TestBootstrapSnapshot_nilClient(t *testing.T) {
	settings, err := pkgconfluence.LoadSettings("../../confluence-configs/settings.yaml")
	require.NoError(t, err)
	sectors, err := pkgconfluence.LoadSICSectors("../../confluence-configs/sic_sectors.yaml")
	require.NoError(t, err)

	registry := intconfluence.NewRegistry(settings.MaxActiveTickers)
	processor := intconfluence.NewProcessor(settings, sectors, registry, nil, nil, nil)

	_, err = processor.BootstrapSnapshot(context.Background(), "NVDA")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "polygon client not configured")
}

func TestRecomputeSnapshot_closedMarketMetadata(t *testing.T) {
	settings, err := pkgconfluence.LoadSettings("../../confluence-configs/settings.yaml")
	require.NoError(t, err)
	sectors, err := pkgconfluence.LoadSICSectors("../../confluence-configs/sic_sectors.yaml")
	require.NoError(t, err)

	registry := intconfluence.NewRegistry(settings.MaxActiveTickers)
	processor := intconfluence.NewProcessor(settings, sectors, registry, nil, nil, nil)

	closed := time.Date(2026, 6, 6, 22, 0, 0, 0, time.UTC) // Saturday evening ET is closed
	dataAsOf := closed.Add(-30 * time.Minute)

	slices := []pkgconfluence.OptionSlice{{
		Ticker:       "NVDA",
		Expiration:   "2026-06-13",
		ExpiryWeight: 1.0,
		OIAsOf:       dataAsOf,
		GreeksAsOf:   dataAsOf,
		Strikes: []pkgconfluence.StrikeProfile{
			{Strike: 117, CallOI: 5000, CallGamma: 0.05, CallDelta: 0.45},
			{Strike: 118, CallOI: 3000, CallGamma: 0.03, CallDelta: 0.4},
			{Strike: 122, CallOI: 4000, CallGamma: 0.035, CallDelta: 0.35},
		},
	}}

	snap, err := processor.RecomputeSnapshot("NVDA", pkgconfluence.ScoreInput{
		Spot:         119.0,
		SpotTime:     dataAsOf,
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
		DataAsOf:     dataAsOf,
		Now:          closed,
	}, closed)
	require.NoError(t, err)
	require.NotNil(t, snap)

	assert.Equal(t, pkgconfluence.MarketStatusClosed, snap.MarketStatus)
	assert.Equal(t, pkgconfluence.OIStatusReady, snap.OIStatus)
	assert.Equal(t, dataAsOf.UTC(), snap.DataAsOf.UTC())
	assert.False(t, intconfluence.SnapshotNeedsBootstrap(snap))
	assert.True(t, snap.Levels.HasNearestSupport || snap.Levels.HasNearestResistance || snap.Levels.GammaFlip > 0)
	assert.Greater(t, snap.Score, 0.0)
}
