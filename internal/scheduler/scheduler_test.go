package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ekinolik/jax/internal/cache"
	"github.com/ekinolik/jax/internal/polygon"
	"github.com/polygon-io/client-go/rest/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockCache struct {
	data map[string]string
}

func newMockCache() *mockCache {
	return &mockCache{
		data: make(map[string]string),
	}
}

func (m *mockCache) Store(key string, value string, ttl time.Duration, compress bool) error {
	m.data[key] = value
	return nil
}

func (m *mockCache) Get(key string) (string, error) {
	if value, ok := m.data[key]; ok {
		return value, nil
	}
	return "", cache.ErrNotFound
}

func (m *mockCache) Delete(key string) error {
	delete(m.data, key)
	return nil
}

type mockPolygonClient struct {
	lastTradeResponse *models.GetLastTradeResponse
	optionData        struct {
		spotPrice float64
		chain     polygon.Chain
	}
	aggregates []models.Agg
}

func newMockPolygonClient() *mockPolygonClient {
	return &mockPolygonClient{
		lastTradeResponse: &models.GetLastTradeResponse{
			Results: models.LastTrade{
				Price: 150.0,
				Size:  100,
			},
		},
		optionData: struct {
			spotPrice float64
			chain     polygon.Chain
		}{
			spotPrice: 150.0,
			chain:     make(polygon.Chain),
		},
		aggregates: []models.Agg{
			{
				Open:   150.0,
				High:   151.0,
				Low:    149.0,
				Close:  150.5,
				Volume: 1000,
			},
		},
	}
}

func (m *mockPolygonClient) GetLastTrade(ctx context.Context, params *models.GetLastTradeParams) (*models.GetLastTradeResponse, error) {
	return m.lastTradeResponse, nil
}

func (m *mockPolygonClient) GetOptionData(symbol string, startStrike, endStrike *float64) (float64, polygon.Chain, error) {
	return m.optionData.spotPrice, m.optionData.chain, nil
}

func (m *mockPolygonClient) GetAggregates(ctx context.Context, ticker string, multiplier int, timespan string, from, to int64, adjusted bool) ([]models.Agg, error) {
	return m.aggregates, nil
}

func createTestConfig(t *testing.T) string {
	config := `cache_tasks:
  - name: test-last-trade
    type: interval
    symbols:
      - AAPL
    function: GetLastTrade
    interval: 1s
    cache:
      type: memory
      size_bytes: 1024
      compression: false

  - name: test-option-data
    type: timed
    symbols:
      - AAPL
    function: GetOptionData
    times:
      - "09:30"
    run_weekends: true
    cache:
      type: memory
      size_bytes: 1024
      compression: false`

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_cache_tasks.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(config), 0644))
	return configPath
}

func TestScheduler(t *testing.T) {
	t.Run("LoadTasks", func(t *testing.T) {
		mockCache := newMockCache()
		mockClient := newMockPolygonClient()
		scheduler := NewScheduler(mockCache, mockClient)
		defer scheduler.Stop()

		configPath := createTestConfig(t)
		err := scheduler.LoadTasks(configPath)
		require.NoError(t, err)

		// Verify tasks were loaded
		assert.Len(t, scheduler.tasks, 2) // One task per symbol per function
		assert.Contains(t, scheduler.tasks, "test-last-trade-AAPL")
		assert.Contains(t, scheduler.tasks, "test-option-data-AAPL")
	})

	t.Run("ExecuteTask", func(t *testing.T) {
		mockCache := newMockCache()
		mockClient := newMockPolygonClient()
		scheduler := NewScheduler(mockCache, mockClient)
		defer scheduler.Stop()

		configPath := createTestConfig(t)
		err := scheduler.LoadTasks(configPath)
		require.NoError(t, err)

		// Start the scheduler
		scheduler.Start()

		// Wait for task execution
		time.Sleep(2 * time.Second)

		// Verify data was cached for the interval task
		cacheKey := "task:test-last-trade-AAPL"
		value, err := mockCache.Get(cacheKey)
		require.NoError(t, err)
		assert.Contains(t, value, "150") // Price from mock response
	})

	t.Run("InvalidConfig", func(t *testing.T) {
		mockCache := newMockCache()
		mockClient := newMockPolygonClient()
		scheduler := NewScheduler(mockCache, mockClient)
		defer scheduler.Stop()

		// Test with non-existent file
		err := scheduler.LoadTasks("non_existent_file.yaml")
		assert.Error(t, err)

		// Test with invalid YAML
		tmpDir := t.TempDir()
		invalidConfigPath := filepath.Join(tmpDir, "invalid.yaml")
		require.NoError(t, os.WriteFile(invalidConfigPath, []byte("invalid: yaml: content"), 0644))
		err = scheduler.LoadTasks(invalidConfigPath)
		assert.Error(t, err)
	})
}
