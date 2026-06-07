package signals

import (
	"github.com/ekinolik/jax/pkg/confluence"
)

const (
	exitIdealPct = 0.003
	exitEarlyPct = 0.015
)

// ExitInput holds spot and levels for distance-to-exit timing.
type ExitInput struct {
	Spot   float64
	Levels confluence.Levels
}

// ComputeDistanceToExit compares spot to rank-1 resistance for early/ideal/late.
func ComputeDistanceToExit(in ExitInput) confluence.ExitTiming {
	if in.Spot <= 0 {
		return confluence.ExitLate
	}
	res, ok := Rank1Resistance(in.Levels)
	if !ok {
		return confluence.ExitLate
	}
	if in.Spot >= res.Price {
		return confluence.ExitLate
	}
	distPct := (res.Price - in.Spot) / in.Spot
	switch {
	case distPct <= exitIdealPct:
		return confluence.ExitIdeal
	case distPct <= exitEarlyPct:
		return confluence.ExitEarly
	default:
		return confluence.ExitLate
	}
}

// UpsideBeyondResistancePct returns room from rank-1 to rank-2 resistance.
func UpsideBeyondResistancePct(levels confluence.Levels) float64 {
	r1, ok1 := Rank1Resistance(levels)
	r2, ok2 := Rank2Resistance(levels)
	if !ok1 || !ok2 || r1.Price <= 0 {
		return 0
	}
	return (r2.Price - r1.Price) / r1.Price
}
