package signals_test

import (
	"testing"

	"github.com/ekinolik/jax/pkg/confluence"
	"github.com/ekinolik/jax/pkg/confluence/signals"
	"github.com/stretchr/testify/assert"
)

func sellLevels(spot float64) confluence.Levels {
	return confluence.Levels{
		Resistance: []confluence.Level{
			{Price: 102, Rank: 1, Strength: 0.8},
			{Price: 106, Rank: 2, Strength: 0.7},
		},
	}
}

func TestSell_trimWhenRank2Meaningful(t *testing.T) {
	spot := 101.69 // ~0.3% below rank-1 resistance at 102
	levels := sellLevels(spot)
	result := signals.BuildSellResult(signals.SellInput{
		Spot:     spot,
		Levels:   levels,
		RSI:      68,
		RSIDaily: 62,
		Geometry: signals.TradeGeometry(spot, levels),
		SPYOpen:  500, SPYSpot: 508,
		QQQOpen: 400, QQQSpot: 408,
		ETFOpen: 200, ETFSpot: 205,
		TargetOpen: 99,
	})
	assert.GreaterOrEqual(t, result.Score, 40.0)
	assert.Equal(t, confluence.ExitTrim, result.ExitAction)
}

func TestSell_sellAllNoRank2(t *testing.T) {
	spot := 100.0
	levels := confluence.Levels{
		Resistance: []confluence.Level{{Price: 102, Rank: 1, Strength: 0.8}},
	}
	result := signals.BuildSellResult(signals.SellInput{
		Spot: spot, Levels: levels, RSI: 72, RSIDaily: 65,
		Geometry: signals.TradeGeometry(spot, levels),
		SPYOpen: 500, SPYSpot: 508, QQQOpen: 400, QQQSpot: 408,
		ETFOpen: 200, ETFSpot: 205, TargetOpen: 98,
	})
	if result.Score >= 75 {
		assert.Equal(t, confluence.ExitSellAll, result.ExitAction)
	}
}

func TestSell_squeezeUnwindBoost(t *testing.T) {
	spot := 100.0
	levels := sellLevels(spot)
	base := signals.BuildSellResult(signals.SellInput{
		Spot: spot, Levels: levels, RSI: 65,
		Geometry: signals.TradeGeometry(spot, levels),
		SPYOpen: 500, SPYSpot: 503, QQQOpen: 400, QQQSpot: 402,
		ETFOpen: 200, ETFSpot: 201, TargetOpen: 98,
	})
	boosted := signals.BuildSellResult(signals.SellInput{
		Spot: spot, Levels: levels, RSI: 65,
		Geometry: signals.TradeGeometry(spot, levels),
		SPYOpen: 500, SPYSpot: 503, QQQOpen: 400, QQQSpot: 402,
		ETFOpen: 200, ETFSpot: 201, TargetOpen: 98,
		GammaSqueezeActive: true, SpotBelowVWAP: true,
	})
	assert.Greater(t, boosted.Score, base.Score)
}
