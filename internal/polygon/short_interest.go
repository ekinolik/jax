package polygon

import (
	"context"
	"fmt"
	"time"

	"github.com/massive-com/client-go/v2/rest/models"
)

// ShortInterestData holds cached short interest fundamentals.
type ShortInterestData struct {
	ShortInterest    int64
	DaysToCover      float64
	AvgDailyVolume   int64
	SettlementDate   time.Time
	ShortInterestPct float64
}

// GetShortInterest fetches the latest short interest record for a ticker.
func (c *Client) GetShortInterest(ctx context.Context, ticker string, floatShares float64) (*ShortInterestData, error) {
	params := models.ListShortInterestParams{}.
		WithTicker(models.EQ, ticker).
		WithOrder(models.Desc).
		WithLimit(1)

	var item models.ShortInterest
	err := WithRetry(ctx, c.retry, func(callCtx context.Context) error {
		iter := c.client.ListShortInterest(callCtx, params)
		if !iter.Next() {
			if iterErr := iter.Err(); iterErr != nil {
				return fmt.Errorf("massive short interest API error: %w", iterErr)
			}
			return fmt.Errorf("no short interest data for %s", ticker)
		}
		item = iter.Item()
		if iterErr := iter.Err(); iterErr != nil {
			return fmt.Errorf("massive short interest API error: %w", iterErr)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	out := &ShortInterestData{}
	if item.ShortInterest != nil {
		out.ShortInterest = *item.ShortInterest
	}
	if item.DaysToCover != nil {
		out.DaysToCover = *item.DaysToCover
	}
	if item.AvgDailyVolume != nil {
		out.AvgDailyVolume = *item.AvgDailyVolume
	}
	if item.SettlementDate != nil && *item.SettlementDate != "" {
		if parsed, parseErr := time.Parse("2006-01-02", *item.SettlementDate); parseErr == nil {
			out.SettlementDate = parsed
		}
	}
	if floatShares > 0 && out.ShortInterest > 0 {
		out.ShortInterestPct = float64(out.ShortInterest) / floatShares * 100
	}
	return out, nil
}
