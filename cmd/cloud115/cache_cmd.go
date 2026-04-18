package main

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/newcoderlife/media115/internal/cloud115"
)

type cacheStatusClient interface {
	CacheStatus() (*cloud115.CacheStats, error)
	Close() error
}

type cacheStatusOpts struct {
	Out       io.Writer
	GetClient func() (cacheStatusClient, error)
}

type cacheClearable interface {
	ClearMetadata()
	Close() error
}

type cacheClearOpts struct {
	Out      io.Writer
	NewCache func() (cacheClearable, error)
}

func newCacheCmd() *cobra.Command {
	parent := &cobra.Command{
		Use:   "cache",
		Short: "缓存管理命令",
	}

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "显示缓存状态统计",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cacheStatusRun(&cacheStatusOpts{
				Out: cmd.OutOrStdout(),
				GetClient: func() (cacheStatusClient, error) {
					return getClient()
				},
			})
		},
	}

	clearCmd := &cobra.Command{
		Use:   "clear",
		Short: "清除缓存（路径和目录 listing）",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cacheClearRun(&cacheClearOpts{
				Out: cmd.OutOrStdout(),
				NewCache: func() (cacheClearable, error) {
					return cloud115.NewCache("")
				},
			})
		},
	}

	parent.AddCommand(statusCmd, clearCmd)
	return parent
}

func cacheStatusRun(o *cacheStatusOpts) error {
	client, err := o.GetClient()
	if err != nil {
		return showCacheStatusNoLogin(o.Out)
	}
	defer func() { _ = client.Close() }()

	stats, err := client.CacheStatus()
	if err != nil {
		return fmt.Errorf("获取缓存状态失败: %w", err)
	}

	fmt.Fprintln(o.Out, "cloud115 缓存:")
	fmt.Fprintf(o.Out, "  路径映射:    %d 条目\n", stats.PathCount)
	fmt.Fprintf(o.Out, "  目录缓存:    %d 个目录, %d 条目\n", stats.DirCount, stats.EntryCount)
	fmt.Fprintf(o.Out, "  目录树:      %d 条目\n", stats.TreeEntries)
	fmt.Fprintf(o.Out, "  数据库大小:  %s\n", formatSize(stats.DBSizeBytes))

	now := float64(time.Now().Unix())
	for name, state := range stats.RateLimit {
		if now < state.CooldownUntil {
			remaining := int(state.CooldownUntil - now)
			fmt.Fprintf(o.Out, "  限流 (%s):  ⚠ cooldown 中 (剩余 %ds)\n", name, remaining)
		} else {
			fmt.Fprintf(o.Out, "  限流 (%s):  正常 (%d/20 QPM)\n", name, state.MinuteCount)
		}
	}
	return nil
}

func showCacheStatusNoLogin(w io.Writer) error {
	fmt.Fprintln(w, "cloud115 缓存: (需要登录查看详情)")
	fmt.Fprintln(w, "  请先运行 cloud115 auth 登录")
	return nil
}

func cacheClearRun(o *cacheClearOpts) error {
	cache, err := o.NewCache()
	if err != nil {
		return fmt.Errorf("打开缓存失败: %w", err)
	}
	defer func() { _ = cache.Close() }()
	cache.ClearMetadata()
	fmt.Fprintln(o.Out, "缓存已清除（路径映射 + 目录 listing）")
	return nil
}

func init() {
	rootCmd.AddCommand(newCacheCmd())
}
