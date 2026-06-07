package confluence_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ekinolik/jax/pkg/confluence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSummaryFromSnapshot_meanReversion(t *testing.T) {
	snap := confluence.ConfluenceSnapshot{
		Ticker:          "NET",
		Spot:            245.0,
		MarketStatus:    confluence.MarketStatusOpen,
		DataAsOf:        time.Date(2026, 6, 6, 14, 32, 0, 0, time.FixedZone("EDT", -4*3600)),
		Score:           62,
		ReadinessBand:   confluence.ReadinessPossibleEntry,
		SellScore:       28,
		SellReadiness:   confluence.SellHold,
		ExitAction:      confluence.ExitHold,
		DistanceToEntry: confluence.EntryEarly,
		DistanceToExit:  confluence.ExitLate,
		GammaRegime:     confluence.GammaPositive,
		ADRRegime:       confluence.ADRStableHigh,
		RSI:             32,
		RSIDaily:        58.9,
		StackedZone:     true,
		UpsidePct:       0.042,
		DownsidePct:     0.018,
		RiskReward:      2.3,
		Levels: confluence.Levels{
			NearestSupport:       245.0,
			NearestResistance:    258.0,
			HasNearestSupport:    true,
			HasNearestResistance: true,
		},
		CallWall: 255,
		PutWall:  243,
		BuySignals: []confluence.Signal{
			{Name: "gamma_support", Status: confluence.SignalAligned},
			{Name: "rsi_minute", Status: confluence.SignalAligned},
			{Name: "sector", Status: confluence.SignalAligned},
			{Name: "market", Status: confluence.SignalAligned},
		},
	}

	sum := confluence.SummaryFromSnapshot(snap)

	assert.Equal(t, "NET", sum.Ticker)
	assert.Equal(t, "possible_entry", sum.Verdict.Buy.Readiness)
	assert.Equal(t, "Watch for entry", sum.Verdict.Buy.Label)
	assert.Equal(t, "hold", sum.Verdict.Sell.Readiness)
	assert.Equal(t, "No exit signal", sum.Verdict.Sell.Label)
	assert.Equal(t, "mean_reversion", sum.TradeSetup.Archetype)
	assert.InDelta(t, 4.2, sum.TradeSetup.UpsidePct, 0.01)
	assert.Contains(t, sum.Reasons, "Gamma support aligned near spot")
	assert.Contains(t, sum.Reasons, "Stacked support zone within 1% of spot")
	assert.True(t, sum.Gates.UpsideOK)
	assert.True(t, sum.Gates.ADROK)
	assert.False(t, sum.Gates.BlockedFromHighConviction)

	b, err := json.Marshal(sum)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"rsi_minute":32`)
}

func TestSummaryFromSnapshot_squeezeMomentum(t *testing.T) {
	snap := confluence.ConfluenceSnapshot{
		Ticker:             "GME",
		Spot:               28.5,
		MarketStatus:       confluence.MarketStatusOpen,
		UpdatedAt:          time.Now().UTC(),
		Score:              68,
		ReadinessBand:      confluence.ReadinessPossibleEntry,
		GammaRegime:        confluence.GammaNegative,
		GammaSqueezeActive: true,
		ShortSqueezeActive: true,
		ShortInterestPct:   22,
		RelativeVolume:     2.1,
		CallWall:           27.0,
		PutWall:            25.0,
		DistanceToEntry:    confluence.EntryLate,
		RSI:                55,
		RSIDaily:           62,
		Levels: confluence.Levels{
			NearestResistance:    30.0,
			HasNearestResistance: true,
		},
		UpsidePct: 0.05,
		BuySignals: []confluence.Signal{
			{Name: "gamma_environment", Status: confluence.SignalAligned},
			{Name: "short_squeeze", Status: confluence.SignalAligned},
			{Name: "gamma_directional", Status: confluence.SignalAligned},
		},
	}

	sum := confluence.SummaryFromSnapshot(snap)

	assert.Equal(t, "squeeze_momentum", sum.TradeSetup.Archetype)
	assert.True(t, sum.Context.GammaSqueeze)
	assert.True(t, sum.Context.ShortSqueeze)
	assert.Contains(t, sum.Reasons, "Short squeeze score aligned")
	assert.Contains(t, sum.Reasons, "Gamma squeeze active — momentum fuel")
}

func TestSummaryFromSnapshot_gatedBlocked(t *testing.T) {
	snap := confluence.ConfluenceSnapshot{
		Ticker:                "AMD",
		Spot:                  150,
		MarketStatus:          confluence.MarketStatusOpen,
		Score:                 78,
		ReadinessBand:         confluence.ReadinessCaution,
		GammaDirectionalScore: -12,
		ADRRegime:             confluence.ADRSpikeWarning,
		RSI:                   0,
		OIStatus:              confluence.OIStatusLoading,
		Levels: confluence.Levels{
			NearestResistance:    152,
			HasNearestResistance: true,
		},
		UpsidePct: 0.015,
		BuySignals: []confluence.Signal{
			{Name: "gamma_support", Status: confluence.SignalAligned},
			{Name: "market", Status: confluence.SignalAgainst},
			{Name: "upside", Status: confluence.SignalAgainst},
		},
	}

	sum := confluence.SummaryFromSnapshot(snap)

	assert.Equal(t, "caution", sum.Verdict.Buy.Readiness)
	assert.Equal(t, "Wait — conditions weak", sum.Verdict.Buy.Label)
	assert.False(t, sum.Gates.UpsideOK)
	assert.False(t, sum.Gates.ADROK)
	assert.True(t, sum.Gates.BlockedFromHighConviction)
	assert.Contains(t, sum.Warnings, "Upside room below 3% minimum — readiness capped")
	assert.Contains(t, sum.Warnings, "ADR spike warning — readiness capped at caution")
	assert.Contains(t, sum.Warnings, "Gamma directional score weak — readiness capped")
	assert.Contains(t, sum.Warnings, "Minute RSI unavailable")
	assert.Contains(t, sum.Warnings, "Open interest still loading — gamma/delta levels suppressed")
	assert.Contains(t, sum.Warnings, "Broad market working against long entry")

	b, err := json.Marshal(sum)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"rsi_minute":"unavailable"`)
}

