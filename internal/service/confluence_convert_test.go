package service

import (
	"testing"
	"time"

	pkgconfluence "github.com/ekinolik/jax/pkg/confluence"
)

func TestSnapshotToProto_roundTripKeyFields(t *testing.T) {
	updatedAt := time.Date(2026, 6, 6, 14, 30, 0, 0, time.UTC)
	spotTime := updatedAt.Add(-2 * time.Second)

	snap := &pkgconfluence.ConfluenceSnapshot{
		Ticker:          "NVDA",
		Spot:            120.5,
		SpotTime:        spotTime,
		OIStatus:        pkgconfluence.OIStatusReady,
		MarketStatus:    pkgconfluence.MarketStatusOpen,
		UpdatedAt:       updatedAt,
		Score:           72.5,
		ReadinessBand:   pkgconfluence.ReadinessPossibleEntry,
		BackgroundLevel: 2,
		HapticLevel:     2,
		StackedZone:     true,
		SectorETF:       "SMH",
		RangePosition:   0.42,
		DistanceToEntry: pkgconfluence.EntryIdeal,
		RSI:             31.2,
		Levels: pkgconfluence.Levels{
			GammaFlip:            118.0,
			NearestSupport:       118.5,
			NearestResistance:    125.0,
			HasNearestSupport:    true,
			HasNearestResistance: true,
			Support: []pkgconfluence.Level{
				{Price: 118.5, Source: pkgconfluence.LevelSourceGEX, Strength: 0.82, Rank: 1, Expiration: "2026-06-06"},
			},
			Resistance: []pkgconfluence.Level{
				{Price: 125.0, Source: pkgconfluence.LevelSourceGEX, Strength: 0.71, Rank: 1, Expiration: "2026-06-06"},
			},
		},
		Signals: []pkgconfluence.Signal{
			{Name: "Gamma support", Weight: 0.25, AxisFill: 0.8, Status: pkgconfluence.SignalAligned, Icon: pkgconfluence.IconGamma, Score: 20, Detail: "near GEX support"},
			{Name: "RSI", Weight: 0.20, AxisFill: 0.9, Status: pkgconfluence.SignalAligned, Icon: pkgconfluence.IconRSI, Score: 18, Detail: "RSI 31"},
		},
	}

	proto := SnapshotToProto(snap)
	if proto == nil {
		t.Fatal("expected non-nil proto")
	}

	if proto.Ticker != "NVDA" {
		t.Errorf("ticker: got %q want NVDA", proto.Ticker)
	}
	if proto.Timestamp != updatedAt.Unix() {
		t.Errorf("timestamp: got %d want %d", proto.Timestamp, updatedAt.Unix())
	}
	if proto.ConfluenceScore != 72.5 {
		t.Errorf("score: got %v want 72.5", proto.ConfluenceScore)
	}
	if proto.Readiness != string(pkgconfluence.ReadinessPossibleEntry) {
		t.Errorf("readiness: got %q", proto.Readiness)
	}
	if proto.OiStatus != string(pkgconfluence.OIStatusReady) {
		t.Errorf("oi_status: got %q", proto.OiStatus)
	}
	if proto.MarketStatus != string(pkgconfluence.MarketStatusOpen) {
		t.Errorf("market_status: got %q", proto.MarketStatus)
	}
	if proto.DailyRangePosition != 0.42 {
		t.Errorf("daily_range_position: got %v", proto.DailyRangePosition)
	}
	if proto.DistanceToEntry != string(pkgconfluence.EntryIdeal) {
		t.Errorf("distance_to_entry: got %q", proto.DistanceToEntry)
	}
	if proto.HapticLevel != 2 || proto.BackgroundLevel != 2 {
		t.Errorf("haptic/background: got %d/%d", proto.HapticLevel, proto.BackgroundLevel)
	}
	if proto.Spot != 120.5 || proto.SpotTimestamp != spotTime.Unix() {
		t.Errorf("spot: got %v @ %d", proto.Spot, proto.SpotTimestamp)
	}
	if proto.Rsi != 31.2 || proto.SectorEtf != "SMH" || !proto.StackedZone {
		t.Errorf("rsi/sector/stacked: got rsi=%v sector=%q stacked=%v", proto.Rsi, proto.SectorEtf, proto.StackedZone)
	}

	if proto.Levels == nil {
		t.Fatal("expected levels")
	}
	if proto.Levels.GammaFlip != 118.0 {
		t.Errorf("gamma_flip: got %v", proto.Levels.GammaFlip)
	}
	if len(proto.Levels.Support) != 1 || proto.Levels.Support[0].Price != 118.5 {
		t.Errorf("support: %+v", proto.Levels.Support)
	}
	if len(proto.Levels.Resistance) != 1 || proto.Levels.Resistance[0].Source != "gex" {
		t.Errorf("resistance: %+v", proto.Levels.Resistance)
	}
	if !proto.Levels.HasNearestSupport || !proto.Levels.HasNearestResistance {
		t.Error("expected nearest flags true")
	}

	if len(proto.Signals) != 2 {
		t.Fatalf("signals len: got %d want 2", len(proto.Signals))
	}
	if proto.Signals[0].Icon != "G" || proto.Signals[1].Icon != "R" {
		t.Errorf("signal icons: %q %q", proto.Signals[0].Icon, proto.Signals[1].Icon)
	}
}

func TestSnapshotToProto_nil(t *testing.T) {
	if SnapshotToProto(nil) != nil {
		t.Error("expected nil for nil input")
	}
}
