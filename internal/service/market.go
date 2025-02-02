package service

import (
	"context"
	"log"
	"strconv"
	"time"

	marketv1 "github.com/ekinolik/jax/api/proto/market/v1"
	"github.com/ekinolik/jax/internal/config"
	"github.com/ekinolik/jax/internal/polygon"
	"google.golang.org/grpc/status"
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
	LogRequest("GetLastTrade", map[string]interface{}{
		"ticker": req.Ticker,
	})

	// Get last trade from Polygon
	trade, _, err := s.client.GetLastTrade(req.Ticker)
	if err != nil {
		st := status.Convert(err)
		log.Printf("[ERROR] GetLastTrade failed - code: %v, message: %v", st.Code(), st.Message())
		return nil, err
	}

	// Build response
	response := &marketv1.GetLastTradeResponse{
		Price:     trade.Results.Price,
		Size:      float64(trade.Results.Size),
		Timestamp: time.Time(trade.Results.ParticipantTimestamp).Unix(),
		Exchange:  strconv.Itoa(int(trade.Results.Exchange)),
	}

	// Log response status
	log.Printf("[RESPONSE] GetLastTrade successful - price: %v, size: %v",
		response.Price, response.Size)

	return response, nil
}

func (s *MarketService) GetAggregates(ctx context.Context, req *marketv1.GetAggregatesRequest) (*marketv1.GetAggregatesResponse, error) {
	// Log request
	LogRequest("GetAggregates", map[string]interface{}{
		"ticker":     req.Ticker,
		"multiplier": req.Multiplier,
		"timespan":   req.Timespan,
		"from":       req.From,
		"to":         req.To,
		"adjusted":   req.Adjusted,
	})

	// Get aggregates from Polygon
	aggs, _, err := s.client.GetAggregates(req.Ticker, int(req.Multiplier), req.Timespan, req.From, req.To, req.Adjusted)
	if err != nil {
		st := status.Convert(err)
		log.Printf("[ERROR] GetAggregates failed - code: %v, message: %v", st.Code(), st.Message())
		return nil, err
	}

	// Build response
	response := &marketv1.GetAggregatesResponse{
		Bars: make([]*marketv1.GetAggregatesResponse_AggregateBar, len(aggs.Results)),
	}

	for i, agg := range aggs.Results {
		response.Bars[i] = &marketv1.GetAggregatesResponse_AggregateBar{
			Open:         agg.Open,
			High:         agg.High,
			Low:          agg.Low,
			Close:        agg.Close,
			Volume:       float64(agg.Volume),
			Vwap:         agg.VWAP,
			Timestamp:    time.Time(agg.Timestamp).Unix(),
			Transactions: int64(agg.Transactions),
		}
	}

	// Log response status
	log.Printf("[RESPONSE] GetAggregates successful - bars count: %d", len(response.Bars))

	return response, nil
}
