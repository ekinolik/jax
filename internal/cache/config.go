package cache

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// TasksConfig represents the root of the cache tasks configuration
type TasksConfig struct {
	CacheTasks         []TaskConfig    `yaml:"cache_tasks"`
	DefaultRetryConfig TaskRetryConfig `yaml:"default_retry_config"`
}

// TaskConfig represents a single cache task configuration
type TaskConfig struct {
	Name         string          `yaml:"name"`
	Type         string          `yaml:"type"` // interval or timed
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

// TaskCacheConfig represents the cache-specific configuration
type TaskCacheConfig struct {
	Type        string `yaml:"type"` // memory or disk
	SizeBytes   int64  `yaml:"size_bytes"`
	Compression bool   `yaml:"compression"`
}

// FunctionArgs represents additional arguments for specific functions
type FunctionArgs struct {
	Multiplier int    `yaml:"multiplier,omitempty"`
	Timespan   string `yaml:"timespan,omitempty"`
	Adjusted   bool   `yaml:"adjusted,omitempty"`
}

// TaskRetryConfig represents retry configuration
type TaskRetryConfig struct {
	MaxRetries        int    `yaml:"max_retries"`
	RetryDelay        string `yaml:"retry_delay"`
	BackoffMultiplier int    `yaml:"backoff_multiplier"`
}

// LoadTasksConfig loads the cache tasks configuration from a YAML file
func LoadTasksConfig(path string) (*TasksConfig, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read cache tasks config: %w", err)
	}

	var config TasksConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse cache tasks config: %w", err)
	}

	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid cache tasks config: %w", err)
	}

	return &config, nil
}

// validateConfig performs validation on the loaded configuration
func validateConfig(config *TasksConfig) error {
	for _, task := range config.CacheTasks {
		// Validate task type
		if task.Type != "interval" && task.Type != "timed" {
			return fmt.Errorf("invalid task type for %s: %s", task.Name, task.Type)
		}

		// Validate symbols
		if len(task.Symbols) == 0 {
			return fmt.Errorf("no symbols specified for task %s", task.Name)
		}

		// Validate function
		switch task.Function {
		case "GetLastTrade", "GetOptionData", "GetAggregates":
			// Valid functions
		default:
			return fmt.Errorf("invalid function for task %s: %s", task.Name, task.Function)
		}

		// Validate interval for interval tasks
		if task.Type == "interval" {
			if task.Interval == "" {
				return fmt.Errorf("interval not specified for interval task %s", task.Name)
			}
			if _, err := time.ParseDuration(task.Interval); err != nil {
				return fmt.Errorf("invalid interval for task %s: %s", task.Name, task.Interval)
			}
		}

		// Validate times for timed tasks
		if task.Type == "timed" {
			if len(task.Times) == 0 {
				return fmt.Errorf("no times specified for timed task %s", task.Name)
			}
			for _, t := range task.Times {
				if _, err := time.Parse("15:04", t); err != nil {
					return fmt.Errorf("invalid time format for task %s: %s", task.Name, t)
				}
			}
		}

		// Validate cache config
		if task.Cache.Type != "memory" && task.Cache.Type != "disk" {
			return fmt.Errorf("invalid cache type for task %s: %s", task.Name, task.Cache.Type)
		}
		if task.Cache.SizeBytes <= 0 {
			return fmt.Errorf("invalid cache size for task %s: %d", task.Name, task.Cache.SizeBytes)
		}
	}

	return nil
}

// readFile reads the contents of a file at the given path
func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
