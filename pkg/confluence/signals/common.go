package signals

import "github.com/ekinolik/jax/pkg/confluence"

func statusFromFill(fill float64) confluence.SignalStatus {
	switch {
	case fill >= 0.7:
		return confluence.SignalAligned
	case fill >= 0.35:
		return confluence.SignalNeutral
	default:
		return confluence.SignalAgainst
	}
}
