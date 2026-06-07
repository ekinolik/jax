package signals

import (
	"fmt"

	"github.com/ekinolik/jax/pkg/confluence"
)

const (
	rsiOversoldThreshold = 35
	rsiMinValid          = 1.0
	rsiMaxValid          = 99.0
)

func validRSI(rsi float64) bool {
	return rsi >= rsiMinValid && rsi <= rsiMaxValid
}

// RSIInput holds data for the RSI axis signal.
type RSIInput struct {
	RSI    float64
	Weight float64
	Daily  bool
}

// ComputeRSI maps RSI-14 to axis fill; oversold ≤35 is aligned for long bias.
// Name is rsi_minute or rsi_daily based on Daily flag.
func ComputeRSI(in RSIInput) confluence.Signal {
	name := "rsi_minute"
	if in.Daily {
		name = "rsi_daily"
	}
	signal := confluence.Signal{
		Name:   name,
		Weight: in.Weight,
		Icon:   confluence.IconRSI,
	}
	if signal.Weight == 0 {
		if in.Daily {
			signal.Weight = 0.03
		} else {
			signal.Weight = 0.12
		}
	}

	if in.RSI <= 0 {
		signal.Status = confluence.SignalNeutral
		signal.Detail = "RSI unavailable"
		return signal
	}
	if !validRSI(in.RSI) {
		signal.Status = confluence.SignalNeutral
		signal.Detail = "RSI out of range"
		return signal
	}

	span := "minute"
	if in.Daily {
		span = "daily"
	}
	signal.Detail = fmt.Sprintf("RSI-14 %s %.1f", span, in.RSI)
	signal.AxisFill = rsiAxisFill(in.RSI, in.Daily)
	signal.Status = statusFromFill(signal.AxisFill)
	signal.Score = signal.AxisFill * signal.Weight * 100
	return signal
}

func rsiAxisFill(rsi float64, daily bool) float64 {
	if daily {
		switch {
		case rsi <= rsiOversoldThreshold:
			return 0.75 - (rsi/rsiOversoldThreshold)*0.15
		case rsi <= 45:
			return 0.5
		case rsi <= 55:
			return 0.35
		default:
			return 0.15
		}
	}
	switch {
	case rsi <= rsiOversoldThreshold:
		return 1.0 - (rsi/rsiOversoldThreshold)*0.25
	case rsi <= 45:
		return 0.6
	case rsi <= 55:
		return 0.4
	default:
		return 0.15
	}
}
