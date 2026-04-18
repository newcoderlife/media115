package config

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/bytedance/gg/gptr"
)

type Config struct {
	Auth       AuthConfig                `toml:"auth"`
	Cloud      CloudConfig               `toml:"cloud"`
	Cache      CacheConfig               `toml:"cache"`
	Categories map[string]CategoryConfig `toml:"categories"`
	Proxy      ProxyConfig               `toml:"proxy"`
	MTeam      MTeamConfig               `toml:"mteam"`
}

type MTeamConfig struct {
	APIKey             string `toml:"api_key"`
	BaseURL            string `toml:"base_url"`
	MinIntervalSeconds *int   `toml:"min_interval_seconds"`
	DefaultWatchDir    string `toml:"default_watch_dir"`
}

type AuthConfig struct {
	Cookies   string        `toml:"cookies"`
	TMDB      TMDBAuth      `toml:"tmdb"`
	Bangumi   BangumiAuth   `toml:"bangumi"`
	ThePornDB ThePornDBAuth `toml:"theporndb"`
	StashDB   StashDBAuth   `toml:"stashdb"`
}

type TMDBAuth struct {
	Token string `toml:"token"`
}
type BangumiAuth struct {
	Token string `toml:"token"`
}
type ThePornDBAuth struct {
	Token string `toml:"token"`
}
type StashDBAuth struct {
	APIKey string `toml:"api_key"`
}

type CloudConfig struct {
	Root            string  `toml:"root"`
	QPS             float64 `toml:"qps"`
	QPM             int     `toml:"qpm"`
	CooldownSeconds int     `toml:"cooldown_seconds"`
}

type CacheConfig struct {
	ListingTTL int `toml:"listing_ttl"`
	PathTTL    int `toml:"path_ttl"`
}

type CategoryConfig struct {
	Type    string   `toml:"type"`
	Naming  string   `toml:"naming"`
	Sources []string `toml:"sources"`
}

type ProxyConfig struct {
	Host        string `toml:"host"`
	Port        int    `toml:"port"`
	JellyfinURL string `toml:"jellyfin_url"`
}

func Default() *Config {
	return &Config{
		Cloud: CloudConfig{Root: "/影音", QPS: 0.5, QPM: 20, CooldownSeconds: 3600},
		Cache: CacheConfig{ListingTTL: 3600, PathTTL: 86400},
		Categories: map[string]CategoryConfig{
			"电影": {Type: "movie", Naming: "{title} ({year})", Sources: []string{"tmdb"}},
			"剧目": {Type: "tv", Naming: "{title} ({year})", Sources: []string{"tmdb", "bangumi"}},
			"AV": {Type: "av", Naming: "{number}", Sources: []string{"jav321", "javfree"}},
			"写真": {Type: "gravure", Naming: "{number}", Sources: []string{"jav321", "javfree"}},
		},
		Proxy: ProxyConfig{Host: "127.0.0.1", Port: 9000, JellyfinURL: "http://localhost:8096"},
		MTeam: MTeamConfig{BaseURL: "https://api.m-team.cc/api", MinIntervalSeconds: gptr.Of(30)},
	}
}

func ConfigDir() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base, _ = os.UserConfigDir()
	}
	return filepath.Join(base, "media115")
}

func ConfigPath() string { return filepath.Join(ConfigDir(), "config.toml") }

func CacheDir() string {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		base, _ = os.UserCacheDir()
	}
	return filepath.Join(base, "media115")
}

func Load() (*Config, error) {
	cfg := Default()
	if _, err := os.Stat(ConfigPath()); os.IsNotExist(err) {
		return cfg, nil
	}
	if _, err := toml.DecodeFile(ConfigPath(), cfg); err != nil {
		return nil, err
	}
	// Fill zero values with defaults
	d := Default()
	if cfg.Cloud.QPS == 0 {
		cfg.Cloud.QPS = d.Cloud.QPS
	}
	if cfg.Cloud.QPM == 0 {
		cfg.Cloud.QPM = d.Cloud.QPM
	}
	if cfg.Cache.ListingTTL == 0 {
		cfg.Cache.ListingTTL = d.Cache.ListingTTL
	}
	if cfg.Cache.PathTTL == 0 {
		cfg.Cache.PathTTL = d.Cache.PathTTL
	}
	if cfg.Proxy.Port == 0 {
		cfg.Proxy.Port = d.Proxy.Port
	}
	if cfg.Proxy.Host == "" {
		cfg.Proxy.Host = d.Proxy.Host
	}
	if cfg.MTeam.BaseURL == "" {
		cfg.MTeam.BaseURL = d.MTeam.BaseURL
	}
	if cfg.MTeam.MinIntervalSeconds == nil {
		cfg.MTeam.MinIntervalSeconds = d.MTeam.MinIntervalSeconds
	}
	return cfg, nil
}

func (c *Config) Save() error {
	if err := os.MkdirAll(ConfigDir(), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(ConfigPath(), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return toml.NewEncoder(f).Encode(c)
}
