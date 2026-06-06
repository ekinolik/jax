package confluence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	pkgconfluence "github.com/ekinolik/jax/pkg/confluence"
)

const (
	defaultCacheDir = "cache/confluence"
	maxOIFileBytes  = 10 * 1024 * 1024
)

// OICache stores same-day OI slices on disk keyed by ticker, date, and expiration.
type OICache struct {
	dir string
}

// NewOICache creates an OI cache at dir (or CONFLUENCE_CACHE_DIR / default).
func NewOICache(dir string) (*OICache, error) {
	if dir == "" {
		dir = os.Getenv("CONFLUENCE_CACHE_DIR")
	}
	if dir == "" {
		dir = defaultCacheDir
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve OI cache dir: %w", err)
	}
	if strings.Contains(absDir, "..") {
		return nil, fmt.Errorf("invalid OI cache dir %q", dir)
	}
	if err := os.MkdirAll(filepath.Join(absDir, "oi"), 0o755); err != nil {
		return nil, fmt.Errorf("create OI cache dir: %w", err)
	}
	return &OICache{dir: absDir}, nil
}

// CacheDir returns the root cache directory.
func (c *OICache) CacheDir() string {
	return c.dir
}

// HasOI reports whether an OI file exists for ticker, date, and expiration.
func (c *OICache) HasOI(ticker string, date time.Time, expiration string) bool {
	path, err := c.oiPath(ticker, date, expiration)
	if err != nil {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Size() <= maxOIFileBytes
}

// LoadOI reads a cached OI slice for ticker, date, and expiration.
func (c *OICache) LoadOI(ticker string, date time.Time, expiration string) (*pkgconfluence.OptionSlice, error) {
	path, err := c.oiPath(ticker, date, expiration)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("read OI cache: %w", err)
	}
	if info.Size() > maxOIFileBytes {
		return nil, fmt.Errorf("OI cache file too large")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read OI cache: %w", err)
	}

	var slice pkgconfluence.OptionSlice
	if err := json.Unmarshal(data, &slice); err != nil {
		return nil, fmt.Errorf("decode OI cache: %w", err)
	}
	return &slice, nil
}

// StoreOI writes an OI slice to disk atomically for the rest of the trading day.
func (c *OICache) StoreOI(ticker string, date time.Time, expiration string, slice *pkgconfluence.OptionSlice) error {
	if slice == nil {
		return fmt.Errorf("nil OI slice")
	}

	path, err := c.oiPath(ticker, date, expiration)
	if err != nil {
		return err
	}

	data, err := json.Marshal(slice)
	if err != nil {
		return fmt.Errorf("encode OI cache: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create OI cache dir: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".oi-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp OI cache file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp OI cache file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp OI cache file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("commit OI cache file: %w", err)
	}
	return nil
}

// PurgeBefore deletes OI files dated before the given day.
func (c *OICache) PurgeBefore(before time.Time) error {
	oiDir := filepath.Join(c.dir, "oi")
	entries, err := os.ReadDir(oiDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read OI cache dir: %w", err)
	}

	cutoff := before.Format("2006-01-02")
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		datePart := extractCacheDate(entry.Name())
		if datePart != "" && datePart < cutoff {
			if err := os.Remove(filepath.Join(oiDir, entry.Name())); err != nil {
				return fmt.Errorf("purge OI cache file %s: %w", entry.Name(), err)
			}
		}
	}
	return nil
}

func (c *OICache) oiPath(ticker string, date time.Time, expiration string) (string, error) {
	if err := pkgconfluence.ValidateTicker(ticker); err != nil {
		return "", err
	}
	if _, err := time.Parse("2006-01-02", expiration); err != nil {
		return "", fmt.Errorf("invalid expiration date %q: %w", expiration, err)
	}

	filename := fmt.Sprintf("%s_%s_%s.json", pkgconfluence.NormalizeTicker(ticker), date.Format("2006-01-02"), expiration)
	path := filepath.Join(c.dir, "oi", filename)
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	oiRoot, err := filepath.Abs(filepath.Join(c.dir, "oi"))
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(oiRoot, absPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("invalid OI cache path for ticker %q", ticker)
	}
	return absPath, nil
}

func extractCacheDate(filename string) string {
	parts := strings.Split(strings.TrimSuffix(filename, ".json"), "_")
	if len(parts) < 2 {
		return ""
	}
	return parts[len(parts)-2]
}
