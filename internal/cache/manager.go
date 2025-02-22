package cache

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ekinolik/jax/internal/config"
)

const (
	// Size thresholds
	SmallDataThreshold = 10 * 1024              // 10KB
	MemoryCacheLimit   = 50 * 1024 * 1024       // 50MB
	DiskCacheLimit     = 2 * 1024 * 1024 * 1024 // 2GB
)

type CacheType int

const (
	MemoryCache CacheType = iota
	DiskCache
)

type RetryConfig struct {
	MaxRetries  int
	RetryDelay  time.Duration
	BackoffFunc func(attempt int) time.Duration
}

// CacheTask represents a task that needs to be cached
type CacheTask struct {
	Name        string
	CacheType   CacheType
	SizeBytes   int64
	Key         string
	Compression bool
	UpdateFunc  func() (interface{}, error)
	RetryConfig RetryConfig
}

// IntervalTask represents a task that runs at regular intervals
type IntervalTask struct {
	Name        string
	Interval    time.Duration
	StartTime   time.Time
	EndTime     time.Time
	RunWeekends bool
	Task        CacheTask
}

// TimedTask represents a task that runs at specific times
type TimedTask struct {
	Name        string
	Times       []time.Time // Daily times to run
	RunWeekends bool
	Task        CacheTask
}

// CacheManager manages different types of caches
type CacheManager struct {
	client      interface{} // The client that makes actual API calls
	memoryLimit int64
	diskLimit   int64
	memoryUsage atomic.Int64
	diskUsage   atomic.Int64
	stopChan    chan struct{}
	wg          sync.WaitGroup
	timezone    *time.Location

	// Caches
	memoryCache map[string]*CacheEntry
	diskCache   *DiskCacheManager
	cacheLock   sync.RWMutex

	// Tasks
	intervalTasks map[string]*IntervalTask
	timedTasks    map[string]*TimedTask
}

type CacheEntry struct {
	Data      interface{}
	Size      int64
	ExpiresAt time.Time
}

// NewCacheManager creates a new cache manager
func NewCacheManager(cfg *config.Config, client interface{}) (*CacheManager, error) {
	tz, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return nil, fmt.Errorf("failed to load timezone: %v", err)
	}

	return &CacheManager{
		client:        client,
		memoryLimit:   MemoryCacheLimit,
		diskLimit:     DiskCacheLimit,
		stopChan:      make(chan struct{}),
		timezone:      tz,
		memoryCache:   make(map[string]*CacheEntry),
		diskCache:     NewDiskCacheManager("cache"),
		intervalTasks: make(map[string]*IntervalTask),
		timedTasks:    make(map[string]*TimedTask),
	}, nil
}

// AddIntervalTask adds a new interval task
func (cm *CacheManager) AddIntervalTask(task *IntervalTask) error {
	if _, exists := cm.intervalTasks[task.Name]; exists {
		return fmt.Errorf("task %s already exists", task.Name)
	}
	cm.intervalTasks[task.Name] = task
	return nil
}

// AddTimedTask adds a new timed task
func (cm *CacheManager) AddTimedTask(task *TimedTask) error {
	if _, exists := cm.timedTasks[task.Name]; exists {
		return fmt.Errorf("task %s already exists", task.Name)
	}
	cm.timedTasks[task.Name] = task
	return nil
}

// Start starts the cache manager
func (cm *CacheManager) Start() {
	cm.wg.Add(2) // One for interval tasks, one for timed tasks
	go cm.runIntervalTasks()
	go cm.runTimedTasks()
}

// Stop stops the cache manager
func (cm *CacheManager) Stop() {
	close(cm.stopChan)
	cm.wg.Wait()
}

// runIntervalTasks runs all interval tasks
func (cm *CacheManager) runIntervalTasks() {
	defer cm.wg.Done()

	// Create a ticker for each task
	tickers := make(map[string]*time.Ticker)
	for name, task := range cm.intervalTasks {
		tickers[name] = time.NewTicker(task.Interval)
		// Execute immediately for the first time
		if cm.shouldRunTask(task.RunWeekends, task.StartTime, task.EndTime) {
			go cm.executeTask(&task.Task)
		}
	}

	for {
		select {
		case <-cm.stopChan:
			// Stop all tickers
			for _, ticker := range tickers {
				ticker.Stop()
			}
			return
		default:
			for name, task := range cm.intervalTasks {
				select {
				case <-cm.stopChan:
					return
				case <-tickers[name].C:
					if cm.shouldRunTask(task.RunWeekends, task.StartTime, task.EndTime) {
						go cm.executeTask(&task.Task)
					}
				default:
					// Check next ticker
				}
			}
			// Small sleep to prevent busy loop
			time.Sleep(time.Millisecond)
		}
	}
}

