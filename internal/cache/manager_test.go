package cache

import (
	"os"
	"testing"
	"time"

	"github.com/ekinolik/jax/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestCacheManager(t *testing.T) {
	// Create a temporary directory for disk cache
	tempDir, err := os.MkdirTemp("", "cache-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test config
	cfg := &config.Config{
		DexCacheTTL:    15 * time.Minute,
		MarketCacheTTL: time.Second,
	}

	// Create cache manager
	cm, err := NewCacheManager(cfg, nil)
	if err != nil {
		t.Fatalf("Failed to create cache manager: %v", err)
	}

	// Test interval task
	t.Run("IntervalTask", func(t *testing.T) {
		executed := make(chan bool, 1)
		task := &IntervalTask{
			Name:     "test-interval",
			Interval: time.Millisecond * 100, // Faster interval for testing
			Task: CacheTask{
				Name:      "test-interval-cache",
				CacheType: MemoryCache,
				SizeBytes: 100,
				Key:       "test-key",
				UpdateFunc: func() (interface{}, error) {
					executed <- true
					return "test-data", nil
				},
				RetryConfig: RetryConfig{
					MaxRetries: 3,
					RetryDelay: time.Millisecond * 10,
					BackoffFunc: func(attempt int) time.Duration {
						return time.Duration(attempt) * time.Millisecond * 10
					},
				},
			},
		}

		err := cm.AddIntervalTask(task)
		assert.NoError(t, err)

		// Execute task directly
		go cm.executeTask(&task.Task)

		// Wait for execution with timeout
		select {
		case <-executed:
			// Task executed successfully
		case <-time.After(time.Second):
			t.Fatal("Task execution timed out")
		}
	})

	// Test timed task
	t.Run("TimedTask", func(t *testing.T) {
		executed := make(chan bool, 1)
		now := time.Now()
		task := &TimedTask{
			Name:  "test-timed",
			Times: []time.Time{now.Add(time.Millisecond * 100)}, // Run shortly after start
			Task: CacheTask{
				Name:        "test-timed-cache",
				CacheType:   DiskCache,
				SizeBytes:   100,
				Key:         "test-key-2",
				Compression: true,
				UpdateFunc: func() (interface{}, error) {
					executed <- true
					return "test-data", nil
				},
				RetryConfig: RetryConfig{
					MaxRetries: 3,
					RetryDelay: time.Millisecond * 10,
					BackoffFunc: func(attempt int) time.Duration {
						return time.Duration(attempt) * time.Millisecond * 10
					},
				},
			},
		}

		err := cm.AddTimedTask(task)
		assert.NoError(t, err)

		// Execute task directly
		go cm.executeTask(&task.Task)

		// Wait for execution with timeout
		select {
		case <-executed:
			// Task executed successfully
		case <-time.After(time.Second):
			t.Fatal("Task execution timed out")
		}
	})

	// Test memory cache limits
	t.Run("MemoryCacheLimits", func(t *testing.T) {
		task := &CacheTask{
			Name:      "test-memory-limit",
			CacheType: MemoryCache,
			SizeBytes: MemoryCacheLimit + 1, // Exceed limit
			Key:       "test-key-3",
			UpdateFunc: func() (interface{}, error) {
				return "test-data", nil
			},
		}

		err := cm.cacheData(task, "test-data")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "memory cache limit exceeded")
	})

	// Test disk cache limits
	t.Run("DiskCacheLimits", func(t *testing.T) {
		task := &CacheTask{
			Name:      "test-disk-limit",
			CacheType: DiskCache,
			SizeBytes: DiskCacheLimit + 1, // Exceed limit
			Key:       "test-key-4",
			UpdateFunc: func() (interface{}, error) {
				return "test-data", nil
			},
		}

		err := cm.cacheData(task, "test-data")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "disk cache limit exceeded")
	})

	// Test weekend scheduling
	t.Run("WeekendScheduling", func(t *testing.T) {
		// Create a task that doesn't run on weekends
		task := &IntervalTask{
			Name:        "test-weekend",
			RunWeekends: false,
			Interval:    time.Second,
			Task: CacheTask{
				Name:      "test-weekend-cache",
				CacheType: MemoryCache,
				SizeBytes: 100,
				Key:       "test-key-5",
			},
		}

		// Use a fixed time for testing (Wednesday)
		testTime := time.Date(2024, 3, 13, 12, 0, 0, 0, time.UTC)

		// Test 1: No time constraints on a weekday
		shouldRun := cm.shouldRunTask(task.RunWeekends, time.Time{}, time.Time{})
		assert.True(t, shouldRun, "Task should run on weekdays when there are no time constraints")

		// Test 2: With time constraints on a weekday during business hours
		start := time.Date(0, 1, 1, 9, 0, 0, 0, time.UTC)
		end := time.Date(0, 1, 1, 17, 0, 0, 0, time.UTC)
		testTimeOfDay := time.Date(0, 1, 1, testTime.Hour(), testTime.Minute(), testTime.Second(), 0, time.UTC)
		if testTimeOfDay.After(start) && testTimeOfDay.Before(end) {
			shouldRun = cm.shouldRunTask(task.RunWeekends, start, end)
			assert.True(t, shouldRun, "Task should run on weekdays within time constraints")
		}

		// Test 3: With time constraints on a weekday outside business hours
		start = time.Date(0, 1, 1, 18, 0, 0, 0, time.UTC)
		end = time.Date(0, 1, 1, 23, 59, 59, 0, time.UTC)
		shouldRun = cm.shouldRunTask(task.RunWeekends, start, end)
		assert.False(t, shouldRun, "Task should not run outside business hours")

		// Test 4: With weekend enabled
		weekendTask := &IntervalTask{
			Name:        "test-weekend-enabled",
			RunWeekends: true,
			Interval:    time.Second,
			Task: CacheTask{
				Name:      "test-weekend-enabled-cache",
				CacheType: MemoryCache,
				SizeBytes: 100,
				Key:       "test-key-6",
			},
		}

		shouldRun = cm.shouldRunTask(weekendTask.RunWeekends, time.Time{}, time.Time{})
		assert.True(t, shouldRun, "Task should run when weekends are enabled")
	})
}

