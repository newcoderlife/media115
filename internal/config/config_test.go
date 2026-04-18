package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	// Cloud defaults
	if cfg.Cloud.Root != "/影音" {
		t.Errorf("Cloud.Root = %q; want /影音", cfg.Cloud.Root)
	}
	if cfg.Cloud.QPS != 0.5 {
		t.Errorf("Cloud.QPS = %v; want 0.5", cfg.Cloud.QPS)
	}
	if cfg.Cloud.QPM != 20 {
		t.Errorf("Cloud.QPM = %d; want 20", cfg.Cloud.QPM)
	}
	if cfg.Cloud.CooldownSeconds != 3600 {
		t.Errorf("Cloud.CooldownSeconds = %d; want 3600", cfg.Cloud.CooldownSeconds)
	}

	// Cache defaults
	if cfg.Cache.ListingTTL != 3600 {
		t.Errorf("Cache.ListingTTL = %d; want 3600", cfg.Cache.ListingTTL)
	}
	if cfg.Cache.PathTTL != 86400 {
		t.Errorf("Cache.PathTTL = %d; want 86400", cfg.Cache.PathTTL)
	}

	// Proxy defaults
	if cfg.Proxy.Host != "127.0.0.1" {
		t.Errorf("Proxy.Host = %q; want 127.0.0.1", cfg.Proxy.Host)
	}
	if cfg.Proxy.Port != 9000 {
		t.Errorf("Proxy.Port = %d; want 9000", cfg.Proxy.Port)
	}
	if cfg.Proxy.JellyfinURL != "http://localhost:8096" {
		t.Errorf("Proxy.JellyfinURL = %q; want http://localhost:8096", cfg.Proxy.JellyfinURL)
	}

	// Categories
	if len(cfg.Categories) != 4 {
		t.Errorf("len(Categories) = %d; want 4", len(cfg.Categories))
	}
	movie, ok := cfg.Categories["电影"]
	if !ok {
		t.Fatal("missing category 电影")
	}
	if movie.Type != "movie" {
		t.Errorf("电影.Type = %q; want movie", movie.Type)
	}
	if movie.Naming != "{title} ({year})" {
		t.Errorf("电影.Naming = %q; want {title} ({year})", movie.Naming)
	}
	if len(movie.Sources) != 1 || movie.Sources[0] != "tmdb" {
		t.Errorf("电影.Sources = %v; want [tmdb]", movie.Sources)
	}

	av, ok := cfg.Categories["AV"]
	if !ok {
		t.Fatal("missing category AV")
	}
	if av.Type != "av" {
		t.Errorf("AV.Type = %q; want av", av.Type)
	}
	if len(av.Sources) != 2 {
		t.Errorf("AV.Sources = %v; want [jav321 javfree]", av.Sources)
	}
}

func TestLoadMissingFile(t *testing.T) {
	// Point config to a temp dir with no config file
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v; want nil", err)
	}
	if cfg == nil {
		t.Fatal("Load() returned nil config")
	}
	// Should have defaults
	if cfg.Cloud.QPS != 0.5 {
		t.Errorf("Cloud.QPS = %v; want 0.5 (default)", cfg.Cloud.QPS)
	}
	if cfg.Proxy.Port != 9000 {
		t.Errorf("Proxy.Port = %d; want 9000 (default)", cfg.Proxy.Port)
	}
}

