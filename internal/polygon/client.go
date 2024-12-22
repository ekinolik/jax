package polygon

import (
	"context"
	"strconv"

	polygonrest "github.com/polygon-io/client-go/rest"
	"github.com/polygon-io/client-go/rest/iter"
	"github.com/polygon-io/client-go/rest/models"
)

type Chain map[string]map[string]map[string]models.OptionContractSnapshot

type Client struct {
	apiKey string
	client *polygonrest.Client
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
	chains, err := createChains(iter)
	if err != nil {
		return 0, nil, err
	}

	// Get the first contract's underlying price (they should all be the same)
	var spotPrice float64
	for _, expirations := range chains {
		for _, contracts := range expirations {
			for _, contract := range contracts {
				spotPrice = contract.UnderlyingAsset.Price
				goto FOUND_PRICE
			}
		}
	}
FOUND_PRICE:

	return spotPrice, chains, nil
}

func createChains(chainIter *iter.Iter[models.OptionContractSnapshot]) (Chain, error) {
	chains := make(Chain)

	for chainIter.Next() {
		current := chainIter.Item()
		exp := current.Details.ExpirationDate
		expDateByte, _ := exp.MarshalJSON()
		expDate := string(expDateByte[1 : len(expDateByte)-1])

		strikePrice := strconv.FormatFloat(current.Details.StrikePrice, 'f', -1, 64)
		contractType := current.Details.ContractType

		if _, ok := chains[strikePrice]; !ok {
			chains[strikePrice] = map[string]map[string]models.OptionContractSnapshot{}
		}
		if _, ok := chains[strikePrice][expDate]; !ok {
			chains[strikePrice][expDate] = map[string]models.OptionContractSnapshot{}
		}

		chains[strikePrice][expDate][contractType] = current
	}

	return chains, chainIter.Err()
}
