package polygon

import (
	"context"
	"fmt"
	"time"

	"github.com/massive-com/client-go/v2/rest/models"
)

// GetRSI fetches the latest RSI value without caching.
// Uses minute timespan, limit=1, order=desc per confluence requirements.
func (c *Client) GetRSI(ctx context.Context, ticker string, window int) (float64, time.Time, error) {
	if window <= 0 {
		window = 14
	}

	limit := 1
	order := models.Desc
	timespan := models.Minute
	seriesType := models.Close
	params := models.GetRSIParams{Ticker: ticker}.
		WithWindow(window).
		WithTimespan(timespan).
		WithSeriesType(seriesType).
		WithLimit(limit).
		WithOrder(order)

	res, err := c.client.GetRSI(ctx, params)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("massive RSI API error: %w", err)
	}
	if len(res.Results.Values) == 0 {
		return 0, time.Time{}, fmt.Errorf("no RSI values returned for %s", ticker)
	}

	latest := res.Results.Values[0]
	return latest.Value, time.Time(latest.Timestamp), nil
}
