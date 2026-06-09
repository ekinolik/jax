package polygon

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/ekinolik/jax/internal/config"
	massive "github.com/massive-com/client-go/v2/rest"
	"github.com/massive-com/client-go/v2/rest/iter"
	"github.com/massive-com/client-go/v2/rest/models"
)

type Chain map[string]map[models.Date]map[string]models.OptionContractSnapshot

// PolygonAPI defines the interface for Polygon.io API operations
type PolygonAPI interface {
	ListOptionsChainSnapshot(context.Context, *models.ListOptionsChainParams, ...models.RequestOption) *iter.Iter[models.OptionContractSnapshot]
	ListAggs(context.Context, *models.ListAggsParams, ...models.RequestOption) *iter.Iter[models.Agg]
}

type Client struct {
	client *massive.Client
	retry  RetryConfig
}

func NewClient(cfg *config.Config) *Client {
	mc := massive.New(cfg.PolygonAPIKey)
	// Disable resty internal retries; confluence uses WithRetry with ctx-aware backoff.
	mc.HTTP.SetRetryCount(0)
	mc.HTTP.SetTimeout(25 * time.Second)

	return &Client{
		client: mc,
		retry:  DefaultRetryConfig(),
	}
}

// SetRetryConfig configures exponential backoff for confluence REST calls.
func (c *Client) SetRetryConfig(cfg RetryConfig) {
	c.retry = cfg.normalized()
}

func (c *Client) GetOptionData(underlyingAsset string, startStrike, endStrike *float64) (float64, Chain, error) {
	params := &models.ListOptionsChainParams{
		UnderlyingAsset: underlyingAsset,
	}

	if startStrike != nil {
		params = params.WithStrikePrice("gte", *startStrike)
	}
	if endStrike != nil {
		params = params.WithStrikePrice("lte", *endStrike)
	}

	iter := c.client.ListOptionsChainSnapshot(context.Background(), params)
	chains := make(Chain)

	var spotPrice float64
	for iter.Next() {
		current := iter.Item()
		strikePrice := strconv.FormatFloat(current.Details.StrikePrice, 'f', -1, 64)
		contractType := current.Details.ContractType
		expDate := current.Details.ExpirationDate

		if _, ok := chains[strikePrice]; !ok {
			chains[strikePrice] = map[models.Date]map[string]models.OptionContractSnapshot{}
		}
		if _, ok := chains[strikePrice][expDate]; !ok {
			chains[strikePrice][expDate] = map[string]models.OptionContractSnapshot{}
		}

		chains[strikePrice][expDate][contractType] = current
		spotPrice = current.UnderlyingAsset.Price
	}

	if err := iter.Err(); err != nil {
		return 0, nil, err
	}

	return spotPrice, chains, nil
}

func (c *Client) GetLastTrade(ctx context.Context, params *models.GetLastTradeParams) (*models.GetLastTradeResponse, error) {
	var res *models.GetLastTradeResponse
	err := WithRetry(ctx, c.retry, func(callCtx context.Context) error {
		var callErr error
		res, callErr = c.client.GetLastTrade(callCtx, params)
		if callErr != nil {
			return fmt.Errorf("polygon API error: %w", callErr)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (c *Client) GetAggregates(ctx context.Context, ticker string, multiplier int, timespan string, from, to int64, adjusted bool) ([]models.Agg, error) {
	params := &models.ListAggsParams{
		Ticker:     ticker,
		Multiplier: multiplier,
		Timespan:   models.Timespan(timespan),
		From:       models.Millis(time.Unix(0, from*int64(time.Millisecond))),
		To:         models.Millis(time.Unix(0, to*int64(time.Millisecond))),
		Adjusted:   &adjusted,
	}

	var aggs []models.Agg
	err := WithRetry(ctx, c.retry, func(callCtx context.Context) error {
		iter := c.client.ListAggs(callCtx, params)
		aggs = aggs[:0]
		for iter.Next() {
			aggs = append(aggs, iter.Item())
		}
		if iterErr := iter.Err(); iterErr != nil {
			return fmt.Errorf("polygon API aggregates error: %w", iterErr)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return aggs, nil
}
