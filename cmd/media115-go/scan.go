package main

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/newcoderlife/media115/internal/cloud115"
	"github.com/newcoderlife/media115/internal/scraper"
	"github.com/spf13/cobra"
)

var (
	avFolderPattern    = regexp.MustCompile(`^[A-Za-z]+-\d+$`)
	movieFolderPattern = regexp.MustCompile(`^.+ \(\d{4}\)$`)
)

var scanCmd = &cobra.Command{
	Use:   "scan [category]",
	Short: "Scan media files from cached directory tree (no API calls)",
	Long: `Scan media files from cached directory tree.
No API calls are made — reads from the local SQLite cache.
Run 'cloud115 sync /影音' first to populate the cache.

CATEGORY: AV, 电影, 剧目, etc. (optional; default shows all)`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		showAll, _ := cmd.Flags().GetBool("all")
		category := ""
		if len(args) > 0 {
			category = args[0]
		}

		entries, err := getTreeEntries(category)
		if err != nil {
			return fmt.Errorf("读取树缓存失败: %w", err)
		}
		if len(entries) == 0 {
			fmt.Println("无树缓存。请先运行: cloud115 sync /影音")
			return nil
		}

		// Separate videos and build NFO stem set.
		var videos []cloud115.TreeEntry
		nfoSet := map[string]bool{} // "parent/stem" → true
		for _, e := range entries {
			if e.IsVideo {
				videos = append(videos, e)
			} else if e.IsNFO {
				stemName := strings.TrimSuffix(e.Name, filepath.Ext(e.Name))
				nfoSet[e.Parent+"/"+stemName] = true
			}
		}

		fmt.Printf("找到 %d 个视频文件（来自缓存树）。\n\n", len(videos))
		fmt.Printf("| %3s | %-55s | %-5s | %-8s | %-30s | %-8s | %-12s |\n",
			"#", "File", "NFO", "Type", "Title", "Source", "Action")
		fmt.Printf("|%s|%s|%s|%s|%s|%s|%s|\n",
			dashes(5), dashes(57), dashes(7), dashes(10), dashes(32), dashes(10), dashes(14))

		skipCount, scrapeCount, unrecognizedCount := 0, 0, 0

		for i, item := range videos {
			name := item.Name
			stemName := strings.TrimSuffix(name, filepath.Ext(name))
			hasNFO := nfoSet[item.Parent+"/"+stemName]
			nfoMark := "✗"
			if hasNFO {
				nfoMark = "✓"
			}

			result := scraper.AnalyzeFilename(name)

			var action string
			if hasNFO {
				action = "skip"
				skipCount++
			} else if result.MediaType == "unknown" {
				action = "unrecognized"
				unrecognizedCount++
			} else {
				action = "scrape"
				scrapeCount++
			}

			if showAll || action != "skip" {
				fmt.Printf("| %3d | %-55s | %-5s | %-8s | %-30s | %-8s | %-12s |\n",
					i+1,
					trunc(name, 55),
					nfoMark,
					result.MediaType,
					trunc(result.Title, 30),
					result.Source,
					action,
				)
			}
		}

		fmt.Printf("\n汇总: %d 个文件 — %d 个待刮削, %d 个跳过（有NFO）, %d 个未识别\n",
			len(videos), scrapeCount, skipCount, unrecognizedCount)

		// Anomaly detection
		type anomaly struct {
			kind, parent, detail string
		}
		var anomalies []anomaly

		// 1. Non-standard naming: has NFO but parent dir doesn't match convention
		for _, v := range videos {
			parentLeaf := v.Parent
			if idx := strings.LastIndex(v.Parent, "/"); idx >= 0 {
				parentLeaf = v.Parent[idx+1:]
			}
			stemName := strings.TrimSuffix(v.Name, filepath.Ext(v.Name))
			if !nfoSet[v.Parent+"/"+stemName] {
				continue // no NFO, skip naming check
			}
			inAV := strings.Contains("/"+v.Path+"/", "/AV/")
			if inAV {
				if !avFolderPattern.MatchString(parentLeaf) {
					anomalies = append(anomalies, anomaly{"naming", v.Parent, v.Name})
				}
			} else {
				if !movieFolderPattern.MatchString(parentLeaf) {
					anomalies = append(anomalies, anomaly{"naming", v.Parent, v.Name})
				}
			}
		}

		// 2. Duplicate NFOs
		nfoCount := map[string]int{}
		for _, e := range entries {
			if e.IsNFO {
				nfoCount[e.Parent+"/"+e.Name]++
			}
		}
		for key, count := range nfoCount {
			if count > 1 {
				parent := key[:strings.LastIndex(key, "/")]
				nfoName := key[strings.LastIndex(key, "/")+1:]
				anomalies = append(anomalies, anomaly{"dup_nfo", parent, fmt.Sprintf("%s x%d", nfoName, count)})
			}
		}

		// 3. Residual directories: NFO present but no video
		videoParents := map[string]bool{}
		for _, v := range videos {
			videoParents[v.Parent] = true
		}
		nfoParents := map[string]bool{}
		for _, e := range entries {
			if e.IsNFO {
				nfoParents[e.Parent] = true
			}
		}
		for parent := range nfoParents {
			if !videoParents[parent] {
				anomalies = append(anomalies, anomaly{"residual", parent, "NFO only, no video"})
			}
		}

		if len(anomalies) > 0 {
			fmt.Printf("\n发现 %d 个异常:\n", len(anomalies))
			labels := map[string]string{
				"naming":   "Non-standard name",
				"dup_nfo":  "Duplicate NFO",
				"residual": "Residual dir",
			}
			for _, a := range anomalies {
				fmt.Printf("  [%s] %s\n    %s\n", labels[a.kind], a.parent, a.detail)
			}
		}

		fmt.Println("\n未进行 API 调用（仅树缓存）。")
		return nil
	},
}

func dashes(n int) string { return strings.Repeat("-", n) }

func init() {
	scanCmd.Flags().Bool("all", false, "显示所有文件，不只是需要操作的")
}
