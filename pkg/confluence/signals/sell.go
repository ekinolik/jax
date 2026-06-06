package signals

import (
	"fmt"
	"math"

	"github.com/ekinolik/jax/pkg/confluence"
)

// SellWeights holds sell signal axis weights (sum = 1.0).
type SellWeights struct {
	ResistanceProximity   float64
	RSIMinuteExit         float64
	RSIDailyExit          float64
	UpsideExhausted       float64
	MarketSectorExtended  float64
}

// DefaultSellWeights returns sell path weight defaults.
func DefaultSellWeights() SellWeights {
	return SellWeights{
		ResistanceProximity:  0.30,
		RSIMinuteExit:        0.25,
		RSIDailyExit:         0.10,
		UpsideExhausted:      0.20,
		MarketSectorExtended: 0.15,
	}
}

// SellInput holds inputs for the sell score path.
type SellInput struct {
	Spot           float64
	Levels         confluence.Levels
	RSI            float64
	RSIDaily       float64
	Geometry       confluence.TradeGeometry
	SPYOpen        float64
	SPYSpot        float64
	QQQOpen        float64
	QQQSpot        float64
	TargetOpen     float64
	ETFOpen        float64
	ETFSpot        float64

	GammaSqueezeActive    bool
	GammaDirectionalScore float64
	SpotBelowVWAP         bool
	SpotBelowPutWall      bool
	RelativeVolume        float64
	ShortSqueezeActive    bool

	Weights SellWeights
}

// SellResult holds sell score, signals, readiness, and exit action.
type SellResult struct {
	Score                 float64
	Readiness             confluence.SellReadinessBand
	ExitAction            confluence.ExitAction
	DistanceToExit        confluence.ExitTiming
	UpsideBeyondResPct    float64
	Signals               []confluence.Signal
}

// BuildSellResult computes sell score and exit guidance for long exits.
func BuildSellResult(in SellInput) SellResult {
	w := in.Weights
	if w.ResistanceProximity == 0 {
		w = DefaultSellWeights()
	}

	var sigs []confluence.Signal
	sigs = append(sigs, sellResistanceProximity(in, w.ResistanceProximity))
	sigs = append(sigs, sellRSIMinute(in.RSI, w.RSIMinuteExit))
	sigs = append(sigs, sellRSIDaily(in.RSIDaily, w.RSIDailyExit))
	sigs = append(sigs, sellUpsideExhausted(in.Geometry, w.UpsideExhausted))
	sigs = append(sigs, sellMarketSectorExtended(in, w.MarketSectorExtended))

	score := 0.0
	for _, s := range sigs {
		score += s.Score
	}

	// Squeeze unwind overlays
	if in.GammaSqueezeActive && in.SpotBelowVWAP {
		score += 10
	}
	if in.GammaDirectionalScore <= -8 && in.SpotBelowPutWall {
		score += 8
	}
	if in.ShortSqueezeActive && in.RelativeVolume > 0 && in.RelativeVolume < 2 {
		score += 5
	}
	score = math.Max(0, math.Min(100, score))

	distExit := ComputeDistanceToExit(ExitInput{Spot: in.Spot, Levels: in.Levels})
	beyondPct := UpsideBeyondResistancePct(in.Levels)
	exitAction := resolveExitAction(score, distExit, in.Levels, in.RSI, beyondPct)
	readiness := sellReadinessBand(score, exitAction)

	return SellResult{
		Score:              score,
		Readiness:          readiness,
		ExitAction:         exitAction,
		DistanceToExit:     distExit,
		UpsideBeyondResPct: beyondPct,
		Signals:            sigs,
	}
}

func sellResistanceProximity(in SellInput, weight float64) confluence.Signal {
	sig := confluence.Signal{Name: "resistance_proximity", Weight: weight, Icon: confluence.IconSell}
	res, ok := Rank1Resistance(in.Levels)
	if !ok || in.Spot <= 0 {
		sig.Detail = "no resistance"
		return sig
	}
	distPct := (res.Price - in.Spot) / in.Spot
	if distPct < 0 {
		sig.AxisFill = 1.0
		sig.Status = confluence.SignalAligned
		sig.Detail = "through resistance"
	} else {
		sig.AxisFill = math.Max(0, 1.0-distPct/0.03)
		sig.Status = statusFromFill(sig.AxisFill)
		sig.Detail = fmt.Sprintf("%.1f%% below rank-1 res %.2f", distPct*100, res.Price)
	}
	sig.Score = sig.AxisFill * weight * 100
	return sig
}

