package config

import (
	"fmt"
	"os"
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

	// Service-specific cache TTLs
	DexCacheTTL       time.Duration
	MarketCacheTTL    time.Duration
	AggregateCacheTTL time.Duration

	// Cache limits
	MemoryCacheLimit int64
	DiskCacheLimit   int64
	CacheDir         string
	NumExecutors     int // Number of cache task executors

	// Environment
	Env Environment
}

// LoadConfig loads configuration from environment variables and .env files
func LoadConfig() (*Config, error) {
	// Load .env files in order of precedence
	env := getEnvWithDefault("JAX_ENV", "local")

	// Load environment-specific .env file first
	_ = godotenv.Load(fmt.Sprintf(".env.%s", env))
	// Then load the default .env file
	_ = godotenv.Load()

	config := &Config{
		Env: Environment(env),
	}

	// Load server configuration
	port, err := getEnvInt64WithDefault("JAX_PORT", 50051)
	if err != nil {
		return nil, err
	}
	config.Port = int(port)

	config.GRPCHost = getEnvWithDefault("JAX_GRPC_HOST", fmt.Sprintf(":%d", config.Port))

	// Load Polygon configuration
	config.PolygonAPIKey = os.Getenv("POLYGON_API_KEY")
	if config.PolygonAPIKey == "" {
		return nil, fmt.Errorf("POLYGON_API_KEY is required")
	}

	// Load cache TTLs
	dexCacheTTL, err := getEnvDurationWithDefault("JAX_DEX_CACHE_TTL", 15*time.Minute)
	if err != nil {
		return nil, err
	}
	config.DexCacheTTL = dexCacheTTL

	marketCacheTTL, err := getEnvDurationWithDefault("JAX_MARKET_CACHE_TTL", 60*time.Second)
	if err != nil {
		return nil, err
	}
	config.MarketCacheTTL = marketCacheTTL

	aggregateCacheTTL, err := getEnvDurationWithDefault("JAX_AGGREGATE_CACHE_TTL", 15*time.Minute)
	if err != nil {
		return nil, err
	}
	config.AggregateCacheTTL = aggregateCacheTTL

	// Load cache limits
	memoryLimit, err := getEnvInt64WithDefault("JAX_MEMORY_CACHE_LIMIT", 50*1024*1024) // Default 50MB
	if err != nil {
		return nil, err
	}
	config.MemoryCacheLimit = memoryLimit

	diskLimit, err := getEnvInt64WithDefault("JAX_DISK_CACHE_LIMIT", 2*1024*1024*1024) // Default 2GB
	if err != nil {
		return nil, err
	}
	config.DiskCacheLimit = diskLimit

	// Load cache directory
	config.CacheDir = getEnvWithDefault("JAX_CACHE_DIR", "cache")

	// Load number of executors
	numExecutors, err := getEnvIntWithDefault("JAX_NUM_EXECUTORS", 1) // Default to 1 executor
	if err != nil {
		return nil, err
	}
	if numExecutors < 1 {
		return nil, fmt.Errorf("JAX_NUM_EXECUTORS must be at least 1")
	}
	config.NumExecutors = numExecutors

	return config, nil
}
