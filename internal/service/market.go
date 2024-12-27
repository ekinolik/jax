package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	marketv1 "github.com/ekinolik/jax/api/proto/market/v1"
	"github.com/ekinolik/jax/internal/config"
	"github.com/ekinolik/jax/internal/polygon"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type MarketService struct {
	marketv1.UnimplementedMarketServiceServer
	client *polygon.CachedClient
}

func NewMarketService(cfg *config.Config) *MarketService {
	return &MarketService{
		client: polygon.NewCachedClient(cfg),
	}
}

func (s *MarketService) GetLastTrade(ctx context.Context, req *marketv1.GetLastTradeRequest) (*marketv1.GetLastTradeResponse, error) {
	// Log request
	reqJSON, _ := json.Marshal(map[string]interface{}{
		"ticker": req.Ticker,
	})
	log.Printf("[REQUEST] GetLastTrade - %s", string(reqJSON))

	// Get cached data or fetch from Polygon
	lastTrade, cacheHit, err := s.client.GetLastTrade(req.Ticker)
	if err != nil {
		log.Printf("[ERROR] GetLastTrade failed - %v", err)
		return nil, err
	}

	// Set cache metadata
	header := metadata.Pairs(
		"x-cache-hit", fmt.Sprintf("%v", cacheHit),
		"x-cache-ttl", fmt.Sprintf("%d", int(s.client.GetMarketCacheTTL().Seconds())),
	)
	grpc.SetHeader(ctx, header)

	// Convert to response
	return &marketv1.GetLastTradeResponse{
		Price:     lastTrade.Results.Price,
		Size:      float64(lastTrade.Results.Size),
		Timestamp: time.Time(lastTrade.Results.ParticipantTimestamp).Unix(),
		Exchange:  strconv.Itoa(int(lastTrade.Results.Exchange)),
	}, nil
}
