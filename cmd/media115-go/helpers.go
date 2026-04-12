package main

import (
	"fmt"
	"os"
	"time"

	"github.com/newcoderlife/media115/internal/cloud115"
	"github.com/newcoderlife/media115/internal/config"
	"github.com/newcoderlife/media115/internal/logging"
	"github.com/newcoderlife/media115/internal/provider/bangumi"
	"github.com/newcoderlife/media115/internal/provider/jav321"
	"github.com/newcoderlife/media115/internal/provider/javfree"
	"github.com/newcoderlife/media115/internal/provider/stashdb"
	"github.com/newcoderlife/media115/internal/provider/theporndb"
	"github.com/newcoderlife/media115/internal/provider/tmdb"
	"github.com/newcoderlife/media115/internal/scraper"
	"github.com/spf13/cobra"
)

var stats *logging.Stats

func getClient() (*cloud115.Client, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if cfg.Auth.Cookies == "" {
		return nil, fmt.Errorf("未登录。请先运行: cloud115 auth")
	}
	stats = logging.Setup(verbose)
	return cloud115.NewClient(
		cfg.Auth.Cookies,
		cloud115.WithListingTTL(time.Duration(cfg.Cache.ListingTTL)*time.Second),
		cloud115.WithPathTTL(time.Duration(cfg.Cache.PathTTL)*time.Second),
		cloud115.WithStats(stats),
	)
}

func init() {
	rootCmd.PersistentPostRun = func(cmd *cobra.Command, args []string) {
		if stats != nil && verbose {
			fmt.Fprintf(os.Stderr, "\n%s\n", stats.Summary())
		}
	}
}

func getConfig() (*config.Config, error) {
	return config.Load()
}

func registerProviders(cfg *config.Config) {
	if cfg.Auth.TMDB.Token != "" {
		scraper.Register(tmdb.New(cfg.Auth.TMDB.Token))
	}
	if cfg.Auth.Bangumi.Token != "" {
		scraper.Register(bangumi.New(cfg.Auth.Bangumi.Token))
	}
	// jav321 and javfree don't need tokens
	scraper.Register(jav321.New())
	scraper.Register(javfree.New())
	if cfg.Auth.ThePornDB.Token != "" {
		scraper.Register(theporndb.New(cfg.Auth.ThePornDB.Token))
	}
	if cfg.Auth.StashDB.APIKey != "" {
		scraper.Register(stashdb.New(cfg.Auth.StashDB.APIKey))
	}
}

func formatSize(size int64) string {
	units := []string{"B", "K", "M", "G", "T"}
	f := float64(size)
	for _, u := range units {
		if f < 1024 {
			if u == "B" {
				return fmt.Sprintf("%dB", int(f))
			}
			return fmt.Sprintf("%.1f%s", f, u)
		}
		f /= 1024
	}
	return fmt.Sprintf("%.1fP", f)
}

// trunc truncates a string to maxLen, appending "…" if truncated.
func trunc(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 1 {
		return "…"
	}
	return string(runes[:maxLen-1]) + "…"
}

// getTreeEntries reads TreeEntry rows from the SQLite cache.
func getTreeEntries(category string) ([]cloud115.TreeEntry, error) {
	cache, err := cloud115.NewCache(cloud115.DefaultCachePath())
	if err != nil {
		return nil, err
	}
	defer cache.Close()
	return cache.GetTreeEntries(category), nil
}
