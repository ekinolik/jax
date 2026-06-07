package signals_test

import (
	"testing"

	"github.com/ekinolik/jax/pkg/confluence"
	"github.com/ekinolik/jax/pkg/confluence/signals"
	"github.com/stretchr/testify/assert"
)

func TestGammaSqueezeActive_allConditions(t *testing.T) {
	spot := 105.0
	slices := []confluence.OptionSlice{{
		ExpiryWeight: 1.0,
		Strikes: []confluence.StrikeProfile{
			{Strike: 100, CallOI: 5000, CallGamma: 0.05, PutOI: 200, PutGamma: -0.01},
			{Strike: 102, CallOI: 8000, CallGamma: 0.06, PutOI: 100, PutGamma: -0.005},
			{Strike: 108, CallOI: 3000, CallGamma: 0.03, PutOI: 100, PutGamma: -0.005},
		},
	}}
	levels := signals.ComputeLevels(slices, spot)
	result := signals.ComputeGammaRegime(signals.GammaRegimeInput{
		Spot: spot, Slices: slices, Levels: levels,
		SessionVWAP: 103, RelativeVolume: 2.0, NeutralBandPct: 0.003,
	})
	if result.Regime == confluence.GammaNegative && result.CallWall > 0 && spot > result.CallWall {
		assert.True(t, result.GammaSqueezeActive)
	}
}

func TestShortSqueezeActive_thresholds(t *testing.T) {
	result := signals.ComputeShortSqueeze(signals.ShortSqueezeInput{
		ShortInterestPct: 15, DaysToCover: 4, ShortVolumeRatio: 62,
		Regime: confluence.GammaNegative, Spot: 110, CallWall: 105,
		RelativeVolume: 2.5, New20DayHigh: true,
	})
	if result.PressureScore >= 50 && result.TriggerScore >= 60 {
		assert.True(t, result.ShortSqueezeActive)
	}
}
