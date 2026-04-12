package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/newcoderlife/media115/internal/cloud115"
)

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "缓存管理命令",
}

var cacheStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "显示缓存状态统计",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := getClient()
		if err != nil {
			// Try without cookies for cache-only stats
			return showCacheStatusNoLogin()
		}
		defer client.Close()

		stats, err := client.CacheStatus()
		if err != nil {
			return fmt.Errorf("获取缓存状态失败: %w", err)
		}

		fmt.Println("cloud115 缓存:")
		fmt.Printf("  路径映射:    %d 条目\n", stats.PathCount)
		fmt.Printf("  目录缓存:    %d 个目录, %d 条目\n", stats.DirCount, stats.EntryCount)
		fmt.Printf("  目录树:      %d 条目\n", stats.TreeEntries)
		fmt.Printf("  数据库大小:  %s\n", formatSize(stats.DBSizeBytes))

		// Rate limit status
		now := float64(time.Now().Unix())
		for name, state := range stats.RateLimit {
			if now < state.CooldownUntil {
				remaining := int(state.CooldownUntil - now)
				fmt.Printf("  限流 (%s):  ⚠ cooldown 中 (剩余 %ds)\n", name, remaining)
			} else {
				fmt.Printf("  限流 (%s):  正常 (%d/20 QPM)\n", name, state.MinuteCount)
			}
		}
		return nil
	},
}

func showCacheStatusNoLogin() error {
	fmt.Println("cloud115 缓存: (需要登录查看详情)")
	fmt.Println("  请先运行 cloud115 auth 登录")
	return nil
}

var cacheClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "清除缓存（路径和目录 listing）",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Use direct Cache access — no login credentials required.
		cache, err := cloud115.NewCache("")
		if err != nil {
			return fmt.Errorf("打开缓存失败: %w", err)
		}
		defer cache.Close()
		cache.ClearMetadata()
		fmt.Println("缓存已清除（路径映射 + 目录 listing）")
		return nil
	},
}

func init() {
	cacheCmd.AddCommand(cacheStatusCmd)
	cacheCmd.AddCommand(cacheClearCmd)
	rootCmd.AddCommand(cacheCmd)
}
