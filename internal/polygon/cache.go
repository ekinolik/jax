package polygon

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/ekinolik/jax/internal/cache"
	"github.com/ekinolik/jax/internal/config"
	"github.com/polygon-io/client-go/rest/models"
)

type CacheEntry struct {
	Data      interface{}
	ExpiresAt time.Time
}

type OptionDataEntry struct {
	SpotPrice float64
	Chain     Chain
}

type CachedClient struct {
	client         *Client
	cache          cache.Cache
	cacheLock      sync.RWMutex
	dexCacheTTL    time.Duration
	marketCacheTTL time.Duration
}

func NewCachedClient(cfg *config.Config, cacheManager cache.Cache) *CachedClient {
	return &CachedClient{
		client:         NewClient(cfg),
		cache:          cacheManager,
		dexCacheTTL:    cfg.DexCacheTTL,
		marketCacheTTL: cfg.MarketCacheTTL,
	}
}

/*
func NewCachedClient(cfg *config.Config) (*CachedClient, error) {
	cacheManager, err := cache.NewManager(cache.Config{
		StorageType: cache.Memory,
		MaxSize:     cfg.MemoryCacheLimit,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create cache manager: %w", err)
	}

	return &CachedClient{
		client:         NewClient(cfg),
		cache:          cacheManager,
		dexCacheTTL:    cfg.DexCacheTTL,
		marketCacheTTL: cfg.MarketCacheTTL,
	}, nil
}
*/

func (c *CachedClient) GetOptionData(underlying string, startStrike, endStrike *float64) (float64, Chain, error) {
	// Check cache using typed method
	if cached, err := c.cache.GetTyped(cache.OptionChains, underlying); err == nil {
		var data OptionDataEntry
		if err := json.Unmarshal([]byte(cached), &data); err == nil {
			return data.SpotPrice, data.Chain, nil
		}
	}

	// Fetch from Polygon
	spotPrice, chain, err := c.client.GetOptionData(underlying, startStrike, endStrike)
	if err != nil {
		return 0, nil, fmt.Errorf("failed to get option data from Polygon: %w", err)
	}

	// Cache the response
	data := &OptionDataEntry{
		SpotPrice: spotPrice,
		Chain:     chain,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return spotPrice, chain, nil // Return data even if caching fails
	}

	if err := c.cache.StoreTyped(cache.OptionChains, underlying, string(jsonData)); err != nil {
		return spotPrice, chain, nil // Return data even if caching fails
	}

	return spotPrice, chain, nil
}

func (c *CachedClient) GetCacheEntry(underlying string) *CacheEntry {
	if _, err := c.cache.GetTyped(cache.OptionChains, underlying); err == nil {
		return &CacheEntry{
			Data:      nil, // We don't need the actual data here
			ExpiresAt: time.Now().Add(c.dexCacheTTL),
		}
	}
	return nil
}

type LastTradeResponse struct {
	Results  models.LastTrade
	CachedAt time.Time
}

func (c *CachedClient) GetLastTrade(ticker string) (*LastTradeResponse, bool, error) {
	// Check cache using typed method
	if cached, err := c.cache.GetTyped(cache.LastTrades, ticker); err == nil {
		var lastTrade LastTradeResponse
		if err := json.Unmarshal([]byte(cached), &lastTrade); err == nil {
			return &lastTrade, true, nil
		}
	}

	// Fetch from Polygon
	params := &models.GetLastTradeParams{
		Ticker: ticker,
	}

	res, err := c.client.GetLastTrade(context.Background(), params)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get last trade from Polygon: %w", err)
	}

	// Cache the response
	lastTrade := &LastTradeResponse{
		Results:  res.Results,
		CachedAt: time.Now(),
	}

	jsonData, err := json.Marshal(lastTrade)
	if err != nil {
		return lastTrade, false, nil // Return data even if caching fails
	}

	if err := c.cache.StoreTyped(cache.LastTrades, ticker, string(jsonData)); err != nil {
		return lastTrade, false, nil // Return data even if caching fails
	}

	return lastTrade, false, nil
}

func (c *CachedClient) GetDexCacheTTL() time.Duration {
	return c.dexCacheTTL
}

func (c *CachedClient) GetMarketCacheTTL() time.Duration {
	return c.marketCacheTTL
}

type AggregatesResponse struct {
	Results  []models.Agg
	CachedAt time.Time
}

func (c *CachedClient) GetAggregates(ticker string, multiplier int, timespan string, from, to int64, adjusted bool) (*AggregatesResponse, bool, error) {
	// Create a unique identifier for this aggregates request
	aggregateID := fmt.Sprintf("%s:%d:%s:%d:%d:%t", ticker, multiplier, timespan, from, to, adjusted)

	// Check cache using typed method
	if cached, err := c.cache.GetTyped(cache.Aggregates, aggregateID); err == nil {
		var aggs AggregatesResponse
		if err := json.Unmarshal([]byte(cached), &aggs); err == nil {
			return &aggs, true, nil
		}
	}

	// Fetch from Polygon
	aggs, err := c.client.GetAggregates(context.Background(), ticker, multiplier, timespan, from, to, adjusted)
	if err != nil {
		return nil, false, fmt.Errorf("failed to get aggregates from Polygon: %w", err)
	}

	// Cache the response
	response := &AggregatesResponse{
		Results:  aggs,
		CachedAt: time.Now(),
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		return response, false, nil // Return data even if caching fails
	}

	if err := c.cache.StoreTyped(cache.Aggregates, aggregateID, string(jsonData)); err != nil {
		return response, false, nil // Return data even if caching fails
	}

	return response, false, nil
}
