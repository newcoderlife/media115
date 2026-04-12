package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	syncDeep  bool
	syncDepth int
)

var videoExts = []string{".mkv", ".mp4", ".avi", ".ts", ".rmvb", ".wmv", ".flv", ".mov", ".m4v", ".iso"}

var syncCmd = &cobra.Command{
	Use:   "sync <path>",
	Short: "刷新路径缓存。默认导出目录树，--deep 额外预热 listing 缓存。",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := args[0]

		client, err := getClient()
		if err != nil {
			return err
		}
		defer client.Close()

		fmt.Fprintf(os.Stderr, "正在导出目录树...\n")
		text, err := client.ExportTree(path)
		if err != nil {
			return fmt.Errorf("导出失败: %w", err)
		}
		if text == "" {
			return fmt.Errorf("导出失败: %s", path)
		}

		// Save to SQLite (tree_entry table — no more tree_cache.txt)
		count, err := client.SaveTree(text, videoExts, path)
		if err != nil {
			return fmt.Errorf("保存到 SQLite 失败: %w", err)
		}
		stats, err := client.TreeStats()
		if err != nil {
			return fmt.Errorf("获取 tree stats 失败: %w", err)
		}
		_ = count
		fmt.Printf("已保存到缓存：%d 条目，%d 个视频，%d 个 NFO\n", stats.Total, stats.Videos, stats.NFOs)

		if syncDeep {
			warmCount := 0
			progressFn := func(p string, n int) {
				warmCount++
				fmt.Fprintf(os.Stderr, "\r  预热中: %d 个目录", warmCount)
			}
			fmt.Fprintf(os.Stderr, "正在预热缓存 (深度=%d)...\n", syncDepth)
			if err := client.Warm(path, syncDepth, progressFn); err != nil {
				fmt.Fprintf(os.Stderr, "\n预热失败: %v\n", err)
			} else {
				fmt.Fprintf(os.Stderr, "\n")
				cacheStats, err := client.CacheStatus()
				if err == nil {
					fmt.Printf("预热完成：%d 路径，%d 目录，%d 条目\n",
						cacheStats.PathCount, cacheStats.DirCount, cacheStats.EntryCount)
				}
			}
		}
		return nil
	},
}


func init() {
	syncCmd.Flags().BoolVar(&syncDeep, "deep", false, "递归预热目录 listing 缓存")
	syncCmd.Flags().IntVar(&syncDepth, "depth", 3, "预热深度（配合 --deep，默认 3）")
	rootCmd.AddCommand(syncCmd)
}
