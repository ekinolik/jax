package signals

import (
	"math"
	"time"

	"github.com/ekinolik/jax/pkg/confluence"
)

// BuildSnapshot computes levels, signals, and composite score from available inputs.
func BuildSnapshot(in confluence.ScoreInput) confluence.ConfluenceSnapshot {
	now := in.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	settings := confluence.EffectiveSettings(in.Settings)
	weights := settings.SignalWeights
	scoring := settings.Scoring

	snap := confluence.ConfluenceSnapshot{
		Ticker:       confluence.NormalizeTicker(in.Ticker),
		Spot:         in.Spot,
		SpotTime:     in.SpotTime,
		Slices:       in.Slices,
		OIStatus:     in.OIStatus,
		MarketStatus: in.MarketStatus,
		UpdatedAt:    now,
		DataAsOf:     in.DataAsOf,
		RSI:          in.RSI,
		RSIDaily:     in.RSIDaily,
		SectorETF:    in.SectorETF,
		SessionVWAP:  in.SessionVWAP,
		RelativeVolume: in.RelativeVolume,
		ShortInterestPct: in.ShortInterestPct,
		ShortVolumeRatio: in.ShortVolumeRatio,
		DaysToCover:      in.DaysToCover,
		FloatShares:      in.FloatShares,
	}

	if in.ADR.HasData {
		snap.ADR30dPct = in.ADR.ADR30dPct
		snap.ADR5dPct = in.ADR.ADR5dPct
		snap.ADRSpikeRatio = in.ADR.SpikeRatio
		snap.ADRRegime = in.ADR.Regime
		snap.ADRPct = in.ADR.ADR30dPct
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

	geo := TradeGeometry(in.Spot, snap.Levels)
	snap.UpsidePct = geo.UpsidePct
	snap.DownsidePct = geo.DownsidePct
	snap.RiskReward = geo.RiskReward

	if in.IntradayHigh > in.IntradayLow && in.Spot > 0 {
		snap.RangePosition = ComputeRangePosition(RangeInput{
			Spot: in.Spot,
			High: in.IntradayHigh,
			Low:  in.IntradayLow,
		})
	}

	var gammaResult GammaRegimeResult
	if levelsAvailable {
		gammaResult = ComputeGammaRegime(GammaRegimeInput{
			Spot:            in.Spot,
			Slices:          in.Slices,
			Levels:          snap.Levels,
			SessionVWAP:     in.SessionVWAP,
			RelativeVolume:  in.RelativeVolume,
			TargetOpen:      in.TargetOpen,
			New20DayHigh:    in.New20DayHigh,
			New52WeekHigh:   in.New52WeekHigh,
			GapUpPct:        in.GapUpPct,
			DistanceToEntry: snap.DistanceToEntry,
			NeutralBandPct:  scoring.NeutralGammaBandPct,
		})
		snap.GammaRegime = gammaResult.Regime
		snap.GammaRegimeStrength = gammaResult.Strength
		snap.NetGEXAtSpot = gammaResult.NetGEXAtSpot
		snap.CallWall = gammaResult.CallWall
		snap.PutWall = gammaResult.PutWall
		snap.GammaEnvironmentScore = gammaResult.EnvironmentScore
		snap.GammaDirectionalScore = gammaResult.DirectionalScore
		snap.GammaSqueezeActive = gammaResult.GammaSqueezeActive
	}

	squeezeResult := ComputeShortSqueeze(ShortSqueezeInput{
		ShortInterestPct: in.ShortInterestPct,
		DaysToCover:      in.DaysToCover,
		ShortVolumeRatio: in.ShortVolumeRatio,
		FloatShares:      in.FloatShares,
		Regime:           snap.GammaRegime,
		Spot:             in.Spot,
		CallWall:         snap.CallWall,
		RelativeVolume:   in.RelativeVolume,
		New20DayHigh:     in.New20DayHigh,
		New52WeekHigh:    in.New52WeekHigh,
		GapUpPct:         in.GapUpPct,
	})
	snap.ShortPressureScore = squeezeResult.PressureScore
	snap.SqueezeTriggerScore = squeezeResult.TriggerScore
	snap.ShortSqueezeScore = squeezeResult.CombinedScore
	snap.ShortSqueezeActive = squeezeResult.ShortSqueezeActive

	var buySigs []confluence.Signal
	if levelsAvailable {
		buySigs = append(buySigs, ComputeGammaSupport(GammaInput{
			Spot:   in.Spot,
			Levels: snap.Levels,
			Weight: weights.GammaSupport,
		}))
		buySigs = append(buySigs, ComputeDeltaSupport(DeltaInput{
			Spot:   in.Spot,
			Levels: snap.Levels,
			Weight: weights.DeltaSupport,
		}))
	} else {
		buySigs = append(buySigs, confluence.SuppressedSignal("gamma_support", weights.GammaSupport, confluence.IconGamma, "OI loading"))
		buySigs = append(buySigs, confluence.SuppressedSignal("delta_support", weights.DeltaSupport, confluence.IconDelta, "OI loading"))
	}

	buySigs = append(buySigs, ComputeRSI(RSIInput{RSI: in.RSI, Weight: weights.RSIMinute}))
	buySigs = append(buySigs, ComputeRSI(RSIInput{RSI: in.RSIDaily, Weight: weights.RSIDaily, Daily: true}))
	buySigs = append(buySigs, ComputeMarket(MarketInput{
		SPYOpen: in.SPYOpen,
		SPYSpot: in.SPYSpot,
		QQQOpen: in.QQQOpen,
		QQQSpot: in.QQQSpot,
		Weight:  weights.Market,
	}))
	buySigs = append(buySigs, ComputeSector(SectorInput{
		TargetTicker: in.Ticker,
		TargetSpot:   in.Spot,
		TargetOpen:   in.TargetOpen,
		ETFSpot:      in.ETFSpot,
		ETFOpen:      in.ETFOpen,
		SectorETF:    in.SectorETF,
		Weight:       weights.Sector,
	}))

	if geo.HasUpside {
		buySigs = append(buySigs, ComputeUpside(UpsideInput{
			Geometry: geo,
			ADR:      in.ADR,
			Scoring:  scoring,
			Weight:   weights.Upside,
		}))
	} else {
		buySigs = append(buySigs, confluence.SuppressedSignal("upside", weights.Upside, confluence.IconUpside, "no upside data"))
	}

	if geo.HasDownside {
		buySigs = append(buySigs, ComputeDownside(DownsideInput{
			Geometry: geo,
			Scoring:  scoring,
			Weight:   weights.Downside,
		}))
	}

	if in.ADR.HasData {
		buySigs = append(buySigs, ComputeADRSignal(ADRInput{Metrics: in.ADR, Weight: weights.ADR}))
	} else {
		buySigs = append(buySigs, confluence.SuppressedSignal("adr", weights.ADR, confluence.IconADR, "ADR unavailable"))
	}

	if levelsAvailable {
		buySigs = append(buySigs, ComputeGammaEnvironmentSignal(gammaResult.EnvironmentScore, weights.GammaEnvironment))
		buySigs = append(buySigs, ComputeGammaDirectionalSignal(gammaResult.DirectionalScore, weights.GammaDirectional))
	} else {
		buySigs = append(buySigs, confluence.SuppressedSignal("gamma_environment", weights.GammaEnvironment, confluence.IconGammaEnv, "OI loading"))
		buySigs = append(buySigs, confluence.SuppressedSignal("gamma_directional", weights.GammaDirectional, confluence.IconGammaDir, "OI loading"))
	}

	buySigs = append(buySigs, ComputeShortSqueezeSignal(squeezeResult, weights.ShortSqueeze))

	snap.BuySignals = buySigs
	snap.Signals = buySigs // backward compat

	score := confluence.CompositeScore(buySigs, snap.StackedZone)
	if snap.GammaSqueezeActive && snap.ShortSqueezeActive {
		score = math.Min(100, score+scoring.CompoundSqueezeBonus)
	}
	snap.Score = score
	snap.ReadinessBand = confluence.BandForScore(snap.Score)

	// Buy gates
	if geo.HasUpside && geo.UpsidePct < scoring.MinUpsidePct {
		snap.ReadinessBand = confluence.CapReadiness(snap.ReadinessBand, confluence.ReadinessCaution)
	}
	if in.ADR.HasData && in.ADR.Regime == confluence.ADRSpikeWarning {
		snap.ReadinessBand = confluence.CapReadiness(snap.ReadinessBand, confluence.ReadinessCaution)
	}
	if snap.GammaDirectionalScore <= scoring.GammaDirectionalBuyCap {
		snap.ReadinessBand = confluence.CapReadiness(snap.ReadinessBand, confluence.ReadinessCaution)
	}

	snap.BackgroundLevel = confluence.BackgroundLevel(snap.ReadinessBand)
	snap.HapticLevel = confluence.HapticLevel(snap.ReadinessBand)

	spotBelowVWAP := in.SessionVWAP > 0 && in.Spot < in.SessionVWAP
	spotBelowPut := snap.PutWall > 0 && in.Spot < snap.PutWall
	sellResult := BuildSellResult(SellInput{
		Spot:                  in.Spot,
		Levels:                snap.Levels,
		RSI:                   in.RSI,
		RSIDaily:              in.RSIDaily,
		Geometry:              geo,
		SPYOpen:               in.SPYOpen,
		SPYSpot:               in.SPYSpot,
		QQQOpen:               in.QQQOpen,
		QQQSpot:               in.QQQSpot,
		TargetOpen:            in.TargetOpen,
		ETFOpen:               in.ETFOpen,
		ETFSpot:               in.ETFSpot,
		GammaSqueezeActive:    snap.GammaSqueezeActive,
		GammaDirectionalScore: snap.GammaDirectionalScore,
		SpotBelowVWAP:         spotBelowVWAP,
		SpotBelowPutWall:      spotBelowPut,
		RelativeVolume:        in.RelativeVolume,
		ShortSqueezeActive:    snap.ShortSqueezeActive,
	})
	snap.SellSignals = sellResult.Signals
	snap.SellScore = sellResult.Score
	snap.SellReadiness = sellResult.Readiness
	snap.ExitAction = sellResult.ExitAction
	snap.DistanceToExit = sellResult.DistanceToExit
	snap.UpsideBeyondResistancePct = sellResult.UpsideBeyondResPct

	return snap
}
