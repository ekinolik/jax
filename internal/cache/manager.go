package cache

import (
	"fmt"
	"log"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ekinolik/jax/internal/config"
	"github.com/ekinolik/jax/internal/polygon"
	"github.com/polygon-io/client-go/rest/models"
)

const (
	// Size thresholds
	SmallDataThreshold = 10 * 1024              // 10KB
	MemoryCacheLimit   = 50 * 1024 * 1024       // 50MB
	DiskCacheLimit     = 2 * 1024 * 1024 * 1024 // 2GB
)

// CacheType represents the type of cache (memory or disk)
type CacheType string

const (
	MemoryCache CacheType = "memory"
	DiskCache   CacheType = "disk"
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

// scheduledTask represents a unified task that can be either interval or timed
type scheduledTask struct {
	task        *CacheTask
	nextRun     time.Time
	interval    *time.Duration // nil for timed tasks
	times       []time.Time    // nil for interval tasks
	lastRun     time.Time
	runWeekends bool
	startTime   time.Time
	endTime     time.Time
}

// TaskErrorType represents different types of task failures
type TaskErrorType int

const (
	ErrTaskExecution TaskErrorType = iota
	ErrCacheStore
	ErrRateLimit
	ErrTimeout
	ErrResourceExhausted
)

// TaskError wraps task-specific errors with context
type TaskError struct {
	TaskName string
	ErrType  TaskErrorType
	Err      error
}

func (e *TaskError) Error() string {
	return fmt.Sprintf("task %s failed: %v (%v)", e.TaskName, e.ErrType, e.Err)
}

// TaskErrorStats tracks error statistics for a task
type TaskErrorStats struct {
	consecutiveFailures int
	lastError           error
	lastErrorTime       time.Time
	circuitOpen         bool
	nextRetryTime       time.Time
}

// LastTradeData wraps the last trade response
type LastTradeData struct {
	Trade     *polygon.LastTradeResponse
	FromCache bool
}

// OptionData wraps the option data response
type OptionData struct {
	SpotPrice float64
	Chain     polygon.Chain
}

// AggregatesData wraps the aggregates data response
type AggregatesData struct {
	Results   []models.Agg
	FromCache bool
	CachedAt  time.Time
}

// PolygonClient defines the interface for interacting with Polygon API
type PolygonClient interface {
	GetLastTrade(symbol string) (*polygon.LastTradeResponse, bool, error)
	GetOptionData(symbol string, strike, expiry *float64) (float64, polygon.Chain, error)
	GetAggregates(symbol string, multiplier int, timespan string, from, to int64, adjusted bool) (*polygon.AggregatesResponse, bool, error)
}

// CacheManager manages different types of caches
type CacheManager struct {
	client      PolygonClient // The client that makes actual API calls
	memoryLimit int64
	diskLimit   int64
	memoryUsage atomic.Int64
	stopChan    chan struct{}
	timezone    *time.Location

	// Caches
	memoryCache map[string]*CacheEntry
	diskCache   *DiskCacheManager
	cacheLock   sync.RWMutex

	// Unified task scheduling
	tasks        map[string]*scheduledTask
	taskLock     sync.RWMutex
	taskQueue    chan *scheduledTask
	numExecutors int

	// Task execution
	executorWg sync.WaitGroup

	// Error handling
	taskErrors     map[string]*TaskErrorStats
	errorStatsLock sync.RWMutex
}

type CacheEntry struct {
	Data      interface{}
	Size      int64
	ExpiresAt time.Time
}

// NewCacheManager creates a new cache manager
func NewCacheManager(cfg *config.Config, client PolygonClient) (*CacheManager, error) {
	tz, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		return nil, fmt.Errorf("failed to load timezone: %v", err)
	}

	cm := &CacheManager{
		client:       client,
		memoryLimit:  MemoryCacheLimit,
		diskLimit:    DiskCacheLimit,
		stopChan:     make(chan struct{}),
		timezone:     tz,
		memoryCache:  make(map[string]*CacheEntry),
		diskCache:    NewDiskCacheManager("cache"),
		tasks:        make(map[string]*scheduledTask),
		taskQueue:    make(chan *scheduledTask, 100),
		numExecutors: cfg.NumExecutors,
		taskErrors:   make(map[string]*TaskErrorStats),
	}

	return cm, nil
}

// AddIntervalTask adds a new interval task
func (cm *CacheManager) AddIntervalTask(task *IntervalTask) error {
	if _, exists := cm.tasks[task.Name]; exists {
		return fmt.Errorf("task %s already exists", task.Name)
	}
	cm.tasks[task.Name] = &scheduledTask{
		task:        &task.Task,
		runWeekends: task.RunWeekends,
		startTime:   task.StartTime,
		endTime:     task.EndTime,
	}
	return nil
}

