package service

import (
	confluencev1 "github.com/ekinolik/jax/api/proto/confluence/v1"
	pkgconfluence "github.com/ekinolik/jax/pkg/confluence"
)

// SnapshotToProto converts an in-memory confluence snapshot to its gRPC representation.
func SnapshotToProto(snap *pkgconfluence.ConfluenceSnapshot) *confluencev1.ConfluenceSnapshot {
	if snap == nil {
		return nil
	}

	buySigs := snap.BuySignals
	if len(buySigs) == 0 {
		buySigs = snap.Signals
	}

	out := &confluencev1.ConfluenceSnapshot{
		Ticker:             snap.Ticker,
		Timestamp:          snap.UpdatedAt.Unix(),
		ConfluenceScore:    snap.Score,
		Readiness:          string(snap.ReadinessBand),
		OiStatus:           string(snap.OIStatus),
		MarketStatus:       string(snap.MarketStatus),
		DailyRangePosition: snap.RangePosition,
		DistanceToEntry:    string(snap.DistanceToEntry),
		HapticLevel:        int32(snap.HapticLevel),
		BackgroundLevel:    int32(snap.BackgroundLevel),
		Spot:               snap.Spot,
		Rsi:                snap.RSI,
		SectorEtf:          snap.SectorETF,
		StackedZone:        snap.StackedZone,
		Levels:             levelsToProto(snap.Levels),
		Signals:            signalsToProto(buySigs),
		BuySignals:         signalsToProto(buySigs),
		SellSignals:        signalsToProto(snap.SellSignals),
		SellScore:          snap.SellScore,
		SellReadiness:      string(snap.SellReadiness),
		ExitAction:         string(snap.ExitAction),
		DistanceToExit:     string(snap.DistanceToExit),
		RsiDaily:           snap.RSIDaily,
		AdrPct:             snap.ADRPct,
		Adr_30DPct:         snap.ADR30dPct,
		Adr_5DPct:          snap.ADR5dPct,
		AdrSpikeRatio:      snap.ADRSpikeRatio,
		AdrRegime:          string(snap.ADRRegime),
		UpsidePct:          snap.UpsidePct,
		DownsidePct:        snap.DownsidePct,
		RiskReward:         snap.RiskReward,
		UpsideBeyondResistancePct: snap.UpsideBeyondResistancePct,
		GammaRegime:               string(snap.GammaRegime),
		GammaRegimeStrength:       string(snap.GammaRegimeStrength),
		NetGexAtSpot:              snap.NetGEXAtSpot,
		CallWall:                  snap.CallWall,
		PutWall:                   snap.PutWall,
		SessionVwap:               snap.SessionVWAP,
		RelativeVolume:            snap.RelativeVolume,
		GammaSqueezeActive:        snap.GammaSqueezeActive,
		ShortSqueezeActive:        snap.ShortSqueezeActive,
		GammaEnvironmentScore:     snap.GammaEnvironmentScore,
		GammaDirectionalScore:     snap.GammaDirectionalScore,
		ShortSqueezeScore:         snap.ShortSqueezeScore,
		ShortPressureScore:        snap.ShortPressureScore,
		SqueezeTriggerScore:       snap.SqueezeTriggerScore,
		ShortInterestPct:          snap.ShortInterestPct,
		ShortVolumeRatio:          snap.ShortVolumeRatio,
		DaysToCover:               snap.DaysToCover,
		FloatShares:               snap.FloatShares,
		TradePlan:                 tradePlanToProto(snap.TradePlan),
	}
	if !snap.SpotTime.IsZero() {
		out.SpotTimestamp = snap.SpotTime.Unix()
	}
	if !snap.DataAsOf.IsZero() {
		out.DataAsOf = snap.DataAsOf.Unix()
	}
	return out
}