func TestLoadFromFile(t *testing.T) {
	tmp := t.TempDir()
	cfgDir := filepath.Join(tmp, "media115")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tomlContent := `
[auth]
cookies = "test-cookie"

[cloud]
root = "/media"
qps = 1.0
qpm = 30
cooldown_seconds = 7200

[cache]
listing_ttl = 600
path_ttl = 1200

[proxy]
host = "10.0.0.1"
port = 8080
jellyfin_url = "http://jellyfin:8096"

[categories.movies]
type = "movie"
naming = "{title}"
sources = ["tmdb"]
`
	cfgFile := filepath.Join(cfgDir, "config.toml")
	if err := os.WriteFile(cfgFile, []byte(tomlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("XDG_CONFIG_HOME", tmp)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Auth.Cookies != "test-cookie" {
		t.Errorf("Auth.Cookies = %q; want test-cookie", cfg.Auth.Cookies)
	}
	if cfg.Cloud.Root != "/media" {
		t.Errorf("Cloud.Root = %q; want /media", cfg.Cloud.Root)
	}
	if cfg.Cloud.QPS != 1.0 {
		t.Errorf("Cloud.QPS = %v; want 1.0", cfg.Cloud.QPS)
	}
	if cfg.Cloud.QPM != 30 {
		t.Errorf("Cloud.QPM = %d; want 30", cfg.Cloud.QPM)
	}
	if cfg.Cloud.CooldownSeconds != 7200 {
		t.Errorf("Cloud.CooldownSeconds = %d; want 7200", cfg.Cloud.CooldownSeconds)
	}
	if cfg.Cache.ListingTTL != 600 {
		t.Errorf("Cache.ListingTTL = %d; want 600", cfg.Cache.ListingTTL)
	}
	if cfg.Cache.PathTTL != 1200 {
		t.Errorf("Cache.PathTTL = %d; want 1200", cfg.Cache.PathTTL)
	}
	if cfg.Proxy.Host != "10.0.0.1" {
		t.Errorf("Proxy.Host = %q; want 10.0.0.1", cfg.Proxy.Host)
	}
	if cfg.Proxy.Port != 8080 {
		t.Errorf("Proxy.Port = %d; want 8080", cfg.Proxy.Port)
	}
	if cfg.Proxy.JellyfinURL != "http://jellyfin:8096" {
		t.Errorf("Proxy.JellyfinURL = %q; want http://jellyfin:8096", cfg.Proxy.JellyfinURL)
	}
	movies, ok := cfg.Categories["movies"]
	if !ok {
		t.Fatal("missing category movies")
	}
	if movies.Type != "movie" {
		t.Errorf("movies.Type = %q; want movie", movies.Type)
	}
}

func TestLoadZeroValueFallsBackToDefaults(t *testing.T) {
	// When file exists but omits qps/qpm/etc, defaults should fill in
	tmp := t.TempDir()
	cfgDir := filepath.Join(tmp, "media115")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tomlContent := `
[cloud]
root = "/custom"
`
	cfgFile := filepath.Join(cfgDir, "config.toml")
	if err := os.WriteFile(cfgFile, []byte(tomlContent), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", tmp)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Cloud.Root != "/custom" {
		t.Errorf("Cloud.Root = %q; want /custom", cfg.Cloud.Root)
	}
	// Zero values should be filled with defaults
	if cfg.Cloud.QPS != 0.5 {
		t.Errorf("Cloud.QPS = %v; want 0.5 (default fallback)", cfg.Cloud.QPS)
	}
	if cfg.Cloud.QPM != 20 {
		t.Errorf("Cloud.QPM = %d; want 20 (default fallback)", cfg.Cloud.QPM)
	}
	if cfg.Proxy.Port != 9000 {
		t.Errorf("Proxy.Port = %d; want 9000 (default fallback)", cfg.Proxy.Port)
	}
	if cfg.Proxy.Host != "127.0.0.1" {
		t.Errorf("Proxy.Host = %q; want 127.0.0.1 (default fallback)", cfg.Proxy.Host)
	}
}

func TestSave(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	cfg := Default()
	cfg.Auth.Cookies = "saved-cookie"
	cfg.Cloud.Root = "/saved"
	cfg.Proxy.Port = 7070

	if err := cfg.Save(); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Verify file exists
	cfgPath := filepath.Join(tmp, "media115", "config.toml")
	if _, err := os.Stat(cfgPath); err != nil {
		t.Fatalf("config file not found after Save: %v", err)
	}

	// Load it back
	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() after Save error = %v", err)
	}

	if loaded.Auth.Cookies != "saved-cookie" {
		t.Errorf("round-trip Auth.Cookies = %q; want saved-cookie", loaded.Auth.Cookies)
	}
	if loaded.Cloud.Root != "/saved" {
		t.Errorf("round-trip Cloud.Root = %q; want /saved", loaded.Cloud.Root)
	}
	if loaded.Proxy.Port != 7070 {
		t.Errorf("round-trip Proxy.Port = %d; want 7070", loaded.Proxy.Port)
	}
	if loaded.Cloud.QPS != 0.5 {
		t.Errorf("round-trip Cloud.QPS = %v; want 0.5", loaded.Cloud.QPS)
	}
}

func TestConfigDir(t *testing.T) {
	// With XDG_CONFIG_HOME set
	t.Setenv("XDG_CONFIG_HOME", "/tmp/custom-config")
	dir := ConfigDir()
	if dir != "/tmp/custom-config/media115" {
		t.Errorf("ConfigDir() = %q; want /tmp/custom-config/media115", dir)
	}

	// Without XDG_CONFIG_HOME (unset)
	t.Setenv("XDG_CONFIG_HOME", "")
	configBase, _ := os.UserConfigDir()
	dir = ConfigDir()
	expected := filepath.Join(configBase, "media115")
	if dir != expected {
		t.Errorf("ConfigDir() = %q; want %q", dir, expected)
	}
}

func TestCacheDir(t *testing.T) {
	// With XDG_CACHE_HOME set
	t.Setenv("XDG_CACHE_HOME", "/tmp/custom-cache")
	dir := CacheDir()
	if dir != "/tmp/custom-cache/media115" {
		t.Errorf("CacheDir() = %q; want /tmp/custom-cache/media115", dir)
	}

	// Without XDG_CACHE_HOME (unset)
	t.Setenv("XDG_CACHE_HOME", "")
	cacheBase, _ := os.UserCacheDir()
	dir = CacheDir()
	expected := filepath.Join(cacheBase, "media115")
	if dir != expected {
		t.Errorf("CacheDir() = %q; want %q", dir, expected)
	}
}
