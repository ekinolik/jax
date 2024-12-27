package service

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	marketv1 "github.com/ekinolik/jax/api/proto/market/v1"
	"github.com/ekinolik/jax/internal/config"
	"github.com/ekinolik/jax/internal/polygon"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
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
	// Get client IP if available
	clientIP := "unknown"
	if p, ok := peer.FromContext(ctx); ok {
		clientIP = p.Addr.String()
	}

	// Log request with parameters
	log.Printf("[REQUEST] GetLastTrade - client_ip=%s ticker=%s", clientIP, req.Ticker)

	// Get cached data or fetch from Polygon
	lastTrade, cacheHit, err := s.client.GetLastTrade(req.Ticker)
	if err != nil {
		log.Printf("[ERROR] GetLastTrade failed - client_ip=%s ticker=%s error=%q",
			clientIP, req.Ticker, err.Error())
		return nil, err
	}

	// Set cache metadata
	header := metadata.Pairs(
		"x-cache-hit", fmt.Sprintf("%v", cacheHit),
		"x-cache-ttl", fmt.Sprintf("%d", int(s.client.GetMarketCacheTTL().Seconds())),
	)
	grpc.SetHeader(ctx, header)

	// Convert to response
	response := &marketv1.GetLastTradeResponse{
		Price:     lastTrade.Results.Price,
		Size:      float64(lastTrade.Results.Size),
		Timestamp: time.Time(lastTrade.Results.ParticipantTimestamp).Unix(),
		Exchange:  strconv.Itoa(int(lastTrade.Results.Exchange)),
	}

	// Log successful response
	log.Printf("[RESPONSE] GetLastTrade - client_ip=%s ticker=%s price=%.2f size=%.0f exchange=%s cache_hit=%v",
		clientIP, req.Ticker, response.Price, response.Size, response.Exchange, cacheHit)

	return response, nil
}
