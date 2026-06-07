package signals

import (
	"fmt"

	"github.com/ekinolik/jax/pkg/confluence"
)

// ADRInput holds ADR metrics for the buy signal axis.
type ADRInput struct {
	Metrics confluence.ADRMetrics
	Weight  float64
}

// ComputeADRSignal maps dual-window ADR to buy axis fill.
func ComputeADRSignal(in ADRInput) confluence.Signal {
	sig := confluence.Signal{
		Name:   "adr",
		Weight: in.Weight,
		Icon:   confluence.IconADR,
	}
	if !in.Metrics.HasData {
		sig.Status = confluence.SignalNeutral
		sig.Detail = "ADR unavailable"
		return sig
	}
	fill := confluence.ADRAxisFill(in.Metrics)
	sig.AxisFill = fill
	sig.Status = statusFromFill(fill)
	sig.Detail = fmt.Sprintf("ADR 30d %.1f%% 5d %.1f%% %s", in.Metrics.ADR30dPct, in.Metrics.ADR5dPct, in.Metrics.Regime)
	sig.Score = sig.AxisFill * sig.Weight * 100
	return sig
}
