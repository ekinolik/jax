package confluence_test

import (
	"testing"
	"time"

	"github.com/ekinolik/jax/pkg/confluence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadSettings(t *testing.T) {
	settings, err := confluence.LoadSettings("../../confluence-configs/settings.yaml")
	require.NoError(t, err)
	require.NotNil(t, settings)

	assert.Contains(t, settings.PrefetchWatchlist, "SPY")
	assert.Contains(t, settings.DualExpirationTickers, "SPY")
	assert.Equal(t, 5, settings.MaxActiveTickers)
	assert.True(t, settings.UsesDualExpiration("SPY"))
	assert.False(t, settings.UsesDualExpiration("NVDA"))
}

func TestSettingsIsRTH(t *testing.T) {
	settings, err := confluence.LoadSettings("../../confluence-configs/settings.yaml")
	require.NoError(t, err)

	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	open := time.Date(2024, 3, 11, 10, 0, 0, 0, loc)
	closed := time.Date(2024, 3, 11, 8, 0, 0, 0, loc)

	openOK, err := settings.IsRTH(open)
	require.NoError(t, err)
	assert.True(t, openOK)

	closedOK, err := settings.IsRTH(closed)
	require.NoError(t, err)
	assert.False(t, closedOK)

	saturday := time.Date(2024, 3, 16, 10, 0, 0, 0, loc)
	saturdayOK, err := settings.IsRTH(saturday)
	require.NoError(t, err)
	assert.False(t, saturdayOK)
}

func TestLoadSICSectors(t *testing.T) {
	sectors, err := confluence.LoadSICSectors("../../confluence-configs/sic_sectors.yaml")
	require.NoError(t, err)

	assert.Equal(t, "SMH", sectors.ResolveSectorETF("3674", ""))
	assert.Equal(t, "XLE", sectors.ResolveSectorETF("", "PETROLEUM REFINING"))
	assert.Equal(t, "IWM", sectors.ResolveSectorETF("9999", "UNKNOWN"))
}
