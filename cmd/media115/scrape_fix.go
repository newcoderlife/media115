package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/newcoderlife/media115/internal/provider/tmdb"
	"github.com/newcoderlife/media115/internal/scraper"
	"github.com/spf13/cobra"
)

var scrapeFixCmd = &cobra.Command{
	Use:   "scrape-fix <filename>",
	Short: "手动修正刮削结果",
	Long: `手动修正 batch-scrape 的匹配错误。

FILENAME: 视频文件名（例如 "Restart.2026.mkv"）

Examples:
  media115 scrape-fix "Restart.2026.mkv" --tmdb-id 1664596
  media115 scrape-fix "Restart.2026.mkv" --search "守护游戏"
  media115 scrape-fix "T-3800040.mkv" --number "T28-003"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		filename := args[0]
		tmdbID, _ := cmd.Flags().GetInt("tmdb-id")
		searchQuery, _ := cmd.Flags().GetString("search")
		number, _ := cmd.Flags().GetString("number")
		category, _ := cmd.Flags().GetString("category")
		season, _ := cmd.Flags().GetInt("season")
		episode, _ := cmd.Flags().GetInt("episode")

		cfg, err := getConfig()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		registerProviders(cfg)

		analysis := scraper.AnalyzeFilename(filename)

		// Auto-detect category.
		if category == "" {
			if number != "" {
				category = "AV"
			} else if analysis.MediaType == "av" || analysis.MediaType == "gravure" {
				category = "AV"
			} else if analysis.MediaType == "tv" || analysis.MediaType == "anime" {
				category = "剧目"
			} else {
				category = "电影"
			}
		}

		// Find or create the output directory.
		entries, _ := getTreeEntries("")
		searchName := ""
		for _, e := range entries {
			if e.Name == filename {
				searchName = strings.ReplaceAll(e.Parent, "/", "_")
				break
			}
		}
		if searchName == "" {
			stemName := strings.TrimSuffix(filename, filepath.Ext(filename))
			searchName = stemName
		}

		outDir := filepath.Join(mediaCacheDir(), "scrape_output", category, searchName)
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return fmt.Errorf("创建输出目录失败: %w", err)
		}

		fmt.Printf("分类: %s, 输出: %s\n", category, outDir)

		var sr *scraper.ScrapeResult
		scrapeQuery := analysis.Title // original query, passed to saveFileMap

		switch category {
		case "AV":
			avNumber := number
			if avNumber == "" {
				avNumber = analysis.Title
			}
			if avNumber == "" {
				return fmt.Errorf("无法确定AV番号。请使用 --number 指定")
			}
			scrapeQuery = avNumber
			fmt.Printf("刮削 AV: %s\n", avNumber)
			sr = scraper.Scrape("av", avNumber, filename, outDir, scraper.ScrapeOpts{}, nil)

		case "剧目":
			if tmdbID != 0 {
				// Direct fetch by TMDB ID, bypass search.
				p := tmdb.New(cfg.Auth.TMDB.Token)
				s := season
				if s == 0 {
					s = analysis.Season
					if s == 0 {
						s = 1
					}
				}
				ep := episode
				if ep == 0 {
					ep = analysis.Episode
				}
				fmt.Printf("刮削 TV (TMDB ID=%d): S%02dE%02d\n", tmdbID, s, ep)
				meta, detailErr := p.Detail(fmt.Sprintf("tv:%d", tmdbID))
				if detailErr != nil {
					return detailErr
				}
				meta.Season = s
				meta.Episode = ep
				stem := scraper.StemFilename(filename)
				nfoPath := filepath.Join(outDir, stem+".nfo")
				if err := scraper.GenerateEpisodeNFO(meta, nfoPath); err != nil {
					return err
				}
				tvshowPath := filepath.Join(outDir, "tvshow.nfo")
				if _, statErr := os.Stat(tvshowPath); os.IsNotExist(statErr) {
					tvMeta := *meta
					tvMeta.Season = 0
					tvMeta.Episode = 0
					_ = scraper.GenerateTVShowNFO(&tvMeta, tvshowPath)
				}
				if meta.PosterURL != "" {
					_ = scraper.DownloadImage(meta.PosterURL, filepath.Join(outDir, "poster.jpg"), 30*time.Second, false)
				}
				sr = &scraper.ScrapeResult{
					Status: "ok",
					Match:  meta.Title,
					IDs:    meta.UniqueIDs,
					Meta:   meta,
				}
				break
			}
			query := searchQuery
			if query == "" {
				return fmt.Errorf("TV 需要 --tmdb-id 或 --search")
			}
			s := season
			if s == 0 {
				s = analysis.Season
				if s == 0 {
					s = 1
				}
			}
			ep := episode
			if ep == 0 {
				ep = analysis.Episode
			}
			fmt.Printf("刮削 TV: %s S%02dE%02d\n", query, s, ep)
			sr = scraper.Scrape("tv", query, filename, outDir, scraper.ScrapeOpts{
				Season:  s,
				Episode: ep,
			}, nil)

		default: // 电影
			if tmdbID != 0 {
				// Direct fetch by TMDB ID, bypass search.
				p := tmdb.New(cfg.Auth.TMDB.Token)
				fmt.Printf("刮削 Movie (TMDB ID=%d)\n", tmdbID)
				meta, detailErr := p.Detail(fmt.Sprintf("movie:%d", tmdbID))
				if detailErr != nil {
					return detailErr
				}
				stem := scraper.StemFilename(filename)
				nfoPath := filepath.Join(outDir, stem+".nfo")
				if err := scraper.GenerateMovieNFO(meta, nfoPath); err != nil {
					return err
				}
				if meta.PosterURL != "" {
					_ = scraper.DownloadImage(meta.PosterURL, filepath.Join(outDir, "poster.jpg"), 30*time.Second, false)
				}
				sr = &scraper.ScrapeResult{
					Status: "ok",
					Match:  meta.Title,
					IDs:    meta.UniqueIDs,
					Meta:   meta,
				}
				break
			}
			query := searchQuery
			if query == "" {
				query = analysis.Title
			}
			if query == "" {
				return fmt.Errorf("无法确定电影标题。请使用 --search 指定")
			}
			fmt.Printf("刮削 Movie: %s\n", query)
			sr = scraper.Scrape("movie", query, filename, outDir, scraper.ScrapeOpts{
				Year: analysis.Year,
			}, nil)
		}

		if sr == nil {
			return fmt.Errorf("刮削返回 nil")
		}

		if sr.Status == "ok" {
			sid := sr.IDs["tmdb"]
			if sid == "" {
				sid = sr.IDs["number"]
			}
			fmt.Printf("✓ %s (%s)\n", sr.Match, sid)
			fmt.Printf("输出: %s\n", outDir)
			fmt.Printf("\n下一步: 运行 'media115 organize %s' 应用更改。\n", category)

			// Save file_map with normalized type.
			typeMap := map[string]string{"电影": "movie", "剧目": "tv", "AV": "av", "写真": "av"}
			fmType := typeMap[category]
			if fmType == "" {
				fmType = "av"
			}
			saveFileMap(filename, fmType, scrapeQuery, sr)
		} else {
			fmt.Printf("✗ 状态: %s", sr.Status)
			if sr.Error != "" {
				fmt.Printf(" — %s", sr.Error)
			}
			fmt.Println()
		}

		return nil
	},
}


func init() {
	scrapeFixCmd.Flags().Int("tmdb-id", 0, "指定 TMDB ID")
	scrapeFixCmd.Flags().String("search", "", "用不同的查询词搜索")
	scrapeFixCmd.Flags().String("number", "", "指定 AV 番号")
	scrapeFixCmd.Flags().String("category", "", "分类（电影/剧目/AV），不指定则自动检测")
	scrapeFixCmd.Flags().Int("season", 0, "季数（TV用）")
	scrapeFixCmd.Flags().Int("episode", 0, "集数（TV用）")
}