func signalsToProto(sigs []pkgconfluence.Signal) []*confluencev1.ConfluenceSignal {
	out := make([]*confluencev1.ConfluenceSignal, 0, len(sigs))
	for _, sig := range sigs {
		out = append(out, signalToProto(sig))
	}
	return out
}

func signalToProto(sig pkgconfluence.Signal) *confluencev1.ConfluenceSignal {
	return &confluencev1.ConfluenceSignal{
		Name:     sig.Name,
		Weight:   sig.Weight,
		AxisFill: sig.AxisFill,
		Status:   string(sig.Status),
		Icon:     string(sig.Icon),
		Score:    sig.Score,
		Detail:   sig.Detail,
	}
}

func levelsToProto(levels pkgconfluence.Levels) *confluencev1.ConfluenceLevels {
	out := &confluencev1.ConfluenceLevels{
		GammaFlip:            levels.GammaFlip,
		NearestSupport:       levels.NearestSupport,
		NearestResistance:    levels.NearestResistance,
		HasNearestSupport:    levels.HasNearestSupport,
		HasNearestResistance: levels.HasNearestResistance,
		Support:              make([]*confluencev1.ConfluenceLevel, 0, len(levels.Support)),
		Resistance:           make([]*confluencev1.ConfluenceLevel, 0, len(levels.Resistance)),
	}
	for _, lvl := range levels.Support {
		out.Support = append(out.Support, levelToProto(lvl))
	}
	for _, lvl := range levels.Resistance {
		out.Resistance = append(out.Resistance, levelToProto(lvl))
	}
	return out
}

func levelToProto(lvl pkgconfluence.Level) *confluencev1.ConfluenceLevel {
	return &confluencev1.ConfluenceLevel{
		Price:      lvl.Price,
		Source:     string(lvl.Source),
		Strength:   lvl.Strength,
		Rank:       int32(lvl.Rank),
		Expiration: lvl.Expiration,
	}
}

// SummaryToProto converts an in-memory confluence summary to its gRPC representation.
func SummaryToProto(sum pkgconfluence.ConfluenceSummary) *confluencev1.ConfluenceSummary {
	rsiMinute := sum.Context.RSIMinute.Value
	rsiUnavailable := sum.Context.RSIMinute.Unavailable

	return &confluencev1.ConfluenceSummary{
		Ticker: sum.Ticker,
		Spot:   sum.Spot,
		AsOf:   sum.AsOf,
		Market: sum.Market,
		Verdict: &confluencev1.ConfluenceSummaryVerdict{
			Buy: &confluencev1.ConfluenceSummaryBuyVerdict{
				Score:     int32(sum.Verdict.Buy.Score),
				Readiness: sum.Verdict.Buy.Readiness,
				Label:     sum.Verdict.Buy.Label,
			},
			Sell: &confluencev1.ConfluenceSummarySellVerdict{
				Score:     int32(sum.Verdict.Sell.Score),
				Readiness: sum.Verdict.Sell.Readiness,
				Action:    sum.Verdict.Sell.Action,
				Label:     sum.Verdict.Sell.Label,
			},
		},
		TradeSetup: &confluencev1.ConfluenceSummaryTradeSetup{
			Archetype:   sum.TradeSetup.Archetype,
			EntryTiming: sum.TradeSetup.EntryTiming,
			ExitTiming:  sum.TradeSetup.ExitTiming,
			UpsidePct:   sum.TradeSetup.UpsidePct,
			DownsidePct: sum.TradeSetup.DownsidePct,
			RiskReward:  sum.TradeSetup.RiskReward,
		},
		Context: &confluencev1.ConfluenceSummaryContext{
			AdrRegime:             sum.Context.ADRRegime,
			GammaRegime:           sum.Context.GammaRegime,
			GammaSqueeze:          sum.Context.GammaSqueeze,
			ShortSqueeze:          sum.Context.ShortSqueeze,
			RsiMinuteUnavailable:  rsiUnavailable,
			RsiMinute:             rsiMinute,
			RsiDaily:              sum.Context.RSIDaily,
		},
		Levels: &confluencev1.ConfluenceSummaryLevels{
			Support:    sum.Levels.Support,
			Resistance: sum.Levels.Resistance,
			CallWall:   sum.Levels.CallWall,
			PutWall:    sum.Levels.PutWall,
		},
		Reasons:  sum.Reasons,
		Warnings: sum.Warnings,
		Gates: &confluencev1.ConfluenceSummaryGates{
			UpsideOk:                  sum.Gates.UpsideOK,
			AdrOk:                     sum.Gates.ADROK,
			BlockedFromHighConviction: sum.Gates.BlockedFromHighConviction,
		},
		TradePlan: summaryTradePlanToProto(sum.TradePlan),
	}
}

