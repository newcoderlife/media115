package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigPath(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	path := ConfigPath()
	expected := filepath.Join(tmp, "media115", "config.toml")
	if path != expected {
		t.Errorf("ConfigPath() = %q, want %q", path, expected)
	}
}

func TestLoadInvalidTOML(t *testing.T) {
	tmp := t.TempDir()
	cfgDir := filepath.Join(tmp, "media115")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte("[[invalid toml"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", tmp)

	_, err := Load()
	if err == nil {
		t.Error("Load() with invalid TOML: expected error, got nil")
	}
}

func TestLoadZeroProxyHostFallback(t *testing.T) {
	tmp := t.TempDir()
	cfgDir := filepath.Join(tmp, "media115")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Config that sets proxy.port but not proxy.host
	toml := `
[proxy]
port = 8080
`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", tmp)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	// host should fall back to default since it's empty
	if cfg.Proxy.Host != "127.0.0.1" {
		t.Errorf("Proxy.Host = %q, want 127.0.0.1 (default)", cfg.Proxy.Host)
	}
	// port is set explicitly
	if cfg.Proxy.Port != 8080 {
		t.Errorf("Proxy.Port = %d, want 8080", cfg.Proxy.Port)
	}
}

func TestLoadZeroCacheTTLFallback(t *testing.T) {
	tmp := t.TempDir()
	cfgDir := filepath.Join(tmp, "media115")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Config that omits cache section entirely
	toml := `
[cloud]
qps = 2.0
`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", tmp)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if cfg.Cache.ListingTTL != 3600 {
		t.Errorf("Cache.ListingTTL = %d, want 3600 (default)", cfg.Cache.ListingTTL)
	}
	if cfg.Cache.PathTTL != 86400 {
		t.Errorf("Cache.PathTTL = %d, want 86400 (default)", cfg.Cache.PathTTL)
	}
}

func TestLoadZeroCloudQPMFallback(t *testing.T) {
	tmp := t.TempDir()
	cfgDir := filepath.Join(tmp, "media115")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Config sets qps but not qpm
	toml := `
[cloud]
qps = 1.5
`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", tmp)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	if cfg.Cloud.QPS != 1.5 {
		t.Errorf("Cloud.QPS = %v, want 1.5", cfg.Cloud.QPS)
	}
	if cfg.Cloud.QPM != 20 {
		t.Errorf("Cloud.QPM = %d, want 20 (default)", cfg.Cloud.QPM)
	}
}

func TestSaveRoundTripAuthTokens(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	cfg := Default()
	cfg.Auth.TMDB.Token = "tmdb-token-xyz"
	cfg.Auth.Bangumi.Token = "bangumi-token-abc"
	cfg.Auth.ThePornDB.Token = "theporndb-token-def"
	cfg.Auth.StashDB.APIKey = "stashdb-key-ghi"

	if err := cfg.Save(); err != nil {
		t.Fatalf("Save(): %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}

	if loaded.Auth.TMDB.Token != "tmdb-token-xyz" {
		t.Errorf("TMDB.Token = %q, want tmdb-token-xyz", loaded.Auth.TMDB.Token)
	}
	if loaded.Auth.Bangumi.Token != "bangumi-token-abc" {
		t.Errorf("Bangumi.Token = %q, want bangumi-token-abc", loaded.Auth.Bangumi.Token)
	}
	if loaded.Auth.ThePornDB.Token != "theporndb-token-def" {
		t.Errorf("ThePornDB.Token = %q, want theporndb-token-def", loaded.Auth.ThePornDB.Token)
	}
	if loaded.Auth.StashDB.APIKey != "stashdb-key-ghi" {
		t.Errorf("StashDB.APIKey = %q, want stashdb-key-ghi", loaded.Auth.StashDB.APIKey)
	}
}

func TestSaveCreatesDirectories(t *testing.T) {
	tmp := t.TempDir()
	// Use a nested path that doesn't exist yet
	nested := filepath.Join(tmp, "nested", "deep")
	t.Setenv("XDG_CONFIG_HOME", nested)

	cfg := Default()
	if err := cfg.Save(); err != nil {
		t.Fatalf("Save() with nested dirs: %v", err)
	}

	cfgPath := filepath.Join(nested, "media115", "config.toml")
	if _, err := os.Stat(cfgPath); err != nil {
		t.Errorf("config file not found: %v", err)
	}
}

func TestSaveFileContent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	cfg := Default()
	cfg.Cloud.Root = "/my/media"

	if err := cfg.Save(); err != nil {
		t.Fatalf("Save(): %v", err)
	}

	cfgPath := filepath.Join(tmp, "media115", "config.toml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "/my/media") {
		t.Errorf("saved config doesn't contain /my/media:\n%s", content)
	}
	// Should be TOML format
	if !strings.Contains(content, "[cloud]") {
		t.Errorf("saved config doesn't contain [cloud] section:\n%s", content)
	}
}

func TestDefaultCategories(t *testing.T) {
	cfg := Default()

	// Check "剧目" category
	drama, ok := cfg.Categories["剧目"]
	if !ok {
		t.Fatal("missing category 剧目")
	}
	if drama.Type != "tv" {
		t.Errorf("剧目.Type = %q, want tv", drama.Type)
	}
	if len(drama.Sources) != 2 {
		t.Errorf("剧目.Sources = %v, want [tmdb bangumi]", drama.Sources)
	}

	// Check "写真" category
	gravure, ok := cfg.Categories["写真"]
	if !ok {
		t.Fatal("missing category 写真")
	}
	if gravure.Type != "gravure" {
		t.Errorf("写真.Type = %q, want gravure", gravure.Type)
	}
	if gravure.Naming != "{number}" {
		t.Errorf("写真.Naming = %q, want {number}", gravure.Naming)
	}
}

func TestCacheDirWithoutXDG(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "")
	home, _ := os.UserHomeDir()
	dir := CacheDir()
	expected := filepath.Join(home, ".cache", "cloud115")
	if dir != expected {
		t.Errorf("CacheDir() = %q, want %q", dir, expected)
	}
	if !strings.HasSuffix(dir, "cloud115") {
		t.Errorf("CacheDir() should end with cloud115, got %q", dir)
	}
}

