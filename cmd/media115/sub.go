package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/newcoderlife/media115/internal/config"
	"github.com/newcoderlife/media115/internal/mteam"
)

var (
	tickKeyword         string
	tickMode            string
	tickWatchDir        string
	tickRequireFree     bool
	tickRequire4K       bool
	tickRequireHQ       bool
	tickNoJunk          bool
	tickMaxFiles        int
	tickMinSize         string
	tickMaxSize         string
	tickMinSeeders      int
	tickExcludeKeywords []string
	tickLabelsAllow     []string
	tickLabelsDeny      []string
	tickFreshHours      int
	tickMinRating       float64
	tickTVComplete      bool
	tickDryRun          bool
	tickPages           int
)

var subCmd = &cobra.Command{
	Use:   "grab",
	Short: "M-Team 搜索 + 过滤 + 下载种子到 watch 文件夹",
	Long: `M-Team 种子抓取器：搜索站内种子，按规则过滤，把 .torrent 原子写入
本地 watch 文件夹，交给外部 BT 客户端（qBittorrent / Transmission）自动加载。

推荐用法：
  media115 grab --keyword "灵武大陆" --require-hq --no-junk --watch-dir /tmp/watch
`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if cfg.MTeam.APIKey == "" {
			return fmt.Errorf("mteam: api_key 未配置（编辑 %s）", config.ConfigPath())
		}
		watchDir := strings.TrimSpace(tickWatchDir)
		if watchDir == "" {
			watchDir = cfg.MTeam.DefaultWatchDir
		}
		if watchDir == "" {
			return fmt.Errorf("必须指定 --watch-dir 或在配置中设置 [mteam].default_watch_dir")
		}
		minSize, err := parseSize(tickMinSize)
		if err != nil {
			return fmt.Errorf("--min-size: %w", err)
		}
		maxSize, err := parseSize(tickMaxSize)
		if err != nil {
			return fmt.Errorf("--max-size: %w", err)
		}
		mode := tickMode
		if mode == "" {
			mode = mteam.ModeNormal
		}
		rule := mteam.Rule{
			Keyword:            tickKeyword,
			Mode:               mode,
			RequireFree:        tickRequireFree,
			Require4K:          tickRequire4K,
			RequireHQ:          tickRequireHQ,
			NoJunk:             tickNoJunk,
			MaxFiles:           tickMaxFiles,
			MinSize:            minSize,
			MaxSize:            maxSize,
			MinSeeders:         tickMinSeeders,
			ExcludeKeywords:    tickExcludeKeywords,
			LabelsAllow:        tickLabelsAllow,
			LabelsDeny:         tickLabelsDeny,
			FreshHours:         tickFreshHours,
			MinRating:          tickMinRating,
			TVCompleteEpisodes: tickTVComplete,
		}

		var minInterval time.Duration
		if cfg.MTeam.MinIntervalSeconds != nil {
			minInterval = time.Duration(*cfg.MTeam.MinIntervalSeconds) * time.Second
		}
		client := mteam.NewClient(
			cfg.MTeam.APIKey,
			mteam.WithBaseURL(cfg.MTeam.BaseURL),
			mteam.WithMinInterval(minInterval),
		)
		parent := cmd.Context()
		if parent == nil {
			parent = context.Background()
		}
		ctx, cancel := context.WithTimeout(parent, 10*time.Minute)
		defer cancel()
		report, err := mteam.Run(ctx, client, mteam.RunOptions{
			Rule:     rule,
			WatchDir: watchDir,
			DryRun:   tickDryRun,
			MaxPages: tickPages,
		})
		if err != nil {
			return err
		}
		printReport(report)
		return nil
	},
}

func printReport(r *mteam.RunReport) {
	fmt.Printf("扫描=%d 匹配=%d 下载=%d 跳过=%d 错误=%d\n",
		r.Scanned, r.Matched, r.Downloaded, r.Skipped, r.Errors)
	if len(r.Rejections) > 0 {
		fmt.Println("跳过原因:")
		for reason, n := range r.Rejections {
			fmt.Printf("  %-40s %d\n", trunc(reason, 40), n)
		}
	}
}

// parseSize parses "500M", "4G", "1.5T", or bare bytes. Empty string = 0.
func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	mult := int64(1)
	switch s[len(s)-1] {
	case 'K', 'k':
		mult = 1 << 10
		s = s[:len(s)-1]
	case 'M', 'm':
		mult = 1 << 20
		s = s[:len(s)-1]
	case 'G', 'g':
		mult = 1 << 30
		s = s[:len(s)-1]
	case 'T', 't':
		mult = 1 << 40
		s = s[:len(s)-1]
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size: %q", s)
	}
	if v < 0 {
		return 0, fmt.Errorf("size must not be negative: %q", s)
	}
	return int64(v * float64(mult)), nil
}

func init() {
	subCmd.Flags().StringVar(&tickKeyword, "keyword", "", "搜索关键字")
	subCmd.Flags().StringVar(&tickMode, "mode", mteam.ModeNormal, "搜索模式: normal/movie/tvshow/music/adult")
	subCmd.Flags().StringVar(&tickWatchDir, "watch-dir", "", "watch 文件夹（默认读 [mteam].default_watch_dir）")
	subCmd.Flags().BoolVar(&tickRequireFree, "require-free", false, "只要免费资源")
	subCmd.Flags().BoolVar(&tickRequire4K, "require-4k", false, "只要 4K/2160p")
	subCmd.Flags().BoolVar(&tickRequireHQ, "require-hq", false, "只要高质量 (4K + HDR/DOVI)")
	subCmd.Flags().BoolVar(&tickNoJunk, "no-junk", false, "排除原盘和肉酱盘")
	subCmd.Flags().IntVar(&tickMaxFiles, "max-files", 0, "torrent 内文件数上限")
	subCmd.Flags().StringVar(&tickMinSize, "min-size", "", "最小体积，例: 500M / 4G")
	subCmd.Flags().StringVar(&tickMaxSize, "max-size", "", "最大体积，例: 80G")
	subCmd.Flags().IntVar(&tickMinSeeders, "min-seeders", 0, "最少做种数")
	subCmd.Flags().StringSliceVar(&tickExcludeKeywords, "exclude", nil, "排除关键字")
	subCmd.Flags().StringSliceVar(&tickLabelsAllow, "labels-allow", nil, "labelsNew 白名单")
	subCmd.Flags().StringSliceVar(&tickLabelsDeny, "labels-deny", nil, "labelsNew 黑名单")
	subCmd.Flags().IntVar(&tickFreshHours, "fresh-hours", 0, "只保留 N 小时内发布的种子")
	subCmd.Flags().Float64Var(&tickMinRating, "min-rating", 0, "评分下限 (IMDB/豆瓣取高者)")
	subCmd.Flags().BoolVar(&tickTVComplete, "tv-complete", false, "仅要剧集合集")
	subCmd.Flags().BoolVar(&tickDryRun, "dry-run", false, "只搜索不下载")
	subCmd.Flags().IntVar(&tickPages, "max-pages", 3, "最多翻 N 页")
}
