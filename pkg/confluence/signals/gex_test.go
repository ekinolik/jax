package signals_test

import (
	"testing"

	"github.com/ekinolik/jax/pkg/confluence"
	"github.com/ekinolik/jax/pkg/confluence/signals"
	"github.com/stretchr/testify/assert"
)

func TestComputeGammaSupport_alignedNearSupport(t *testing.T) {
	levels := confluence.Levels{
		Support: []confluence.Level{
			{Price: 119.5, Source: confluence.LevelSourceGEX, Rank: 1, Strength: 0.9},
		},
		HasNearestSupport: true,
		NearestSupport:    119.5,
	}

	sig := signals.ComputeGammaSupport(signals.GammaInput{Spot: 120.0, Levels: levels})
	assert.Equal(t, confluence.SignalAligned, sig.Status)
	assert.GreaterOrEqual(t, sig.AxisFill, 0.7)
	assert.Equal(t, confluence.IconGamma, sig.Icon)
}
