package confluence

import "math"

const adrWindowLong = 30
const adrWindowShort = 5

// ComputeADR derives dual-window ADR metrics from daily bars (newest last).
func ComputeADR(bars []DailyBar, cfg ScoringConfig) ADRMetrics {
	out := ADRMetrics{}
	if len(bars) < adrWindowShort {
		return out
	}

	sessionADRs := make([]float64, 0, len(bars))
	for _, bar := range bars {
		if bar.Close <= 0 || bar.High < bar.Low {
			continue
		}
		sessionADRs = append(sessionADRs, (bar.High-bar.Low)/bar.Close*100)
	}
	if len(sessionADRs) < adrWindowShort {
		return out
	}

	n := len(sessionADRs)
	start30 := n - adrWindowLong
	if start30 < 0 {
		start30 = 0
	}
	out.ADR30dPct = mean(sessionADRs[start30:])
	start5 := n - adrWindowShort
	if start5 < 0 {
		start5 = 0
	}
	out.ADR5dPct = mean(sessionADRs[start5:])
	out.SpikeRatio = out.ADR5dPct / math.Max(out.ADR30dPct, 0.01)
	out.Regime = classifyADRRegime(out.ADR30dPct, out.ADR5dPct, out.SpikeRatio, cfg)
	out.HasData = true
	return out
}

func classifyADRRegime(adr30, adr5, spikeRatio float64, cfg ScoringConfig) ADRRegime {
	if spikeRatio < 0.8 {
		return ADRContracting
	}
	if adr30 < cfg.ADR30dSpikeCeilingPct && adr5 >= cfg.ADR5dSpikeFloorPct && spikeRatio >= cfg.ADRSpikeRatioWarn {
		return ADRSpikeWarning
	}
	if adr30 >= 4 && spikeRatio < 1.3 {
		return ADRStableHigh
	}
	if adr30 >= 3 && spikeRatio >= 1.6 {
		return ADRSpikeFade
	}
	if adr30 >= 2 && adr30 < 4 && spikeRatio < 1.4 {
		return ADRWorkable
	}
	if adr30 < 2 && spikeRatio < 1.3 {
		return ADRStableLow
	}
	return ADRWorkable
}

// ADRBaseFill maps 30d ADR to component A fill (0–1).
func ADRBaseFill(adr30 float64) float64 {
	switch {
	case adr30 < 2:
		return 0.15
	case adr30 < 3:
		return 0.45
	case adr30 < 5:
		return 0.70
	default:
		return 0.90
	}
}

// ADRModifierFill maps ADR regime to component B fill (0–1).
func ADRModifierFill(regime ADRRegime) float64 {
	switch regime {
	case ADRStableHigh:
		return 1.0
	case ADRWorkable:
		return 0.85
	case ADRStableLow:
		return 0.5
	case ADRSpikeWarning:
		return 0.25
	case ADRSpikeFade:
		return 0.55
	case ADRContracting:
		return 0.60
	default:
		return 0.5
	}
}

// ADRAxisFill combines 30d base (70%) and regime modifier (30%).
func ADRAxisFill(metrics ADRMetrics) float64 {
	if !metrics.HasData {
		return 0
	}
	base := ADRBaseFill(metrics.ADR30dPct)
	mod := ADRModifierFill(metrics.Regime)
	return 0.70*base + 0.30*mod
}

// ADRUpsideMultiplier scales upside fill when room ≥ great threshold.
func ADRUpsideMultiplier(adr30 float64, regime ADRRegime) float64 {
	var mult float64
	switch {
	case adr30 < 2:
		mult = 0.4
	case adr30 < 4:
		mult = 0.75
	default:
		mult = 1.0
	}
	if regime == ADRSpikeWarning {
		mult *= 0.6
	}
	return mult
}

// AvgDailyVolume returns mean volume over available daily bars.
func AvgDailyVolume(bars []DailyBar) float64 {
	if len(bars) == 0 {
		return 0
	}
	var sum float64
	var n int
	for _, b := range bars {
		if b.Volume > 0 {
			sum += b.Volume
			n++
		}
	}
	if n == 0 {
		return 0
	}
	return sum / float64(n)
}

// BreakoutFlags detects new 20-day and 52-week highs from daily bars.
func BreakoutFlags(bars []DailyBar, spot float64) (new20d, new52w bool) {
	if spot <= 0 || len(bars) == 0 {
		return false, false
	}
	window20 := bars
	if len(window20) > 20 {
		window20 = window20[len(window20)-20:]
	}
	max20 := 0.0
	for _, b := range window20 {
		if b.High > max20 {
			max20 = b.High
		}
	}
	new20d = spot >= max20

	max52 := 0.0
	for _, b := range bars {
		if b.High > max52 {
			max52 = b.High
		}
	}
	new52w = spot >= max52
	return new20d, new52w
}

func mean(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	return sum / float64(len(vals))
}
