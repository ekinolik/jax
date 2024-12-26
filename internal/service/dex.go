package service

import (
	"context"
	"encoding/json"
	"log"
	"time"

	dexv1 "github.com/ekinolik/jax/api/proto/dex/v1"
	"github.com/ekinolik/jax/internal/polygon"
	"github.com/polygon-io/client-go/rest/models"
	"google.golang.org/grpc/status"
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
	// Log request
	reqJSON, _ := json.Marshal(map[string]interface{}{
		"underlyingAsset":  req.UnderlyingAsset,
		"startStrikePrice": req.StartStrikePrice,
		"endStrikePrice":   req.EndStrikePrice,
	})
	log.Printf("[REQUEST] GetDex - %s", string(reqJSON))

	// Extract strike price range
	startStrike, endStrike := s.extractStrikePriceRange(req)

	// Fetch option data from Polygon
	spotPrice, chains, err := s.polygonClient.GetOptionData(req.UnderlyingAsset, startStrike, endStrike)
	if err != nil {
		st := status.Convert(err)
		log.Printf("[ERROR] GetDex failed - code: %v, message: %v", st.Code(), st.Message())
		return nil, err
	}

	// Process chains into response format
	strikePrices := s.processOptionChains(chains)

	// Build response
	response := &dexv1.GetDexResponse{
		SpotPrice:    spotPrice,
		StrikePrices: strikePrices,
	}

	// Log response status
	log.Printf("[RESPONSE] GetDex successful - spot price: %v, strike prices count: %d",
		spotPrice, len(strikePrices))

	return response, nil
}

// extractStrikePriceRange extracts the strike price range from the request
func (s *DexService) extractStrikePriceRange(req *dexv1.GetDexRequest) (*float64, *float64) {
	var startStrike, endStrike *float64
	if req.StartStrikePrice != nil {
		startStrike = req.StartStrikePrice
	}
	if req.EndStrikePrice != nil {
		endStrike = req.EndStrikePrice
	}
	return startStrike, endStrike
}

// processOptionChains converts the polygon chains into the response format
func (s *DexService) processOptionChains(chains polygon.Chain) map[string]*dexv1.ExpirationDateMap {
	strikePrices := make(map[string]*dexv1.ExpirationDateMap)

	for strike, expirations := range chains {
		strikePrices[strike] = s.processExpirationDates(expirations)
	}

	return strikePrices
}

// processExpirationDates processes all expiration dates for a strike price
func (s *DexService) processExpirationDates(expirations map[models.Date]map[string]models.OptionContractSnapshot) *dexv1.ExpirationDateMap {
	expDateMap := &dexv1.ExpirationDateMap{
		ExpirationDates: make(map[string]*dexv1.OptionTypeMap),
	}

	for expDate, contracts := range expirations {
		expDateStr := formatExpirationDate(expDate)
		expDateMap.ExpirationDates[expDateStr] = s.processOptionTypes(contracts)
	}

	return expDateMap
}

// processOptionTypes processes all option types for an expiration date
func (s *DexService) processOptionTypes(contracts map[string]models.OptionContractSnapshot) *dexv1.OptionTypeMap {
	optionTypeMap := &dexv1.OptionTypeMap{
		OptionTypes: make(map[string]*dexv1.DexValue),
	}

	for optType, contract := range contracts {
		dex := calculateDelta(contract)
		optionTypeMap.OptionTypes[optType] = &dexv1.DexValue{
			Value: dex,
		}
	}

	return optionTypeMap
}

// formatExpirationDate formats the expiration date in the required format
func formatExpirationDate(date models.Date) string {
	return time.Time(date).Format("2006-01-02")
}

// calculateDelta calculates the delta exposure for a single option contract
func calculateDelta(contract models.OptionContractSnapshot) float64 {
	return contract.OpenInterest * float64(contract.Details.SharesPerContract) * contract.Greeks.Delta * contract.UnderlyingAsset.Price
}
