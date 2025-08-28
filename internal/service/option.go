package service

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"time"

	optionv1 "github.com/ekinolik/jax/api/proto/option/v1"
	"github.com/ekinolik/jax/internal/cache"
	"github.com/ekinolik/jax/internal/config"
	"github.com/ekinolik/jax/internal/polygon"
	"github.com/polygon-io/client-go/rest/models"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type OptionService struct {
	optionv1.UnimplementedOptionServiceServer
	client *polygon.CachedClient
}

func NewOptionService(cfg *config.Config, cacheManager cache.Cache) *OptionService {
	client := polygon.NewCachedClient(cfg, cacheManager)
	return &OptionService{
		client: client,
	}
}

/*
func NewOptionService(cfg *config.Config) (*OptionService, error) {
	client, err := polygon.NewCachedClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create cached client: %w", err)
	}
	return &OptionService{
		client: client,
	}, nil
}
*/

type ExposureType int

const (
	DeltaExposure ExposureType = iota
	GammaExposure
)

func (s *OptionService) GetDex(ctx context.Context, req *optionv1.GetDexRequest) (*optionv1.GetDexResponse, error) {
	return s.getExposure(ctx, "GetDex", req.UnderlyingAsset, req.StartStrikePrice, req.EndStrikePrice, nil, DeltaExposure)
}

func (s *OptionService) GetDexByStrikes(ctx context.Context, req *optionv1.GetDexByStrikesRequest) (*optionv1.GetDexResponse, error) {
	return s.getExposure(ctx, "GetDexByStrikes", req.UnderlyingAsset, nil, nil, &req.NumStrikes, DeltaExposure)
}

func (s *OptionService) GetGex(ctx context.Context, req *optionv1.GetDexRequest) (*optionv1.GetDexResponse, error) {
	return s.getExposure(ctx, "GetGex", req.UnderlyingAsset, req.StartStrikePrice, req.EndStrikePrice, nil, GammaExposure)
}

func (s *OptionService) GetGexByStrikes(ctx context.Context, req *optionv1.GetDexByStrikesRequest) (*optionv1.GetDexResponse, error) {
	return s.getExposure(ctx, "GetGexByStrikes", req.UnderlyingAsset, nil, nil, &req.NumStrikes, GammaExposure)
}

func (s *OptionService) getExposure(
	ctx context.Context,
	method string,
	underlyingAsset string,
	startStrikePrice, endStrikePrice *float64,
	numStrikes *int32,
	exposureType ExposureType,
) (*optionv1.GetDexResponse, error) {
	// Log request
	reqParams := map[string]interface{}{
		"underlyingAsset": underlyingAsset,
	}
	if numStrikes != nil {
		reqParams["numStrikes"] = *numStrikes
	} else {
		reqParams["startStrikePrice"] = startStrikePrice
		reqParams["endStrikePrice"] = endStrikePrice
	}
	LogRequest(method, reqParams)

	// Fetch all option data from Polygon (without strike price filters)
	spotPrice, chains, err := s.client.GetOptionData(underlyingAsset, nil, nil)
	if err != nil {
		st := status.Convert(err)
		log.Printf("[ERROR] %s failed - code: %v, message: %v", method, st.Code(), st.Message())
		return nil, err
	}

	var strikePrices map[string]*optionv1.ExpirationDateMap
	if numStrikes != nil {
		// Convert all strikes to float64 and sort them
		var strikes []float64
		for strike := range chains {
			strikePrice, err := strconv.ParseFloat(strike, 64)
			if err != nil {
				continue
			}
			strikes = append(strikes, strikePrice)
		}

		// If no valid strikes are found, return empty response
		if len(strikes) == 0 {
			log.Printf("[WARN] %s - no valid strikes found for %s", method, underlyingAsset)
			return &optionv1.GetDexResponse{
				SpotPrice:    spotPrice,
				StrikePrices: make(map[string]*optionv1.ExpirationDateMap),
			}, nil
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
		strikesBelow := (int(*numStrikes) - 1) / 2
		strikesAbove := int(*numStrikes) - strikesBelow
		startIndex := spotIndex - strikesBelow
		endIndex := spotIndex + strikesAbove - 1

		// Adjust indices if they're out of bounds
		if startIndex < 0 {
			extraNeeded := -startIndex
			startIndex = 0
			endIndex = min(len(strikes)-1, endIndex+extraNeeded)
		}
		if endIndex >= len(strikes) {
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
		strikePrices = s.processOptionChains(filteredChains, nil, nil, exposureType)
	} else {
		strikePrices = s.processOptionChains(chains, startStrikePrice, endStrikePrice, exposureType)
	}

	// Build response
	response := &optionv1.GetDexResponse{
		SpotPrice:    spotPrice,
		StrikePrices: strikePrices,
	}

	// Add cache expiration to response metadata
	if cacheEntry := s.client.GetCacheEntry(underlyingAsset); cacheEntry != nil {
		grpc.SetHeader(ctx, metadata.Pairs(
			"cache-expires-at", fmt.Sprintf("%d", cacheEntry.ExpiresAt.Unix()),
		))
	}

	// Log response status
	log.Printf("[RESPONSE] %s successful - spot price: %v, strike prices count: %d",
		method, spotPrice, len(strikePrices))

	return response, nil
}

// processOptionChains converts the polygon chains into the response format
func (s *OptionService) processOptionChains(chains polygon.Chain, startStrike, endStrike *float64, exposureType ExposureType) map[string]*optionv1.ExpirationDateMap {
	strikePrices := make(map[string]*optionv1.ExpirationDateMap)

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

		strikePrices[strike] = s.processExpirationDates(expirations, exposureType)
	}

	return strikePrices
}

// processExpirationDates processes all expiration dates for a strike price
func (s *OptionService) processExpirationDates(expirations map[models.Date]map[string]models.OptionContractSnapshot, exposureType ExposureType) *optionv1.ExpirationDateMap {
	expDateMap := &optionv1.ExpirationDateMap{
		ExpirationDates: make(map[string]*optionv1.OptionTypeMap),
	}

	for expDate, contracts := range expirations {
		expDateStr := formatExpirationDate(expDate)
		expDateMap.ExpirationDates[expDateStr] = s.processOptionTypes(contracts, exposureType)
	}

	return expDateMap
}

// processOptionTypes processes all option types for an expiration date
func (s *OptionService) processOptionTypes(contracts map[string]models.OptionContractSnapshot, exposureType ExposureType) *optionv1.OptionTypeMap {
	optionTypeMap := &optionv1.OptionTypeMap{
		OptionTypes: make(map[string]*optionv1.DexValue),
	}

	for optType, contract := range contracts {
		var exposure float64
		switch exposureType {
		case DeltaExposure:
			exposure = calculateDelta(contract)
		case GammaExposure:
			exposure = calculateGamma(contract)
		}
		optionTypeMap.OptionTypes[optType] = &optionv1.DexValue{
			Value: exposure,
		}
	}

	return optionTypeMap
}

// calculateGamma calculates the gamma exposure for a single option contract
func calculateGamma(contract models.OptionContractSnapshot) float64 {
	return contract.OpenInterest * float64(contract.Details.SharesPerContract) * contract.Greeks.Gamma * contract.UnderlyingAsset.Price
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
