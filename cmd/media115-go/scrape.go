package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/newcoderlife/media115/internal/config"
	"github.com/newcoderlife/media115/internal/scraper"
	"github.com/spf13/cobra"
)

var scrapeCmd = &cobra.Command{
	Use:   "scrape <category>",
	Short: "Scrape metadata for a category from tree cache",
	Long: `Scrape metadata for a media category and write NFO + poster locally.

CATEGORY: AV, 电影, 剧目, etc.

Reads video list from local SQLite cache (no API calls for planning).
For each file: analyzes filename → calls scraper → writes NFO/poster to
~/.cache/media115/scrape_output/<category>/.

Use --force to re-scrape files that already have NFO on 115.
Use --limit N to scrape only the first N files.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		category := args[0]
		force, _ := cmd.Flags().GetBool("force")
		limit, _ := cmd.Flags().GetInt("limit")

		cfg, err := getConfig()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		registerProviders(cfg)

		entries, err := getTreeEntries(category)
		if err != nil {
			return fmt.Errorf("读取树缓存失败: %w", err)
		}
		if len(entries) == 0 {
			fmt.Println("无树缓存。请先运行: cloud115 sync /影音")
			return nil
		}

		// Filter to videos in the requested category.
		var videos []struct {
			Name   string
			Parent string
		}
		nfoStems := map[string]bool{} // "parent/stem" for existing NFOs
		for _, e := range entries {
			if e.IsNFO {
				stemName := strings.TrimSuffix(e.Name, filepath.Ext(e.Name))
				nfoStems[e.Parent+"/"+stemName] = true
			}
		}
		for _, e := range entries {
			if !e.IsVideo {
				continue
			}
			pathSlash := "/" + e.Path + "/"
			if !strings.Contains(pathSlash, "/"+category+"/") {
				continue
			}
			if !force {
				stemName := strings.TrimSuffix(e.Name, filepath.Ext(e.Name))
				if nfoStems[e.Parent+"/"+stemName] {
					continue // already has NFO
				}
			}
			videos = append(videos, struct{ Name, Parent string }{e.Name, e.Parent})
		}

		if limit > 0 && len(videos) > limit {
			videos = videos[:limit]
		}

		if len(videos) == 0 {
			fmt.Printf("'%s' 分类中没有需要刮削的文件。\n", category)
			return nil
		}

		// Determine output dir.
		outDir := filepath.Join(mediaCacheDir(), "scrape_output", category)
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return fmt.Errorf("创建输出目录失败: %w", err)
		}

		fmt.Printf("刮削 %d 个文件 '%s' → %s\n", len(videos), category, outDir)

		type result struct {
			File      string `json:"file"`
			Status    string `json:"status"`
			Match     string `json:"match,omitempty"`
			SourceID  string `json:"source_id,omitempty"`
			Error     string `json:"error,omitempty"`
			FileYear  int    `json:"_file_year,omitempty"`
			MatchYear int    `json:"_match_year,omitempty"`
		}

		var results []result
		okCount, failCount, skipCount := 0, 0, 0

		for i, item := range videos {
			name := item.Name
			parent := item.Parent
			analysis := scraper.AnalyzeFilename(name)

			fileOutDir := filepath.Join(outDir, strings.ReplaceAll(parent, "/", "_"))
			if err := os.MkdirAll(fileOutDir, 0o755); err != nil {
				continue
			}

			pct := 100 * (i + 1) / len(videos)
			fmt.Printf("  [%d/%d %d%%] %s ...", i+1, len(videos), pct, trunc(name, 60))

			opts := scraper.ScrapeOpts{
				Year:    analysis.Year,
				Season:  analysis.Season,
				Episode: analysis.Episode,
			}

			var mediaType string
			switch analysis.MediaType {
			case "av", "gravure", "av_west":
				mediaType = analysis.MediaType
			case "tv", "anime":
				mediaType = analysis.MediaType
			case "movie", "unknown":
				mediaType = "movie"
			default:
				fmt.Printf(" skipped (%s)\n", analysis.MediaType)
				results = append(results, result{File: name, Status: "skip"})
				skipCount++
				continue
			}

			sr := scraper.Scrape(mediaType, analysis.Title, name, fileOutDir, opts, nil)

			r := result{File: name, FileYear: analysis.Year}
			if sr.Status == "ok" {
				r.Status = "ok"
				r.Match = sr.Match
				r.SourceID = sr.IDs["tmdb"]
				if r.SourceID == "" {
					r.SourceID = sr.IDs["number"]
				}
				fmt.Printf(" → %s (%s)\n", sr.Match, r.SourceID)
				okCount++

				// Save file_map cache entry
				saveFileMap(name, mediaType, sr)
			} else if sr.Status == "not_found" {
				r.Status = "not_found"
				fmt.Printf(" not_found\n")
				failCount++
			} else {
				r.Status = "error"
				r.Error = sr.Error
				fmt.Printf(" error: %s\n", sr.Error)
				failCount++
			}
			results = append(results, r)
		}

		fmt.Printf("\n完成: %d 个刮削, %d 个失败, %d 个跳过\n", okCount, failCount, skipCount)

		// Print review table
		if len(results) > 0 {
			fmt.Printf("\n%4s | %-6s | %-45s | %-30s | Note\n", "#", "Status", "File", "Match")
			fmt.Printf("%s-+-%-s-+-%-s-+-%-s-+------\n", dashes(4), dashes(6), dashes(45), dashes(30))
			for i, r := range results {
				flag, matchStr, note := "—", "—", ""
				switch r.Status {
				case "ok":
					flag = "✓"
					matchStr = trunc(r.Match+" ("+r.SourceID+")", 30)
				case "not_found":
					flag = "✗"
				case "error":
					flag = "✗"
					note = trunc(r.Error, 40)
				}
				fmt.Printf("%4d | %-6s | %-45s | %-30s | %s\n",
					i+1, flag, trunc(r.File, 45), matchStr, note)
			}
		}

		// Save log
		logDir := filepath.Join(mediaCacheDir(), "logs")
		_ = os.MkdirAll(logDir, 0o755)
		logFile := filepath.Join(logDir, fmt.Sprintf("scrape_%s_%d.json", category, time.Now().Unix()))
		if data, err := json.MarshalIndent(results, "", "  "); err == nil {
			_ = os.WriteFile(logFile, data, 0o644)
			fmt.Printf("\n日志保存至: %s\n", logFile)
		}

		if okCount > 0 {
			fmt.Printf("下一步: 运行 'media115 organize %s' 预览重命名计划。\n", category)
		}
		return nil
	},
}

// saveFileMap writes a file_map cache entry matching the Python format.
func saveFileMap(filename, mediaType string, sr *scraper.ScrapeResult) {
	stemName := strings.TrimSuffix(filename, filepath.Ext(filename))
	cacheDir := mediaCacheDir()
	dir := filepath.Join(cacheDir, "scrape", "file_map")
	_ = os.MkdirAll(dir, 0o755)

	entry := map[string]any{
		"type":       mediaType,
		"_cached_at": float64(time.Now().Unix()),
	}
	if tmdbID, ok := sr.IDs["tmdb"]; ok && tmdbID != "" {
		entry["tmdb_id"] = tmdbID
	}
	if num, ok := sr.IDs["number"]; ok && num != "" {
		entry["number"] = num
	}

	path := filepath.Join(dir, stemName+".json")
	if data, err := json.MarshalIndent(entry, "", "  "); err == nil {
		_ = os.WriteFile(path, data, 0o644)
	}
}

// mediaCacheDir returns the media115 cache directory path.
func mediaCacheDir() string {
	return strings.Replace(config.CacheDir(), "cloud115", "media115", 1)
}

func init() {
	scrapeCmd.Flags().Bool("force", false, "重新刮削已有NFO的文件")
	scrapeCmd.Flags().Int("limit", 0, "最多刮削N个文件（0表示全部）")
}