func TestReadinessLabels(t *testing.T) {
	tests := []struct {
		band  confluence.ReadinessBand
		label string
	}{
		{confluence.ReadinessHighConviction, "Strong buy setup"},
		{confluence.ReadinessPossibleEntry, "Watch for entry"},
		{confluence.ReadinessCaution, "Wait — conditions weak"},
		{confluence.ReadinessNoTrade, "Avoid"},
	}
	for _, tt := range tests {
		snap := confluence.ConfluenceSnapshot{ReadinessBand: tt.band}
		sum := confluence.SummaryFromSnapshot(snap)
		assert.Equal(t, tt.label, sum.Verdict.Buy.Label, "band %s", tt.band)
	}
}

func TestSummaryFromSnapshot_tradePlan(t *testing.T) {
	plan := confluence.BuildTradePlan(confluence.ConfluenceSnapshot{
		Ticker:          "NET",
		Spot:            246.0,
		ReadinessBand:   confluence.ReadinessPossibleEntry,
		DistanceToEntry: confluence.EntryIdeal,
		Levels: confluence.Levels{
			Support: []confluence.Level{
				{Price: 245.0, Source: confluence.LevelSourceGEX, Rank: 1},
				{Price: 240.0, Source: confluence.LevelSourceDEX, Rank: 2},
			},
			NearestSupport:    245.0,
			HasNearestSupport: true,
		},
	}, confluence.DefaultTradePlanConfig())
	require.NotNil(t, plan)

	snap := confluence.ConfluenceSnapshot{
		Ticker:          "NET",
		Spot:            246.0,
		ReadinessBand:   confluence.ReadinessPossibleEntry,
		DistanceToEntry: confluence.EntryIdeal,
		TradePlan:       plan,
		Levels: confluence.Levels{
			NearestSupport:    245.0,
			HasNearestSupport: true,
		},
	}

	sum := confluence.SummaryFromSnapshot(snap)
	require.NotNil(t, sum.TradePlan)
	assert.InDelta(t, 245.0, sum.TradePlan.EntryZone.Price, 0.01)
	require.NotEmpty(t, sum.Reasons)
	assert.Contains(t, sum.Reasons[0], "Enter ~245")
	assert.Contains(t, sum.Reasons[0], "thesis dead below 240 (DEX)")
	assert.Contains(t, sum.Reasons[0], "emergency exit 237.6")
	assert.Contains(t, sum.Reasons[0], "add at 240 only if 237.6 holds and reclaims 240")

	snap.Spot = 235.0
	sumBelow := confluence.SummaryFromSnapshot(snap)
	assert.Contains(t, sumBelow.Warnings, "Spot below hard stop — do not add, consider exit")

	b, err := json.Marshal(sum)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"trade_plan"`)
	assert.Contains(t, string(b), `"thesis_failure"`)
}

func TestSummaryFromSnapshot_CRWDHumanLabels(t *testing.T) {
	plan := confluence.BuildTradePlan(crwdStyleSnapshotForSummary(), confluence.DefaultTradePlanConfig())
	require.NotNil(t, plan)

	snap := confluence.ConfluenceSnapshot{
		Ticker:        "CRWD",
		Spot:          665.0,
		MarketStatus:  confluence.MarketStatusOpen,
		ReadinessBand: confluence.ReadinessPossibleEntry,
		TradePlan:     plan,
	}

	sum := confluence.SummaryFromSnapshot(snap)
	require.NotNil(t, sum.TradePlan)

	assert.InDelta(t, 640.0, sum.TradePlan.PrimaryExitPrice, 0.01)
	assert.InDelta(t, 640.0, sum.TradePlan.TradeInvalidationPrice, 0.01)
	assert.InDelta(t, 590.0, sum.TradePlan.StructureInvalidationPrice, 0.01)

	require.NotNil(t, sum.TradePlan.Invalidation)
	require.NotNil(t, sum.TradePlan.Invalidation.Trade)
	assert.InDelta(t, 640.0, sum.TradePlan.Invalidation.Trade.Price, 0.01)
	assert.Equal(t, "Trade failure", sum.TradePlan.Invalidation.Trade.Label)
	require.NotNil(t, sum.TradePlan.Invalidation.Structure)
	assert.InDelta(t, 590.0, sum.TradePlan.Invalidation.Structure.Price, 0.01)
	assert.Equal(t, "Thesis failure", sum.TradePlan.Invalidation.Structure.Label)

	require.NotNil(t, sum.TradePlan.PrimaryExit)
	assert.Equal(t, "Trade failure", sum.TradePlan.PrimaryExit.Label)
	assert.True(t, sum.TradePlan.PrimaryExit.Emphasis)

	cluster := findSummaryStop(sum.TradePlan.Stops, "cluster_floor")
	require.NotNil(t, cluster)
	assert.Equal(t, "trade_failure", cluster.HumanLabel)

	b, err := json.Marshal(sum)
	require.NoError(t, err)
	body := string(b)
	assert.Contains(t, body, `"invalidation"`)
	assert.Contains(t, body, `"primary_exit"`)
	assert.Contains(t, body, `"trade_failure"`)
	assert.Contains(t, body, `"thesis_failure"`)
	assert.Contains(t, body, `"Trade failure"`)
	assert.Contains(t, body, `"emphasis":true`)

	require.NotEmpty(t, sum.Reasons)
	assert.Contains(t, sum.Reasons[0], "exit day trade if cluster fails ~640")
	assert.Contains(t, sum.Reasons[0], "thesis dead below 590 (DEX)")
}

func findSummaryStop(stops []confluence.StopLevel, tier string) *confluence.StopLevel {
	for i := range stops {
		if stops[i].Tier == tier {
			return &stops[i]
		}
	}
	return nil
}

func TestSummaryFromSnapshot_poorRiskReward(t *testing.T) {
	snap := confluence.ConfluenceSnapshot{
		Ticker:        "CRWD",
		Spot:          665.0,
		MarketStatus:  confluence.MarketStatusOpen,
		ReadinessBand: confluence.ReadinessPossibleEntry,
		RiskReward:    0.5,
		UpsidePct:     0.03,
		DownsidePct:   0.06,
		BuySignals: []confluence.Signal{
			{Name: "upside", Status: confluence.SignalAligned},
			{Name: "downside", Status: confluence.SignalAgainst},
		},
	}

	sum := confluence.SummaryFromSnapshot(snap)

	assert.Contains(t, sum.Warnings, "Poor risk/reward — downside exceeds upside")
	assert.NotContains(t, sum.Reasons, "Adequate upside room to resistance")
}

func TestSummaryFromSnapshot_gexDexGapWarning(t *testing.T) {
	plan := confluence.BuildTradePlan(crwdStyleSnapshotForSummary(), confluence.DefaultTradePlanConfig())
	require.NotNil(t, plan)

	snap := confluence.ConfluenceSnapshot{
		Ticker:        "CRWD",
		Spot:          665.0,
		MarketStatus:  confluence.MarketStatusOpen,
		ReadinessBand: confluence.ReadinessPossibleEntry,
		TradePlan:     plan,
	}

	sum := confluence.SummaryFromSnapshot(snap)
	found := false
	for _, w := range sum.Warnings {
		if strings.Contains(w, "Large air pocket") {
			found = true
			break
		}
	}
	assert.True(t, found, "warnings: %v", sum.Warnings)
}

func crwdStyleSnapshotForSummary() confluence.ConfluenceSnapshot {
	return confluence.ConfluenceSnapshot{
		Ticker:          "CRWD",
		Spot:            665.0,
		ReadinessBand:   confluence.ReadinessPossibleEntry,
		DistanceToEntry: confluence.EntryEarly,
		Levels: confluence.Levels{
			Support: []confluence.Level{
				{Price: 650.0, Source: confluence.LevelSourceGEX, Rank: 1},
				{Price: 645.0, Source: confluence.LevelSourceGEX, Rank: 2},
				{Price: 640.0, Source: confluence.LevelSourceGEX, Rank: 3},
				{Price: 590.0, Source: confluence.LevelSourceDEX, Rank: 4},
			},
			NearestSupport:    650.0,
			HasNearestSupport: true,
		},
	}
}

func TestSellReadinessLabels(t *testing.T) {
	tests := []struct {
		band  confluence.SellReadinessBand
		label string
	}{
		{confluence.SellTakeProfit, "Take profit"},
		{confluence.SellConsiderTrim, "Consider trimming"},
		{confluence.SellWatch, "Watch for exit"},
		{confluence.SellHold, "No exit signal"},
	}
	for _, tt := range tests {
		snap := confluence.ConfluenceSnapshot{SellReadiness: tt.band}
		sum := confluence.SummaryFromSnapshot(snap)
		assert.Equal(t, tt.label, sum.Verdict.Sell.Label, "band %s", tt.band)
	}
}
