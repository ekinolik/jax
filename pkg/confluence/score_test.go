package confluence_test

import (
	"testing"
	"time"

	"github.com/ekinolik/jax/pkg/confluence"
	"github.com/ekinolik/jax/pkg/confluence/signals"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func baseScoreInput() confluence.ScoreInput {
	return confluence.ScoreInput{
		Ticker:       "NVDA",
		Spot:         120.0,
		OIStatus:     confluence.OIStatusReady,
		MarketStatus: confluence.MarketStatusOpen,
		RSI:          32,
		SPYOpen:      500,
		SPYSpot:      502,
		QQQOpen:      400,
		QQQSpot:      401,
		TargetOpen:   118,
		ETFOpen:      200,
		ETFSpot:      201,
		SectorETF:    "SMH",
		IntradayHigh: 122,
		IntradayLow:  117,
		Now:          time.Date(2024, 6, 7, 15, 0, 0, 0, time.UTC),
		Slices: []confluence.OptionSlice{{
			Ticker:       "NVDA",
			Expiration:   "2024-06-14",
			ExpiryWeight: 1.0,
			Strikes: []confluence.StrikeProfile{
				{Strike: 118, CallOI: 5000, CallGamma: 0.05, CallDelta: 0.45, PutOI: 200, PutGamma: -0.01, PutDelta: -0.2},
				{Strike: 119, CallOI: 3000, CallGamma: 0.04, CallDelta: 0.4, PutOI: 150, PutGamma: -0.008, PutDelta: -0.18},
				{Strike: 122, CallOI: 4000, CallGamma: 0.035, CallDelta: 0.35, PutOI: 100, PutGamma: -0.005, PutDelta: -0.15},
			},
		}},
	}
}

func TestBuildSnapshot_highConviction(t *testing.T) {
	in := baseScoreInput()
	in.Spot = 118.05
	in.RSI = 28
	in.SPYSpot = 508
	in.QQQSpot = 408
	in.TargetOpen = 115
	in.ETFOpen = 198
	in.ETFSpot = 202
	in.Slices[0].Strikes = []confluence.StrikeProfile{
		{Strike: 117, CallOI: 8000, CallGamma: 0.06, CallDelta: 0.5, PutOI: 100, PutGamma: -0.005, PutDelta: -0.1},
		{Strike: 118, CallOI: 6000, CallGamma: 0.04, CallDelta: 0.45, PutOI: 200, PutGamma: -0.01, PutDelta: -0.2},
		{Strike: 119, CallOI: 2000, CallGamma: 0.02, CallDelta: 0.35, PutOI: 500, PutGamma: -0.02, PutDelta: -0.3},
		{Strike: 122, CallOI: 5000, CallGamma: 0.04, CallDelta: 0.3, PutOI: 100, PutGamma: -0.005, PutDelta: -0.15},
	}

	snap := signals.BuildSnapshot(in)

	assert.GreaterOrEqual(t, snap.Score, 75.0, "signals: %+v", snap.Signals)
	assert.Equal(t, confluence.ReadinessHighConviction, snap.ReadinessBand)
	assert.Equal(t, 3, snap.BackgroundLevel)
	assert.Equal(t, 3, snap.HapticLevel)
	require.Len(t, snap.Signals, 5)
}

func TestBuildSnapshot_possibleEntry(t *testing.T) {
	in := baseScoreInput()
	in.Spot = 119.2
	in.RSI = 38
	in.SPYSpot = 502
	in.QQQSpot = 401
	in.TargetOpen = 117
	in.Slices[0].Strikes = []confluence.StrikeProfile{
		{Strike: 117, CallOI: 5000, CallGamma: 0.05, CallDelta: 0.48},
		{Strike: 118, CallOI: 3000, CallGamma: 0.03, CallDelta: 0.4},
		{Strike: 121, CallOI: 4000, CallGamma: 0.035, CallDelta: 0.32},
	}

	snap := signals.BuildSnapshot(in)

	assert.GreaterOrEqual(t, snap.Score, 55.0)
	assert.Less(t, snap.Score, 75.0)
	assert.Equal(t, confluence.ReadinessPossibleEntry, snap.ReadinessBand)
}

func TestBuildSnapshot_caution(t *testing.T) {
	in := baseScoreInput()
	in.Spot = 120.5
	in.RSI = 46
	in.SPYSpot = 500.5
	in.QQQSpot = 399.5
	in.TargetOpen = 119
	in.ETFSpot = 200.5
	in.Slices[0].Strikes = []confluence.StrikeProfile{
		{Strike: 115, CallOI: 3000, CallGamma: 0.04, CallDelta: 0.45},
		{Strike: 117, CallOI: 2500, CallGamma: 0.03, CallDelta: 0.4},
		{Strike: 122, CallOI: 3500, CallGamma: 0.035, CallDelta: 0.3},
	}

	snap := signals.BuildSnapshot(in)

	assert.GreaterOrEqual(t, snap.Score, 35.0)
	assert.Less(t, snap.Score, 55.0)
	assert.Equal(t, confluence.ReadinessCaution, snap.ReadinessBand)
}

func TestBuildSnapshot_noTrade(t *testing.T) {
	in := baseScoreInput()
	in.OIStatus = confluence.OIStatusLoading
	in.Slices = nil
	in.RSI = 62
	in.SPYSpot = 495
	in.QQQSpot = 392
	in.Spot = 123
	in.TargetOpen = 122
	in.ETFSpot = 198

	snap := signals.BuildSnapshot(in)

	assert.Less(t, snap.Score, 35.0)
	assert.Equal(t, confluence.ReadinessNoTrade, snap.ReadinessBand)
	assert.Equal(t, 0, snap.HapticLevel)
}

func TestCompositeScore_stackedZoneBonus(t *testing.T) {
	sigs := []confluence.Signal{
		{Score: 20},
		{Score: 20},
		{Score: 20},
		{Score: 15},
		{Score: 10},
	}
	score := confluence.CompositeScore(sigs, true)
	assert.InDelta(t, 90, score, 0.01)
}

func TestBandForScore_thresholds(t *testing.T) {
	assert.Equal(t, confluence.ReadinessNoTrade, confluence.BandForScore(20))
	assert.Equal(t, confluence.ReadinessCaution, confluence.BandForScore(40))
	assert.Equal(t, confluence.ReadinessPossibleEntry, confluence.BandForScore(60))
	assert.Equal(t, confluence.ReadinessHighConviction, confluence.BandForScore(80))
}
