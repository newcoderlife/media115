package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/newcoderlife/media115/internal/config"
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

		// Write to tree_cache.txt
		treePath := filepath.Join(config.CacheDir(), "tree_cache.txt")
		if err := os.MkdirAll(filepath.Dir(treePath), 0o755); err != nil {
			return fmt.Errorf("创建缓存目录失败: %w", err)
		}

		// Guard: don't overwrite full tree with smaller subtree
		existingLines := 0
		if data, err := os.ReadFile(treePath); err == nil {
			for _, b := range data {
				if b == '\n' {
					existingLines++
				}
			}
		}
		newLines := 0
		for _, b := range []byte(text) {
			if b == '\n' {
				newLines++
			}
		}

		if existingLines > 0 && newLines < existingLines/2 {
			fmt.Fprintf(os.Stderr, "警告: 当前 tree_cache 有 %d 行，新导出只有 %d 行\n", existingLines, newLines)
			fmt.Fprintf(os.Stderr, "  这通常意味着你导出了子目录而非根目录。已跳过写入。\n")
		} else {
			if err := os.WriteFile(treePath, []byte(text), 0o644); err != nil {
				return fmt.Errorf("写入 tree_cache.txt 失败: %w", err)
			}

			videoCount := countVideoLines(text)
			fmt.Printf("已保存 tree_cache.txt：%d 行，%d 个视频文件\n", newLines, videoCount)
		}

		// Save to SQLite
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

func countVideoLines(text string) int {
	count := 0
	extSet := map[string]bool{}
	for _, e := range videoExts {
		extSet[e] = true
	}
	for _, line := range splitLines(text) {
		lower := lowerStr(line)
		for ext := range extSet {
			if len(lower) > len(ext) && lower[len(lower)-len(ext):] == ext {
				count++
				break
			}
		}
	}
	return count
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, b := range s {
		if b == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func lowerStr(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

func init() {
	syncCmd.Flags().BoolVar(&syncDeep, "deep", false, "递归预热目录 listing 缓存")
	syncCmd.Flags().IntVar(&syncDepth, "depth", 3, "预热深度（配合 --deep，默认 3）")
	rootCmd.AddCommand(syncCmd)
}
