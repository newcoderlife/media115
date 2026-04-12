package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/newcoderlife/media115/internal/config"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "检查环境、凭证和缓存状态",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("media115 环境检查:")
		fmt.Println()

		// Config
		cfgPath := config.ConfigPath()
		fmt.Println("配置:")
		fmt.Printf("  配置文件:      %s\n", cfgPath)

		cfg, err := config.Load()
		if err != nil {
			fmt.Printf("  配置加载:      ✗ 失败 (%v)\n", err)
			cfg = config.Default()
		} else {
			fmt.Printf("  配置加载:      ✓\n")
		}
		fmt.Printf("  云盘根目录:    %s\n", cfg.Cloud.Root)
		fmt.Println()

		// Auth tokens
		fmt.Println("凭证:")
		if cfg.Auth.Cookies == "" {
			fmt.Println("  115 Cookies:   ✗ 未配置（请运行: cloud115 auth）")
		} else {
			fmt.Printf("  115 Cookies:   ✓ 已配置 (%d 字节)\n", len(cfg.Auth.Cookies))
		}
		check := func(name, token string) {
			if token != "" {
				fmt.Printf("  %-15s ✓ 已配置\n", name+":")
			} else {
				fmt.Printf("  %-15s ✗ 未配置\n", name+":")
			}
		}
		check("TMDB Token", cfg.Auth.TMDB.Token)
		check("Bangumi Token", cfg.Auth.Bangumi.Token)
		check("ThePornDB Token", cfg.Auth.ThePornDB.Token)
		check("StashDB API Key", cfg.Auth.StashDB.APIKey)
		fmt.Println()

		// 115 login
		fmt.Println("登录状态:")
		if cfg.Auth.Cookies == "" {
			fmt.Println("  115 登录:      ✗ 未配置")
		} else {
			client, err := getClient()
			if err != nil {
				fmt.Printf("  115 登录:      ✗ 初始化失败 (%v)\n", err)
			} else {
				defer client.Close()
				if client.CheckLogin() {
					fmt.Println("  115 登录:      ✓ 有效")
				} else {
					fmt.Println("  115 登录:      ✗ cookie 已过期（请运行: cloud115 auth）")
				}

				// Cache stats
				fmt.Println()
				fmt.Println("缓存 (cloud115):")
				fmt.Printf("  缓存目录:      %s\n", config.CacheDir())
				stats, err := client.CacheStatus()
				if err != nil {
					fmt.Printf("  缓存状态:      ✗ 获取失败 (%v)\n", err)
				} else {
					fmt.Printf("  路径映射:      %d 条目\n", stats.PathCount)
					fmt.Printf("  目录缓存:      %d 个目录, %d 条目\n", stats.DirCount, stats.EntryCount)
					fmt.Printf("  目录树:        %d 条目\n", stats.TreeEntries)
					fmt.Printf("  数据库大小:    %s\n", formatSize(stats.DBSizeBytes))

					now := float64(time.Now().Unix())
					for name, state := range stats.RateLimit {
						if now < state.CooldownUntil {
							remaining := int(state.CooldownUntil - now)
							fmt.Printf("  限流 (%s):    ⚠ cooldown 中 (剩余 %ds)\n", name, remaining)
						}
					}
				}
			}
		}

		// media115 cache (scrape output)
		fmt.Println()
		fmt.Println("缓存 (media115 scrape):")
		m115CacheDir := mediaCacheDir()
		fmt.Printf("  缓存目录:      %s\n", m115CacheDir)

		scrapeDir := filepath.Join(m115CacheDir, "scrape")
		if fi, err := os.Stat(scrapeDir); err == nil && fi.IsDir() {
			total := countJSONFiles(scrapeDir)
			fmt.Printf("  缓存条目:      %d 个JSON文件\n", total)
		} else {
			fmt.Println("  缓存条目:      0 (scrape目录不存在)")
		}

		scrapeOutputDir := filepath.Join(m115CacheDir, "scrape_output")
		if fi, err := os.Stat(scrapeOutputDir); err == nil && fi.IsDir() {
			entries, _ := os.ReadDir(scrapeOutputDir)
			fmt.Printf("  刮削输出目录:  %d 个分类\n", len(entries))
		} else {
			fmt.Println("  刮削输出目录:  不存在")
		}

		// Categories
		fmt.Println()
		fmt.Println("分类配置:")
		for name, cat := range cfg.Categories {
			fmt.Printf("  %-8s type=%-8s sources=%v\n", name, cat.Type, cat.Sources)
		}

		return nil
	},
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

func init() {
	// doctorCmd is registered in main.go init()
}
