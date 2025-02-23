package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"os"

	"github.com/ekinolik/jax/internal/cache"
	"github.com/ekinolik/jax/internal/polygon"
	"github.com/polygon-io/client-go/rest/models"
	"gopkg.in/yaml.v3"
)

// PolygonClient defines the interface for Polygon API operations
type PolygonClient interface {
	GetLastTrade(ctx context.Context, params *models.GetLastTradeParams) (*models.GetLastTradeResponse, error)
	GetOptionData(symbol string, startStrike, endStrike *float64) (float64, polygon.Chain, error)
	GetAggregates(ctx context.Context, ticker string, multiplier int, timespan string, from, to int64, adjusted bool) ([]models.Agg, error)
}

// TaskConfig represents a single task configuration from YAML
type TaskConfig struct {
	Name         string          `yaml:"name"`
	Type         string          `yaml:"type"`
	Symbols      []string        `yaml:"symbols"`
	Function     string          `yaml:"function"`
	Interval     string          `yaml:"interval,omitempty"`
	StartTime    string          `yaml:"start_time,omitempty"`
	EndTime      string          `yaml:"end_time,omitempty"`
	Times        []string        `yaml:"times,omitempty"`
	RunWeekends  bool            `yaml:"run_weekends"`
	FunctionArgs *FunctionArgs   `yaml:"function_args,omitempty"`
	Cache        TaskCacheConfig `yaml:"cache"`
}

// TaskCacheConfig represents cache configuration for a task
type TaskCacheConfig struct {
	Type        string `yaml:"type"`
	SizeBytes   int64  `yaml:"size_bytes"`
	Compression bool   `yaml:"compression"`
}

// FunctionArgs represents additional arguments for specific functions
type FunctionArgs struct {
	Multiplier int    `yaml:"multiplier,omitempty"`
	Timespan   string `yaml:"timespan,omitempty"`
	Adjusted   bool   `yaml:"adjusted,omitempty"`
}

// TasksConfig represents the root configuration
type TasksConfig struct {
	CacheTasks []TaskConfig `yaml:"cache_tasks"`
}

// Task represents a scheduled task
type Task struct {
	Name        string
	Schedule    Schedule
	Handler     TaskHandler
	UseCache    bool
	Compression bool
	TTL         time.Duration
}

// Schedule defines when a task should run
type Schedule struct {
	Type        string   // "interval" or "timed"
	Interval    string   // for interval tasks, e.g., "1h", "30m"
	Times       []string // for timed tasks, e.g., ["09:30", "16:00"]
	RunWeekends bool
	StartTime   string // Optional start time window
	EndTime     string // Optional end time window
}

// TaskHandler defines the function that will be executed for a task
type TaskHandler func(ctx context.Context, useCache bool) (interface{}, error)

// Scheduler manages and executes scheduled tasks
type Scheduler struct {
	tasks    map[string]*Task
	cache    cache.Cache
	client   PolygonClient
	stopChan chan struct{}
	wg       sync.WaitGroup
	mu       sync.RWMutex
}

// NewScheduler creates a new scheduler
func NewScheduler(cache cache.Cache, client PolygonClient) *Scheduler {
	return &Scheduler{
		tasks:    make(map[string]*Task),
		cache:    cache,
		client:   client,
		stopChan: make(chan struct{}),
	}
}

// LoadTasks loads tasks from the YAML configuration file
func (s *Scheduler) LoadTasks(configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var config TasksConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	for _, taskConfig := range config.CacheTasks {
		for _, symbol := range taskConfig.Symbols {
			task := &Task{
				Name: fmt.Sprintf("%s-%s", taskConfig.Name, symbol),
				Schedule: Schedule{
					Type:        taskConfig.Type,
					Interval:    taskConfig.Interval,
					Times:       taskConfig.Times,
					RunWeekends: taskConfig.RunWeekends,
					StartTime:   taskConfig.StartTime,
					EndTime:     taskConfig.EndTime,
				},
				UseCache:    true,
				Compression: taskConfig.Cache.Compression,
				TTL:         24 * time.Hour, // Default TTL, should be configurable
			}

			// Create handler based on function type
			task.Handler = s.createHandler(taskConfig.Function, symbol, taskConfig.FunctionArgs)

			if err := s.AddTask(task); err != nil {
				return fmt.Errorf("failed to add task %s: %w", task.Name, err)
			}
		}
	}

	return nil
}

