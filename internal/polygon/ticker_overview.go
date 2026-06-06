package polygon

import (
	"context"
	"fmt"

	"github.com/massive-com/client-go/v2/rest/models"
)

// TickerOverview holds reference data used for sector resolution.
type TickerOverview struct {
	Ticker         string
	SICCode        string
	SICDescription string
	Name           string
}

// GetTickerOverview fetches ticker reference details including SIC metadata.
func (c *Client) GetTickerOverview(ctx context.Context, ticker string) (*TickerOverview, error) {
	params := &models.GetTickerDetailsParams{Ticker: ticker}
	var res *models.GetTickerDetailsResponse
	err := WithRetry(ctx, c.retry, func(callCtx context.Context) error {
		var callErr error
		res, callErr = c.client.GetTickerDetails(callCtx, params)
		if callErr != nil {
			return fmt.Errorf("massive ticker overview API error: %w", callErr)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if res.Results.Ticker == "" {
		return nil, fmt.Errorf("no ticker overview returned for %s", ticker)
	}

	return &TickerOverview{
		Ticker:         res.Results.Ticker,
		SICCode:        res.Results.SICCode,
		SICDescription: res.Results.SICDescription,
		Name:           res.Results.Name,
	}, nil
}
