package cache

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// Cache defines the interface for a simple key-value store
type Cache interface {
	// Store saves a value with the given key and TTL. If compress is true, the data will be compressed
	Store(key string, value string, dataType DataType, ttl time.Duration, compress bool) error

	// Get retrieves a value by key. Returns ErrNotFound if the key doesn't exist or has expired
	Get(key string, dataType DataType) (string, error)

	// Delete removes a value by key
	Delete(key string, dataType DataType) error

	// StoreTyped stores a value with type-specific caching rules
	StoreTyped(dataType DataType, identifier string, value string) error

	// GetTyped retrieves a value using type-specific caching rules
	GetTyped(dataType DataType, identifier string) (string, error)

	// DeleteTyped deletes a value using type-specific caching rules
	DeleteTyped(dataType DataType, identifier string) error
}

// Storage type (memory or disk)
type StorageType int

const (
	Memory StorageType = iota
	Disk
)

// DataType represents different types of data that can be cached
type DataType string

const (
	OptionChains DataType = "option-chains"
	LastTrades   DataType = "last-trades"
	Aggregates   DataType = "daily-aggregates"
)

// TypeConfig defines caching rules for a specific data type
type TypeConfig struct {
	StorageType StorageType
	TTL         time.Duration
	Compression bool
	KeyPrefix   string
}

// Config holds configuration for the cache
type Config struct {
	StorageType StorageType
	BasePath    string // Only used for disk storage
	MaxSize     int64  // Maximum size in bytes
	TypeConfigs map[DataType]TypeConfig
}

var ErrNotFound = fmt.Errorf("key not found")

// entry represents a stored cache entry
type entry struct {
	value     string
	expiresAt time.Time
}

// Manager implements the Cache interface
type Manager struct {
	config Config
	mu     sync.RWMutex
	data   map[string]entry
}

// NewManager creates a new cache manager
func NewManager(config Config) (*Manager, error) {
	if config.StorageType == Disk {
		if err := os.MkdirAll(config.BasePath, 0755); err != nil {
			return nil, fmt.Errorf("failed to create cache directory: %w", err)
		}
	}

	return &Manager{
		config: config,
		data:   make(map[string]entry),
	}, nil
}

// generateKey creates a standardized cache key for a given data type and identifier
func (m *Manager) generateKey(dataType DataType, identifier string) string {
	typeConfig, exists := m.config.TypeConfigs[dataType]
	if !exists {
		return fmt.Sprintf("%s:%s", string(dataType), identifier)
	}
	return fmt.Sprintf("%s:%s", typeConfig.KeyPrefix, identifier)
}

// StoreTyped stores a value with type-specific caching rules
func (m *Manager) StoreTyped(dataType DataType, identifier string, value string) error {
	typeConfig, exists := m.config.TypeConfigs[dataType]
	if !exists {
		return fmt.Errorf("no cache configuration for data type: %s", dataType)
	}

	key := m.generateKey(dataType, identifier)
	return m.Store(key, value, dataType, typeConfig.TTL, typeConfig.Compression)
}

// GetTyped retrieves a value using type-specific caching rules
func (m *Manager) GetTyped(dataType DataType, identifier string) (string, error) {
	key := m.generateKey(dataType, identifier)
	return m.Get(key, dataType)
}

// DeleteTyped deletes a value using type-specific caching rules
func (m *Manager) DeleteTyped(dataType DataType, identifier string) error {
	key := m.generateKey(dataType, identifier)
	return m.Delete(key, dataType)
}

// Store implements Cache.Store
func (m *Manager) Store(key string, value string, dataType DataType, ttl time.Duration, compress bool) error {
	fmt.Printf("Storing in cache: %s\n", key)
	if m.config.TypeConfigs[dataType].StorageType == Memory {
		m.mu.Lock()
		defer m.mu.Unlock()

		m.data[key] = entry{
			value:     value,
			expiresAt: time.Now().Add(ttl),
		}
		return nil
	}

	// Disk storage
	data := []byte(value)
	if compress {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		if _, err := gz.Write(data); err != nil {
			return fmt.Errorf("failed to compress data: %w", err)
		}
		if err := gz.Close(); err != nil {
			return fmt.Errorf("failed to close gzip writer: %w", err)
		}
		data = buf.Bytes()
	}

	filePath := filepath.Join(m.config.BasePath, fmt.Sprintf("%s.cache", key))
	if err := ioutil.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	// Store expiration in a separate metadata file
	metaPath := filepath.Join(m.config.BasePath, fmt.Sprintf("%s.meta", key))
	expiresAt := time.Now().Add(ttl).Unix()
	if err := ioutil.WriteFile(metaPath, []byte(fmt.Sprintf("%d", expiresAt)), 0644); err != nil {
		return fmt.Errorf("failed to write metadata file: %w", err)
	}

	return nil
}

// Get implements Cache.Get
func (m *Manager) Get(key string, dataType DataType) (string, error) {
	fmt.Println("Getting from cache...")
	if m.config.TypeConfigs[dataType].StorageType == Memory {
		fmt.Println("Getting from memory")
		m.mu.RLock()
		defer m.mu.RUnlock()

		entry, exists := m.data[key]
		if !exists {
			fmt.Println("Not found in memory")
			return "", ErrNotFound
		}

		if time.Now().After(entry.expiresAt) {
			delete(m.data, key)
			return "", ErrNotFound
		}

		fmt.Println("Found in memory")
		return entry.value, nil
	}

	// Disk storage
	metaPath := filepath.Join(m.config.BasePath, fmt.Sprintf("%s.meta", key))
	metaData, err := ioutil.ReadFile(metaPath)
	if err != nil {
		return "", ErrNotFound
	}

	expiresAtUnix, err := strconv.ParseInt(string(metaData), 10, 64)
	if err != nil {
		return "", fmt.Errorf("failed to parse expiration time: %w", err)
	}

	if time.Now().After(time.Unix(expiresAtUnix, 0)) {
		m.Delete(key, dataType)
		return "", ErrNotFound
	}

	filePath := filepath.Join(m.config.BasePath, fmt.Sprintf("%s.cache", key))
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		return "", ErrNotFound
	}

	// Try to decompress
	if gz, err := gzip.NewReader(bytes.NewReader(data)); err == nil {
		data, err = ioutil.ReadAll(gz)
		if err != nil {
			return "", fmt.Errorf("failed to decompress data: %w", err)
		}
		gz.Close()
	}

	return string(data), nil
}

// Delete implements Cache.Delete
func (m *Manager) Delete(key string, dataType DataType) error {
	if m.config.TypeConfigs[dataType].StorageType == Memory {
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.data, key)
		return nil
	}

	// Delete both the data file and metadata file for disk storage
	basePath := filepath.Join(m.config.BasePath, key)
	os.Remove(basePath + ".cache") // Ignore errors as file might not exist
	os.Remove(basePath + ".meta")  // Ignore errors as file might not exist
	return nil
}
