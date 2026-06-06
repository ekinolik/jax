package signals

import (
	"fmt"
	"math"

	"github.com/ekinolik/jax/pkg/confluence"
)

const (
	gammaAlignedPct   = 0.005
	gammaNeutralPct   = 0.015
	gammaMaxLookPct   = 0.03
	stackedZoneBonus  = 0.15
)

// GammaInput holds data for the gamma support axis signal.
type GammaInput struct {
	Spot   float64
	Levels confluence.Levels
}

// ComputeGammaSupport scores proximity to rank-1 GEX support and stacked-zone bonus.
func ComputeGammaSupport(in GammaInput) confluence.Signal {
	signal := confluence.Signal{
		Name:   "gamma_support",
		Weight: 0.25,
		Icon:   confluence.IconGamma,
	}

	support, ok := Rank1GEXSupport(in.Levels)
	if !ok || in.Spot <= 0 {
		signal.Status = confluence.SignalAgainst
		signal.Detail = "no GEX support level"
		return signal
	}

	distPct := (in.Spot - support.Price) / in.Spot
	fill := gammaAxisFill(distPct)
	signal.AxisFill = fill
	signal.Status = statusFromFill(fill)
	signal.Detail = fmt.Sprintf("rank-1 GEX support %.2f (%.2f%% above)", support.Price, distPct*100)

	if HasStackedZone(in.Levels, in.Spot) {
		signal.AxisFill = math.Min(1, signal.AxisFill+stackedZoneBonus)
		if signal.AxisFill >= 0.7 {
			signal.Status = confluence.SignalAligned
		}
		signal.Detail += "; stacked zone"
	}

	signal.Score = signal.AxisFill * signal.Weight * 100
	return signal
}

func gammaAxisFill(distPct float64) float64 {
	if distPct < 0 {
		return 0.15
	}
	if distPct <= gammaAlignedPct {
		return 1.0
	}
	if distPct <= gammaNeutralPct {
		return 0.75
	}
	if distPct <= gammaMaxLookPct {
		return 0.5
	}
	return 0.2
}
