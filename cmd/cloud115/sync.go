package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/newcoderlife/media115/internal/cloud115"
)

type syncClient interface {
	ExportTree(path string) (string, error)
	SaveTree(text string, videoExts []string, rootPath string) (int, error)
	TreeStats() (*cloud115.TreeStats, error)
	Warm(path string, depth int, progress func(string, int)) error
	CacheStatus() (*cloud115.CacheStats, error)
	Close() error
}

type syncOpts struct {
	Out       io.Writer
	Path      string
	Deep      bool
	Depth     int
	VideoExts []string
	GetClient func() (syncClient, error)
}

var videoExts = []string{".mkv", ".mp4", ".avi", ".ts", ".rmvb", ".wmv", ".flv", ".mov", ".m4v", ".iso"}

func newSyncCmd() *cobra.Command {
	var (
		deep  bool
		depth int
	)
	cmd := &cobra.Command{
		Use:   "sync <path>",
		Short: "刷新路径缓存。默认导出目录树，--deep 额外预热 listing 缓存。",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return syncRun(&syncOpts{
				Out:       cmd.OutOrStdout(),
				Path:      args[0],
				Deep:      deep,
				Depth:     depth,
				VideoExts: videoExts,
				GetClient: func() (syncClient, error) {
					return getClient()
				},
			})
		},
	}
	cmd.Flags().BoolVar(&deep, "deep", false, "递归预热目录 listing 缓存")
	cmd.Flags().IntVar(&depth, "depth", 3, "预热深度（配合 --deep，默认 3）")
	return cmd
}

func syncRun(o *syncOpts) error {
	client, err := o.GetClient()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	fmt.Fprintf(o.Out, "正在导出目录树...\n")
	text, err := client.ExportTree(o.Path)
	if err != nil {
		return fmt.Errorf("导出失败: %w", err)
	}
	if text == "" {
		return fmt.Errorf("导出失败: %s", o.Path)
	}

	_, err = client.SaveTree(text, o.VideoExts, o.Path)
	if err != nil {
		return fmt.Errorf("保存到 SQLite 失败: %w", err)
	}
	stats, err := client.TreeStats()
	if err != nil {
		return fmt.Errorf("获取 tree stats 失败: %w", err)
	}
	fmt.Fprintf(o.Out, "已保存到缓存：%d 条目，%d 个视频，%d 个 NFO\n", stats.Total, stats.Videos, stats.NFOs)

	if o.Deep {
		warmCount := 0
		progressFn := func(p string, n int) {
			warmCount++
			fmt.Fprintf(o.Out, "\r  预热中: %d 个目录", warmCount)
		}
		fmt.Fprintf(o.Out, "正在预热缓存 (深度=%d)...\n", o.Depth)
		if err := client.Warm(o.Path, o.Depth, progressFn); err != nil {
			fmt.Fprintf(o.Out, "\n预热失败: %v\n", err)
		} else {
			fmt.Fprintf(o.Out, "\n")
			cacheStats, err := client.CacheStatus()
			if err == nil {
				fmt.Fprintf(o.Out, "预热完成：%d 路径，%d 目录，%d 条目\n",
					cacheStats.PathCount, cacheStats.DirCount, cacheStats.EntryCount)
			}
		}
	}
	return nil
}

func init() {
	rootCmd.AddCommand(newSyncCmd())
}
