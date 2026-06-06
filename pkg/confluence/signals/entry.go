package signals

import (
	"github.com/ekinolik/jax/pkg/confluence"
)

const (
	entryIdealPct = 0.003
	entryEarlyPct = 0.015
)

// EntryInput holds spot and levels for distance-to-entry timing.
type EntryInput struct {
	Spot   float64
	Levels confluence.Levels
}

// ComputeDistanceToEntry compares spot to rank-1 support for early/ideal/late timing.
func ComputeDistanceToEntry(in EntryInput) confluence.EntryTiming {
	if in.Spot <= 0 || !in.Levels.HasNearestSupport {
		return confluence.EntryLate
	}

	support := in.Levels.NearestSupport
	distPct := (in.Spot - support) / in.Spot

	if distPct < 0 {
		return confluence.EntryLate
	}
	if distPct <= entryIdealPct {
		return confluence.EntryIdeal
	}
	if distPct <= entryEarlyPct {
		return confluence.EntryEarly
	}
	return confluence.EntryLate
}
