package signals_test

import (
	"testing"

	"github.com/ekinolik/jax/pkg/confluence"
	"github.com/ekinolik/jax/pkg/confluence/signals"
	"github.com/stretchr/testify/assert"
)

func TestShortSqueeze_highSINoActivity(t *testing.T) {
	result := signals.ComputeShortSqueeze(signals.ShortSqueezeInput{
		ShortInterestPct: 25,
		DaysToCover:      7,
		ShortVolumeRatio: 35,
		FloatShares:      100_000_000,
		Regime:           confluence.GammaPositive,
		Spot:             100,
		RelativeVolume:   1.0,
	})
	assert.Less(t, result.CombinedScore, 15.0)
}

func TestShortSqueeze_activePressure(t *testing.T) {
	result := signals.ComputeShortSqueeze(signals.ShortSqueezeInput{
		ShortInterestPct: 12,
		DaysToCover:      2.5,
		ShortVolumeRatio: 65,
		FloatShares:      80_000_000,
		Regime:           confluence.GammaNegative,
		Spot:             105,
		CallWall:         102,
		RelativeVolume:   3.5,
		New20DayHigh:     true,
	})
	assert.Greater(t, result.CombinedScore, 30.0)
	assert.True(t, result.ShortSqueezeActive)
}

func TestShortSqueeze_multiplicativeForm(t *testing.T) {
	highSI := signals.ComputeShortSqueeze(signals.ShortSqueezeInput{
		ShortInterestPct: 30, DaysToCover: 8, ShortVolumeRatio: 70,
		Regime: confluence.GammaPositive, RelativeVolume: 0.8,
	})
	active := signals.ComputeShortSqueeze(signals.ShortSqueezeInput{
		ShortInterestPct: 12, DaysToCover: 2, ShortVolumeRatio: 60,
		Regime: confluence.GammaNegative, Spot: 105, CallWall: 100,
		RelativeVolume: 3, New20DayHigh: true,
	})
	assert.Greater(t, active.CombinedScore, highSI.CombinedScore)
}
