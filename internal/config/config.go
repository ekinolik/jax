package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Environment represents the running environment
type Environment string

const (
	EnvLocal      Environment = "local"
	EnvDev        Environment = "dev"
	EnvProduction Environment = "prod"
)

// Config holds all configuration for the service
type Config struct {
	// Server configuration
	Port     int
	GRPCHost string

	// Polygon configuration
	PolygonAPIKey string

	// Cache configuration
	CacheTTL time.Duration

	// Environment
	Env Environment
}

// LoadConfig loads configuration from environment variables and .env files
func LoadConfig() (*Config, error) {
	// Load .env files in order of precedence
	env := os.Getenv("JAX_ENV")
	if env == "" {
		env = "local"
	}

	// Load environment-specific .env file first
	_ = godotenv.Load(fmt.Sprintf(".env.%s", env))
	// Then load the default .env file
	_ = godotenv.Load()

	config := &Config{
		Env: Environment(env),
	}

	// Load server configuration
	port := os.Getenv("JAX_PORT")
	if port == "" {
		config.Port = 50051 // Default port
	} else {
		p, err := strconv.Atoi(port)
		if err != nil {
			return nil, fmt.Errorf("invalid port number: %s", port)
		}
		config.Port = p
	}

	config.GRPCHost = os.Getenv("JAX_GRPC_HOST")
	if config.GRPCHost == "" {
		config.GRPCHost = fmt.Sprintf(":%d", config.Port)
	}

	// Load Polygon configuration
	config.PolygonAPIKey = os.Getenv("POLYGON_API_KEY")
	if config.PolygonAPIKey == "" {
		return nil, fmt.Errorf("POLYGON_API_KEY is required")
	}

	// Load cache configuration
	cacheTTL := os.Getenv("JAX_CACHE_TTL")
	if cacheTTL == "" {
		config.CacheTTL = 15 * time.Minute // Default cache TTL
	} else {
		duration, err := time.ParseDuration(cacheTTL)
		if err != nil {
			return nil, fmt.Errorf("invalid cache TTL duration: %s", cacheTTL)
		}
		config.CacheTTL = duration
	}

	return config, nil
}
