package polygon

import (
	"context"
	"fmt"
	"time"

	"github.com/massive-com/client-go/v2/rest/models"
)

// ShortVolumeData holds prior-session short volume ratio.
type ShortVolumeData struct {
	ShortVolumeRatio float64
	Date             time.Time
}

// GetShortVolumeRatio fetches the most recent short volume ratio for a ticker.
func (c *Client) GetShortVolumeRatio(ctx context.Context, ticker string) (*ShortVolumeData, error) {
	params := models.ListShortVolumeParams{}.
		WithTicker(models.EQ, ticker).
		WithOrder(models.Desc).
		WithLimit(1)

	var item models.ShortVolume
	err := WithRetry(ctx, c.retry, func(callCtx context.Context) error {
		iter := c.client.ListShortVolume(callCtx, params)
		if !iter.Next() {
			if iterErr := iter.Err(); iterErr != nil {
				return fmt.Errorf("massive short volume API error: %w", iterErr)
			}
			return fmt.Errorf("no short volume data for %s", ticker)
		}
		item = iter.Item()
		if iterErr := iter.Err(); iterErr != nil {
			return fmt.Errorf("massive short volume API error: %w", iterErr)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

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
