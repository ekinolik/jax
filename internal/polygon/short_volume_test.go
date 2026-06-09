package polygon_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Massive returns total_volume as a float for high-volume tickers; we only decode fields we use.
func TestShortVolumeResponse_ignoresFloatVolumeFields(t *testing.T) {
	body := `{"results":[{"ticker":"SNDK","date":"2026-06-06","short_volume_ratio":45.2,"total_volume":4796249.0}]}`
	var resp struct {
		Results []struct {
			Date             *string  `json:"date,omitempty"`
			ShortVolumeRatio *float64 `json:"short_volume_ratio,omitempty"`
		} `json:"results,omitempty"`
	}
	err := json.Unmarshal([]byte(body), &resp)
	require.NoError(t, err)
	require.Len(t, resp.Results, 1)
	require.NotNil(t, resp.Results[0].ShortVolumeRatio)
	assert.InDelta(t, 45.2, *resp.Results[0].ShortVolumeRatio, 0.001)
	assert.Equal(t, "2026-06-06", *resp.Results[0].Date)
}
