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
	assert.Equal(t, "starter", plan.AverageDown[0].Tier)
	assert.InDelta(t, 245.0, plan.AverageDown[0].Price, 0.01)
	assert.Equal(t, "add_1", plan.AverageDown[1].Tier)
	assert.InDelta(t, 240.0, plan.AverageDown[1].Price, 0.01)
	assert.Equal(t, "exit_instead", plan.AverageDown[1].IfBelow)

	assert.Equal(t, "above", plan.SpotContext.VsHardStop)
	assert.Contains(t, plan.SpotContext.Guidance, "playbook applies")
	assert.NotEmpty(t, plan.IntradayNotes)
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
	assert.Equal(t, "starter", plan.AverageDown[0].Tier)
	assert.InDelta(t, 97.51, findStop(plan.Stops, "soft_stop").Price, 0.01)
	assert.Zero(t, plan.ExitInsteadOfAddBelow)
}

func TestBuildTradePlan_cautionIncludesNote(t *testing.T) {
	snap := netStyleSnapshot(confluence.ReadinessCaution, 246.0)
	plan := confluence.BuildTradePlan(snap, confluence.DefaultTradePlanConfig())
	require.NotNil(t, plan)
	assert.Equal(t, "setup not fully confirmed", plan.EntryZone.Note)
	assert.Contains(t, plan.SpotContext.Guidance, "caution")
}

func TestTradePlanReasonLine(t *testing.T) {
	snap := netStyleSnapshot(confluence.ReadinessPossibleEntry, 246.0)
	plan := confluence.BuildTradePlan(snap, confluence.DefaultTradePlanConfig())
	line := confluence.TradePlanReasonLine(plan)
	assert.Contains(t, line, "Enter ~245")
	assert.Contains(t, line, "soft stop 243.8")
	assert.Contains(t, line, "hard exit 237.6")
	assert.Contains(t, line, "add at 240")
}

func findStop(stops []confluence.StopLevel, tier string) *confluence.StopLevel {
	for i := range stops {
		if stops[i].Tier == tier {
			return &stops[i]
		}
	}
	return nil
}