// runTimedTasks runs all timed tasks
func (cm *CacheManager) runTimedTasks() {
	defer cm.wg.Done()

	// Execute immediately for tasks that should run now
	now := time.Now().In(cm.timezone)
	for _, task := range cm.timedTasks {
		if cm.shouldRunTimedTask(task, now) {
			go cm.executeTask(&task.Task)
		}
	}

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-cm.stopChan:
			return
		case now := <-ticker.C:
			now = now.In(cm.timezone)
			for _, task := range cm.timedTasks {
				if cm.shouldRunTimedTask(task, now) {
					go cm.executeTask(&task.Task)
				}
			}
		}
	}
}

// shouldRunTask checks if a task should run based on weekday and time constraints
func (cm *CacheManager) shouldRunTask(runWeekends bool, start, end time.Time) bool {
	// If no time constraints, task can run
	if start.IsZero() && end.IsZero() {
		return true
	}

	// Get current time in the correct timezone
	now := time.Now().In(cm.timezone)

	// Convert current time to time of day for comparison
	currentTime := time.Date(0, 1, 1, now.Hour(), now.Minute(), now.Second(), 0, time.UTC)
	startTime := time.Date(0, 1, 1, start.Hour(), start.Minute(), start.Second(), 0, time.UTC)
	endTime := time.Date(0, 1, 1, end.Hour(), end.Minute(), end.Second(), 0, time.UTC)

	// Check if current time is within the specified range
	return currentTime.After(startTime) && currentTime.Before(endTime)
}

// shouldRunTimedTask checks if a timed task should run
func (cm *CacheManager) shouldRunTimedTask(task *TimedTask, now time.Time) bool {
	if !task.RunWeekends && (now.Weekday() == time.Saturday || now.Weekday() == time.Sunday) {
		return false
	}

	for _, t := range task.Times {
		taskTime := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, cm.timezone)
		if now.Sub(taskTime) < time.Minute && now.Sub(taskTime) >= 0 {
			return true
		}
	}
	return false
}

// executeTask executes a cache task with retries
func (cm *CacheManager) executeTask(task *CacheTask) {
	var attempt int
	var lastErr error

	for attempt = 0; attempt <= task.RetryConfig.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := task.RetryConfig.BackoffFunc(attempt)
			time.Sleep(delay)
		}

		data, err := task.UpdateFunc()
		if err != nil {
			lastErr = err
			log.Printf("Failed to execute task %s (attempt %d/%d): %v",
				task.Name, attempt+1, task.RetryConfig.MaxRetries+1, err)
			continue
		}

		// Cache the data
		if err := cm.cacheData(task, data); err != nil {
			lastErr = err
			log.Printf("Failed to cache data for task %s: %v", task.Name, err)
			continue
		}

		return // Success
	}

	log.Printf("Task %s failed after %d attempts. Last error: %v",
		task.Name, attempt, lastErr)
}

// cacheData stores data in the appropriate cache
func (cm *CacheManager) cacheData(task *CacheTask, data interface{}) error {
	switch task.CacheType {
	case MemoryCache:
		return cm.cacheInMemory(task, data)
	case DiskCache:
		return cm.cacheOnDisk(task, data)
	default:
		return fmt.Errorf("unknown cache type")
	}
}

// cacheInMemory stores data in memory cache
func (cm *CacheManager) cacheInMemory(task *CacheTask, data interface{}) error {
	cm.cacheLock.Lock()
	defer cm.cacheLock.Unlock()

	// Check if we're overwriting existing data
	existingSize := int64(0)
	if existing, exists := cm.memoryCache[task.Key]; exists {
		existingSize = existing.Size
	}

	// Check memory limits
	newSize := task.SizeBytes
	currentUsage := cm.memoryUsage.Load()
	if currentUsage-existingSize+newSize > cm.memoryLimit {
		return fmt.Errorf("memory cache limit exceeded")
	}

	// Update cache and usage tracking
	cm.memoryCache[task.Key] = &CacheEntry{
		Data:      data,
		Size:      newSize,
		ExpiresAt: time.Now().Add(24 * time.Hour), // Default TTL
	}
	cm.memoryUsage.Add(newSize - existingSize)

	return nil
}

// cacheOnDisk stores data in disk cache
func (cm *CacheManager) cacheOnDisk(task *CacheTask, data interface{}) error {
	// Implementation will be added in the next file
	return cm.diskCache.Store(task.Key, data, task.SizeBytes, task.Compression)
}
