package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"time"

	dexv1 "github.com/ekinolik/jax/api/proto/dex/v1"
	"github.com/ekinolik/jax/internal/config"
	"github.com/ekinolik/jax/internal/polygon"
	"github.com/polygon-io/client-go/rest/models"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type DexService struct {
	dexv1.UnimplementedDexServiceServer
	client *polygon.CachedClient
}

func NewDexService(cfg *config.Config) *DexService {
	return &DexService{
		client: polygon.NewCachedClient(cfg),
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

	// Fetch all option data from Polygon (without strike price filters)
	spotPrice, chains, err := s.client.GetOptionData(req.UnderlyingAsset, nil, nil)
	if err != nil {
		st := status.Convert(err)
		log.Printf("[ERROR] GetDex failed - code: %v, message: %v", st.Code(), st.Message())
		return nil, err
	}

	// Process chains into response format, filtering by strike price range
	strikePrices := s.processOptionChains(chains, req.StartStrikePrice, req.EndStrikePrice)

	// Build response
	response := &dexv1.GetDexResponse{
		SpotPrice:    spotPrice,
		StrikePrices: strikePrices,
	}

	// Add cache expiration to response metadata
	if cacheEntry := s.client.GetCacheEntry(req.UnderlyingAsset); cacheEntry != nil {
		grpc.SetHeader(ctx, metadata.Pairs(
			"cache-expires-at", fmt.Sprintf("%d", cacheEntry.ExpiresAt.Unix()),
		))
	}

	// Log response status
	log.Printf("[RESPONSE] GetDex successful - spot price: %v, strike prices count: %d",
		spotPrice, len(strikePrices))

	return response, nil
}

func (s *DexService) GetDexByStrikes(ctx context.Context, req *dexv1.GetDexByStrikesRequest) (*dexv1.GetDexResponse, error) {
	// Log request
	reqJSON, _ := json.Marshal(map[string]interface{}{
		"underlyingAsset": req.UnderlyingAsset,
		"numStrikes":      req.NumStrikes,
	})
	log.Printf("[REQUEST] GetDexByStrikes - %s", string(reqJSON))

	// Fetch all option data from Polygon (without strike price filters)
	spotPrice, chains, err := s.client.GetOptionData(req.UnderlyingAsset, nil, nil)
	if err != nil {
		st := status.Convert(err)
		log.Printf("[ERROR] GetDexByStrikes failed - code: %v, message: %v", st.Code(), st.Message())
		return nil, err
	}

	// Convert all strikes to float64 and sort them
	var strikes []float64
	for strike := range chains {
		strikePrice, err := strconv.ParseFloat(strike, 64)
		if err != nil {
			continue
		}
		strikes = append(strikes, strikePrice)
	}
	sort.Float64s(strikes)

	// Find the index of the closest strike to spot price
	spotIndex := 0
	minDiff := math.Abs(strikes[0] - spotPrice)
	for i, strike := range strikes {
		diff := math.Abs(strike - spotPrice)
		if diff < minDiff {
			minDiff = diff
			spotIndex = i
		}
	}

	// Calculate the range of strikes to include
	numStrikes := int(req.NumStrikes)
	strikesBelow := (numStrikes - 1) / 2
	strikesAbove := numStrikes - strikesBelow // This will be (n+1)/2 for odd numbers, and n/2 + 1 for even numbers
	startIndex := spotIndex - strikesBelow
	endIndex := spotIndex + strikesAbove - 1 // Subtract 1 since we want to include the spot price only once

	// Adjust indices if they're out of bounds
	if startIndex < 0 {
		// Not enough strikes below, try to get more from above
		extraNeeded := -startIndex
		startIndex = 0
		endIndex = min(len(strikes)-1, endIndex+extraNeeded)
	}
	if endIndex >= len(strikes) {
		// Not enough strikes above, try to get more from below
		extraNeeded := endIndex - (len(strikes) - 1)
		endIndex = len(strikes) - 1
		startIndex = max(0, startIndex-extraNeeded)
	}

	// Filter chains to only include the selected strikes
	filteredChains := make(polygon.Chain)
	for i := startIndex; i <= endIndex; i++ {
		strikeStr := strconv.FormatFloat(strikes[i], 'f', -1, 64)
		if expirations, ok := chains[strikeStr]; ok {
			filteredChains[strikeStr] = expirations
		}
	}

	// Process chains into response format
	strikePrices := s.processOptionChains(filteredChains, nil, nil)

	// Build response
	response := &dexv1.GetDexResponse{
		SpotPrice:    spotPrice,
		StrikePrices: strikePrices,
	}

	// Add cache expiration to response metadata
	if cacheEntry := s.client.GetCacheEntry(req.UnderlyingAsset); cacheEntry != nil {
		grpc.SetHeader(ctx, metadata.Pairs(
			"cache-expires-at", fmt.Sprintf("%d", cacheEntry.ExpiresAt.Unix()),
		))
	}

	// Log response status
	log.Printf("[RESPONSE] GetDexByStrikes successful - spot price: %v, strike prices count: %d",
		spotPrice, len(strikePrices))

	return response, nil
}

// processOptionChains converts the polygon chains into the response format
func (s *DexService) processOptionChains(chains polygon.Chain, startStrike, endStrike *float64) map[string]*dexv1.ExpirationDateMap {
	strikePrices := make(map[string]*dexv1.ExpirationDateMap)

	for strike, expirations := range chains {
		strikePrice, err := strconv.ParseFloat(strike, 64)
		if err != nil {
			continue
		}

		// Filter strikes based on range
		if startStrike != nil && strikePrice < *startStrike {
			continue
		}
		if endStrike != nil && strikePrice > *endStrike {
			continue
		}

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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
