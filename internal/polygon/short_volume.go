package polygon

import (
	"context"
	"fmt"
	"time"

	"github.com/massive-com/client-go/v2/rest/models"
)

const listShortVolumePath = "/stocks/v1/short-volume"

// ShortVolumeData holds prior-session short volume ratio.
type ShortVolumeData struct {
	ShortVolumeRatio float64
	Date             time.Time
}

// shortVolumeRow decodes only fields we use. The Massive API may return volume
// counts as floats (e.g. 4796249.0); the SDK ShortVolume type uses *int64 and fails.
type shortVolumeRow struct {
	Date             *string  `json:"date,omitempty"`
	ShortVolumeRatio *float64 `json:"short_volume_ratio,omitempty"`
}

type listShortVolumeAPIResponse struct {
	Results []shortVolumeRow `json:"results,omitempty"`
}

// GetShortVolumeRatio fetches the most recent short volume ratio for a ticker.
func (c *Client) GetShortVolumeRatio(ctx context.Context, ticker string) (*ShortVolumeData, error) {
	params := models.ListShortVolumeParams{}.
		WithTicker(models.EQ, ticker).
		WithOrder(models.Desc).
		WithLimit(1)

	var resp listShortVolumeAPIResponse
	err := WithRetry(ctx, c.retry, func(callCtx context.Context) error {
		if err := c.client.Call(callCtx, "GET", listShortVolumePath, params, &resp); err != nil {
			return fmt.Errorf("massive short volume API error: %w", err)
		}
		if len(resp.Results) == 0 {
			return fmt.Errorf("no short volume data for %s", ticker)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	item := resp.Results[0]
	out := &ShortVolumeData{}
	if item.ShortVolumeRatio != nil {
		out.ShortVolumeRatio = *item.ShortVolumeRatio
	}
	if item.Date != nil && *item.Date != "" {
		if parsed, parseErr := time.Parse("2006-01-02", *item.Date); parseErr == nil {
			out.Date = parsed
		}
	}
	return out, nil
}
