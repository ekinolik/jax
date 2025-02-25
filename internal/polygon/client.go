package polygon

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/ekinolik/jax/internal/config"
	polygon "github.com/polygon-io/client-go/rest"
	"github.com/polygon-io/client-go/rest/iter"
	"github.com/polygon-io/client-go/rest/models"
)

type Chain map[string]map[models.Date]map[string]models.OptionContractSnapshot

// PolygonAPI defines the interface for Polygon.io API operations
type PolygonAPI interface {
	ListOptionsChainSnapshot(context.Context, *models.ListOptionsChainParams, ...models.RequestOption) *iter.Iter[models.OptionContractSnapshot]
	ListAggs(context.Context, *models.ListAggsParams, ...models.RequestOption) *iter.Iter[models.Agg]
}

type Client struct {
	client *polygon.Client
}

func NewClient(cfg *config.Config) *Client {
	return &Client{
		client: polygon.New(cfg.PolygonAPIKey),
	}
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
	res, err := c.client.GetLastTrade(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("polygon API error: %w", err)
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

	iter := c.client.ListAggs(ctx, params)
	var aggs []models.Agg

	for iter.Next() {
		aggs = append(aggs, iter.Item())
	}

	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("polygon API aggregates error: %w", err)
	}

	return aggs, nil
}
