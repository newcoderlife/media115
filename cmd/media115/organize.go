package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/newcoderlife/media115/internal/cloud115"
	"github.com/newcoderlife/media115/internal/config"
	"github.com/newcoderlife/media115/internal/organizer"
)

type organizeOpts struct {
	Out            io.Writer
	Category       string
	Execute        bool
	GetClient      func() (*cloud115.Client, error)
	GetTreeEntries func(string) ([]cloud115.TreeEntry, error)
	CacheDir       func() string
	Now            func() time.Time
}

func newOrganizeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "organize <category>",
		Short: "Rename and reorganize files on 115 to Jellyfin standard",
		Long: `Rename and reorganize files on 115 to Jellyfin standard naming.

Dry-run by default (shows plan). Use --execute to apply.
Reads from tree cache + scrape cache — no extra API calls for planning.

CATEGORY: AV, 电影, 剧目, etc.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			execute, _ := cmd.Flags().GetBool("execute")
			return organizeRun(&organizeOpts{
				Out:            cmd.OutOrStdout(),
				Category:       args[0],
				Execute:        execute,
				GetClient:      getClient,
				GetTreeEntries: getTreeEntries,
				CacheDir:       mediaCacheDir,
				Now:            time.Now,
			})
		},
	}
	cmd.Flags().Bool("execute", false, "实际执行重命名/移动（默认为演练）")
	return cmd
}

func organizeRun(o *organizeOpts) error {
	w := o.Out

	allEntries, err := o.GetTreeEntries("")
	if err != nil {
		return fmt.Errorf("读取树缓存失败: %w", err)
	}
	if len(allEntries) == 0 {
		fmt.Fprintln(w, "无树缓存。请先运行: cloud115 sync /影音")
		return nil
	}
	// Filter by category
	var entries []cloud115.TreeEntry
	for _, e := range allEntries {
		if strings.Contains("/"+e.Path+"/", "/"+o.Category+"/") {
			entries = append(entries, e)
		}
	}

	plan := organizer.BuildPlan(o.Category, entries, nil)

	var renames, skips []organizer.Op
	for _, op := range plan {
		if op.Action == "rename" {
			renames = append(renames, op)
		} else {
			skips = append(skips, op)
		}
	}

	fmt.Fprintf(w, "'%s' 分类整理计划: %d 个重命名, %d 个不变\n\n", o.Category, len(renames), len(skips))

	if len(renames) == 0 {
		fmt.Fprintln(w, "没有需要重命名的文件。")
		if o.Execute {
			client, err := o.GetClient()
			if err == nil {
				defer client.Close()
				categoryPath := "影音/" + o.Category
				logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
				uploadMissingNFO(client, categoryPath, skips, entries, logger)
			}
		}
		return nil
	}

	// Print plan table.
	fmt.Fprintf(w, "| %3s | %-40s | %-30s | %10s |\n", "#", "Current", "→ New Name", "Source")
	fmt.Fprintf(w, "|%s|%s|%s|%s|\n", dashes(5), dashes(42), dashes(32), dashes(12))
	for i, op := range renames {
		cur := trunc(op.File, 40)
		nn := op.NewName
		if nn == "" {
			nn = op.NewFolder
		}
		if nn == "" {
			nn = "-"
		}
		nn = trunc(nn, 30)
		src := op.SourceID
		if op.Type == "movie" || op.Type == "tv" {
			src = "tmdb:" + op.SourceID
		}
		fmt.Fprintf(w, "| %3d | %-40s | %-30s | %10s |\n", i+1, cur, nn, src)
	}

	// Check for target conflicts.
	type targetKey struct{ folder, name string }
	targetCount := map[targetKey]int{}
	for _, op := range renames {
		k := targetKey{op.NewFolder, op.NewName}
		if k.folder != "" || k.name != "" {
			targetCount[k]++
		}
	}
	conflicts := map[targetKey]int{}
	for k, v := range targetCount {
		if v > 1 {
			conflicts[k] = v
		}
	}
	if len(conflicts) > 0 {
		fmt.Fprintf(w, "\n⚠ 发现 %d 个目标冲突:\n", len(conflicts))
		for k, count := range conflicts {
			fmt.Fprintf(w, "  %dx → %s/%s\n", count, k.folder, k.name)
		}
		fmt.Fprintln(w, "请在执行前解决冲突。")
		return nil
	}

	if !o.Execute {
		fmt.Fprintf(w, "\n演练完成。使用 --execute 应用 %d 个重命名。\n", len(renames))
		return nil
	}

	// Execute.
	client, err := o.GetClient()
	if err != nil {
		return fmt.Errorf("获取客户端失败: %w", err)
	}
	defer client.Close()

	categoryPath := "影音/" + o.Category
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	fmt.Fprintf(w, "\n执行 %d 个重命名...\n", len(renames))
	results, err := organizer.Execute(client, renames, categoryPath, logger)
	if err != nil {
		return fmt.Errorf("执行失败: %w", err)
	}

	okCount := 0
	failCount := 0
	for _, r := range results {
		switch r.Status {
		case "ok":
			okCount++
		case "error", "not_found":
			failCount++
			fmt.Fprintf(w, "  错误: %s — %s\n", r.File, r.Error)
		}
	}
	fmt.Fprintf(w, "\n完成: %d 个成功, %d 个失败\n", okCount, failCount)

	// Save log.
	logDir := filepath.Join(o.CacheDir(), "history")
	_ = os.MkdirAll(logDir, 0o755)
	logFile := filepath.Join(logDir, fmt.Sprintf("organize_%s_%d.json", o.Category, o.Now().Unix()))
	type logEntry struct {
		OriginalFile string `json:"original_file"`
		OriginalPath string `json:"original_path"`
		NewFolder    string `json:"new_folder"`
		NewName      string `json:"new_name"`
		Status       string `json:"status"`
		Error        string `json:"error,omitempty"`
	}
	var logEntries []logEntry
	for _, r := range results {
		logEntries = append(logEntries, logEntry{
			OriginalFile: r.File,
			OriginalPath: r.Parent,
			NewFolder:    r.NewFolder,
			NewName:      r.NewName,
			Status:       r.Status,
			Error:        r.Error,
		})
	}
	// Also include skip ops in the log.
	for _, op := range skips {
		logEntries = append(logEntries, logEntry{
			OriginalFile: op.File,
			OriginalPath: op.Parent,
			Status:       "skip",
			Error:        op.Reason,
		})
	}
	if data, err := json.MarshalIndent(logEntries, "", "  "); err == nil {
		_ = os.WriteFile(logFile, data, 0o644)
		fmt.Fprintf(w, "日志保存至: %s\n", logFile)
	}

	// Verify.
	if okCount > 0 {
		fmt.Fprintln(w, "\n验证中...")
		catPath := "/" + strings.TrimPrefix(categoryPath, "/")
		verified, mismatches := organizer.Verify(client, renames, catPath)
		if mismatches > 0 {
			fmt.Fprintf(w, "⚠ %d 个文件验证未通过\n", mismatches)
		} else {
			fmt.Fprintf(w, "✓ %d 个文件验证通过\n", verified)
		}
	}

	// Upload NFOs for files that are already correctly named (skips).
	uploadMissingNFO(client, categoryPath, skips, entries, logger)

	return nil
}

// uploadMissingNFO uploads NFO/poster sidecars for skip ops (already-organized
// files) that don't yet have a matching NFO in the tree cache (matched by stem).
func uploadMissingNFO(client *cloud115.Client, categoryPath string, skips []organizer.Op, treeEntries []cloud115.TreeEntry, logger *slog.Logger) {
	if len(skips) == 0 {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}
	cacheRoot := config.CacheDir()
	catPath := "/" + strings.TrimPrefix(categoryPath, "/")

	// Extract category name from catPath (last component).
	category := leafName(catPath)

	// Build set of "parent/stem" that already have a matching NFO in the tree.
	nfoStems := map[string]bool{}
	for _, e := range treeEntries {
		if e.IsNFO && strings.Contains("/"+e.Path+"/", "/"+category+"/") {
			stem := strings.TrimSuffix(e.Name, filepath.Ext(e.Name))
			nfoStems[e.Parent+"/"+stem] = true
		}
	}

	// Group videos without a matching NFO by directory.
	byDir := map[string][]organizer.VideoFile{}
	for _, op := range skips {
		if op.Reason != "already correct" && op.Reason != "already in standard format" {
			continue
		}
		videoStem := strings.TrimSuffix(op.File, filepath.Ext(op.File))
		if nfoStems[op.Parent+"/"+videoStem] {
			continue
		}
		parentDir := catPath + "/" + leafName(op.Parent)
		byDir[parentDir] = append(byDir[parentDir], organizer.VideoFile{
			File:   op.File,
			Parent: op.Parent,
		})
	}

	// Sync all videos per directory at once.
	total := 0
	for dir, vfs := range byDir {
		n := organizer.SyncSidecars(client, dir, vfs, false, logger, cacheRoot)
		if n > 0 {
			logger.Info("organizer: uploaded missing NFO", "dir", dir, "count", n)
		}
		total += n
	}
	if total > 0 {
		logger.Info("organizer: total NFO uploads", "count", total)
	}
}

// leafName returns the last path component.
func leafName(p string) string {
	if idx := strings.LastIndex(p, "/"); idx >= 0 {
		return p[idx+1:]
	}
	return p
}
