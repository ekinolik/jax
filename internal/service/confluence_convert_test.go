package service

import (
	"testing"
	"time"

	confluencev1 "github.com/ekinolik/jax/api/proto/confluence/v1"
	pkgconfluence "github.com/ekinolik/jax/pkg/confluence"
)

func TestSnapshotToProto_roundTripKeyFields(t *testing.T) {
	updatedAt := time.Date(2026, 6, 6, 14, 30, 0, 0, time.UTC)
	spotTime := updatedAt.Add(-2 * time.Second)
	dataAsOf := updatedAt.Add(-5 * time.Minute)

	snap := &pkgconfluence.ConfluenceSnapshot{
		Ticker:          "NVDA",
		Spot:            120.5,
		SpotTime:        spotTime,
		OIStatus:        pkgconfluence.OIStatusReady,
		MarketStatus:    pkgconfluence.MarketStatusOpen,
		UpdatedAt:       updatedAt,
		DataAsOf:        dataAsOf,
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
	if proto.DataAsOf != dataAsOf.Unix() {
		t.Errorf("data_as_of: got %d want %d", proto.DataAsOf, dataAsOf.Unix())
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
	if len(proto.BuySignals) != 2 {
		t.Fatalf("buy_signals len: got %d want 2", len(proto.BuySignals))
	}
	if proto.Signals[0].Icon != "G" || proto.Signals[1].Icon != "R" {
		t.Errorf("signal icons: %q %q", proto.Signals[0].Icon, proto.Signals[1].Icon)
	}
}

func TestSnapshotToProto_v2Fields(t *testing.T) {
	snap := &pkgconfluence.ConfluenceSnapshot{
		Ticker:        "GME",
		Score:         65,
		ReadinessBand: pkgconfluence.ReadinessPossibleEntry,
		ADR30dPct:     4.5,
		ADRRegime:     pkgconfluence.ADRStableHigh,
		UpsidePct:     0.05,
		GammaRegime:   pkgconfluence.GammaNegative,
		SellScore:     55,
		SellReadiness: pkgconfluence.SellConsiderTrim,
		ExitAction:    pkgconfluence.ExitTrim,
		DistanceToExit: pkgconfluence.ExitIdeal,
		RSIDaily:      45,
		BuySignals:    []pkgconfluence.Signal{{Name: "upside", Weight: 0.07}},
		SellSignals:   []pkgconfluence.Signal{{Name: "resistance_proximity", Weight: 0.30}},
	}
	proto := SnapshotToProto(snap)
	if proto.Adr_30DPct != 4.5 {
		t.Errorf("adr_30d: got %v", proto.Adr_30DPct)
	}
	if proto.SellScore != 55 || proto.ExitAction != "trim" {
		t.Errorf("sell: score=%v action=%q", proto.SellScore, proto.ExitAction)
	}
	if len(proto.SellSignals) != 1 {
		t.Errorf("sell_signals len: %d", len(proto.SellSignals))
	}
}

func TestSnapshotToProto_nil(t *testing.T) {
	if SnapshotToProto(nil) != nil {
		t.Error("expected nil for nil input")
	}
}

func TestSummaryToProto_enrichedTradePlan(t *testing.T) {
	plan := pkgconfluence.BuildTradePlan(crwdStyleSnapshotForConvert(), pkgconfluence.DefaultTradePlanConfig())
	if plan == nil {
		t.Fatal("expected trade plan")
	}

	sum := pkgconfluence.SummaryFromSnapshot(pkgconfluence.ConfluenceSnapshot{
		Ticker:        "CRWD",
		Spot:          665.0,
		MarketStatus:  pkgconfluence.MarketStatusOpen,
		ReadinessBand: pkgconfluence.ReadinessPossibleEntry,
		TradePlan:     plan,
	})

	proto := SummaryToProto(sum)
	if proto.TradePlan == nil {
		t.Fatal("expected trade_plan on summary proto")
	}
	tp := proto.TradePlan

	if tp.TradeInvalidationPrice != 640 || tp.StructureInvalidationPrice != 590 || tp.PrimaryExitPrice != 640 {
		t.Errorf("invalidation prices: trade=%v structure=%v primary=%v",
			tp.TradeInvalidationPrice, tp.StructureInvalidationPrice, tp.PrimaryExitPrice)
	}
	if tp.Invalidation == nil || tp.Invalidation.Trade == nil || tp.Invalidation.Structure == nil {
		t.Fatalf("invalidation: %+v", tp.Invalidation)
	}
	if tp.Invalidation.Trade.Label != "Trade failure" || tp.Invalidation.Structure.Label != "Thesis failure" {
		t.Errorf("invalidation labels: trade=%q structure=%q",
			tp.Invalidation.Trade.Label, tp.Invalidation.Structure.Label)
	}
	if tp.PrimaryExit == nil || tp.PrimaryExit.Label != "Trade failure" || !tp.PrimaryExit.Emphasis {
		t.Errorf("primary_exit: %+v", tp.PrimaryExit)
	}

	var cluster *confluencev1.StopLevel
	for _, s := range tp.Stops {
		if s.Tier == "cluster_floor" {
			cluster = s
			break
		}
	}
	if cluster == nil {
		t.Fatal("expected cluster_floor stop")
	}
	if cluster.Label != "trade_failure" {
		t.Errorf("cluster label: got %q want trade_failure", cluster.Label)
	}
	if cluster.Meaning == "" {
		t.Error("expected cluster meaning")
	}
}

func crwdStyleSnapshotForConvert() pkgconfluence.ConfluenceSnapshot {
	return pkgconfluence.ConfluenceSnapshot{
		Ticker:          "CRWD",
		Spot:            665.0,
		ReadinessBand:   pkgconfluence.ReadinessPossibleEntry,
		DistanceToEntry: pkgconfluence.EntryEarly,
		Levels: pkgconfluence.Levels{
			Support: []pkgconfluence.Level{
				{Price: 650.0, Source: pkgconfluence.LevelSourceGEX, Rank: 1},
				{Price: 645.0, Source: pkgconfluence.LevelSourceGEX, Rank: 2},
				{Price: 640.0, Source: pkgconfluence.LevelSourceGEX, Rank: 3},
				{Price: 590.0, Source: pkgconfluence.LevelSourceDEX, Rank: 4},
			},
			NearestSupport:    650.0,
			HasNearestSupport: true,
		},
	}
}

func TestSnapshotToProto_tradePlanLabels(t *testing.T) {
	plan := pkgconfluence.BuildTradePlan(crwdStyleSnapshotForConvert(), pkgconfluence.DefaultTradePlanConfig())
	if plan == nil {
		t.Fatal("expected trade plan")
	}

	proto := SnapshotToProto(&pkgconfluence.ConfluenceSnapshot{
		Ticker:        "CRWD",
		Spot:          665.0,
		ReadinessBand: pkgconfluence.ReadinessPossibleEntry,
		TradePlan:     plan,
	})
	if proto.TradePlan == nil {
		t.Fatal("expected trade_plan on snapshot proto")
	}
	if proto.TradePlan.TradeInvalidationPrice != 640 {
		t.Errorf("trade_invalidation_price: got %v", proto.TradePlan.TradeInvalidationPrice)
	}
	var cluster *confluencev1.StopLevel
	for _, s := range proto.TradePlan.Stops {
		if s.Tier == "cluster_floor" {
			cluster = s
			break
		}
	}
	if cluster == nil || cluster.Label != "trade_failure" {
		t.Errorf("cluster stop: %+v", cluster)
	}
}

func TestSummaryToProto_keyFields(t *testing.T) {
	sum := pkgconfluence.SummaryFromSnapshot(pkgconfluence.ConfluenceSnapshot{
		Ticker:          "NET",
		Spot:            248.5,
		MarketStatus:    pkgconfluence.MarketStatusOpen,
		DataAsOf:        time.Date(2026, 6, 6, 14, 32, 0, 0, time.FixedZone("EDT", -4*3600)),
		Score:           62,
		ReadinessBand:   pkgconfluence.ReadinessPossibleEntry,
		SellScore:       28,
		SellReadiness:   pkgconfluence.SellHold,
		ExitAction:      pkgconfluence.ExitHold,
		GammaRegime:     pkgconfluence.GammaPositive,
		DistanceToEntry: pkgconfluence.EntryEarly,
		RSI:             32,
		Levels: pkgconfluence.Levels{
			NearestSupport:       245,
			NearestResistance:    258,
			HasNearestSupport:    true,
			HasNearestResistance: true,
		},
		UpsidePct: 0.042,
		BuySignals: []pkgconfluence.Signal{
			{Name: "gamma_support", Status: pkgconfluence.SignalAligned},
		},
	})

	proto := SummaryToProto(sum)
	if proto.Ticker != "NET" || proto.Verdict.Buy.Label != "Watch for entry" {
		t.Errorf("summary proto: %+v", proto)
	}
	if !proto.Context.RsiMinuteUnavailable && proto.Context.RsiMinute != 32 {
		t.Errorf("rsi_minute: unavailable=%v value=%v", proto.Context.RsiMinuteUnavailable, proto.Context.RsiMinute)
	}
	if proto.TradeSetup.Archetype != "mean_reversion" {
		t.Errorf("archetype: %q", proto.TradeSetup.Archetype)
	}
}
