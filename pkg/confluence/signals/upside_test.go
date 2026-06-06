package signals_test

import (
	"testing"

	"github.com/ekinolik/jax/pkg/confluence"
	"github.com/ekinolik/jax/pkg/confluence/signals"
	"github.com/stretchr/testify/assert"
)

func TestUpsideTierFill_belowMinimum(t *testing.T) {
	levels := confluence.Levels{
		Resistance: []confluence.Level{{Price: 101.5, Rank: 1, Strength: 0.8}},
	}
	geo := signals.TradeGeometry(100, levels)
	sig := signals.ComputeUpside(signals.UpsideInput{
		Geometry: geo,
		Scoring:  confluence.DefaultScoringConfig(),
		Weight:   0.07,
	})
	assert.Less(t, sig.AxisFill, 0.35)
}

func TestUpsideTierFill_acceptableRange(t *testing.T) {
	levels := confluence.Levels{
		Resistance: []confluence.Level{{Price: 104, Rank: 1, Strength: 0.8}},
	}
	geo := signals.TradeGeometry(100, levels)
	sig := signals.ComputeUpside(signals.UpsideInput{
		Geometry: geo,
		ADR:      confluence.ADRMetrics{ADR30dPct: 4, Regime: confluence.ADRStableHigh, HasData: true},
		Scoring:  confluence.DefaultScoringConfig(),
		Weight:   0.07,
	})
	assert.GreaterOrEqual(t, sig.AxisFill, 0.5)
}

func TestUpsideADRModifier_spikePenalty(t *testing.T) {
	levels := confluence.Levels{
		Resistance: []confluence.Level{{Price: 107, Rank: 1, Strength: 0.8}},
	}
	geo := signals.TradeGeometry(100, levels)
	withPenalty := signals.ComputeUpside(signals.UpsideInput{
		Geometry: geo,
		ADR:      confluence.ADRMetrics{ADR30dPct: 1.8, Regime: confluence.ADRSpikeWarning, HasData: true},
		Scoring:  confluence.DefaultScoringConfig(),
		Weight:   0.07,
	})
	without := signals.ComputeUpside(signals.UpsideInput{
		Geometry: geo,
		ADR:      confluence.ADRMetrics{ADR30dPct: 4.5, Regime: confluence.ADRStableHigh, HasData: true},
		Scoring:  confluence.DefaultScoringConfig(),
		Weight:   0.07,
	})
	assert.Less(t, withPenalty.AxisFill, without.AxisFill)
}
