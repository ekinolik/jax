package signals

import (
	"math"
)

// RangeInput holds intraday high/low for daily range position.
type RangeInput struct {
	Spot float64
	High float64
	Low  float64
}

// ComputeRangePosition returns where spot sits in the daily range (0=low, 1=high).
func ComputeRangePosition(in RangeInput) float64 {
	if in.High <= in.Low || in.Spot <= 0 {
		return 0.5
	}
	pos := (in.Spot - in.Low) / (in.High - in.Low)
	return math.Max(0, math.Min(1, pos))
}
