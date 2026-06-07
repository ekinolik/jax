package polygon_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/ekinolik/jax/internal/polygon"
	"github.com/massive-com/client-go/v2/rest/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseOptionSnapshots(t *testing.T) {
	data, err := os.ReadFile("testdata/option_chain_snapshot.json")
	require.NoError(t, err)

	var snapshots []models.OptionContractSnapshot
	require.NoError(t, json.Unmarshal(data, &snapshots))

	asOf := time.Date(2024, 3, 11, 15, 0, 0, 0, time.UTC)
	slice := polygon.ParseOptionSnapshots("SPY", "2024-03-15", snapshots, true, asOf, asOf, 1.0, 0.65)

	require.NotNil(t, slice)
	assert.Equal(t, "SPY", slice.Ticker)
	assert.Equal(t, "2024-03-15", slice.Expiration)
	assert.True(t, slice.IsMonthly)
	assert.Equal(t, float32(1.0), slice.ExpiryWeight)
	assert.Equal(t, asOf, slice.OIAsOf)
	assert.Equal(t, asOf, slice.GreeksAsOf)
	require.Len(t, slice.Strikes, 2)

	assert.Equal(t, float32(500), slice.Strikes[0].Strike)
	assert.Equal(t, uint32(1200), slice.Strikes[0].CallOI)
	assert.Equal(t, uint32(980), slice.Strikes[0].PutOI)
	assert.InDelta(t, 0.55, float64(slice.Strikes[0].CallDelta), 0.001)
	assert.InDelta(t, -0.45, float64(slice.Strikes[0].PutDelta), 0.001)
	assert.InDelta(t, 0.02, float64(slice.Strikes[0].CallGamma), 0.001)
	assert.InDelta(t, 0.018, float64(slice.Strikes[0].PutGamma), 0.001)

	assert.Equal(t, float32(505), slice.Strikes[1].Strike)
	assert.Equal(t, uint32(640), slice.Strikes[1].CallOI)
	assert.Equal(t, uint32(0), slice.Strikes[1].PutOI)
}

func TestParseOptionSnapshotsWithoutOI(t *testing.T) {
	snapshots := []models.OptionContractSnapshot{
		{
			Details: models.OptionDetails{
				ContractType:   "call",
				StrikePrice:    100,
				ExpirationDate: models.Date(time.Date(2024, 3, 8, 0, 0, 0, 0, time.UTC)),
			},
			Greeks:       models.Greeks{Delta: 0.5, Gamma: 0.01},
			OpenInterest: 500,
		},
	}

	asOf := time.Now().UTC()
	slice := polygon.ParseOptionSnapshots("QQQ", "2024-03-08", snapshots, false, asOf, asOf, 1.0, 0.65)
	require.Len(t, slice.Strikes, 1)
	assert.Equal(t, uint32(0), slice.Strikes[0].CallOI)
	assert.InDelta(t, 0.5, float64(slice.Strikes[0].CallDelta), 0.001)
	assert.False(t, slice.IsMonthly)
	assert.Equal(t, float32(0.65), slice.ExpiryWeight)
}
