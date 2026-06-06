package signals

import (
	"fmt"

	"github.com/ekinolik/jax/pkg/confluence"
)

const rsiOversoldThreshold = 35

// RSIInput holds data for the RSI axis signal.
type RSIInput struct {
	RSI float64
}

// ComputeRSI maps RSI-14 to axis fill; oversold ≤35 is aligned for long bias.
func ComputeRSI(in RSIInput) confluence.Signal {
	signal := confluence.Signal{
		Name:   "rsi",
		Weight: 0.20,
		Icon:   confluence.IconRSI,
	}

	if in.RSI <= 0 {
		signal.Status = confluence.SignalNeutral
		signal.Detail = "RSI unavailable"
		return signal
	}

	signal.Detail = fmt.Sprintf("RSI-14 %.1f", in.RSI)
	switch {
	case in.RSI <= rsiOversoldThreshold:
		signal.AxisFill = 1.0 - (in.RSI/rsiOversoldThreshold)*0.25
		signal.Status = confluence.SignalAligned
	case in.RSI <= 45:
		signal.AxisFill = 0.6
		signal.Status = confluence.SignalNeutral
	case in.RSI <= 55:
		signal.AxisFill = 0.4
		signal.Status = confluence.SignalNeutral
	default:
		signal.AxisFill = 0.15
		signal.Status = confluence.SignalAgainst
	}

	signal.Score = signal.AxisFill * signal.Weight * 100
	return signal
}
