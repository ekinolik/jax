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

	iter := c.client.ListOptionsContracts(ctx, params)
	seen := make(map[int64]struct{})
	var dates []time.Time

	for iter.Next() {
		contract := iter.Item()
		d := confluence.DateOnly(time.Time(contract.ExpirationDate))
		if _, ok := seen[d.Unix()]; ok {
			continue
		}
		seen[d.Unix()] = struct{}{}
		dates = append(dates, d)
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("massive list option contracts error: %w", err)
	}
	return dates, nil
}
