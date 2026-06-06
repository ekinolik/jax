package signals

import (
	"fmt"

	"github.com/ekinolik/jax/pkg/confluence"
)

const (
	dexAlignedPct = 0.008
	dexNeutralPct = 0.02
	dexMaxLookPct = 0.03
)

// DeltaInput holds data for the delta support axis signal.
type DeltaInput struct {
	Spot   float64
	Levels confluence.Levels
}

// ComputeDeltaSupport scores proximity to rank-1 DEX support below spot.
func ComputeDeltaSupport(in DeltaInput) confluence.Signal {
	signal := confluence.Signal{
		Name:   "delta_support",
		Weight: 0.15,
		Icon:   confluence.IconDelta,
	}

	support, ok := Rank1DEXSupport(in.Levels)
	if !ok || in.Spot <= 0 {
		signal.Status = confluence.SignalAgainst
		signal.Detail = "no DEX support level"
		return signal
	}

	distPct := (in.Spot - support.Price) / in.Spot
	fill := dexAxisFill(distPct)
	signal.AxisFill = fill
	signal.Status = statusFromFill(fill)
	signal.Detail = fmt.Sprintf("rank-1 DEX support %.2f (%.2f%% above)", support.Price, distPct*100)
	signal.Score = signal.AxisFill * signal.Weight * 100
	return signal
}

func dexAxisFill(distPct float64) float64 {
	if distPct < 0 {
		return 0.15
	}
	if distPct <= dexAlignedPct {
		return 1.0
	}
	if distPct <= dexNeutralPct {
		return 0.7
	}
	if distPct <= dexMaxLookPct {
		return 0.45
	}
	return 0.2
}