func TestConfigDirWithoutXDG(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "")
	home, _ := os.UserHomeDir()
	dir := ConfigDir()
	expected := filepath.Join(home, ".config", "media115")
	if dir != expected {
		t.Errorf("ConfigDir() = %q, want %q", dir, expected)
	}
}

func TestLoadAllZeroValuesFallback(t *testing.T) {
	// A config that explicitly sets everything to zero/empty to trigger all fallback paths
	tmp := t.TempDir()
	cfgDir := filepath.Join(tmp, "media115")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Empty config file — all numeric fields will be 0, host will be ""
	toml := `
[cloud]
root = ""

[proxy]
host = ""
`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", tmp)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	// All zero fields should have defaults
	if cfg.Cloud.QPS != 0.5 {
		t.Errorf("Cloud.QPS = %v, want 0.5", cfg.Cloud.QPS)
	}
	if cfg.Cloud.QPM != 20 {
		t.Errorf("Cloud.QPM = %d, want 20", cfg.Cloud.QPM)
	}
	if cfg.Cache.ListingTTL != 3600 {
		t.Errorf("Cache.ListingTTL = %d, want 3600", cfg.Cache.ListingTTL)
	}
	if cfg.Cache.PathTTL != 86400 {
		t.Errorf("Cache.PathTTL = %d, want 86400", cfg.Cache.PathTTL)
	}
	if cfg.Proxy.Port != 9000 {
		t.Errorf("Proxy.Port = %d, want 9000", cfg.Proxy.Port)
	}
	if cfg.Proxy.Host != "127.0.0.1" {
		t.Errorf("Proxy.Host = %q, want 127.0.0.1", cfg.Proxy.Host)
	}
}

// TestLoadExplicitZerosFallback explicitly sets all numeric fields to zero in
// TOML to trigger every fallback branch in Load().
func TestLoadExplicitZerosFallback(t *testing.T) {
	tmp := t.TempDir()
	cfgDir := filepath.Join(tmp, "media115")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `
[cloud]
root = "/custom"
qps = 0.0
qpm = 0
cooldown_seconds = 0

[cache]
listing_ttl = 0
path_ttl = 0

[proxy]
host = ""
port = 0
`
	if err := os.WriteFile(filepath.Join(cfgDir, "config.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", tmp)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load(): %v", err)
	}
	// Explicit-zero fields should be replaced with defaults.
	if cfg.Cloud.QPS != 0.5 {
		t.Errorf("Cloud.QPS = %v, want 0.5 (default fallback)", cfg.Cloud.QPS)
	}
	if cfg.Cloud.QPM != 20 {
		t.Errorf("Cloud.QPM = %d, want 20 (default fallback)", cfg.Cloud.QPM)
	}
	if cfg.Cache.ListingTTL != 3600 {
		t.Errorf("Cache.ListingTTL = %d, want 3600 (default fallback)", cfg.Cache.ListingTTL)
	}
	if cfg.Cache.PathTTL != 86400 {
		t.Errorf("Cache.PathTTL = %d, want 86400 (default fallback)", cfg.Cache.PathTTL)
	}
	if cfg.Proxy.Port != 9000 {
		t.Errorf("Proxy.Port = %d, want 9000 (default fallback)", cfg.Proxy.Port)
	}
	if cfg.Proxy.Host != "127.0.0.1" {
		t.Errorf("Proxy.Host = %q, want 127.0.0.1 (default fallback)", cfg.Proxy.Host)
	}
	// Non-zero field should keep its value.
	if cfg.Cloud.Root != "/custom" {
		t.Errorf("Cloud.Root = %q, want /custom", cfg.Cloud.Root)
	}
}

// TestSaveReadOnlyDir verifies Save returns an error when the config directory
// is read-only.
func TestSaveReadOnlyDir(t *testing.T) {
	tmp := t.TempDir()
	// Create a read-only directory
	roDir := filepath.Join(tmp, "readonly")
	if err := os.MkdirAll(roDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create the media115 dir inside it, then make parent read-only
	cfgDir := filepath.Join(roDir, "media115")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Make the config file non-writable
	cfgFile := filepath.Join(cfgDir, "config.toml")
	if err := os.WriteFile(cfgFile, []byte("old"), 0o444); err != nil {
		t.Fatal(err)
	}
	// Make directory read-only to prevent creating new files
	if err := os.Chmod(cfgDir, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(cfgDir, 0o755) })

	t.Setenv("XDG_CONFIG_HOME", roDir)

	cfg := Default()
	err := cfg.Save()
	if err == nil {
		t.Error("expected error saving to read-only directory")
	}
}
