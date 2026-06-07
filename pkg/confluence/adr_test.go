package confluence_test

import (
	"testing"

	"github.com/ekinolik/jax/pkg/confluence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeADRBars(pcts []float64) []confluence.DailyBar {
	bars := make([]confluence.DailyBar, len(pcts))
	for i, pct := range pcts {
		close := 100.0
		high := close * (1 + pct/200)
		low := close * (1 - pct/200)
		bars[i] = confluence.DailyBar{High: high, Low: low, Close: close, Volume: 1e6}
	}
	return bars
}

func TestComputeADR_stableHigh(t *testing.T) {
	pcts := make([]float64, 30)
	for i := range pcts {
		pcts[i] = 4.2
	}
	cfg := confluence.DefaultScoringConfig()
	m := confluence.ComputeADR(makeADRBars(pcts), cfg)
	require.True(t, m.HasData)
	assert.InDelta(t, 4.2, m.ADR30dPct, 0.2)
	assert.Equal(t, confluence.ADRStableHigh, m.Regime)
}

func TestComputeADR_spikeWarning(t *testing.T) {
	pcts := make([]float64, 30)
	for i := range pcts {
		if i >= 25 {
			pcts[i] = 5.1
		} else {
			pcts[i] = 1.8
		}
	}
	cfg := confluence.DefaultScoringConfig()
	m := confluence.ComputeADR(makeADRBars(pcts), cfg)
	require.True(t, m.HasData)
	assert.Equal(t, confluence.ADRSpikeWarning, m.Regime)
	assert.Greater(t, m.SpikeRatio, 1.4)
}

func TestADRAxisFill_stableHighFullCredit(t *testing.T) {
	m := confluence.ADRMetrics{ADR30dPct: 4.2, Regime: confluence.ADRStableHigh, HasData: true}
	fill := confluence.ADRAxisFill(m)
	assert.Greater(t, fill, 0.78)
}

func TestADRAxisFill_spikeWarningPenalty(t *testing.T) {
	m := confluence.ADRMetrics{ADR30dPct: 1.8, Regime: confluence.ADRSpikeWarning, HasData: true}
	fill := confluence.ADRAxisFill(m)
	assert.Less(t, fill, 0.4)
}
