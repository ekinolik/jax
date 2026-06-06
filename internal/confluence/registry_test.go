package confluence_test

import (
	"testing"
	"time"

	"github.com/ekinolik/jax/internal/confluence"
	pkgconfluence "github.com/ekinolik/jax/pkg/confluence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegistryMaxActive(t *testing.T) {
	reg := confluence.NewRegistry(2)

	_, err := reg.Subscribe("SPY")
	require.NoError(t, err)
	_, err = reg.Subscribe("QQQ")
	require.NoError(t, err)
	_, err = reg.Subscribe("NVDA")
	require.Error(t, err)
}

func TestRegistryIdleDoesNotBlockCap(t *testing.T) {
	reg := confluence.NewRegistry(1)

	_, err := reg.Subscribe("SPY")
	require.NoError(t, err)
	reg.Unsubscribe("SPY")

	_, err = reg.Subscribe("QQQ")
	require.NoError(t, err)
}

func TestRegistryRefCount(t *testing.T) {
	reg := confluence.NewRegistry(5)

	entry, err := reg.Subscribe("SPY")
	require.NoError(t, err)
	assert.Equal(t, 1, entry.SubscriberCount)

	entry, err = reg.Subscribe("SPY")
	require.NoError(t, err)
	assert.Equal(t, 2, entry.SubscriberCount)

	reg.Unsubscribe("SPY")
	entry, ok := reg.Get("SPY")
	require.True(t, ok)
	assert.Equal(t, 1, entry.SubscriberCount)

	reg.Unsubscribe("SPY")
	entry, ok = reg.Get("SPY")
	require.True(t, ok)
	assert.Equal(t, 0, entry.SubscriberCount)
	assert.False(t, entry.IdleSince.IsZero())
}

func TestOICacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONFLUENCE_CACHE_DIR", dir)

	cache, err := confluence.NewOICache("")
	require.NoError(t, err)

	date := time.Date(2024, 3, 11, 0, 0, 0, 0, time.UTC)
	expiration := "2024-03-15"
	slice := &pkgconfluence.OptionSlice{
		Ticker:     "SPY",
		Expiration: expiration,
		Strikes: []pkgconfluence.StrikeProfile{
			{Strike: 500, CallOI: 100, PutOI: 200},
		},
		OIAsOf: date,
	}

	assert.False(t, cache.HasOI("SPY", date, expiration))
	require.NoError(t, cache.StoreOI("SPY", date, expiration, slice))
	assert.True(t, cache.HasOI("SPY", date, expiration))

	loaded, err := cache.LoadOI("SPY", date, expiration)
	require.NoError(t, err)
	assert.Equal(t, slice.Ticker, loaded.Ticker)
	require.Len(t, loaded.Strikes, 1)
	assert.Equal(t, uint32(100), loaded.Strikes[0].CallOI)
}

func TestOICacheRejectsInvalidTicker(t *testing.T) {
	dir := t.TempDir()
	cache, err := confluence.NewOICache(dir)
	require.NoError(t, err)

	date := time.Date(2024, 3, 11, 0, 0, 0, 0, time.UTC)
	slice := &pkgconfluence.OptionSlice{Ticker: "BAD", Expiration: "2024-03-15"}
	err = cache.StoreOI("../../etc/passwd", date, "2024-03-15", slice)
	require.Error(t, err)
}
