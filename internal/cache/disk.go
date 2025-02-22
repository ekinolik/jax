package cache

import (
	"bytes"
	"compress/gzip"
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

// DataWrapper wraps data for gob encoding/decoding
type DataWrapper struct {
	Data interface{}
}

func init() {
	// Register types for gob encoding/decoding
	gob.Register(map[string]interface{}{})
	gob.Register([]interface{}{})
	gob.Register(string(""))
	gob.Register(int(0))
	gob.Register(float64(0))
	gob.Register(bool(false))
}

// DiskCacheManager manages disk-based caching
type DiskCacheManager struct {
	baseDir   string
	usage     atomic.Int64
	limit     int64
	cacheLock sync.RWMutex
}

// NewDiskCacheManager creates a new disk cache manager
func NewDiskCacheManager(baseDir string) *DiskCacheManager {
	// Create cache directory if it doesn't exist
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		log.Printf("Failed to create cache directory: %v", err)
	}

	return &DiskCacheManager{
		baseDir: baseDir,
		limit:   DiskCacheLimit,
	}
}

// Store stores data in the disk cache
func (dc *DiskCacheManager) Store(key string, data interface{}, size int64, compress bool) error {
	dc.cacheLock.Lock()
	defer dc.cacheLock.Unlock()

	// Check if we're overwriting existing data
	existingSize := int64(0)
	if info, err := os.Stat(dc.getFilePath(key)); err == nil {
		existingSize = info.Size()
	}

	// Check disk limits
	currentUsage := dc.usage.Load()
	if currentUsage-existingSize+size > dc.limit {
		return fmt.Errorf("disk cache limit exceeded")
	}

	// Wrap data for encoding
	wrapper := DataWrapper{Data: data}

	// Encode data
	var buf bytes.Buffer
	enc := gob.NewEncoder(&buf)
	if err := enc.Encode(wrapper); err != nil {
		return fmt.Errorf("failed to encode data: %v", err)
	}

	// Compress if requested
	var finalData []byte
	if compress {
		var compBuf bytes.Buffer
		gz := gzip.NewWriter(&compBuf)
		if _, err := gz.Write(buf.Bytes()); err != nil {
			return fmt.Errorf("failed to compress data: %v", err)
		}
		if err := gz.Close(); err != nil {
			return fmt.Errorf("failed to close gzip writer: %v", err)
		}
		finalData = compBuf.Bytes()
	} else {
		finalData = buf.Bytes()
	}

	// Write to file
	if err := ioutil.WriteFile(dc.getFilePath(key), finalData, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %v", err)
	}

	// Update usage tracking
	actualSize := int64(len(finalData))
	dc.usage.Add(actualSize - existingSize)

	return nil
}

// Load loads data from the disk cache
func (dc *DiskCacheManager) Load(key string, compressed bool) (interface{}, error) {
	dc.cacheLock.RLock()
	defer dc.cacheLock.RUnlock()

	// Read file
	data, err := ioutil.ReadFile(dc.getFilePath(key))
	if err != nil {
		return nil, fmt.Errorf("failed to read cache file: %v", err)
	}

	// Decompress if needed
	var finalData []byte
	if compressed {
		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %v", err)
		}
		finalData, err = ioutil.ReadAll(gz)
		if err != nil {
			return nil, fmt.Errorf("failed to decompress data: %v", err)
		}
		if err := gz.Close(); err != nil {
			return nil, fmt.Errorf("failed to close gzip reader: %v", err)
		}
	} else {
		finalData = data
	}

	// Decode data
	var wrapper DataWrapper
	dec := gob.NewDecoder(bytes.NewReader(finalData))
	if err := dec.Decode(&wrapper); err != nil {
		return nil, fmt.Errorf("failed to decode data: %v", err)
	}

	return wrapper.Data, nil
}

// Delete removes data from the disk cache
func (dc *DiskCacheManager) Delete(key string) error {
	dc.cacheLock.Lock()
	defer dc.cacheLock.Unlock()

	// Get file size before deleting
	info, err := os.Stat(dc.getFilePath(key))
	if err != nil {
		return fmt.Errorf("failed to get file info: %v", err)
	}

	// Delete file
	if err := os.Remove(dc.getFilePath(key)); err != nil {
		return fmt.Errorf("failed to delete cache file: %v", err)
	}

	// Update usage tracking
	dc.usage.Add(-info.Size())

	return nil
}

// getFilePath returns the full path for a cache key
func (dc *DiskCacheManager) getFilePath(key string) string {
	return filepath.Join(dc.baseDir, fmt.Sprintf("%s.cache", key))
}

// GetUsage returns the current disk usage
func (dc *DiskCacheManager) GetUsage() int64 {
	return dc.usage.Load()
}

// GetLimit returns the disk cache limit
func (dc *DiskCacheManager) GetLimit() int64 {
	return dc.limit
}
