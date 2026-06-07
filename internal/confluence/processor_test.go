package confluence_test

import (
	"testing"
	"time"

	intconfluence "github.com/ekinolik/jax/internal/confluence"
	pkgconfluence "github.com/ekinolik/jax/pkg/confluence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessor_RecomputeSnapshot(t *testing.T) {
	settings, err := pkgconfluence.LoadSettings("../../confluence-configs/settings.yaml")
	require.NoError(t, err)

	sectors, err := pkgconfluence.LoadSICSectors("../../confluence-configs/sic_sectors.yaml")
	require.NoError(t, err)

	registry := intconfluence.NewRegistry(settings.MaxActiveTickers)
	processor := intconfluence.NewProcessor(settings, sectors, registry, nil, nil, nil)

	now := time.Date(2024, 6, 7, 14, 0, 0, 0, time.UTC)
	slices := []pkgconfluence.OptionSlice{{
		Ticker:       "NVDA",
		Expiration:   "2024-06-14",
		ExpiryWeight: 1.0,
		Strikes: []pkgconfluence.StrikeProfile{
			{Strike: 117, CallOI: 5000, CallGamma: 0.05, CallDelta: 0.45},
			{Strike: 118, CallOI: 3000, CallGamma: 0.03, CallDelta: 0.4},
			{Strike: 122, CallOI: 4000, CallGamma: 0.035, CallDelta: 0.35},
		},
	}}

	snap, err := processor.RecomputeSnapshot("NVDA", pkgconfluence.ScoreInput{
		Spot:         119.0,
		Slices:       slices,
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
	}, now)
	require.NoError(t, err)
	require.NotNil(t, snap)

	assert.Equal(t, "NVDA", snap.Ticker)
	assert.Greater(t, snap.Score, 0.0)
	assert.NotEmpty(t, snap.Signals)
	assert.True(t, snap.Levels.HasNearestSupport || snap.Levels.HasNearestResistance)

	stored, ok := processor.LatestSnapshot("NVDA")
	require.True(t, ok)
	assert.Equal(t, snap.Score, stored.Score)
}
