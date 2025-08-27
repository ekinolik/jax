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
	Name         string        `yaml:"name"`
	Type         string        `yaml:"type"`
	Symbols      []string      `yaml:"symbols"`
	Function     string        `yaml:"function"`
	Interval     string        `yaml:"interval,omitempty"`
	StartTime    string        `yaml:"start_time,omitempty"`
	EndTime      string        `yaml:"end_time,omitempty"`
	Times        []string      `yaml:"times,omitempty"`
	RunWeekends  bool          `yaml:"run_weekends"`
	FunctionArgs *FunctionArgs `yaml:"function_args,omitempty"`
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
	Name     string
	Schedule Schedule
	Handler  TaskHandler
	DataType cache.DataType
	Symbol   string
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
	tasks         map[string]*Task
	cache         cache.Cache
	client        PolygonClient
	stopChan      chan struct{}
	wg            sync.WaitGroup
	mu            sync.RWMutex
	configPath    string
	lastConfigMod time.Time
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

// checkConfigModified checks if the config file has been modified since last load
func (s *Scheduler) checkConfigModified() (bool, error) {
	info, err := os.Stat(s.configPath)
	if err != nil {
		return false, fmt.Errorf("failed to stat config file: %w", err)
	}

	modified := info.ModTime()
	if modified.After(s.lastConfigMod) {
		return true, nil
	}
	return false, nil
}

// reloadConfig reloads the configuration if it has been modified
func (s *Scheduler) reloadConfig() error {
	modified, err := s.checkConfigModified()
	if err != nil {
		return err
	}

	if !modified {
		return nil
	}

	data, err := os.ReadFile(s.configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var config TasksConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}

	// Create a new map for the updated tasks
	newTasks := make(map[string]*Task)

	// Load new tasks
	for _, taskConfig := range config.CacheTasks {
		for _, symbol := range taskConfig.Symbols {
			handler, dataType := s.createHandler(taskConfig.Function, symbol, taskConfig.FunctionArgs)

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
				Handler:  handler,
				DataType: dataType,
				Symbol:   symbol,
			}
			newTasks[task.Name] = task
		}
	}

	// Update tasks atomically
	s.mu.Lock()
	defer s.mu.Unlock()

	// Stop existing tasks that are not in the new config
	for name := range s.tasks {
		if _, exists := newTasks[name]; !exists {
			s.stopChan <- struct{}{}
		}
	}

	s.tasks = newTasks
	s.lastConfigMod = time.Now()

	// Start new tasks
	for _, task := range newTasks {
		go s.runTask(task)
	}

	log.Printf("Successfully reloaded configuration from %s", s.configPath)
	return nil
}

// startConfigReloader starts a goroutine that periodically checks for config changes
func (s *Scheduler) startConfigReloader(interval time.Duration) {
	ticker := time.NewTicker(interval)
	go func() {
		for {
			select {
			case <-s.stopChan:
				ticker.Stop()
				return
			case <-ticker.C:
				if err := s.reloadConfig(); err != nil {
					log.Printf("Error reloading config: %v", err)
				}
			}
		}
	}()
}

// LoadTasks loads tasks from the YAML configuration file
func (s *Scheduler) LoadTasks(configPath string, configReloadInterval time.Duration) error {
	s.configPath = configPath
	s.lastConfigMod = time.Now()

	if err := s.reloadConfig(); err != nil {
		return err
	}

	// Start the config reloader with the specified interval
	s.startConfigReloader(configReloadInterval)
	return nil
}

// createHandler creates a task handler based on the function type
func (s *Scheduler) createHandler(functionType string, symbol string, args *FunctionArgs) (TaskHandler, cache.DataType) {
	var dataType cache.DataType

	switch functionType {
	case "GetLastTrade":
		dataType = cache.LastTrades
		return func(ctx context.Context, useCache bool) (interface{}, error) {
			params := &models.GetLastTradeParams{
				Ticker: symbol,
			}
			return s.client.GetLastTrade(ctx, params)
		}, dataType

	case "GetOptionData":
		dataType = cache.OptionChains
		return func(ctx context.Context, useCache bool) (interface{}, error) {
			spotPrice, chain, err := s.client.GetOptionData(symbol, nil, nil)
			if err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"spotPrice": spotPrice,
				"chain":     chain,
			}, nil
		}, dataType

	case "GetAggregates":
		dataType = cache.Aggregates
		return func(ctx context.Context, useCache bool) (interface{}, error) {
			if args == nil {
				return nil, fmt.Errorf("function args required for GetAggregates")
			}
			now := time.Now()
			return s.client.GetAggregates(ctx, symbol, args.Multiplier, args.Timespan,
				now.Add(-24*time.Hour).Unix(), now.Unix(), args.Adjusted)
		}, dataType

	default:
		return func(ctx context.Context, useCache bool) (interface{}, error) {
			return nil, fmt.Errorf("unknown function type: %s", functionType)
		}, ""
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

	// Try to get from cache first
	if cachedData, err := s.cache.GetTyped(task.DataType, task.Symbol); err == nil {
		var result interface{}
		if err := json.Unmarshal([]byte(cachedData), &result); err == nil {
			log.Printf("Task %s: using cached data", task.Name)
			return
		}
	}

	// Execute the task to get fresh data
	result, err := task.Handler(ctx, false)
	if err != nil {
		log.Printf("Task %s failed: %v", task.Name, err)
		return
	}

	// Cache the result
	jsonData, err := json.Marshal(result)
	if err != nil {
		log.Printf("Task %s: failed to marshal result: %v", task.Name, err)
		return
	}

	if err := s.cache.StoreTyped(task.DataType, task.Symbol, string(jsonData)); err != nil {
		log.Printf("Task %s: failed to cache result: %v", task.Name, err)
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
