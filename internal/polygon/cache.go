package polygon

import (
	"context"
	"fmt"
	"sync"
	"time"

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
	cache          map[string]CacheEntry
	cacheLock      sync.RWMutex
	dexCacheTTL    time.Duration
	marketCacheTTL time.Duration
}

func NewCachedClient(cfg *config.Config) *CachedClient {
	return &CachedClient{
		client:         NewClient(cfg),
		cache:          make(map[string]CacheEntry),
		dexCacheTTL:    cfg.DexCacheTTL,
		marketCacheTTL: cfg.MarketCacheTTL,
	}
}

func (c *CachedClient) GetOptionData(underlying string, startStrike, endStrike *float64) (float64, Chain, error) {
	cacheKey := fmt.Sprintf("option_data:%s", underlying)

	// Check cache
	c.cacheLock.RLock()
	if entry, ok := c.cache[cacheKey]; ok {
		if time.Now().Before(entry.ExpiresAt) {
			if data, ok := entry.Data.(*OptionDataEntry); ok {
				c.cacheLock.RUnlock()
				return data.SpotPrice, data.Chain, nil
			}
		}
	}
	c.cacheLock.RUnlock()

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

	c.cacheLock.Lock()
	c.cache[cacheKey] = CacheEntry{
		Data:      data,
		ExpiresAt: time.Now().Add(c.dexCacheTTL),
	}
	c.cacheLock.Unlock()

	return spotPrice, chain, nil
}

func (c *CachedClient) GetCacheEntry(underlying string) *CacheEntry {
	cacheKey := fmt.Sprintf("option_data:%s", underlying)
	c.cacheLock.RLock()
	defer c.cacheLock.RUnlock()
	if entry, ok := c.cache[cacheKey]; ok {
		return &entry
	}
	return nil
}

type LastTradeResponse struct {
	Results  models.LastTrade
	CachedAt time.Time
}

func (c *CachedClient) GetLastTrade(ticker string) (*LastTradeResponse, bool, error) {
	cacheKey := fmt.Sprintf("last_trade:%s", ticker)

	// Check cache
	c.cacheLock.RLock()
	if entry, ok := c.cache[cacheKey]; ok {
		if time.Now().Before(entry.ExpiresAt) {
			if lastTrade, ok := entry.Data.(*LastTradeResponse); ok {
				c.cacheLock.RUnlock()
				return lastTrade, true, nil
			}
		}
	}
	c.cacheLock.RUnlock()

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

	c.cacheLock.Lock()
	c.cache[cacheKey] = CacheEntry{
		Data:      lastTrade,
		ExpiresAt: time.Now().Add(c.marketCacheTTL),
	}
	c.cacheLock.Unlock()

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
	cacheKey := fmt.Sprintf("aggregates:%s:%d:%s:%d:%d:%t", ticker, multiplier, timespan, from, to, adjusted)

	// Check cache
	c.cacheLock.RLock()
	if entry, ok := c.cache[cacheKey]; ok {
		if time.Now().Before(entry.ExpiresAt) {
			if aggs, ok := entry.Data.(*AggregatesResponse); ok {
				c.cacheLock.RUnlock()
				return aggs, true, nil
			}
		}
	}
	c.cacheLock.RUnlock()

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

	c.cacheLock.Lock()
	c.cache[cacheKey] = CacheEntry{
		Data:      response,
		ExpiresAt: time.Now().Add(c.marketCacheTTL),
	}
	c.cacheLock.Unlock()

	return response, false, nil
}
