package signals

import (
	"time"

	"github.com/ekinolik/jax/pkg/confluence"
)

// BuildSnapshot computes levels, signals, and composite score from available inputs.
func BuildSnapshot(in confluence.ScoreInput) confluence.ConfluenceSnapshot {
	now := in.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	snap := confluence.ConfluenceSnapshot{
		Ticker:       confluence.NormalizeTicker(in.Ticker),
		Spot:         in.Spot,
		SpotTime:     in.SpotTime,
		Slices:       in.Slices,
		OIStatus:     in.OIStatus,
		MarketStatus: in.MarketStatus,
		UpdatedAt:    now,
		RSI:          in.RSI,
		SectorETF:    in.SectorETF,
	}

	levelsAvailable := in.OIStatus == confluence.OIStatusReady && len(in.Slices) > 0 && in.Spot > 0
	if levelsAvailable {
		snap.Levels = ComputeLevels(in.Slices, in.Spot)
		snap.StackedZone = HasStackedZone(snap.Levels, in.Spot)
		snap.DistanceToEntry = ComputeDistanceToEntry(EntryInput{
			Spot:   in.Spot,
			Levels: snap.Levels,
		})
	}

	if in.IntradayHigh > in.IntradayLow && in.Spot > 0 {
		snap.RangePosition = ComputeRangePosition(RangeInput{
			Spot: in.Spot,
			High: in.IntradayHigh,
			Low:  in.IntradayLow,
		})
	}

	var sigs []confluence.Signal
	if levelsAvailable {
		sigs = append(sigs, ComputeGammaSupport(GammaInput{
			Spot:   in.Spot,
			Levels: snap.Levels,
		}))
		sigs = append(sigs, ComputeDeltaSupport(DeltaInput{
			Spot:   in.Spot,
			Levels: snap.Levels,
		}))
	} else {
		sigs = append(sigs, confluence.SuppressedSignal("gamma_support", 0.25, confluence.IconGamma, "OI loading"))
		sigs = append(sigs, confluence.SuppressedSignal("delta_support", 0.15, confluence.IconDelta, "OI loading"))
	}

	sigs = append(sigs, ComputeRSI(RSIInput{RSI: in.RSI}))
	sigs = append(sigs, ComputeMarket(MarketInput{
		SPYOpen: in.SPYOpen,
		SPYSpot: in.SPYSpot,
		QQQOpen: in.QQQOpen,
		QQQSpot: in.QQQSpot,
	}))
	sigs = append(sigs, ComputeSector(SectorInput{
		TargetTicker: in.Ticker,
		TargetSpot:   in.Spot,
		TargetOpen:   in.TargetOpen,
		ETFSpot:      in.ETFSpot,
		ETFOpen:      in.ETFOpen,
		SectorETF:    in.SectorETF,
	}))

	snap.Signals = sigs
	snap.Score = confluence.CompositeScore(sigs, snap.StackedZone)
	snap.ReadinessBand = confluence.BandForScore(snap.Score)
	snap.BackgroundLevel = confluence.BackgroundLevel(snap.ReadinessBand)
	snap.HapticLevel = confluence.HapticLevel(snap.ReadinessBand)

	return snap
}
