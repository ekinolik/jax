package signals

import (
	"fmt"
	"math"

	"github.com/ekinolik/jax/pkg/confluence"
)

// DownsideInput holds geometry for downside / R:R scoring.
type DownsideInput struct {
	Geometry confluence.TradeGeometry
	Scoring  confluence.ScoringConfig
	Weight   float64
}

// ComputeDownside favors tight stops and R/R ≥ min threshold.
func ComputeDownside(in DownsideInput) confluence.Signal {
	sig := confluence.Signal{
		Name:   "downside",
		Weight: in.Weight,
		Icon:   confluence.IconDownside,
	}
	if !in.Geometry.HasDownside {
		sig.Status = confluence.SignalNeutral
		sig.Detail = "no downside reference"
		return sig
	}

	fill := 0.5
	if in.Geometry.RiskReward >= in.Scoring.MinRiskReward {
		fill = 0.85
		if in.Geometry.RiskReward >= 2.5 {
			fill = 1.0
		}
	} else if in.Geometry.RiskReward >= 1.0 {
		fill = 0.55
	} else {
		fill = 0.25
	}
	if in.Geometry.DownsidePct > 0.04 {
		fill *= 0.6
	}

	fill = math.Max(0, math.Min(1, fill))
	sig.AxisFill = fill
	sig.Status = statusFromFill(fill)
	detail := fmt.Sprintf("R/R %.1f stop %.1f%% below", in.Geometry.RiskReward, in.Geometry.DownsidePct*100)
	if in.Geometry.StopEstimated {
		detail += " (est)"
	}
	sig.Detail = detail
	sig.Score = sig.AxisFill * sig.Weight * 100
	return sig
}
