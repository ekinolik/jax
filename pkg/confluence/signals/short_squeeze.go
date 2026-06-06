package signals

import (
	"fmt"
	"math"

	"github.com/ekinolik/jax/pkg/confluence"
)

// ShortSqueezeInput holds structural and trigger inputs for squeeze scoring.
type ShortSqueezeInput struct {
	ShortInterestPct float64
	DaysToCover      float64
	ShortVolumeRatio float64
	FloatShares      float64

	Regime         confluence.GammaRegime
	Spot           float64
	CallWall       float64
	RelativeVolume float64
	New20DayHigh   bool
	New52WeekHigh  bool
	GapUpPct       float64
}

// ShortSqueezeResult holds two-tier squeeze scores and active flag.
type ShortSqueezeResult struct {
	PressureScore      float64
	TriggerScore       float64
	CombinedScore      float64
	ShortSqueezeActive bool
}

// ComputeShortSqueeze derives pressure × trigger squeeze model.
func ComputeShortSqueeze(in ShortSqueezeInput) ShortSqueezeResult {
	pressure := shortPressureScore(in.ShortInterestPct, in.DaysToCover, in.ShortVolumeRatio, in.FloatShares)
	trigger := squeezeTriggerScore(in)
	combined := (pressure / 100) * (trigger / 100) * 100

	active := pressure >= 50 && trigger >= 60 && combined >= 35
	return ShortSqueezeResult{
		PressureScore:      pressure,
		TriggerScore:       trigger,
		CombinedScore:      combined,
		ShortSqueezeActive: active,
	}
}

func shortPressureScore(siPct, dtc, shortVolRatio, floatShares float64) float64 {
	siScore := tierScore(siPct, []tier{{5, 0}, {10, 25}, {20, 50}, {30, 75}, {math.MaxFloat64, 100}})
	if floatShares > 500_000_000 {
		siScore *= 0.85
	} else if floatShares > 0 && floatShares < 50_000_000 {
		siScore *= 1.1
	}
	dtcScore := tierScore(dtc, []tier{{1, 0}, {3, 40}, {5, 70}, {math.MaxFloat64, 100}})
	svScore := tierScore(shortVolRatio, []tier{{40, 0}, {50, 40}, {60, 65}, {70, 85}, {math.MaxFloat64, 100}})
	return 0.35*siScore + 0.25*dtcScore + 0.40*svScore
}

func squeezeTriggerScore(in ShortSqueezeInput) float64 {
	score := 0.0
	if in.Regime == confluence.GammaNegative {
		score += 25
	}
	if in.CallWall > 0 && in.Spot > in.CallWall {
		score += 25
	}
	switch {
	case in.RelativeVolume >= 3.0:
		score += 25
	case in.RelativeVolume >= 1.5:
		score += 15
	}
	if in.New52WeekHigh {
		score += 35
	} else if in.New20DayHigh || in.GapUpPct >= 0.05 {
		score += 25
	}
	return math.Min(100, score)
}

type tier struct {
	threshold float64
	score     float64
}

func tierScore(value float64, tiers []tier) float64 {
	if value <= 0 {
		return 0
	}
	for _, t := range tiers {
		if value < t.threshold {
			return t.score
		}
	}
	return tiers[len(tiers)-1].score
}

// ComputeShortSqueezeSignal maps combined squeeze score to buy axis.
func ComputeShortSqueezeSignal(result ShortSqueezeResult, weight float64) confluence.Signal {
	fill := result.CombinedScore / 100
	sig := confluence.Signal{
		Name:     "short_squeeze",
		Weight:   weight,
		AxisFill: fill,
		Icon:     confluence.IconSqueeze,
		Status:   statusFromFill(fill),
		Detail:   fmt.Sprintf("squeeze pressure %.0f trigger %.0f", result.PressureScore, result.TriggerScore),
	}
	sig.Score = sig.AxisFill * sig.Weight * 100
	return sig
}
