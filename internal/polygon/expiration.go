package polygon

import (
	"context"
	"fmt"
	"time"

	"github.com/ekinolik/jax/pkg/confluence"
	"github.com/massive-com/client-go/v2/rest/models"
)

// ResolveExpirations fetches available expirations and resolves soonest (+ optional weekly Friday).
func (c *Client) ResolveExpirations(ctx context.Context, ticker string, dualExpiration bool, today time.Time) ([]time.Time, error) {
	dates, err := c.listExpirationDates(ctx, ticker)
	if err != nil {
		return nil, err
	}
	return confluence.ResolveExpirations(dates, today, dualExpiration), nil
}

func (c *Client) listExpirationDates(ctx context.Context, ticker string) ([]time.Time, error) {
	expired := false
	limit := 1000
	params := models.ListOptionsContractsParams{}.
		WithUnderlyingTicker(models.EQ, ticker).
		WithExpired(expired).
		WithLimit(limit)

	var dates []time.Time
	err := WithRetry(ctx, c.retry, func(callCtx context.Context) error {
		iter := c.client.ListOptionsContracts(callCtx, params)
		seen := make(map[int64]struct{})
		dates = dates[:0]
		for {
			if err := callCtx.Err(); err != nil {
				return err
			}
			if !iter.Next() {
				break
			}
			contract := iter.Item()
			d := confluence.DateOnly(time.Time(contract.ExpirationDate))
			if _, ok := seen[d.Unix()]; ok {
				continue
			}
			seen[d.Unix()] = struct{}{}
			dates = append(dates, d)
		}
		if iterErr := iter.Err(); iterErr != nil {
			return fmt.Errorf("massive list option contracts error: %w", iterErr)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return dates, nil
}
