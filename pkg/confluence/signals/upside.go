package signals

import (
	"fmt"
	"math"

	"github.com/ekinolik/jax/pkg/confluence"
)

// UpsideInput holds geometry and ADR context for upside scoring.
type UpsideInput struct {
	Geometry       confluence.TradeGeometry
	ADR            confluence.ADRMetrics
	Scoring        confluence.ScoringConfig
	Weight         float64
}

// ComputeUpside scores room to rank-1 resistance with tiered fill and ADR modifier.
func ComputeUpside(in UpsideInput) confluence.Signal {
	sig := confluence.Signal{
		Name:   "upside",
		Weight: in.Weight,
		Icon:   confluence.IconUpside,
	}
	if !in.Geometry.HasUpside {
		sig.Status = confluence.SignalNeutral
		sig.Detail = "no upside target"
		return sig
	}

	fill := upsideTierFill(in.Geometry.UpsidePct, in.Scoring.MinUpsidePct, in.Scoring.UpsideGreatPct)
	if in.Geometry.UpsidePct >= in.Scoring.UpsideGreatPct && in.ADR.HasData {
		fill *= confluence.ADRUpsideMultiplier(in.ADR.ADR30dPct, in.ADR.Regime)
	}
	fill = math.Max(0, math.Min(1, fill))

	sig.AxisFill = fill
	sig.Status = statusFromFill(fill)
	sig.Detail = fmt.Sprintf("upside %.1f%% to %.2f", in.Geometry.UpsidePct*100, in.Geometry.UpsideTarget)
	sig.Score = sig.AxisFill * sig.Weight * 100
	return sig
}

func upsideTierFill(upsidePct, minPct, greatPct float64) float64 {
	switch {
	case upsidePct < 0.01:
		return 0
	case upsidePct < minPct:
		// 1–3% linear 0–0.3
		if upsidePct < 0.01 {
			return 0
		}
		return 0.3 * (upsidePct - 0.01) / (minPct - 0.01)
	case upsidePct < greatPct:
		// 3–6% linear 0.5–0.85
		return 0.5 + 0.35*(upsidePct-minPct)/(greatPct-minPct)
	default:
		// ≥6% linear 0.85–1.0 capped at 10%
		extra := math.Min(upsidePct-greatPct, 0.04)
		return 0.85 + 0.15*extra/0.04
	}
}