// AddTimedTask adds a new timed task
func (cm *CacheManager) AddTimedTask(task *TimedTask) error {
	if _, exists := cm.tasks[task.Name]; exists {
		return fmt.Errorf("task %s already exists", task.Name)
	}
	cm.tasks[task.Name] = &scheduledTask{
		task:        &task.Task,
		runWeekends: task.RunWeekends,
		times:       task.Times,
	}
	return nil
}

// Start starts the cache manager
func (cm *CacheManager) Start() {
	// Initialize executors
	cm.executorWg.Add(cm.numExecutors + 1) // +1 for the scheduler

	// Start the task scheduler
	go cm.runTaskScheduler()

	// Start the executors
	for i := 0; i < cm.numExecutors; i++ {
		go cm.runExecutor()
	}

	log.Printf("Started cache manager with %d executors", cm.numExecutors)
}

// Stop stops the cache manager
func (cm *CacheManager) Stop() {
	close(cm.stopChan)
	cm.executorWg.Wait()
}

// runTaskScheduler runs the unified task scheduler
func (cm *CacheManager) runTaskScheduler() {
	defer cm.executorWg.Done()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-cm.stopChan:
			return
		case now := <-ticker.C:
			cm.checkAndScheduleTasks(now)
		}
	}
}

// checkAndScheduleTasks checks and schedules all tasks that should run
func (cm *CacheManager) checkAndScheduleTasks(now time.Time) {
	cm.taskLock.Lock()
	defer cm.taskLock.Unlock()

	now = now.In(cm.timezone)
	currentMinute := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), 0, 0, cm.timezone)

	for _, task := range cm.tasks {
		if task.nextRun.IsZero() {
			// Initialize next run time for new tasks
			if task.interval != nil {
				task.nextRun = now
			} else {
				// For timed tasks, set to the next occurrence
				task.nextRun = cm.getNextTimedRun(task, now)
			}
		}

		// Check if task should run
		if now.After(task.nextRun) || now.Equal(task.nextRun) {
			if task.interval != nil {
				// Interval task
				if cm.shouldRunTask(task.runWeekends, task.startTime, task.endTime) {
					select {
					case cm.taskQueue <- task:
						task.nextRun = now.Add(*task.interval)
					default:
						log.Printf("Warning: Task queue is full, skipping task %s", task.task.Name)
					}
				}
			} else {
				// Timed task
				if cm.shouldRunTimedTask(task, now) {
					select {
					case cm.taskQueue <- task:
						task.nextRun = cm.getNextTimedRun(task, now)
						task.lastRun = currentMinute
					default:
						log.Printf("Warning: Task queue is full, skipping task %s", task.task.Name)
					}
				}
			}
		}
	}
}

// getNextTimedRun calculates the next run time for a timed task
func (cm *CacheManager) getNextTimedRun(task *scheduledTask, now time.Time) time.Time {
	if len(task.times) == 0 {
		return now.Add(24 * time.Hour) // Default to tomorrow if no times specified
	}

	// Find the next scheduled time
	nextRun := time.Date(3000, 1, 1, 0, 0, 0, 0, cm.timezone) // Far future date
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, cm.timezone)
	tomorrow := today.Add(24 * time.Hour)

	// Check today's remaining times
	for _, t := range task.times {
		candidateTime := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, cm.timezone)
		if candidateTime.After(now) && candidateTime.Before(nextRun) {
			nextRun = candidateTime
		}
	}

	// If no times found today, check tomorrow
	if nextRun.Equal(time.Date(3000, 1, 1, 0, 0, 0, 0, cm.timezone)) {
		for _, t := range task.times {
			candidateTime := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), t.Hour(), t.Minute(), 0, 0, cm.timezone)
			if candidateTime.Before(nextRun) {
				nextRun = candidateTime
			}
		}
	}

	return nextRun
}

// runExecutor runs a task executor
func (cm *CacheManager) runExecutor() {
	defer cm.executorWg.Done()

	for {
		select {
		case <-cm.stopChan:
			return
		case task := <-cm.taskQueue:
			cm.executeTask(task.task)
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
func (cm *CacheManager) shouldRunTimedTask(task *scheduledTask, now time.Time) bool {
	// Check weekend constraint first
	if !task.runWeekends && (now.Weekday() == time.Saturday || now.Weekday() == time.Sunday) {
		return false
	}

	// Round current time to minute for comparison
	currentMinute := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), now.Minute(), 0, 0, cm.timezone)

	for _, t := range task.times {
		// Create task time for today
		taskTime := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, cm.timezone)

		// Check if this is the scheduled minute
		if currentMinute.Equal(taskTime) {
			return true
		}
	}
	return false
}

