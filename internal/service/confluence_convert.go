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
		Signals:            make([]*confluencev1.ConfluenceSignal, 0, len(snap.Signals)),
	}
	if !snap.SpotTime.IsZero() {
		out.SpotTimestamp = snap.SpotTime.Unix()
	}
	for _, sig := range snap.Signals {
		out.Signals = append(out.Signals, signalToProto(sig))
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
