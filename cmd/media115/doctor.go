package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/newcoderlife/media115/internal/cloud115"
	"github.com/newcoderlife/media115/internal/config"
)

// doctorClient is the subset of cloud115.Client used by the doctor command.
type doctorClient interface {
	CheckLogin() bool
	CacheStatus() (*cloud115.CacheStats, error)
	Close() error
}

type doctorOpts struct {
	Out        io.Writer
	ConfigPath func() string
	LoadConfig func() (*config.Config, error)
	GetClient  func() (doctorClient, error)
	CacheDir   func() string
}

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "检查环境、凭证和缓存状态",
		RunE: func(cmd *cobra.Command, args []string) error {
			return doctorRun(&doctorOpts{
				Out:        cmd.OutOrStdout(),
				ConfigPath: config.ConfigPath,
				LoadConfig: config.Load,
				GetClient:  func() (doctorClient, error) { return getClient() },
				CacheDir:   config.CacheDir,
			})
		},
	}
}

func doctorRun(o *doctorOpts) error {
	w := o.Out
	fmt.Fprintln(w, "media115 环境检查:")
	fmt.Fprintln(w)

	// Config
	cfgPath := o.ConfigPath()
	fmt.Fprintln(w, "配置:")
	fmt.Fprintf(w, "  配置文件:      %s\n", cfgPath)

	cfg, err := o.LoadConfig()
	if err != nil {
		fmt.Fprintf(w, "  配置加载:      ✗ 失败 (%v)\n", err)
		cfg = config.Default()
	} else {
		fmt.Fprintf(w, "  配置加载:      ✓\n")
	}
	fmt.Fprintf(w, "  云盘根目录:    %s\n", cfg.Cloud.Root)
	fmt.Fprintln(w)

	// Auth tokens
	fmt.Fprintln(w, "凭证:")
	if cfg.Auth.Cookies == "" {
		fmt.Fprintln(w, "  115 Cookies:   ✗ 未配置（请运行: cloud115 auth）")
	} else {
		fmt.Fprintf(w, "  115 Cookies:   ✓ 已配置 (%d 字节)\n", len(cfg.Auth.Cookies))
	}
	check := func(name, token string) {
		if token != "" {
			fmt.Fprintf(w, "  %-15s ✓ 已配置\n", name+":")
		} else {
			fmt.Fprintf(w, "  %-15s ✗ 未配置\n", name+":")
		}
	}
	check("TMDB Token", cfg.Auth.TMDB.Token)
	check("Bangumi Token", cfg.Auth.Bangumi.Token)
	check("ThePornDB Token", cfg.Auth.ThePornDB.Token)
	check("StashDB API Key", cfg.Auth.StashDB.APIKey)
	fmt.Fprintln(w)

	// 115 login
	fmt.Fprintln(w, "登录状态:")
	var client doctorClient
	if cfg.Auth.Cookies == "" {
		fmt.Fprintln(w, "  115 登录:      ✗ 未配置")
	} else {
		var err error
		client, err = o.GetClient()
		if err != nil {
			fmt.Fprintf(w, "  115 登录:      ✗ 初始化失败 (%v)\n", err)
		} else {
			defer func() { _ = client.Close() }()
			if client.CheckLogin() {
				fmt.Fprintln(w, "  115 登录:      ✓ 有效")
			} else {
				fmt.Fprintln(w, "  115 登录:      ✗ cookie 已过期（请运行: cloud115 auth）")
			}
		}
	}

	// Cache
	fmt.Fprintln(w)
	fmt.Fprintln(w, "缓存:")
	cacheDir := o.CacheDir()
	fmt.Fprintf(w, "  缓存目录:      %s\n", cacheDir)

	if client != nil {
		stats, err := client.CacheStatus()
		if err != nil {
			fmt.Fprintf(w, "  缓存状态:      ✗ 获取失败 (%v)\n", err)
		} else {
			fmt.Fprintf(w, "  路径映射:      %d 条目\n", stats.PathCount)
			fmt.Fprintf(w, "  目录缓存:      %d 个目录, %d 条目\n", stats.DirCount, stats.EntryCount)
			fmt.Fprintf(w, "  目录树:        %d 条目\n", stats.TreeEntries)
			fmt.Fprintf(w, "  数据库大小:    %s\n", formatSize(stats.DBSizeBytes))

			now := float64(time.Now().Unix())
			for name, state := range stats.RateLimit {
				if now < state.CooldownUntil {
					remaining := int(state.CooldownUntil - now)
					fmt.Fprintf(w, "  限流 (%s):    ⚠ cooldown 中 (剩余 %ds)\n", name, remaining)
				}
			}
		}
	}

	scrapeDir := filepath.Join(cacheDir, "scrape")
	if fi, err := os.Stat(scrapeDir); err == nil && fi.IsDir() {
		total := countJSONFiles(scrapeDir)
		fmt.Fprintf(w, "  刮削缓存:      %d 个JSON文件\n", total)
	} else {
		fmt.Fprintf(w, "  刮削缓存:      0 (scrape目录不存在)\n")
	}

	scrapeOutputDir := filepath.Join(cacheDir, "scrape_output")
	if fi, err := os.Stat(scrapeOutputDir); err == nil && fi.IsDir() {
		entries, _ := os.ReadDir(scrapeOutputDir)
		fmt.Fprintf(w, "  刮削输出:      %d 个分类\n", len(entries))
	} else {
		fmt.Fprintf(w, "  刮削输出:      不存在\n")
	}

	// Categories
	fmt.Fprintln(w)
	fmt.Fprintln(w, "分类配置:")
	for name, cat := range cfg.Categories {
		fmt.Fprintf(w, "  %-8s type=%-8s sources=%v\n", name, cat.Type, cat.Sources)
	}

	return nil
}

// countJSONFiles counts JSON files recursively under dir.
func countJSONFiles(dir string) int {
	count := 0
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	for _, e := range entries {
		if e.IsDir() {
			count += countJSONFiles(filepath.Join(dir, e.Name()))
		} else if filepath.Ext(e.Name()) == ".json" {
			count++
		}
	}
	return count
}
