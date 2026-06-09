package confluence_test

import (
	"testing"

	"github.com/ekinolik/jax/pkg/confluence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func netStyleSnapshot(readiness confluence.ReadinessBand, spot float64) confluence.ConfluenceSnapshot {
	return confluence.ConfluenceSnapshot{
		Ticker:          "NET",
		Spot:            spot,
		ReadinessBand:   readiness,
		DistanceToEntry: confluence.EntryIdeal,
		StackedZone:     false,
		Levels: confluence.Levels{
			Support: []confluence.Level{
				{Price: 245.0, Source: confluence.LevelSourceGEX, Rank: 1},
				{Price: 240.0, Source: confluence.LevelSourceDEX, Rank: 2},
			},
			NearestSupport:    245.0,
			HasNearestSupport: true,
		},
	}
}

func TestBuildTradePlan_NETStyleLadder(t *testing.T) {
	snap := netStyleSnapshot(confluence.ReadinessPossibleEntry, 246.0)
	cfg := confluence.DefaultTradePlanConfig()

	plan := confluence.BuildTradePlan(snap, cfg)
	require.NotNil(t, plan)

	assert.InDelta(t, 245.0, plan.EntryZone.Price, 0.01)
	assert.Equal(t, "gex_support", plan.EntryZone.Source)
	assert.Equal(t, "ideal", plan.EntryZone.Timing)

	soft := findStop(plan.Stops, "soft_stop")
	require.NotNil(t, soft)
	assert.InDelta(t, 243.78, soft.Price, 0.01)
	assert.Equal(t, "watch", soft.Action)

	structStop := findStop(plan.Stops, "structure_stop")
	require.NotNil(t, structStop)
	assert.InDelta(t, 240.0, structStop.Price, 0.01)

	hard := findStop(plan.Stops, "hard_stop")
	require.NotNil(t, hard)
	assert.InDelta(t, 237.6, hard.Price, 0.01)
	assert.Equal(t, "exit", hard.Action)

	assert.InDelta(t, 237.6, plan.ExitInsteadOfAddBelow, 0.01)

	require.Len(t, plan.AverageDown, 2)
	assert.Equal(t, "initial_entry", plan.AverageDown[0].Tier)
	assert.InDelta(t, 245.0, plan.AverageDown[0].Price, 0.01)
	assert.Equal(t, "add_1", plan.AverageDown[1].Tier)
	assert.InDelta(t, 240.0, plan.AverageDown[1].Price, 0.01)
	assert.Equal(t, "exit_instead", plan.AverageDown[1].IfBelow)

	assert.Equal(t, "above", plan.SpotContext.VsHardStop)
	assert.Contains(t, plan.SpotContext.Guidance, "playbook applies")
	assert.Empty(t, plan.IntradayNotes)
}

func TestBuildTradePlan_noTradeNil(t *testing.T) {
	snap := netStyleSnapshot(confluence.ReadinessNoTrade, 246.0)
	plan := confluence.BuildTradePlan(snap, confluence.DefaultTradePlanConfig())
	assert.Nil(t, plan)
}

func TestBuildTradePlan_spotBelowHardStop(t *testing.T) {
	snap := netStyleSnapshot(confluence.ReadinessPossibleEntry, 235.0)
	plan := confluence.BuildTradePlan(snap, confluence.DefaultTradePlanConfig())
	require.NotNil(t, plan)

	assert.Equal(t, "below", plan.SpotContext.VsHardStop)
	assert.Contains(t, plan.SpotContext.Guidance, "do not add")
	assert.Contains(t, plan.SpotContext.Guidance, "consider exit")
}

func TestBuildTradePlan_singleSupportNoAdd1(t *testing.T) {
	snap := confluence.ConfluenceSnapshot{
		Spot:            100.0,
		ReadinessBand:   confluence.ReadinessPossibleEntry,
		DistanceToEntry: confluence.EntryIdeal,
		Levels: confluence.Levels{
			Support: []confluence.Level{
				{Price: 98.0, Source: confluence.LevelSourceGEX, Rank: 1},
			},
			NearestSupport:    98.0,
			HasNearestSupport: true,
		},
	}

	plan := confluence.BuildTradePlan(snap, confluence.DefaultTradePlanConfig())
	require.NotNil(t, plan)
	require.Len(t, plan.AverageDown, 1)
	assert.Equal(t, "initial_entry", plan.AverageDown[0].Tier)
	assert.InDelta(t, 97.51, findStop(plan.Stops, "soft_stop").Price, 0.01)
	assert.Zero(t, plan.ExitInsteadOfAddBelow)
}

