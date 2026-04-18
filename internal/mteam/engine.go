package mteam

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/newcoderlife/media115/internal/logging"
)

// humanSize formats bytes into a human-readable string.
func humanSize(b int64) string {
	switch {
	case b >= 1<<40:
		return fmt.Sprintf("%.1fT", float64(b)/float64(1<<40))
	case b >= 1<<30:
		return fmt.Sprintf("%.1fG", float64(b)/float64(1<<30))
	case b >= 1<<20:
		return fmt.Sprintf("%.1fM", float64(b)/float64(1<<20))
	default:
		return fmt.Sprintf("%dB", b)
	}
}

// RunOptions controls engine behaviour.
type RunOptions struct {
	// Rule is the filter rule applied to search results.
	Rule Rule
	// WatchDir is the directory to write .torrent files into.
	WatchDir string
	// DryRun skips downloading; it still calls Search.
	DryRun bool
	// MaxPages caps pagination (default 3).
	MaxPages int
	// PageSize for Search (default 50).
	PageSize int
	// Now allows tests to pin the reference time.
	Now func() time.Time
}

// RunReport summarises what happened for a single run.
type RunReport struct {
	Scanned    int
	Matched    int
	Skipped    int
	Downloaded int
	Errors     int
	Rejections map[string]int // reason → count
}

// Run performs one search+filter+download pass.
func Run(ctx context.Context, client *Client, opts RunOptions) (*RunReport, error) {
	if opts.MaxPages <= 0 {
		opts.MaxPages = 3
	}
	if opts.PageSize <= 0 {
		opts.PageSize = 50
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	rr := &RunReport{Rejections: map[string]int{}}
	now := opts.Now()

	for page := 1; page <= opts.MaxPages; page++ {
		if ctx.Err() != nil {
			rr.Errors++
			return rr, nil
		}
		data, err := client.Search(ctx, opts.Rule.ToSearchRequest(page, opts.PageSize))
		if err != nil {
			logging.Log(slog.LevelError, fmt.Sprintf("search page=%d: %v", page, err), "mteam")
			rr.Errors++
			return rr, nil
		}
		logging.Log(slog.LevelInfo, fmt.Sprintf("page %d/%d: %d torrents", page, opts.MaxPages, len(data.List)), "mteam")
		if len(data.List) == 0 {
			break
		}
		freshExhausted := false
		for i := range data.List {
			t := &data.List[i]
			rr.Scanned++
			m := opts.Rule.Match(t, now)
			if !m.Match {
				rr.Skipped++
				rr.Rejections[m.Reason]++
				if opts.Rule.FreshHours > 0 && m.Reason == "older than fresh_hours window" {
					freshExhausted = true
				}
				continue
			}
			rr.Matched++
			// Skip if .torrent already exists in watch dir.
			filename := torrentFilename(t)
			finalPath := filepath.Join(opts.WatchDir, filename)
			if _, err := os.Stat(finalPath); err == nil {
				rr.Skipped++
				rr.Rejections["already downloaded"]++
				continue
			} else if !os.IsNotExist(err) {
				logging.Log(slog.LevelError, fmt.Sprintf("stat id=%s: %v", t.ID, err), "mteam")
				rr.Errors++
				continue
			}
			// NoJunk: check for 肉酱盘 (too many video files).
			if opts.Rule.NoJunk {
				threshold := JunkVideoThreshold(opts.Rule.Mode)
				if threshold > 0 {
					numFiles, _ := strconv.ParseInt(t.NumFiles, 10, 64)
					// Pre-filter: if total files ≤ threshold, video count can't exceed it.
					if numFiles <= 0 || numFiles > int64(threshold) {
						files, err := client.Files(ctx, t.ID)
						if err != nil {
							logging.Log(slog.LevelError, fmt.Sprintf("files id=%s: %v", t.ID, err), "mteam")
							rr.Errors++
							continue
						}
						fm := CheckFiles(files, threshold)
						if !fm.Match {
							rr.Skipped++
							rr.Rejections[fm.Reason]++
							continue
						}
					}
				}
			}
			if opts.DryRun {
				size, _ := strconv.ParseInt(t.Size, 10, 64)
				logging.Log(slog.LevelInfo, fmt.Sprintf("[dry-run] id=%s %s size=%s seeders=%s %s",
					t.ID, t.Name, humanSize(size), t.Status.Seeders, t.Status.Discount), "mteam")
				continue
			}
			if err := download(ctx, client, opts.WatchDir, t); err != nil {
				logging.Log(slog.LevelError, fmt.Sprintf("download id=%s: %v", t.ID, err), "mteam")
				rr.Errors++
				continue
			}
			rr.Downloaded++
			logging.Log(slog.LevelInfo, fmt.Sprintf("grabbed id=%s name=%s", t.ID, t.Name), "mteam")
		}
		if freshExhausted {
			break
		}
		if len(data.List) < opts.PageSize {
			break
		}
	}

	return rr, nil
}

func download(ctx context.Context, client *Client, watchDir string, t *Torrent) error {
	link, err := client.GenDlToken(ctx, t.ID)
	if err != nil {
		return fmt.Errorf("gen token: %w", err)
	}
	data, err := client.DownloadTorrent(ctx, link)
	if err != nil {
		return fmt.Errorf("download: %w", err)
	}
	if err := os.MkdirAll(watchDir, 0o755); err != nil {
		return fmt.Errorf("mkdir watch_dir: %w", err)
	}
	filename := torrentFilename(t)
	finalPath := filepath.Join(watchDir, filename)
	tmp, err := os.CreateTemp(watchDir, ".mteam-*.part")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename temp: %w", err)
	}
	return nil
}

// reUnsafe strips characters that would misbehave as filenames.
var reUnsafe = regexp.MustCompile(`[\\/:*?"<>|\x00-\x1f]+`)

// torrentFilename returns the atomic on-disk filename for a torrent.
// Format: [M-TEAM]<id>_<safe-name>.torrent
func torrentFilename(t *Torrent) string {
	name := strings.TrimSpace(t.Name)
	name = reUnsafe.ReplaceAllString(name, "_")
	if len([]rune(name)) > 80 {
		r := []rune(name)
		name = string(r[:80])
	}
	if name == "" {
		name = "torrent"
	}
	id := reUnsafe.ReplaceAllString(t.ID, "_")
	if id == "" {
		id = "unknown"
	}
	return fmt.Sprintf("[M-TEAM]%s_%s.torrent", id, name)
}
