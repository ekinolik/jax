package signals

import (
	"fmt"
	"math"

	"github.com/ekinolik/jax/pkg/confluence"
)

const marketFlatThreshold = -0.002
const marketWeakThreshold = -0.005

// MarketInput holds SPY/QQQ spot and day-open prices for market strength.
type MarketInput struct {
	SPYOpen float64
	SPYSpot float64
	QQQOpen float64
	QQQSpot float64
	Weight  float64
}

// ComputeMarket scores broad market strength from SPY and QQQ day change vs open.
func ComputeMarket(in MarketInput) confluence.Signal {
	signal := confluence.Signal{
		Name:   "market",
		Weight: in.Weight,
		Icon:   confluence.IconMarket,
	}
	if signal.Weight == 0 {
		signal.Weight = 0.08
	}

	if in.SPYOpen <= 0 || in.SPYSpot <= 0 || in.QQQOpen <= 0 || in.QQQSpot <= 0 {
		signal.Status = confluence.SignalNeutral
		signal.Detail = "market data unavailable"
		return signal
	}

	spyFill := dayChangeFill(in.SPYSpot, in.SPYOpen)
	qqqFill := dayChangeFill(in.QQQSpot, in.QQQOpen)
	signal.AxisFill = (spyFill + qqqFill) / 2
	signal.Status = statusFromFill(signal.AxisFill)

	spyChg := dayChangePct(in.SPYSpot, in.SPYOpen)
	qqqChg := dayChangePct(in.QQQSpot, in.QQQOpen)
	signal.Detail = fmt.Sprintf("SPY %+.2f%% QQQ %+.2f%%", spyChg*100, qqqChg*100)

	if spyChg < marketWeakThreshold && qqqChg < marketWeakThreshold {
		signal.Status = confluence.SignalAgainst
		signal.AxisFill = math.Min(signal.AxisFill, 0.2)
	} else if spyChg >= marketFlatThreshold && qqqChg >= marketFlatThreshold {
		signal.Status = confluence.SignalAligned
	}

	signal.Score = signal.AxisFill * signal.Weight * 100
	return signal
}

func dayChangePct(spot, open float64) float64 {
	if open <= 0 || spot <= 0 {
		return 0
	}
	return (spot - open) / open
}

func dayChangeFill(spot, open float64) float64 {
	chg := dayChangePct(spot, open)
	switch {
	case chg >= 0.003:
		return 1.0
	case chg >= marketFlatThreshold:
		return 0.75
	case chg >= marketWeakThreshold:
		return 0.45
	default:
		return 0.15
	}
}