// canExecuteTask checks if a task can be executed based on its error history
func (cm *CacheManager) canExecuteTask(taskName string) bool {
	cm.errorStatsLock.RLock()
	defer cm.errorStatsLock.RUnlock()

	stats, exists := cm.taskErrors[taskName]
	if !exists {
		return true
	}

	// If circuit is open, check if we should allow a retry
	if stats.circuitOpen {
		if time.Now().Before(stats.nextRetryTime) {
			return false
		}
		// Allow a retry attempt
		stats.circuitOpen = false
	}

	return true
}

// updateErrorStats updates error statistics for a task
func (cm *CacheManager) updateErrorStats(taskName string, err error) {
	cm.errorStatsLock.Lock()
	defer cm.errorStatsLock.Unlock()

	stats, exists := cm.taskErrors[taskName]
	if !exists {
		stats = &TaskErrorStats{}
		cm.taskErrors[taskName] = stats
	}

	stats.consecutiveFailures++
	stats.lastError = err
	stats.lastErrorTime = time.Now()

	// Implement circuit breaker logic
	if stats.consecutiveFailures >= 5 { // Open circuit after 5 consecutive failures
		stats.circuitOpen = true
		// Exponential backoff for retry time
		backoff := time.Duration(math.Pow(2, float64(stats.consecutiveFailures))) * time.Second
		if backoff > 1*time.Hour { // Cap at 1 hour
			backoff = 1 * time.Hour
		}
		stats.nextRetryTime = time.Now().Add(backoff)
		log.Printf("Circuit breaker opened for task %s, next retry at %v", taskName, stats.nextRetryTime)
	}
}

// resetErrorStats resets error statistics for a task after successful execution
func (cm *CacheManager) resetErrorStats(taskName string) {
	cm.errorStatsLock.Lock()
	defer cm.errorStatsLock.Unlock()

	delete(cm.taskErrors, taskName)
}

// handleTaskFailure handles final task failure after all retries
func (cm *CacheManager) handleTaskFailure(taskName string, err error) {
	// Determine error type
	var errType TaskErrorType
	switch {
	case strings.Contains(err.Error(), "rate limit"):
		errType = ErrRateLimit
	case strings.Contains(err.Error(), "timeout"):
		errType = ErrTimeout
	case strings.Contains(err.Error(), "memory cache limit"):
		errType = ErrResourceExhausted
	default:
		errType = ErrTaskExecution
	}

	taskErr := &TaskError{
		TaskName: taskName,
		ErrType:  errType,
		Err:      err,
	}

	// Log detailed error information
	log.Printf("Task failure: %v", taskErr)

	// Update error stats
	cm.updateErrorStats(taskName, taskErr)
}

// executeTask executes a cache task with retries
func (cm *CacheManager) executeTask(task *CacheTask) {
	// Check circuit breaker
	if !cm.canExecuteTask(task.Name) {
		log.Printf("Circuit breaker open for task %s, skipping execution", task.Name)
		return
	}

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

		// Reset error stats on success
		cm.resetErrorStats(task.Name)
		return // Success
	}

	// Handle final failure
	cm.handleTaskFailure(task.Name, lastErr)
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

// GetTaskErrorStats returns error statistics for a task
func (cm *CacheManager) GetTaskErrorStats(taskName string) *TaskErrorStats {
	cm.errorStatsLock.RLock()
	defer cm.errorStatsLock.RUnlock()

	if stats, exists := cm.taskErrors[taskName]; exists {
		return &TaskErrorStats{
			consecutiveFailures: stats.consecutiveFailures,
			lastErrorTime:       stats.lastErrorTime,
			circuitOpen:         stats.circuitOpen,
			nextRetryTime:       stats.nextRetryTime,
		}
	}
	return nil
}