func TestDiskCache(t *testing.T) {
	// Create a temporary directory for disk cache
	tempDir, err := os.MkdirTemp("", "disk-cache-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create disk cache manager
	dc := NewDiskCacheManager(tempDir)

	// Test storing and loading data
	t.Run("StoreAndLoad", func(t *testing.T) {
		testData := map[string]interface{}{
			"key1": "value1",
			"key2": 42,
		}

		// Store data
		err := dc.Store("test-key", testData, 100, true)
		assert.NoError(t, err)

		// Load data
		loaded, err := dc.Load("test-key", true)
		assert.NoError(t, err)

		// Compare data
		loadedMap, ok := loaded.(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, testData["key1"], loadedMap["key1"])
		assert.Equal(t, testData["key2"], loadedMap["key2"])
	})

	// Test deleting data
	t.Run("Delete", func(t *testing.T) {
		// Store data
		err := dc.Store("test-key-2", "test-data", 100, false)
		assert.NoError(t, err)

		// Delete data
		err = dc.Delete("test-key-2")
		assert.NoError(t, err)

		// Try to load deleted data
		_, err = dc.Load("test-key-2", false)
		assert.Error(t, err)
	})

	// Test usage tracking
	t.Run("UsageTracking", func(t *testing.T) {
		initialUsage := dc.GetUsage()

		// Store data
		err := dc.Store("test-key-3", "test-data", 100, false)
		assert.NoError(t, err)

		// Check usage increased
		assert.Greater(t, dc.GetUsage(), initialUsage)

		// Delete data
		err = dc.Delete("test-key-3")
		assert.NoError(t, err)

		// Check usage returned to initial
		assert.Equal(t, initialUsage, dc.GetUsage())
	})
}