// createHandler creates a task handler based on the function type
func (s *Scheduler) createHandler(functionType string, symbol string, args *FunctionArgs) TaskHandler {
	return func(ctx context.Context, useCache bool) (interface{}, error) {
		switch functionType {
		case "GetLastTrade":
			params := &models.GetLastTradeParams{
				Ticker: symbol,
			}
			return s.client.GetLastTrade(ctx, params)

		case "GetOptionData":
			spotPrice, chain, err := s.client.GetOptionData(symbol, nil, nil)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"spotPrice": spotPrice,
				"chain":     chain,
			}, nil

		case "GetAggregates":
			if args == nil {
				return nil, fmt.Errorf("function args required for GetAggregates")
			}
			now := time.Now()
			return s.client.GetAggregates(ctx, symbol, args.Multiplier, args.Timespan,
				now.Add(-24*time.Hour).Unix(), now.Unix(), args.Adjusted)

		default:
			return nil, fmt.Errorf("unknown function type: %s", functionType)
		}
	}
}

// AddTask adds a new task to the scheduler
func (s *Scheduler) AddTask(task *Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.tasks[task.Name]; exists {
		return fmt.Errorf("task %s already exists", task.Name)
	}

	s.tasks[task.Name] = task
	return nil
}

// Start starts the scheduler
func (s *Scheduler) Start() {
	s.mu.RLock()
	for _, task := range s.tasks {
		// Execute task immediately on startup
		go s.executeTask(task)

		// Start the scheduled execution
		s.wg.Add(1)
		go s.runTask(task)
	}
	s.mu.RUnlock()
}

// Stop stops the scheduler
func (s *Scheduler) Stop() {
	close(s.stopChan)
	s.wg.Wait()
}

// runTask executes a task according to its schedule
func (s *Scheduler) runTask(task *Task) {
	defer s.wg.Done()

	var ticker *time.Ticker
	if task.Schedule.Type == "interval" {
		interval, err := time.ParseDuration(task.Schedule.Interval)
		if err != nil {
			log.Printf("Invalid interval for task %s: %v", task.Name, err)
			return
		}
		ticker = time.NewTicker(interval)
		defer ticker.Stop()
	} else if task.Schedule.Type == "timed" {
		// For timed tasks, use a 1-second ticker to check the schedule
		ticker = time.NewTicker(time.Second)
		defer ticker.Stop()
	} else {
		log.Printf("Invalid schedule type for task %s: %s", task.Name, task.Schedule.Type)
		return
	}

	// For timed tasks, check if we should execute now
	if task.Schedule.Type == "timed" && s.shouldRunTimedTask(task) {
		s.executeTask(task)
	}

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			if task.Schedule.Type == "interval" {
				s.executeTask(task)
			} else if task.Schedule.Type == "timed" && s.shouldRunTimedTask(task) {
				s.executeTask(task)
			}
		}
	}
}

// executeTask executes a single task
func (s *Scheduler) executeTask(task *Task) {
	ctx := context.Background()
	cacheKey := fmt.Sprintf("task:%s", task.Name)

	// Try to get from cache if enabled
	if task.UseCache {
		if cachedData, err := s.cache.Get(cacheKey); err == nil {
			var result interface{}
			if err := json.Unmarshal([]byte(cachedData), &result); err == nil {
				log.Printf("Task %s: using cached data", task.Name)
				return
			}
		}
	}

	// Execute the task
	result, err := task.Handler(ctx, task.UseCache)
	if err != nil {
		log.Printf("Task %s failed: %v", task.Name, err)
		return
	}

	// Cache the result if caching is enabled
	if task.UseCache {
		jsonData, err := json.Marshal(result)
		if err != nil {
			log.Printf("Task %s: failed to marshal result: %v", task.Name, err)
			return
		}

		if err := s.cache.Store(cacheKey, string(jsonData), task.TTL, task.Compression); err != nil {
			log.Printf("Task %s: failed to cache result: %v", task.Name, err)
		}
	}
}

// shouldRunTimedTask checks if a timed task should run now
func (s *Scheduler) shouldRunTimedTask(task *Task) bool {
	now := time.Now()

	// Check weekend constraint
	if !task.Schedule.RunWeekends && (now.Weekday() == time.Saturday || now.Weekday() == time.Sunday) {
		return false
	}

	// Check time window if specified
	if task.Schedule.StartTime != "" && task.Schedule.EndTime != "" {
		currentTime := now.Format("15:04")
		if currentTime < task.Schedule.StartTime || currentTime > task.Schedule.EndTime {
			return false
		}
	}

	// Check if current time matches any scheduled time
	currentTime := now.Format("15:04")
	for _, scheduledTime := range task.Schedule.Times {
		if currentTime == scheduledTime {
			return true
		}
	}

	return false
}
