package main

import (
	"fmt"
	"time"

	"github.com/newcoderlife/media115/internal/cloud115"
	"github.com/newcoderlife/media115/internal/config"
	"github.com/newcoderlife/media115/internal/logging"
)

func getClient() (*cloud115.Client, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}
	if cfg.Auth.Cookies == "" {
		return nil, fmt.Errorf("未登录。请先运行: cloud115 auth")
	}
	logger := logging.Setup(verbose)
	return cloud115.NewClient(
		cfg.Auth.Cookies,
		cloud115.WithListingTTL(time.Duration(cfg.Cache.ListingTTL)*time.Second),
		cloud115.WithPathTTL(time.Duration(cfg.Cache.PathTTL)*time.Second),
		cloud115.WithLogger(logger),
	)
}

func getConfig() (*config.Config, error) {
	return config.Load()
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
