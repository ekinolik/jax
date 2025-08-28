package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCache(t *testing.T) {
	// Test configurations
	configs := []struct {
		name   string
		config Config
	}{
		{
			name: "Memory Cache",
			config: Config{
				StorageType: Memory,
				MaxSize:     1024 * 1024, // 1MB
			},
		},
		{
			name: "Disk Cache",
			config: Config{
				StorageType: Disk,
				BasePath:    t.TempDir(), // Use testing's temporary directory
				MaxSize:     1024 * 1024, // 1MB
			},
		},
	}

	for _, tc := range configs {
		t.Run(tc.name, func(t *testing.T) {
			cache, err := NewManager(tc.config)
			require.NoError(t, err)

			t.Run("Store and Get", func(t *testing.T) {
				err := cache.Store("test-key", "test-value", OptionChains, time.Minute, false)
				require.NoError(t, err)

				value, err := cache.Get("test-key", OptionChains)
				require.NoError(t, err)
				assert.Equal(t, "test-value", value)

				if tc.config.TypeConfigs[OptionChains].StorageType == Disk {
					// Verify files exist
					_, err = os.Stat(filepath.Join(tc.config.BasePath, "test-key.cache"))
					assert.NoError(t, err)
					_, err = os.Stat(filepath.Join(tc.config.BasePath, "test-key.meta"))
					assert.NoError(t, err)
				}
			})

			t.Run("Expiration", func(t *testing.T) {
				err := cache.Store("expire-key", "expire-value", OptionChains, time.Millisecond*100, false)
				require.NoError(t, err)

				// Wait for expiration
				time.Sleep(time.Millisecond * 150)

				_, err = cache.Get("expire-key", OptionChains)
				assert.Equal(t, ErrNotFound, err)
			})

			t.Run("Delete", func(t *testing.T) {
				err := cache.Store("delete-key", "delete-value", OptionChains, time.Minute, false)
				require.NoError(t, err)

				err = cache.Delete("delete-key", OptionChains)
				require.NoError(t, err)

				_, err = cache.Get("delete-key", OptionChains)
				assert.Equal(t, ErrNotFound, err)

				if tc.config.StorageType == Disk {
					// Verify files are deleted
					_, err = os.Stat(filepath.Join(tc.config.BasePath, "delete-key.cache"))
					assert.True(t, os.IsNotExist(err))
					_, err = os.Stat(filepath.Join(tc.config.BasePath, "delete-key.meta"))
					assert.True(t, os.IsNotExist(err))
				}
			})

			t.Run("Non-existent Key", func(t *testing.T) {
				_, err := cache.Get("non-existent", OptionChains)
				assert.Equal(t, ErrNotFound, err)
			})

			if tc.config.StorageType == Disk {
				t.Run("Compression", func(t *testing.T) {
					longString := "test-value-repeated-many-times"
					for i := 0; i < 5; i++ {
						longString += longString
					}

					err := cache.Store("compressed-key", longString, OptionChains, time.Minute, true)
					require.NoError(t, err)

					value, err := cache.Get("compressed-key", OptionChains)
					require.NoError(t, err)
					assert.Equal(t, longString, value)
				})
			}
		})
	}
}

