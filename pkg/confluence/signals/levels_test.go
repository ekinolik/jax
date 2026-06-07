package signals_test

import (
	"testing"

	"github.com/ekinolik/jax/pkg/confluence"
	"github.com/ekinolik/jax/pkg/confluence/signals"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fixtureSlice(expiration string, weight float32, profiles []confluence.StrikeProfile) confluence.OptionSlice {
	return confluence.OptionSlice{
		Ticker:       "TEST",
		Expiration:   expiration,
		ExpiryWeight: weight,
		Strikes:      profiles,
	}
}

func TestComputeLevels_supportAndResistance(t *testing.T) {
	spot := 120.0
	slice := fixtureSlice("2024-06-07", 1.0, []confluence.StrikeProfile{
		{Strike: 110, CallOI: 1000, CallGamma: 0.02, PutOI: 500, PutGamma: -0.01},
		{Strike: 115, CallOI: 5000, CallGamma: 0.05, PutOI: 200, PutGamma: -0.005},
		{Strike: 118, CallOI: 800, CallGamma: 0.03, PutOI: 300, PutGamma: -0.008},
		{Strike: 122, CallOI: 4000, CallGamma: 0.04, PutOI: 100, PutGamma: -0.003},
		{Strike: 125, CallOI: 2000, CallGamma: 0.025, PutOI: 800, PutGamma: -0.015},
		{Strike: 130, CallOI: 600, CallGamma: 0.01, PutOI: 2000, PutGamma: -0.02},
	})

	levels := signals.ComputeLevels([]confluence.OptionSlice{slice}, spot)

	require.True(t, levels.HasNearestSupport)
	require.True(t, levels.HasNearestResistance)
	assert.Less(t, levels.NearestSupport, spot)
	assert.Greater(t, levels.NearestResistance, spot)
	assert.NotEmpty(t, levels.Support)
	assert.NotEmpty(t, levels.Resistance)

	for _, lvl := range levels.Support {
		assert.Less(t, lvl.Price, spot)
		assert.Equal(t, 1, lvl.Rank) // only checking first rank exists
		break
	}
}

func TestComputeLevels_dualExpirationMerge(t *testing.T) {
	spot := 450.0
	daily := fixtureSlice("2024-06-07", 0.65, []confluence.StrikeProfile{
		{Strike: 445, CallOI: 3000, CallGamma: 0.03},
		{Strike: 448, CallOI: 1000, CallGamma: 0.01},
	})
	weekly := fixtureSlice("2024-06-14", 1.0, []confluence.StrikeProfile{
		{Strike: 445, CallOI: 5000, CallGamma: 0.04},
		{Strike: 455, CallOI: 4000, CallGamma: 0.035},
	})

	levels := signals.ComputeLevels([]confluence.OptionSlice{daily, weekly}, spot)

	require.True(t, levels.HasNearestSupport)
	assert.InDelta(t, 445, levels.NearestSupport, 1.0)
}

func TestHasStackedZone(t *testing.T) {
	spot := 100.0
	levels := confluence.Levels{
		Support: []confluence.Level{
			{Price: 99.5, Source: confluence.LevelSourceGEX, Rank: 1},
			{Price: 99.2, Source: confluence.LevelSourceDEX, Rank: 2},
		},
	}
	assert.True(t, signals.HasStackedZone(levels, spot))

	levels.Support = []confluence.Level{{Price: 95, Source: confluence.LevelSourceGEX, Rank: 1}}
	assert.False(t, signals.HasStackedZone(levels, spot))
}

func TestRank1Resistance_andSecondSupport(t *testing.T) {
	levels := confluence.Levels{
		Support: []confluence.Level{
			{Price: 98, Rank: 1, Strength: 0.9},
			{Price: 96, Rank: 2, Strength: 0.7},
		},
		Resistance: []confluence.Level{
			{Price: 104, Rank: 1, Strength: 0.85},
			{Price: 108, Rank: 2, Strength: 0.6},
		},
	}
	r1, ok := signals.Rank1Resistance(levels)
	require.True(t, ok)
	assert.Equal(t, 104.0, r1.Price)
	s2, ok := signals.SecondSupport(levels)
	require.True(t, ok)
	assert.Equal(t, 96.0, s2.Price)
}

func TestTradeGeometry_clusterFloorStop(t *testing.T) {
	spot := 665.0
	levels := confluence.Levels{
		Support: []confluence.Level{
			{Price: 650, Source: confluence.LevelSourceGEX, Rank: 1},
			{Price: 645, Source: confluence.LevelSourceGEX, Rank: 2},
			{Price: 640, Source: confluence.LevelSourceGEX, Rank: 3},
			{Price: 590, Source: confluence.LevelSourceDEX, Rank: 4},
		},
		Resistance: []confluence.Level{{Price: 680, Rank: 1}},
	}
	geo := signals.TradeGeometry(spot, levels)
	assert.True(t, geo.HasDownside)
	assert.InDelta(t, 640.0, geo.StopSupport, 0.01)
	assert.InDelta(t, (spot-640)/spot, geo.DownsidePct, 0.001)
	assert.Less(t, geo.RiskReward, 1.0)
}

func TestTradeGeometry_upsideDownside(t *testing.T) {
	spot := 100.0
	levels := confluence.Levels{
		Support: []confluence.Level{
			{Price: 98, Rank: 1},
			{Price: 96, Rank: 2},
		},
		Resistance: []confluence.Level{{Price: 106, Rank: 1}},
	}
	geo := signals.TradeGeometry(spot, levels)
	assert.True(t, geo.HasUpside)
	assert.InDelta(t, 0.06, geo.UpsidePct, 0.001)
	assert.True(t, geo.HasDownside)
	assert.Greater(t, geo.RiskReward, 1.0)
}

func TestRank1GEXResistance(t *testing.T) {
	levels := confluence.Levels{
		Resistance: []confluence.Level{
			{Price: 105, Source: confluence.LevelSourceDEX, Rank: 1},
			{Price: 108, Source: confluence.LevelSourceGEX, Rank: 2},
		},
	}
	gex, ok := signals.Rank1GEXResistance(levels)
	require.True(t, ok)
	assert.Equal(t, 108.0, gex.Price)
}

func TestStrikeGEX_matchesOptionServiceConvention(t *testing.T) {
	spot := 120.0
	sp := confluence.StrikeProfile{
		Strike:    115,
		CallOI:    100,
		CallGamma: 0.05,
		PutOI:     200,
		PutGamma:  -0.03,
	}
	gex := signals.StrikeGEX(sp, spot)
	// call: 100 * 100 * 0.05 * 120 = 60000
	// put:  200 * 100 * -0.03 * 120 = -72000
	assert.InDelta(t, -12000, gex, 1)
}
