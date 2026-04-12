package scraper

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/newcoderlife/media115/internal/config"
)

// CacheDir returns the media115 scrape cache root directory.
func CacheDir() string {
	return strings.Replace(config.CacheDir(), "cloud115", "media115", 1)
}

// CachePath returns the full path for a scrape cache entry.
func CachePath(source, key string) string {
	return filepath.Join(CacheDir(), "scrape", source, key+".json")
}

// CacheGet reads a JSON cache entry. Returns nil if absent or unreadable.
func CacheGet(source, key string) map[string]any {
	data, err := os.ReadFile(CachePath(source, key))
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return m
}

// CacheGetFileMap reads a file_map cache entry, treating _not_found as absent.
func CacheGetFileMap(key string) map[string]any {
	m := CacheGet("file_map", key)
	if m == nil {
		return nil
	}
	if nf, _ := m["_not_found"].(bool); nf {
		return nil
	}
	return m
}

// CachePut writes a JSON cache entry.
func CachePut(source, key string, data map[string]any) {
	p := CachePath(source, key)
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	if b, err := json.Marshal(data); err == nil {
		_ = os.WriteFile(p, b, 0o644)
	}
}

// CacheIsNotFound returns true if the entry exists with _not_found=true
// and was stored within the last ttlDays days.
func CacheIsNotFound(source, key string, ttlDays int) bool {
	m := CacheGet(source, key)
	if m == nil {
		return false
	}
	nf, _ := m["_not_found"].(bool)
	if !nf {
		return false
	}
	cachedAt, _ := m["_cached_at"].(float64)
	if cachedAt == 0 {
		return true
	}
	age := time.Now().Unix() - int64(cachedAt)
	return age < int64(ttlDays*24*3600)
}
