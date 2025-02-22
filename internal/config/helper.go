package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// getEnvWithDefault returns the environment variable value or the default if not set
func getEnvWithDefault(key string, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt64WithDefault returns the environment variable as int64 or the default if not set/invalid
func getEnvInt64WithDefault(key string, defaultValue int64) (int64, error) {
	if value := os.Getenv(key); value != "" {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid value for %s: %s", key, value)
		}
		return parsed, nil
	}
	return defaultValue, nil
}

// getEnvDurationWithDefault returns the environment variable as duration or the default if not set/invalid
func getEnvDurationWithDefault(key string, defaultValue time.Duration) (time.Duration, error) {
	if value := os.Getenv(key); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil {
			return 0, fmt.Errorf("invalid duration for %s: %s", key, value)
		}
		return parsed, nil
	}
	return defaultValue, nil
}