func sndkStyleSnapshot(spot float64) confluence.ConfluenceSnapshot {
	return confluence.ConfluenceSnapshot{
		Ticker:          "SNDK",
		Spot:            spot,
		ReadinessBand:   confluence.ReadinessPossibleEntry,
		DistanceToEntry: confluence.EntryIdeal,
		Levels: confluence.Levels{
			Support: []confluence.Level{
				{Price: 1600.0, Source: confluence.LevelSourceGEX, Rank: 1},
				{Price: 1580.0, Source: confluence.LevelSourceGEX, Rank: 2},
				{Price: 1450.0, Source: confluence.LevelSourceDEX, Rank: 3},
			},
			NearestSupport:    1600.0,
			HasNearestSupport: true,
		},
	}
}

func TestBuildTradePlan_SNDKStyleSyntheticAdd1(t *testing.T) {
	snap := sndkStyleSnapshot(1610.0)
	cfg := confluence.DefaultTradePlanConfig()

	plan := confluence.BuildTradePlan(snap, cfg)
	require.NotNil(t, plan)

	assert.InDelta(t, 1600.0, plan.EntryZone.Price, 0.01)

	cluster := findStop(plan.Stops, "cluster_floor")
	require.NotNil(t, cluster, "expected cluster_floor stop")
	assert.InDelta(t, 1580.0, cluster.Price, 0.01)

	structStop := findStop(plan.Stops, "structure_stop")
	require.NotNil(t, structStop)
	assert.InDelta(t, 1450.0, structStop.Price, 0.01)

	assert.InDelta(t, 1572.1, plan.ExitInsteadOfAddBelow, 0.01)
	assert.InDelta(t, 0.094, plan.GEXDEXGapPct, 0.002)

	require.Len(t, plan.AverageDown, 2)
	assert.Equal(t, "initial_entry", plan.AverageDown[0].Tier)
	assert.InDelta(t, 1600.0, plan.AverageDown[0].Price, 0.01)
	assert.Contains(t, plan.AverageDown[0].Condition, "Initial entry at rank-1 GEX")

	assert.Equal(t, "add_1", plan.AverageDown[1].Tier)
	assert.InDelta(t, 1590.0, plan.AverageDown[1].Price, 0.01)
	assert.Equal(t, "exit_instead", plan.AverageDown[1].IfBelow)
	assert.Contains(t, plan.AverageDown[1].Condition, "cluster floor 1580 holds and reclaims 1590")

	line := confluence.TradePlanReasonLine(plan)
	assert.Contains(t, line, "Enter ~1600")
	assert.Contains(t, line, "exit day trade if cluster fails ~1580")
	assert.Contains(t, line, "add at 1590 only if 1580 holds and reclaims 1590")
}

func TestBuildTradePlan_clusterGapTooSmallNoSyntheticAdd1(t *testing.T) {
	snap := confluence.ConfluenceSnapshot{
		Spot:            101.0,
		ReadinessBand:   confluence.ReadinessPossibleEntry,
		DistanceToEntry: confluence.EntryIdeal,
		Levels: confluence.Levels{
			Support: []confluence.Level{
				{Price: 100.0, Source: confluence.LevelSourceGEX, Rank: 1},
				{Price: 99.5, Source: confluence.LevelSourceGEX, Rank: 2},
				{Price: 90.0, Source: confluence.LevelSourceDEX, Rank: 3},
			},
			NearestSupport:    100.0,
			HasNearestSupport: true,
		},
	}

	plan := confluence.BuildTradePlan(snap, confluence.DefaultTradePlanConfig())
	require.NotNil(t, plan)

	cluster := findStop(plan.Stops, "cluster_floor")
	require.NotNil(t, cluster)
	assert.InDelta(t, 99.5, cluster.Price, 0.01)

	require.Len(t, plan.AverageDown, 1, "gap below min_add_gap_pct should not emit synthetic add_1")
	assert.Equal(t, "initial_entry", plan.AverageDown[0].Tier)
}

func TestBuildTradePlan_cautionIncludesNote(t *testing.T) {
	snap := netStyleSnapshot(confluence.ReadinessCaution, 246.0)
	plan := confluence.BuildTradePlan(snap, confluence.DefaultTradePlanConfig())
	require.NotNil(t, plan)
	assert.Equal(t, "setup not fully confirmed", plan.EntryZone.Note)
	assert.Contains(t, plan.SpotContext.Guidance, "caution")
}

