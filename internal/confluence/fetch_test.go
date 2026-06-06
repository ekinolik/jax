package confluence

import (
	"testing"
	"time"

	pkgconfluence "github.com/ekinolik/jax/pkg/confluence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAllOIReady(t *testing.T) {
	dir := t.TempDir()
	cache, err := NewOICache(dir)
	require.NoError(t, err)

	date := time.Date(2024, 6, 7, 0, 0, 0, 0, time.UTC)
	exps := []time.Time{date.AddDate(0, 0, 7)}
	assert.False(t, AllOIReady(cache, "SPY", date, exps))

	slice := &pkgconfluence.OptionSlice{Ticker: "SPY", Expiration: exps[0].Format("2006-01-02")}
	require.NoError(t, cache.StoreOI("SPY", date, exps[0].Format("2006-01-02"), slice))
	assert.True(t, AllOIReady(cache, "SPY", date, exps))
}

func TestMergeSliceOI_nilSafe(t *testing.T) {
	oi := &pkgconfluence.OptionSlice{Ticker: "X"}
	assert.Equal(t, oi, MergeSliceOI(nil, oi))
	greeks := &pkgconfluence.OptionSlice{Ticker: "X"}
	assert.Equal(t, greeks, MergeSliceOI(greeks, nil))
}

func TestStrikeBand_defaults(t *testing.T) {
	low, high := StrikeBand(200)
	assert.Less(t, low, 200.0)
	assert.Greater(t, high, 200.0)
}