func sellRSIMinute(rsi, weight float64) confluence.Signal {
	sig := confluence.Signal{Name: "rsi_minute_exit", Weight: weight, Icon: confluence.IconRSI}
	if rsi <= 0 {
		sig.Detail = "RSI unavailable"
		return sig
	}
	if !validRSI(rsi) {
		sig.Detail = "RSI out of range"
		return sig
	}
	switch {
	case rsi >= 70:
		sig.AxisFill = 1.0
	case rsi >= 65:
		sig.AxisFill = 0.85
	case rsi >= 60:
		sig.AxisFill = 0.6
	default:
		sig.AxisFill = 0.2
	}
	sig.Status = statusFromFill(sig.AxisFill)
	sig.Detail = fmt.Sprintf("RSI-14 %.1f", rsi)
	sig.Score = sig.AxisFill * weight * 100
	return sig
}

func sellRSIDaily(rsi, weight float64) confluence.Signal {
	sig := confluence.Signal{Name: "rsi_daily_exit", Weight: weight, Icon: confluence.IconRSI}
	if rsi <= 0 {
		sig.Detail = "daily RSI unavailable"
		return sig
	}
	if !validRSI(rsi) {
		sig.Detail = "RSI out of range"
		return sig
	}
	if rsi > 60 {
		sig.AxisFill = 0.7 + math.Min(0.3, (rsi-60)/20)
	} else {
		sig.AxisFill = 0.2
	}
	sig.Status = statusFromFill(sig.AxisFill)
	sig.Detail = fmt.Sprintf("RSI daily %.1f", rsi)
	sig.Score = sig.AxisFill * weight * 100
	return sig
}

func sellUpsideExhausted(geo confluence.TradeGeometry, weight float64) confluence.Signal {
	sig := confluence.Signal{Name: "upside_exhausted", Weight: weight, Icon: confluence.IconUpside}
	if !geo.HasUpside {
		sig.AxisFill = 0.8
	} else if geo.UpsidePct < 0.01 {
		sig.AxisFill = 1.0
	} else if geo.UpsidePct < 0.02 {
		sig.AxisFill = 0.75
	} else {
		sig.AxisFill = math.Max(0, 1.0-geo.UpsidePct/0.06)
	}
	sig.Status = statusFromFill(sig.AxisFill)
	sig.Detail = fmt.Sprintf("remaining upside %.1f%%", geo.UpsidePct*100)
	sig.Score = sig.AxisFill * weight * 100
	return sig
}

func sellMarketSectorExtended(in SellInput, weight float64) confluence.Signal {
	sig := confluence.Signal{Name: "market_sector_extended", Weight: weight, Icon: confluence.IconMarket}
	spyFill := dayChangeFill(in.SPYSpot, in.SPYOpen)
	qqqFill := dayChangeFill(in.QQQSpot, in.QQQOpen)
	etfFill := dayChangeFill(in.ETFSpot, in.ETFOpen)
	sig.AxisFill = (spyFill + qqqFill + etfFill) / 3
	sig.Status = statusFromFill(sig.AxisFill)
	sig.Detail = "selling into market/sector strength"
	sig.Score = sig.AxisFill * weight * 100
	return sig
}

func resolveExitAction(score float64, dist confluence.ExitTiming, levels confluence.Levels, rsi, beyondPct float64) confluence.ExitAction {
	if score >= 75 && rsi > 70 {
		return confluence.ExitSellAll
	}
	r1, ok1 := Rank1Resistance(levels)
	if !ok1 {
		return confluence.ExitHold
	}
	if score < 40 {
		return confluence.ExitHold
	}
	if dist == confluence.ExitLate {
		return confluence.ExitHold
	}
	r2, ok2 := Rank2Resistance(levels)
	if ok2 && beyondPct >= 0.03 && r2.Strength >= 0.5 {
		return confluence.ExitTrim
	}
	if !ok2 || beyondPct < 0.03 || (ok2 && r2.Strength < 0.5) {
		return confluence.ExitSellAll
	}
	_ = r1
	return confluence.ExitHold
}

func sellReadinessBand(score float64, action confluence.ExitAction) confluence.SellReadinessBand {
	switch {
	case score >= 75 || action == confluence.ExitSellAll:
		return confluence.SellTakeProfit
	case score >= 55 && action == confluence.ExitTrim:
		return confluence.SellConsiderTrim
	case score >= 40:
		return confluence.SellWatch
	default:
		return confluence.SellHold
	}
}