func TestBuildTradePlan_cautionPoorRRIntradayNote(t *testing.T) {
	snap := netStyleSnapshot(confluence.ReadinessCaution, 246.0)
	snap.RiskReward = 0.6
	plan := confluence.BuildTradePlan(snap, confluence.DefaultTradePlanConfig())
	require.NotNil(t, plan)
	require.Len(t, plan.IntradayNotes, 1)
	assert.Contains(t, plan.IntradayNotes[0], "Single entry recommended")
}

func TestTradePlanReasonLine(t *testing.T) {
	snap := netStyleSnapshot(confluence.ReadinessPossibleEntry, 246.0)
	plan := confluence.BuildTradePlan(snap, confluence.DefaultTradePlanConfig())
	line := confluence.TradePlanReasonLine(plan)
	assert.Contains(t, line, "Enter ~245")
	assert.Contains(t, line, "thesis dead below 240 (DEX)")
	assert.Contains(t, line, "emergency exit 237.6")
	assert.Contains(t, line, "add at 240 only if 237.6 holds and reclaims 240")
}

func TestStopTierDisplay(t *testing.T) {
	label, meaning := confluence.StopTierDisplay("cluster_floor")
	assert.Equal(t, "trade_failure", label)
	assert.Contains(t, meaning, "GEX cluster lost")

	label, meaning = confluence.StopTierDisplay("structure_stop")
	assert.Equal(t, "thesis_failure", label)
	assert.Contains(t, meaning, "DEX structure lost")

	label, _ = confluence.StopTierDisplay("final_stop")
	assert.Equal(t, "emergency_stop", label)
}

func crwdStyleSnapshot(spot float64) confluence.ConfluenceSnapshot {
	return confluence.ConfluenceSnapshot{
		Ticker:          "CRWD",
		Spot:            spot,
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

func TestBuildTradePlan_CRWDStyleClusterGap(t *testing.T) {
	snap := crwdStyleSnapshot(665.0)
	cfg := confluence.DefaultTradePlanConfig()

	plan := confluence.BuildTradePlan(snap, cfg)
	require.NotNil(t, plan)

	assert.InDelta(t, 650.0, plan.EntryZone.Price, 0.01)

	cluster := findStop(plan.Stops, "cluster_floor")
	require.NotNil(t, cluster, "expected cluster_floor stop")
	assert.InDelta(t, 640.0, cluster.Price, 0.01)
	assert.Equal(t, "exit_if_lost", cluster.Action)

	structStop := findStop(plan.Stops, "structure_stop")
	require.NotNil(t, structStop)
	assert.InDelta(t, 590.0, structStop.Price, 0.01)

	hard := findStop(plan.Stops, "hard_stop")
	require.NotNil(t, hard)
	assert.InDelta(t, 584.1, hard.Price, 0.01)

	assert.InDelta(t, 636.8, plan.ExitInsteadOfAddBelow, 0.01,
		"exit_instead should be near cluster floor, not distant DEX hard stop")

	require.GreaterOrEqual(t, len(plan.AverageDown), 2)
	assert.Equal(t, "add_1", plan.AverageDown[1].Tier)
	assert.InDelta(t, 645.0, plan.AverageDown[1].Price, 0.01)
	assert.Contains(t, plan.AverageDown[1].Condition, "reclaims 645")
	assert.NotContains(t, plan.AverageDown[1].Condition, "reclaims 650")

	assert.InDelta(t, 0.092, plan.GEXDEXGapPct, 0.002)

	assert.InDelta(t, 640.0, plan.TradeInvalidationPrice, 0.01)
	assert.InDelta(t, 590.0, plan.StructureInvalidationPrice, 0.01)
	assert.InDelta(t, 640.0, plan.PrimaryExitPrice, 0.01)

	require.NotNil(t, findStop(plan.Stops, "cluster_floor"))
	assert.Equal(t, "trade_failure", findStop(plan.Stops, "cluster_floor").HumanLabel)
	assert.Equal(t, "thesis_failure", findStop(plan.Stops, "structure_stop").HumanLabel)

	assert.Contains(t, plan.SpotContext.Guidance, "Above entry zone")
	assert.Contains(t, plan.SpotContext.Guidance, "650")

	line := confluence.TradePlanReasonLine(plan)
	assert.Contains(t, line, "exit day trade if cluster fails ~640")
	assert.Contains(t, line, "thesis dead below 590 (DEX)")
	assert.Contains(t, line, "add at 645 only if 640 holds and reclaims 645")
}

func findStop(stops []confluence.StopLevel, tier string) *confluence.StopLevel {
	for i := range stops {
		if stops[i].Tier == tier {
			return &stops[i]
		}
	}
	return nil
}