func TestTypedCache(t *testing.T) {
	// Create a cache with type-specific configurations
	config := Config{
		StorageType: Memory,
		MaxSize:     1024 * 1024,
		TypeConfigs: map[DataType]TypeConfig{
			OptionChains: {
				StorageType: Memory,
				TTL:         time.Hour,
				Compression: false,
				KeyPrefix:   "options",
			},
			LastTrades: {
				StorageType: Memory,
				TTL:         5 * time.Minute,
				Compression: false,
				KeyPrefix:   "last-trade",
			},
			Aggregates: {
				StorageType: Memory,
				TTL:         12 * time.Hour,
				Compression: true,
				KeyPrefix:   "aggs",
			},
		},
	}

	cache, err := NewManager(config)
	require.NoError(t, err)

	t.Run("StoreTyped and GetTyped", func(t *testing.T) {
		// Test option chains
		err := cache.StoreTyped(OptionChains, "AAPL", `{"spotPrice": 150.0, "chain": {}}`)
		require.NoError(t, err)

		value, err := cache.GetTyped(OptionChains, "AAPL")
		require.NoError(t, err)
		assert.Equal(t, `{"spotPrice": 150.0, "chain": {}}`, value)

		// Test last trades
		err = cache.StoreTyped(LastTrades, "AAPL", `{"price": 151.0, "timestamp": 1234567890}`)
		require.NoError(t, err)

		value, err = cache.GetTyped(LastTrades, "AAPL")
		require.NoError(t, err)
		assert.Equal(t, `{"price": 151.0, "timestamp": 1234567890}`, value)

		// Test aggregates
		err = cache.StoreTyped(Aggregates, "AAPL:1:day", `[{"open": 150.0, "close": 151.0}]`)
		require.NoError(t, err)

		value, err = cache.GetTyped(Aggregates, "AAPL:1:day")
		require.NoError(t, err)
		assert.Equal(t, `[{"open": 150.0, "close": 151.0}]`, value)
	})

	t.Run("DeleteTyped", func(t *testing.T) {
		// Store and then delete
		err := cache.StoreTyped(OptionChains, "GOOGL", `{"spotPrice": 2500.0}`)
		require.NoError(t, err)

		err = cache.DeleteTyped(OptionChains, "GOOGL")
		require.NoError(t, err)

		_, err = cache.GetTyped(OptionChains, "GOOGL")
		assert.Equal(t, ErrNotFound, err)
	})

	t.Run("Key Generation", func(t *testing.T) {
		// Test that different data types with same identifier generate different keys
		err := cache.StoreTyped(OptionChains, "AAPL", "option-data")
		require.NoError(t, err)

		err = cache.StoreTyped(LastTrades, "AAPL", "trade-data")
		require.NoError(t, err)

		// Both should be retrievable independently
		optionValue, err := cache.GetTyped(OptionChains, "AAPL")
		require.NoError(t, err)
		assert.Equal(t, "option-data", optionValue)

		tradeValue, err := cache.GetTyped(LastTrades, "AAPL")
		require.NoError(t, err)
		assert.Equal(t, "trade-data", tradeValue)
	})

	t.Run("Unconfigured DataType", func(t *testing.T) {
		// Test with a data type that doesn't have configuration
		err := cache.StoreTyped("unknown-type", "test", "data")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "no cache configuration for data type")
	})
}

func TestTypedCacheDisk(t *testing.T) {
	// Test typed cache with disk storage
	tempDir := t.TempDir()
	config := Config{
		StorageType: Disk,
		BasePath:    tempDir,
		MaxSize:     1024 * 1024,
		TypeConfigs: map[DataType]TypeConfig{
			OptionChains: {
				StorageType: Disk,
				TTL:         time.Hour,
				Compression: true,
				KeyPrefix:   "options",
			},
		},
	}

	cache, err := NewManager(config)
	require.NoError(t, err)

	t.Run("Disk Storage with Compression", func(t *testing.T) {
		longData := "test-data-repeated"
		for i := 0; i < 10; i++ {
			longData += longData
		}

		err := cache.StoreTyped(OptionChains, "AAPL", longData)
		require.NoError(t, err)

		// Verify files exist with correct key prefix
		cacheFile := filepath.Join(tempDir, "options:AAPL.cache")
		metaFile := filepath.Join(tempDir, "options:AAPL.meta")

		_, err = os.Stat(cacheFile)
		assert.NoError(t, err)
		_, err = os.Stat(metaFile)
		assert.NoError(t, err)

		// Retrieve and verify data
		value, err := cache.GetTyped(OptionChains, "AAPL")
		require.NoError(t, err)
		assert.Equal(t, longData, value)
	})
}

func TestConcurrentAccess(t *testing.T) {
	config := Config{
		StorageType: Memory,
		MaxSize:     1024 * 1024,
	}

	cache, err := NewManager(config)
	require.NoError(t, err)

	// Run concurrent operations
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			key := fmt.Sprintf("test-key-%d", id)
			value := fmt.Sprintf("test-value-%d", id)

			err := cache.Store(key, value, OptionChains, time.Minute, false)
			assert.NoError(t, err)

			retrievedValue, err := cache.Get(key, OptionChains)
			assert.NoError(t, err)
			assert.Equal(t, value, retrievedValue)

			err = cache.Delete(key, OptionChains)
			assert.NoError(t, err)

			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}
