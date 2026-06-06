package signals

import (
	"math"

	"github.com/ekinolik/jax/pkg/confluence"
)

// GammaRegimeResult holds gamma regime and sub-scores for buy/sell paths.
type GammaRegimeResult struct {
	Regime            confluence.GammaRegime
	Strength          confluence.GammaStrength
	NetGEXAtSpot      float64
	CallWall          float64
	PutWall           float64
	EnvironmentScore  float64
	DirectionalScore  float64
	GammaSqueezeActive bool
}

// GammaRegimeInput holds inputs for gamma regime computation.
type GammaRegimeInput struct {
	Spot             float64
	Slices           []confluence.OptionSlice
	Levels           confluence.Levels
	SessionVWAP      float64
	RelativeVolume   float64
	TargetOpen       float64
	New20DayHigh     bool
	New52WeekHigh    bool
	GapUpPct         float64
	DistanceToEntry  confluence.EntryTiming
	NeutralBandPct   float64
}

// ComputeGammaRegime derives regime, walls, and environment/directional sub-scores.
func ComputeGammaRegime(in GammaRegimeInput) GammaRegimeResult {
	out := GammaRegimeResult{}
	if in.Spot <= 0 || len(in.Slices) == 0 {
		return out
	}

	exposures := buildExposures(in.Slices, in.Spot)
	out.NetGEXAtSpot = interpolateNetGEX(exposures, in.Spot)
	out.Regime, out.Strength = classifyGammaRegime(in.Levels.GammaFlip, out.NetGEXAtSpot, exposures, in.Spot, in.NeutralBandPct)

	if gexRes, ok := Rank1GEXResistance(in.Levels); ok {
		out.CallWall = gexRes.Price
	}
	if gexSup, ok := Rank1GEXSupport(in.Levels); ok {
		out.PutWall = gexSup.Price
	}

	out.EnvironmentScore = gammaEnvironmentScore(out.Regime, out.Strength, in.DistanceToEntry)
	out.DirectionalScore = gammaDirectionalScore(GammaDirectionalInput{
		Regime:         out.Regime,
		Spot:           in.Spot,
		CallWall:       out.CallWall,
		PutWall:        out.PutWall,
		SessionVWAP:    in.SessionVWAP,
		RelativeVolume: in.RelativeVolume,
		TargetOpen:     in.TargetOpen,
		New20DayHigh:   in.New20DayHigh,
	})

	out.GammaSqueezeActive = out.Regime == confluence.GammaNegative &&
		out.CallWall > 0 && in.Spot > out.CallWall &&
		in.SessionVWAP > 0 && in.Spot > in.SessionVWAP &&
		in.RelativeVolume >= 1.5

	return out
}

func interpolateNetGEX(exposures []strikeExposure, spot float64) float64 {
	if len(exposures) == 0 || spot <= 0 {
		return 0
	}
	if spot <= exposures[0].strike {
		return exposures[0].netGEX
	}
	last := exposures[len(exposures)-1]
	if spot >= last.strike {
		return last.netGEX
	}
	for i := 0; i < len(exposures)-1; i++ {
		a, b := exposures[i], exposures[i+1]
		if spot >= a.strike && spot <= b.strike {
			if b.strike == a.strike {
				return a.netGEX
			}
			ratio := (spot - a.strike) / (b.strike - a.strike)
			return a.netGEX + ratio*(b.netGEX-a.netGEX)
		}
	}
	return 0
}