func summaryTradePlanToProto(plan *pkgconfluence.SummaryTradePlan) *confluencev1.TradePlan {
	if plan == nil {
		return nil
	}
	out := tradePlanToProto(plan.BasePlan())
	if plan.Invalidation != nil {
		inv := &confluencev1.PlanInvalidation{}
		if plan.Invalidation.Trade != nil {
			inv.Trade = invalidationPointToProto(*plan.Invalidation.Trade)
		}
		if plan.Invalidation.Structure != nil {
			inv.Structure = invalidationPointToProto(*plan.Invalidation.Structure)
		}
		out.Invalidation = inv
	}
	if plan.PrimaryExit != nil {
		out.PrimaryExit = &confluencev1.PlanPrimaryExit{
			Price:    plan.PrimaryExit.Price,
			Label:    plan.PrimaryExit.Label,
			Emphasis: plan.PrimaryExit.Emphasis,
		}
	}
	return out
}

func invalidationPointToProto(pt pkgconfluence.InvalidationPoint) *confluencev1.InvalidationPoint {
	return &confluencev1.InvalidationPoint{
		Price:   pt.Price,
		Label:   pt.Label,
		Meaning: pt.Meaning,
	}
}

func tradePlanToProto(plan *pkgconfluence.TradePlan) *confluencev1.TradePlan {
	if plan == nil {
		return nil
	}
	out := &confluencev1.TradePlan{
		EntryZone: &confluencev1.EntryZone{
			Price:               plan.EntryZone.Price,
			Source:              plan.EntryZone.Source,
			Timing:              plan.EntryZone.Timing,
			DistanceFromSpotPct: plan.EntryZone.DistanceFromSpotPct,
			Note:                plan.EntryZone.Note,
		},
		ExitInsteadOfAddBelow:      plan.ExitInsteadOfAddBelow,
		GexDexGapPct:                 plan.GEXDEXGapPct,
		IntradayNotes:                plan.IntradayNotes,
		TradeInvalidationPrice:       plan.TradeInvalidationPrice,
		StructureInvalidationPrice:   plan.StructureInvalidationPrice,
		PrimaryExitPrice:             plan.PrimaryExitPrice,
		SpotContext: &confluencev1.SpotContext{
			VsEntryZone: plan.SpotContext.VsEntryZone,
			VsSoftStop:  plan.SpotContext.VsSoftStop,
			VsHardStop:  plan.SpotContext.VsHardStop,
			Guidance:    plan.SpotContext.Guidance,
		},
	}
	for _, s := range plan.Stops {
		out.Stops = append(out.Stops, &confluencev1.StopLevel{
			Tier:    s.Tier,
			Price:   s.Price,
			Source:  s.Source,
			Rule:    s.Rule,
			Action:  s.Action,
			Label:   s.HumanLabel,
			Meaning: s.Meaning,
		})
	}
	for _, a := range plan.AverageDown {
		out.AverageDown = append(out.AverageDown, &confluencev1.AddLevel{
			Tier:      a.Tier,
			Price:     a.Price,
			SizeHint:  a.SizeHint,
			Condition: a.Condition,
			IfBelow:   a.IfBelow,
		})
	}
	return out
}