// CreateTasksFromConfig creates cache tasks from the provided configuration
func (cm *CacheManager) CreateTasksFromConfig(config *TasksConfig) error {
	for _, taskConfig := range config.CacheTasks {
		// Convert retry config
		retryDelay, err := time.ParseDuration(config.DefaultRetryConfig.RetryDelay)
		if err != nil {
			return fmt.Errorf("invalid retry delay: %w", err)
		}

		retryConfig := RetryConfig{
			MaxRetries: config.DefaultRetryConfig.MaxRetries,
			RetryDelay: retryDelay,
			BackoffFunc: func(attempt int) time.Duration {
				multiplier := float64(config.DefaultRetryConfig.BackoffMultiplier)
				return time.Duration(float64(retryDelay) * math.Pow(multiplier, float64(attempt)))
			},
		}

		// Create cache tasks for each symbol
		for _, symbol := range taskConfig.Symbols {
			cacheTask := CacheTask{
				Name:        fmt.Sprintf("%s:%s", taskConfig.Name, symbol),
				CacheType:   CacheType(taskConfig.Cache.Type),
				SizeBytes:   taskConfig.Cache.SizeBytes,
				Key:         fmt.Sprintf("%s:%s", taskConfig.Name, symbol),
				Compression: taskConfig.Cache.Compression,
				RetryConfig: retryConfig,
			}

			// Set the update function based on the function type
			switch taskConfig.Function {
			case "GetLastTrade":
				cacheTask.UpdateFunc = func() (interface{}, error) {
					trade, fromCache, err := cm.client.GetLastTrade(symbol)
					if err != nil {
						return nil, err
					}
					return &LastTradeData{
						Trade:     trade,
						FromCache: fromCache,
					}, nil
				}

			case "GetOptionData":
				cacheTask.UpdateFunc = func() (interface{}, error) {
					spotPrice, chain, err := cm.client.GetOptionData(symbol, nil, nil)
					if err != nil {
						return nil, err
					}
					return &OptionData{
						SpotPrice: spotPrice,
						Chain:     chain,
					}, nil
				}

			case "GetAggregates":
				if taskConfig.FunctionArgs == nil {
					return fmt.Errorf("function args required for GetAggregates")
				}
				cacheTask.UpdateFunc = func() (interface{}, error) {
					now := time.Now()
					yesterday := now.Add(-24 * time.Hour)
					from := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, time.UTC).Unix()
					to := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 23, 59, 59, 0, time.UTC).Unix()

					response, fromCache, err := cm.client.GetAggregates(
						symbol,
						taskConfig.FunctionArgs.Multiplier,
						taskConfig.FunctionArgs.Timespan,
						from,
						to,
						taskConfig.FunctionArgs.Adjusted,
					)
					if err != nil {
						return nil, fmt.Errorf("failed to get aggregates for %s: %w", symbol, err)
					}

					return &AggregatesData{
						Results:   response.Results,
						FromCache: fromCache,
						CachedAt:  response.CachedAt,
					}, nil
				}
			}

			// Create the appropriate task type
			if taskConfig.Type == "interval" {
				interval, err := time.ParseDuration(taskConfig.Interval)
				if err != nil {
					return fmt.Errorf("invalid interval for task %s: %w", taskConfig.Name, err)
				}

				intervalTask := &IntervalTask{
					Name:     cacheTask.Name,
					Interval: interval,
					Task:     cacheTask,
				}

				// Set start and end times if specified
				if taskConfig.StartTime != "" {
					startTime, err := parseTimeOfDay(taskConfig.StartTime)
					if err != nil {
						return fmt.Errorf("invalid start time for task %s: %w", taskConfig.Name, err)
					}
					intervalTask.StartTime = startTime
				}
				if taskConfig.EndTime != "" {
					endTime, err := parseTimeOfDay(taskConfig.EndTime)
					if err != nil {
						return fmt.Errorf("invalid end time for task %s: %w", taskConfig.Name, err)
					}
					intervalTask.EndTime = endTime
				}

				cm.AddIntervalTask(intervalTask)

			} else if taskConfig.Type == "timed" {
				times := make([]time.Time, len(taskConfig.Times))
				for i, t := range taskConfig.Times {
					parsedTime, err := parseTimeOfDay(t)
					if err != nil {
						return fmt.Errorf("invalid time for task %s: %w", taskConfig.Name, err)
					}
					times[i] = parsedTime
				}

				timedTask := &TimedTask{
					Name:        cacheTask.Name,
					Times:       times,
					RunWeekends: taskConfig.RunWeekends,
					Task:        cacheTask,
				}

				cm.AddTimedTask(timedTask)
			}
		}
	}

	return nil
}

// parseTimeOfDay parses a time string in "HH:MM" format
func parseTimeOfDay(timeStr string) (time.Time, error) {
	t, err := time.Parse("15:04", timeStr)
	if err != nil {
		return time.Time{}, err
	}
	return time.Date(0, 1, 1, t.Hour(), t.Minute(), 0, 0, time.UTC), nil
}
