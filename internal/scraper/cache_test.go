package scraper

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCacheDir(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/tmp/test-cache-dir")
	dir := CacheDir()
	if !strings.Contains(dir, "media115") {
		t.Errorf("CacheDir() = %q; want path containing media115", dir)
	}
	// Should replace cloud115 with media115
	if strings.Contains(dir, "cloud115") {
		t.Errorf("CacheDir() = %q; should not contain cloud115", dir)
	}
}

func TestCachePath(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "/tmp/test-cache-path")
	p := CachePath("tmdb", "12345")
	if !strings.HasSuffix(p, filepath.Join("scrape", "tmdb", "12345.json")) {
		t.Errorf("CachePath() = %q; want path ending with scrape/tmdb/12345.json", p)
	}
}

func TestCacheGetMissingFile(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	m := CacheGet("nonexistent_source", "nonexistent_key")
	if m != nil {
		t.Errorf("CacheGet for missing file = %v; want nil", m)
	}
}

func TestCacheGetMalformedJSON(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)
	p := CachePath("bad", "data")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("not json {{{"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := CacheGet("bad", "data")
	if m != nil {
		t.Errorf("CacheGet for malformed JSON = %v; want nil", m)
	}
}

func TestCachePutAndGet(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	data := map[string]any{"title": "Test Movie", "year": float64(2024)}
	CachePut("tmdb", "movie_123", data)

	got := CacheGet("tmdb", "movie_123")
	if got == nil {
		t.Fatal("CacheGet after CachePut returned nil")
	}
	if got["title"] != "Test Movie" {
		t.Errorf("title = %v; want Test Movie", got["title"])
	}
	if got["year"] != float64(2024) {
		t.Errorf("year = %v; want 2024", got["year"])
	}
}

func TestCacheGetFileMapNil(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)
	m := CacheGetFileMap("nonexistent")
	if m != nil {
		t.Errorf("CacheGetFileMap for missing entry = %v; want nil", m)
	}
}

func TestCacheGetFileMapNotFound(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	data := map[string]any{"_not_found": true, "_cached_at": float64(time.Now().Unix())}
	CachePut("file_map", "missing_file", data)

	m := CacheGetFileMap("missing_file")
	if m != nil {
		t.Errorf("CacheGetFileMap for _not_found entry = %v; want nil", m)
	}
}

func TestCacheGetFileMapValid(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	data := map[string]any{"file_id": "abc123", "parent": "AV"}
	CachePut("file_map", "valid_file", data)

	m := CacheGetFileMap("valid_file")
	if m == nil {
		t.Fatal("CacheGetFileMap for valid entry returned nil")
	}
	if m["file_id"] != "abc123" {
		t.Errorf("file_id = %v; want abc123", m["file_id"])
	}
}

func TestCacheIsNotFoundMissing(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)
	if CacheIsNotFound("src", "missing", 30) {
		t.Error("CacheIsNotFound for missing entry should be false")
	}
}

func TestCacheIsNotFoundNoFlag(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	data := map[string]any{"title": "Found Item"}
	CachePut("src", "found", data)

	if CacheIsNotFound("src", "found", 30) {
		t.Error("CacheIsNotFound for entry without _not_found should be false")
	}
}

func TestCacheIsNotFoundNoTimestamp(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	data := map[string]any{"_not_found": true}
	CachePut("src", "notfound_no_ts", data)

	if !CacheIsNotFound("src", "notfound_no_ts", 30) {
		t.Error("CacheIsNotFound with _not_found=true but no timestamp should be true")
	}
}

func TestCacheIsNotFoundTTLValid(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	data := map[string]any{
		"_not_found": true,
		"_cached_at": float64(time.Now().Unix()),
	}
	CachePut("src", "recent", data)

	if !CacheIsNotFound("src", "recent", 30) {
		t.Error("CacheIsNotFound for recent _not_found entry should be true")
	}
}

func TestCacheIsNotFoundTTLExpired(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	// Set cached_at to 31 days ago
	data := map[string]any{
		"_not_found": true,
		"_cached_at": float64(time.Now().Unix() - 31*24*3600),
	}
	CachePut("src", "old", data)

	if CacheIsNotFound("src", "old", 30) {
		t.Error("CacheIsNotFound for expired _not_found entry should be false")
	}
}

func TestCachePutCreatesDirectories(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	data := map[string]any{"key": "value"}
	CachePut("deep/nested/source", "key1", data)

	p := CachePath("deep/nested/source", "key1")
	if _, err := os.Stat(p); err != nil {
		t.Errorf("CachePut should create parent directories; file not found: %v", err)
	}
}

func TestCachePutOverwrite(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	CachePut("src", "overwrite", map[string]any{"v": float64(1)})
	CachePut("src", "overwrite", map[string]any{"v": float64(2)})

	got := CacheGet("src", "overwrite")
	if got == nil {
		t.Fatal("CacheGet after overwrite returned nil")
	}
	if got["v"] != float64(2) {
		t.Errorf("v = %v; want 2 after overwrite", got["v"])
	}
}

func TestCachePutUnmarshalableValue(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	// json.Marshal will fail for unmarshalable types like functions
	data := map[string]any{"fn": func() {}}

	// Should not panic
	CachePut("src", "bad_marshal", data)

	// File should not exist since marshal failed
	p := CachePath("src", "bad_marshal")
	raw, err := os.ReadFile(p)
	if err == nil {
		// If directory was created but file should be empty or not valid JSON
		var m map[string]any
		if json.Unmarshal(raw, &m) == nil {
			t.Error("expected no valid JSON file for unmarshalable data")
		}
	}
}
