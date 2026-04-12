package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/newcoderlife/media115/internal/config"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "检查环境状态和配置",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("cloud115 环境检查:")
		fmt.Println()

		// Config file
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
		fmt.Println()

		// Auth + Login check — create one client and reuse it.
		fmt.Println("认证:")
		if cfg.Auth.Cookies == "" {
			fmt.Println("  115 Cookies:   ✗ 未配置")
			if cfg.Auth.TMDB.Token != "" {
				fmt.Println("  TMDB Token:    ✓ 已配置")
			} else {
				fmt.Println("  TMDB Token:    ✗ 未配置")
			}
			fmt.Println()
			fmt.Println("登录状态:")
			fmt.Println("  115 登录:      ✗ 未配置 cookies（请运行 cloud115 auth）")
		} else {
			client, clientErr := getClient()
			if clientErr != nil {
				fmt.Println("  115 Cookies:   ✗ 配置但无法创建客户端")
				if cfg.Auth.TMDB.Token != "" {
					fmt.Println("  TMDB Token:    ✓ 已配置")
				} else {
					fmt.Println("  TMDB Token:    ✗ 未配置")
				}
			} else {
				defer client.Close()
				loggedIn := client.CheckLogin()
				if loggedIn {
					fmt.Println("  115 Cookies:   ✓ 已登录")
				} else {
					fmt.Println("  115 Cookies:   ✗ Cookie 过期")
				}
				if cfg.Auth.TMDB.Token != "" {
					fmt.Println("  TMDB Token:    ✓ 已配置")
				} else {
					fmt.Println("  TMDB Token:    ✗ 未配置")
				}
				fmt.Println()
				fmt.Println("登录状态:")
				if loggedIn {
					fmt.Println("  115 登录:      ✓ 有效")
				} else {
					fmt.Println("  115 登录:      ✗ cookie 已过期（请运行 cloud115 auth）")
				}

				// Cache stats
				fmt.Println()
				fmt.Println("缓存:")
				fmt.Printf("  缓存目录:      %s\n", config.CacheDir())
				cstats, err := client.CacheStatus()
				if err != nil {
					fmt.Printf("  缓存状态:      ✗ 获取失败 (%v)\n", err)
				} else {
					fmt.Printf("  路径映射:      %d 条目\n", cstats.PathCount)
					fmt.Printf("  目录缓存:      %d 个目录, %d 条目\n", cstats.DirCount, cstats.EntryCount)
					fmt.Printf("  目录树:        %d 条目\n", cstats.TreeEntries)
					fmt.Printf("  数据库大小:    %s\n", formatSize(cstats.DBSizeBytes))

					// Rate limit
					now := float64(time.Now().Unix())
					for name, state := range cstats.RateLimit {
						if now < state.CooldownUntil {
							remaining := int(state.CooldownUntil - now)
							fmt.Printf("  限流 (%s):    ⚠ cooldown 中 (剩余 %ds)\n", name, remaining)
						} else {
							fmt.Printf("  限流 (%s):    正常 (%d/20 QPM)\n", name, state.MinuteCount)
						}
					}
				}
			}
		}

		fmt.Println()
		fmt.Println("代理配置:")
		fmt.Printf("  strm-proxy:    %s:%d\n", cfg.Proxy.Host, cfg.Proxy.Port)
		fmt.Printf("  Jellyfin URL:  %s\n", cfg.Proxy.JellyfinURL)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(doctorCmd)
}
