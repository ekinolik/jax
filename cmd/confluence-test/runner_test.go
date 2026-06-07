package main

import (
	"testing"
	"time"

	intconfluence "github.com/ekinolik/jax/internal/confluence"
	pkgconfluence "github.com/ekinolik/jax/pkg/confluence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeSliceOI_overlaysCachedOI(t *testing.T) {
	greeks := &pkgconfluence.OptionSlice{
		Strikes: []pkgconfluence.StrikeProfile{
			{Strike: 100, CallDelta: 0.5, PutDelta: -0.4},
			{Strike: 105, CallDelta: 0.4, PutDelta: -0.3},
		},
		GreeksAsOf: time.Date(2024, 6, 7, 14, 0, 0, 0, time.UTC),
	}
	oi := &pkgconfluence.OptionSlice{
		Strikes: []pkgconfluence.StrikeProfile{
			{Strike: 100, CallOI: 1200, PutOI: 800},
		},
		OIAsOf: time.Date(2024, 6, 7, 8, 0, 0, 0, time.UTC),
	}

	merged := intconfluence.MergeSliceOI(greeks, oi)
	require.NotNil(t, merged)
	require.Len(t, merged.Strikes, 2)
	assert.Equal(t, uint32(1200), merged.Strikes[0].CallOI)
	assert.Equal(t, uint32(800), merged.Strikes[0].PutOI)
	assert.InDelta(t, 0.5, float64(merged.Strikes[0].CallDelta), 0.001)
	assert.Equal(t, uint32(0), merged.Strikes[1].CallOI)
}

func TestStrikeBand(t *testing.T) {
	low, high := intconfluence.StrikeBand(100)
	assert.Equal(t, 85.0, low)
	assert.Equal(t, 115.0, high)
}
