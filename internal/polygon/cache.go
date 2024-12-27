package polygon

import (
	"sync"
	"time"
)

type CacheEntry struct {
	SpotPrice float64
	Data      Chain
	Timestamp time.Time
}

type CachedClient struct {
	client   *Client
	cache    map[string]CacheEntry
	mu       sync.RWMutex
	cacheTTL time.Duration
}

func NewCachedClient(apiKey string) *CachedClient {
	return &CachedClient{
		client:   NewClient(apiKey),
		cache:    make(map[string]CacheEntry),
		cacheTTL: 15 * time.Minute,
	}
}

func (c *CachedClient) GetOptionData(underlying string, startStrike, endStrike *float64) (float64, Chain, error) {
	c.mu.RLock()
	if entry, exists := c.cache[underlying]; exists {
		if time.Since(entry.Timestamp) < c.cacheTTL {
			c.mu.RUnlock()
			return entry.SpotPrice, entry.Data, nil
		}
	}
	c.mu.RUnlock()

	spotPrice, chain, err := c.client.GetOptionData(underlying, startStrike, endStrike)
	if err != nil {
		return 0, nil, err
	}

	c.mu.Lock()
	c.cache[underlying] = CacheEntry{
		SpotPrice: spotPrice,
		Data:      chain,
		Timestamp: time.Now(),
	}
	c.mu.Unlock()

	return spotPrice, chain, nil
}

func (c *CachedClient) GetCacheEntry(underlying string) *CacheEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if entry, exists := c.cache[underlying]; exists {
		return &entry
	}
	return nil
}

func (c *CachedClient) GetCacheTTL() time.Duration {
	return c.cacheTTL
}
