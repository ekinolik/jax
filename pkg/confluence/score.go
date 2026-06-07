package confluence

import "math"

const StackedZoneScoreBonus = 5.0

// CompositeScore sums weighted signal scores and applies stacked-zone bonus.
func CompositeScore(sigs []Signal, stackedZone bool) float64 {
	var total float64
	for _, s := range sigs {
		total += s.Score
	}
	if stackedZone {
		total += StackedZoneScoreBonus
	}
	return math.Max(0, math.Min(100, total))
}

// BandForScore maps a composite score to trade-readiness tiers.
func BandForScore(score float64) ReadinessBand {
	switch {
	case score >= 75:
		return ReadinessHighConviction
	case score >= 55:
		return ReadinessPossibleEntry
	case score >= 35:
		return ReadinessCaution
	default:
		return ReadinessNoTrade
	}
}

// CapReadiness limits readiness to at most the given ceiling band.
func CapReadiness(band ReadinessBand, ceiling ReadinessBand) ReadinessBand {
	order := map[ReadinessBand]int{
		ReadinessNoTrade:        0,
		ReadinessCaution:        1,
		ReadinessPossibleEntry:  2,
		ReadinessHighConviction: 3,
	}
	if order[band] > order[ceiling] {
		return ceiling
	}
	return band
}

func bandIntensity(band ReadinessBand) int {
	switch band {
	case ReadinessHighConviction:
		return 3
	case ReadinessPossibleEntry:
		return 2
	case ReadinessCaution:
		return 1
	default:
		return 0
	}
}

// BackgroundLevel returns UI background intensity for a readiness band.
func BackgroundLevel(band ReadinessBand) int { return bandIntensity(band) }

// HapticLevel returns haptic feedback intensity for a readiness band.
func HapticLevel(band ReadinessBand) int { return bandIntensity(band) }

// SuppressedSignal returns a neutral placeholder when OI-dependent signals are unavailable.
func SuppressedSignal(name string, weight float64, icon SignalIcon, detail string) Signal {
	return Signal{
		Name:     name,
		Weight:   weight,
		AxisFill: 0,
		Status:   SignalNeutral,
		Icon:     icon,
		Detail:   detail,
	}
}
