package polygon

import (
	"context"
	"fmt"
	"time"

	"github.com/massive-com/client-go/v2/rest/models"
)

const (
	rsiMinValid = 1.0
	rsiMaxValid = 99.0
)

// ValidRSI reports whether v is in the conventional RSI range (1–99).
// Values outside this band usually indicate bad upstream indicator data
// (e.g. synthetic flat minute bars) rather than a true extreme reading.
func ValidRSI(v float64) bool {
	return v >= rsiMinValid && v <= rsiMaxValid
}

// GetRSI fetches the latest RSI value without caching.
// timespan should be "minute" or "day"; uses limit=1, order=desc.
func (c *Client) GetRSI(ctx context.Context, ticker string, window int, timespan string) (float64, time.Time, error) {
	if window <= 0 {
		window = 14
	}
	if timespan == "" {
		timespan = "minute"
	}

	limit := 1
	order := models.Desc
	ts := models.Timespan(timespan)
	seriesType := models.Close
	params := models.GetRSIParams{Ticker: ticker}.
		WithWindow(window).
		WithTimespan(ts).
		WithSeriesType(seriesType).
		WithLimit(limit).
		WithOrder(order)

	var res *models.GetRSIResponse
	err := WithRetry(ctx, c.retry, func(callCtx context.Context) error {
		var callErr error
		res, callErr = c.client.GetRSI(callCtx, params)
		if callErr != nil {
			return fmt.Errorf("massive RSI API error: %w", callErr)
		}
		return nil
	})
	if err != nil {
		return 0, time.Time{}, err
	}
	if len(res.Results.Values) == 0 {
		return 0, time.Time{}, fmt.Errorf("no RSI values returned for %s", ticker)
	}

	latest := res.Results.Values[0]
	if !ValidRSI(latest.Value) {
		return 0, time.Time{}, fmt.Errorf("RSI out of range for %s (%s): %.2f", ticker, timespan, latest.Value)
	}
	return latest.Value, time.Time(latest.Timestamp), nil
}
