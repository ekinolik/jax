package signals_test

import (
	"testing"

	"github.com/ekinolik/jax/pkg/confluence"
	"github.com/ekinolik/jax/pkg/confluence/signals"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gammaTestSlices(spot float64) []confluence.OptionSlice {
	return []confluence.OptionSlice{{
		Ticker:       "TEST",
		Expiration:   "2024-06-14",
		ExpiryWeight: 1.0,
		Strikes: []confluence.StrikeProfile{
			{Strike: float32(spot - 5), CallOI: 2000, CallGamma: 0.04, PutOI: 500, PutGamma: -0.02},
			{Strike: float32(spot - 2), CallOI: 8000, CallGamma: 0.06, PutOI: 200, PutGamma: -0.01},
			{Strike: float32(spot + 2), CallOI: 6000, CallGamma: 0.05, PutOI: 100, PutGamma: -0.005},
			{Strike: float32(spot + 5), CallOI: 3000, CallGamma: 0.03, PutOI: 800, PutGamma: -0.015},
		},
	}}
}

func TestComputeGammaRegime_negativeFuel(t *testing.T) {
	spot := 100.0
	levels := signals.ComputeLevels(gammaTestSlices(spot), spot)
	result := signals.ComputeGammaRegime(signals.GammaRegimeInput{
		Spot:           spot,
		Slices:         gammaTestSlices(spot),
		Levels:         levels,
		NeutralBandPct: 0.003,
	})
	require.NotEmpty(t, string(result.Regime))
	assert.NotZero(t, result.NetGEXAtSpot)
}

func TestGammaEnvironment_negativeModeratePositive(t *testing.T) {
	sig := signals.ComputeGammaEnvironmentSignal(5, 0.04)
	assert.Greater(t, sig.AxisFill, 0.5)
}

func TestGammaDirectional_bearishCap(t *testing.T) {
	sig := signals.ComputeGammaDirectionalSignal(-10, 0.06)
	assert.Less(t, sig.AxisFill, 0.35)
}
