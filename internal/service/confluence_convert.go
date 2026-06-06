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