func classifyGammaRegime(gammaFlip, netGEX float64, exposures []strikeExposure, spot, neutralBand float64) (confluence.GammaRegime, confluence.GammaStrength) {
	regime := confluence.GammaNeutral
	if gammaFlip > 0 {
		dist := math.Abs(spot-gammaFlip) / spot
		if dist <= neutralBand {
			regime = confluence.GammaNeutral
		} else if spot > gammaFlip {
			regime = confluence.GammaPositive
		} else {
			regime = confluence.GammaNegative
		}
	} else if netGEX > 0 {
		regime = confluence.GammaPositive
	} else if netGEX < 0 {
		regime = confluence.GammaNegative
	}

	maxGEX := 0.0
	for _, e := range exposures {
		abs := math.Abs(e.netGEX)
		if abs > maxGEX {
			maxGEX = abs
		}
	}
	ratio := 0.0
	if maxGEX > 0 {
		ratio = math.Abs(netGEX) / maxGEX
	}
	strength := confluence.GammaMild
	switch {
	case ratio >= 0.60:
		strength = confluence.GammaExtreme
	case ratio >= 0.25:
		strength = confluence.GammaModerate
	}
	return regime, strength
}

func gammaEnvironmentScore(regime confluence.GammaRegime, strength confluence.GammaStrength, entry confluence.EntryTiming) float64 {
	var raw float64
	switch regime {
	case confluence.GammaPositive:
		switch strength {
		case confluence.GammaModerate:
			raw = -1
		case confluence.GammaExtreme:
			raw = -3
		}
	case confluence.GammaNegative:
		switch strength {
		case confluence.GammaMild:
			raw = 2
		case confluence.GammaModerate:
			raw = 5
		case confluence.GammaExtreme:
			raw = 8
		}
	}
	if entry == confluence.EntryIdeal && regime == confluence.GammaPositive {
		raw = math.Min(0, raw+1)
	}
	return raw
}

// GammaDirectionalInput holds inputs for directional gamma scoring.
type GammaDirectionalInput struct {
	Regime         confluence.GammaRegime
	Spot           float64
	CallWall       float64
	PutWall        float64
	SessionVWAP    float64
	RelativeVolume float64
	TargetOpen     float64
	New20DayHigh   bool
}

func gammaDirectionalScore(in GammaDirectionalInput) float64 {
	raw := 0.0
	if in.Regime == confluence.GammaNegative {
		raw += 3
		if in.CallWall > 0 && in.Spot > in.CallWall {
			raw += 4
		}
		if in.SessionVWAP > 0 && in.Spot > in.SessionVWAP {
			raw += 3
		}
		if in.RelativeVolume >= 1.5 {
			raw += 3
		}
		if in.New20DayHigh {
			raw += 2
		}
	}

	// Bearish penalties for long bias
	if in.Regime == confluence.GammaNegative {
		if in.PutWall > 0 && in.Spot < in.PutWall {
			raw -= 5
		}
		if in.SessionVWAP > 0 && in.Spot < in.SessionVWAP {
			raw -= 3
		}
		if in.RelativeVolume >= 1.5 && in.TargetOpen > 0 && in.Spot < in.TargetOpen {
			raw -= 4
		}
	}

	return math.Max(-15, math.Min(15, raw))
}

// ComputeGammaEnvironmentSignal maps environment score to buy axis.
func ComputeGammaEnvironmentSignal(score float64, weight float64) confluence.Signal {
	fill := (score + 10) / 20
	if fill < 0 {
		fill = 0
	}
	if fill > 1 {
		fill = 1
	}
	sig := confluence.Signal{
		Name:     "gamma_environment",
		Weight:   weight,
		AxisFill: fill,
		Icon:     confluence.IconGammaEnv,
		Status:   statusFromFill(fill),
		Detail:   "gamma environment",
	}
	sig.Score = sig.AxisFill * sig.Weight * 100
	return sig
}

// ComputeGammaDirectionalSignal maps directional score to buy axis.
func ComputeGammaDirectionalSignal(score float64, weight float64) confluence.Signal {
	fill := (score + 15) / 30
	if fill < 0 {
		fill = 0
	}
	if fill > 1 {
		fill = 1
	}
	sig := confluence.Signal{
		Name:     "gamma_directional",
		Weight:   weight,
		AxisFill: fill,
		Icon:     confluence.IconGammaDir,
		Status:   statusFromFill(fill),
		Detail:   "gamma directional setup",
	}
	sig.Score = sig.AxisFill * sig.Weight * 100
	return sig
}
