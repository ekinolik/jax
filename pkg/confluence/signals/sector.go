package signals

import (
	"fmt"

	"github.com/ekinolik/jax/pkg/confluence"
)

const (
	sectorAlignedRelative   = -0.003
	sectorAgainstRelative   = -0.01
)

// SectorInput holds target and sector ETF day performance for relative strength.
type SectorInput struct {
	TargetTicker string
	TargetSpot   float64
	TargetOpen   float64
	ETFSpot      float64
	ETFOpen      float64
	SectorETF    string
}

// ComputeSector scores target vs resolved sector ETF day change.
func ComputeSector(in SectorInput) confluence.Signal {
	signal := confluence.Signal{
		Name:   "sector",
		Weight: 0.20,
		Icon:   confluence.IconSector,
	}

	if in.TargetOpen <= 0 || in.TargetSpot <= 0 || in.ETFOpen <= 0 || in.ETFSpot <= 0 {
		signal.Status = confluence.SignalNeutral
		signal.Detail = "sector data unavailable"
		return signal
	}

	targetChg := dayChangePct(in.TargetSpot, in.TargetOpen)
	etfChg := dayChangePct(in.ETFSpot, in.ETFOpen)
	relative := targetChg - etfChg

	ticker := in.TargetTicker
	if ticker == "" {
		ticker = "target"
	}
	signal.Detail = fmt.Sprintf("%s vs %s rel %+.2f%%", ticker, in.SectorETF, relative*100)

	switch {
	case relative >= 0 && etfChg >= marketFlatThreshold:
		signal.AxisFill = 1.0
		signal.Status = confluence.SignalAligned
	case relative >= sectorAlignedRelative:
		signal.AxisFill = 0.7
		signal.Status = confluence.SignalNeutral
	case relative >= sectorAgainstRelative:
		signal.AxisFill = 0.4
		signal.Status = confluence.SignalNeutral
	default:
		signal.AxisFill = 0.15
		signal.Status = confluence.SignalAgainst
	}

	signal.Score = signal.AxisFill * signal.Weight * 100
	return signal
}

// ResolveSectorETF maps ticker overview SIC fields to a sector benchmark ETF.
func ResolveSectorETF(sectors *confluence.SICSectors, sicCode, sicDescription string) string {
	if sectors == nil {
		return "IWM"
	}
	return sectors.ResolveSectorETF(sicCode, sicDescription)
}
