package service

import (
	"context"

	dexv1 "github.com/ekinolik/jax/api/proto/dex/v1"
	"github.com/ekinolik/jax/internal/polygon"
	"github.com/polygon-io/client-go/rest/models"
)

type DexService struct {
	dexv1.UnimplementedDexServiceServer
	polygonClient *polygon.Client
}

func NewDexService(polygonClient *polygon.Client) *DexService {
	return &DexService{
		polygonClient: polygonClient,
	}
}

func (s *DexService) GetDex(ctx context.Context, req *dexv1.GetDexRequest) (*dexv1.GetDexResponse, error) {
	var startStrike, endStrike *float64
	if req.StartStrikePrice != nil {
		startStrike = req.StartStrikePrice
	}
	if req.EndStrikePrice != nil {
		endStrike = req.EndStrikePrice
	}

	spotPrice, chains, err := s.polygonClient.GetOptionData(req.UnderlyingAsset, startStrike, endStrike)
	if err != nil {
		return nil, err
	}

	strikePrices := make(map[string]*dexv1.ExpirationDateMap)
	for strike, expirations := range chains {
		strikePrices[strike] = &dexv1.ExpirationDateMap{
			ExpirationDates: make(map[string]*dexv1.OptionTypeMap),
		}

		for expDate, contracts := range expirations {
			strikePrices[strike].ExpirationDates[expDate] = &dexv1.OptionTypeMap{
				OptionTypes: make(map[string]*dexv1.DexValue),
			}

			for optType, contract := range contracts {
				dex := calculateDelta(contract)
				strikePrices[strike].ExpirationDates[expDate].OptionTypes[optType] = &dexv1.DexValue{
					Value: dex,
				}
			}
		}
	}

	response := &dexv1.GetDexResponse{
		SpotPrice:    spotPrice,
		StrikePrices: strikePrices,
	}

	return response, nil
}

func calculateDelta(contract models.OptionContractSnapshot) float64 {
	return contract.OpenInterest * float64(contract.Details.SharesPerContract) * contract.Greeks.Delta * contract.UnderlyingAsset.Price
}
