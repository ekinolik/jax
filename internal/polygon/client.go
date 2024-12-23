package polygon

import (
	"context"
	"strconv"

	polygonrest "github.com/polygon-io/client-go/rest"
	"github.com/polygon-io/client-go/rest/iter"
	"github.com/polygon-io/client-go/rest/models"
)

type Chain map[string]map[models.Date]map[string]models.OptionContractSnapshot

// PolygonAPI defines the interface for Polygon.io API operations
type PolygonAPI interface {
	ListOptionsChainSnapshot(context.Context, *models.ListOptionsChainParams, ...models.RequestOption) *iter.Iter[models.OptionContractSnapshot]
}

type Client struct {
	apiKey string
	client PolygonAPI
}

func NewClient(apiKey string) *Client {
	return &Client{
		apiKey: apiKey,
		client: polygonrest.New(apiKey),
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
