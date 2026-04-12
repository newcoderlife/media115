package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	cloud115 "github.com/newcoderlife/media115/internal/cloud115"
	"github.com/newcoderlife/media115/internal/logging"
	"github.com/newcoderlife/media115/internal/scraper"
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

		allEntries, err := getTreeEntries("")
		if err != nil {
			return fmt.Errorf("读取树缓存失败: %w", err)
		}
		if len(allEntries) == 0 {
			fmt.Println("无树缓存。请先运行: cloud115 sync /影音")
			return nil
		}

		// Filter by category (path contains /category/)
		var entries []cloud115.TreeEntry
		for _, e := range allEntries {
			if strings.Contains("/"+e.Path+"/", "/"+category+"/") {
				entries = append(entries, e)
			}
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

		logging.Log(slog.LevelInfo, fmt.Sprintf("scrape start: %d files in %s → %s", len(videos), category, outDir), "scraper")

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

		cases := loadCases()

		for i, item := range videos {
			name := item.Name
			parent := item.Parent
			analysis := scraper.AnalyzeWithCases(name, cases)

			fileOutDir := filepath.Join(outDir, strings.ReplaceAll(parent, "/", "_"))
			if err := os.MkdirAll(fileOutDir, 0o755); err != nil {
				continue
			}

			logging.Log(slog.LevelDebug, fmt.Sprintf("[%d/%d] %s analyzing...", i+1, len(videos), trunc(name, 60)), "scraper")

			season := analysis.Season
			if (analysis.MediaType == "tv" || analysis.MediaType == "anime") && season == 0 {
				season = 1 // default to season 1
			}
			opts := scraper.ScrapeOpts{
				Year:    analysis.Year,
				Season:  season,
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
				logging.Log(slog.LevelDebug, fmt.Sprintf("[%d/%d] %s skipped (%s)", i+1, len(videos), trunc(name, 40), analysis.MediaType), "scraper")
				results = append(results, result{File: name, Status: "skip"})
				skipCount++
				continue
			}

			// Cache key for this scrape.
			stemName := strings.TrimSuffix(name, filepath.Ext(name))
			cacheSource := normalizeMediaType(mediaType)

			r := result{File: name, FileYear: analysis.Year}

			// Check not_found cache (skip re-scraping recently failed titles).
			if !force && scrapeIsNotFound(cacheSource, stemName) {
				r.Status = "not_found"
				logging.Log(slog.LevelDebug, fmt.Sprintf("[%d/%d] %s not_found (cached)", i+1, len(videos), trunc(name, 40)), "scraper")
				failCount++
				results = append(results, r)
				continue
			}

			// Check success cache.
			if !force {
				if cached := scrapeGet(cacheSource, stemName); cached != nil {
					if _, isNotFound := cached["_not_found"]; !isNotFound {
						// Reconstruct a fake ScrapeResult from cache to save file_map.
						fakeSR := &scraper.ScrapeResult{
							Status: "ok",
							IDs:    map[string]string{},
						}
						if title, ok := cached["title"].(string); ok {
							fakeSR.Match = title
						}
						if tmdbID, ok := cached["tmdb_id"].(string); ok {
							fakeSR.IDs["tmdb"] = tmdbID
						}
						if num, ok := cached["number"].(string); ok {
							fakeSR.IDs["number"] = num
						}
						r.Status = "ok"
						r.Match = fakeSR.Match
						r.SourceID = fakeSR.IDs["tmdb"]
						if r.SourceID == "" {
							r.SourceID = fakeSR.IDs["number"]
						}
						logging.Log(slog.LevelDebug, fmt.Sprintf("[%d/%d] %s → %s (%s) (cached)", i+1, len(videos), trunc(name, 40), r.Match, r.SourceID), "scraper")
						okCount++
						// Rebuild file_map from cached data.
						saveFileMap(stemName, mediaType, analysis.Title, fakeSR)
						results = append(results, r)
						continue
					}
				}
			}

			sr := scraper.Scrape(mediaType, analysis.Title, name, fileOutDir, opts, nil)

			if sr.Status == "ok" {
				r.Status = "ok"
				r.Match = sr.Match
				r.SourceID = sr.IDs["tmdb"]
				if r.SourceID == "" {
					r.SourceID = sr.IDs["number"]
				}
				logging.Log(slog.LevelInfo, fmt.Sprintf("[%d/%d] %s → %s (%s)", i+1, len(videos), trunc(name, 40), sr.Match, r.SourceID), "scraper")
				okCount++

				// Save file_map cache entry
				saveFileMap(name, mediaType, analysis.Title, sr)

				// Cache the successful scrape result.
				cacheEntry := map[string]any{
					"_cached_at": float64(time.Now().Unix()),
					"title":      sr.Match,
				}
				if tmdbID := sr.IDs["tmdb"]; tmdbID != "" {
					cacheEntry["tmdb_id"] = tmdbID
				}
				if num := sr.IDs["number"]; num != "" {
					cacheEntry["number"] = num
				}
				scrapePut(cacheSource, stemName, cacheEntry)
			} else if sr.Status == "not_found" {
				r.Status = "not_found"
				logging.Log(slog.LevelWarn, fmt.Sprintf("[%d/%d] %s not_found", i+1, len(videos), trunc(name, 40)), "scraper")
				failCount++
				// Cache the not_found result.
				scrapePut(cacheSource, stemName, map[string]any{
					"_not_found": true,
					"_cached_at": float64(time.Now().Unix()),
				})
			} else {
				r.Status = "error"
				r.Error = sr.Error
				logging.Log(slog.LevelError, fmt.Sprintf("[%d/%d] %s error: %s", i+1, len(videos), trunc(name, 40), sr.Error), "scraper")
				failCount++
			}
			results = append(results, r)
		}

		logging.Log(slog.LevelInfo, fmt.Sprintf("scrape done: %d ok, %d fail, %d skip", okCount, failCount, skipCount), "scraper")

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

// normalizeMediaType maps internal type aliases to canonical Python types.
func normalizeMediaType(mediaType string) string {
	switch mediaType {
	case "anime":
		return "tv"
	case "gravure":
		return "av"
	case "av_west":
		return "av"
	default:
		return mediaType
	}
}

// saveFileMap writes a file_map cache entry matching the Python format.
// It persists the full metadata needed by the organizer.
// query is the original search query (analysis.Title), used as the number for av_west.
func saveFileMap(filename, mediaType, query string, sr *scraper.ScrapeResult) {
	stemName := strings.TrimSuffix(filename, filepath.Ext(filename))
	cacheDir := mediaCacheDir()
	dir := filepath.Join(cacheDir, "scrape", "file_map")
	_ = os.MkdirAll(dir, 0o755)

	canonicalType := normalizeMediaType(mediaType)

	entry := map[string]any{
		"type":       canonicalType,
		"_cached_at": float64(time.Now().Unix()),
	}

	m := sr.Meta

	switch canonicalType {
	case "movie":
		if m != nil {
			entry["title"] = m.Title
			entry["originaltitle"] = m.OriginalTitle
			if m.Year != 0 {
				entry["year"] = m.Year
			}
		} else {
			entry["title"] = sr.Match
		}
		if tmdbID, ok := sr.IDs["tmdb"]; ok && tmdbID != "" {
			entry["tmdb_id"] = tmdbID
		}

	case "tv":
		if m != nil {
			showTitle := m.ShowTitle
			if showTitle == "" {
				showTitle = m.Title
			}
			entry["title"] = m.Title
			entry["showtitle"] = showTitle
			if m.Year != 0 {
				entry["year"] = m.Year
			}
			if m.Season != 0 {
				entry["season"] = m.Season
			}
			if m.Episode != 0 {
				entry["episode"] = m.Episode
			}
		} else {
			entry["title"] = sr.Match
			entry["showtitle"] = sr.Match
		}
		if tmdbID, ok := sr.IDs["tmdb"]; ok && tmdbID != "" {
			entry["tmdb_id"] = tmdbID
		}

	case "av":
		// For av_west, the original query (analysis.Title) is the number/identifier.
		// For regular av, number comes from provider IDs or the match.
		number := ""
		if mediaType == "av_west" {
			number = query
		}
		if number == "" {
			for _, key := range []string{"jav321", "javfree", "number"} {
				if v, ok := sr.IDs[key]; ok && v != "" {
					number = v
					break
				}
			}
		}
		if number == "" {
			number = sr.Match
		}
		entry["number"] = number
		if m != nil {
			entry["title"] = m.Title
		} else {
			entry["title"] = sr.Match
		}
	}

	path := filepath.Join(dir, stemName+".json")
	if data, err := json.MarshalIndent(entry, "", "  "); err == nil {
		_ = os.WriteFile(path, data, 0o644)
	}
}

const scrapeCacheTTLDays = 7

func mediaCacheDir() string                             { return scraper.CacheDir() }
func scrapeCachePath(source, key string) string         { return scraper.CachePath(source, key) }
func scrapeGet(source, key string) map[string]any       { return scraper.CacheGet(source, key) }
func scrapePut(source, key string, data map[string]any) { scraper.CachePut(source, key, data) }
func scrapeIsNotFound(source, key string) bool {
	return scraper.CacheIsNotFound(source, key, scrapeCacheTTLDays)
}

func init() {
	scrapeCmd.Flags().Bool("force", false, "重新刮削已有NFO的文件")
	scrapeCmd.Flags().Int("limit", 0, "最多刮削N个文件（0表示全部）")
}
