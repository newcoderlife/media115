package organizer

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/newcoderlife/media115/internal/cloud115"
)

// FilePair pairs a local file path with its desired remote filename.
type FilePair struct {
	LocalPath  string
	RemoteName string
}

// SyncSidecars uploads matching local scrape_output (NFO/poster) files to a
// 115 directory.
//
// For each VideoFile, finds matching sidecar files and uploads them.
// Deduplicates shared files (tvshow.nfo, poster.jpg) across multiple videos.
// If skipListing is true the initial remote listing is skipped (useful for
// freshly created empty directories where no stale sidecars exist).
//
// Returns the number of files uploaded.
func SyncSidecars(client *cloud115.Client, targetDir string, videoFiles []VideoFile, skipListing bool, logger *slog.Logger, cacheRoot string) int {
	if logger == nil {
		logger = slog.Default()
	}
	// Step 1: collect all sidecar pairs, deduplicating by remote name.
	var uploads []FilePair
	seen := map[string]bool{}
	for _, vf := range videoFiles {
		pairs := PlanScrapeUpload(Op{File: vf.File, Parent: vf.Parent, NewName: vf.NewName}, vf.NewName, cacheRoot)
		for _, p := range pairs {
			if !seen[p.RemoteName] {
				uploads = append(uploads, p)
				seen[p.RemoteName] = true
			}
		}
	}
	if len(uploads) == 0 {
		return 0
	}

	// Step 2: optionally list target dir to find stale sidecars.
	if !skipListing {
		existing := map[string]bool{}
		if entries, err := client.ListDir(targetDir); err == nil {
			for _, e := range entries {
				if e.Type == "file" {
					existing[e.Name] = true
				}
			}
		}
		// Step 3: delete stale sidecars.
		var toDelete []string
		for _, p := range uploads {
			if existing[p.RemoteName] {
				toDelete = append(toDelete, targetDir+"/"+p.RemoteName)
			}
		}
		if len(toDelete) > 0 {
			if err := client.Delete(toDelete); err != nil {
				logger.Warn("organizer: delete old sidecars failed", "dir", targetDir, "error", err)
			} else {
				logger.Debug("organizer: deleted old sidecars", "count", len(toDelete), "dir", targetDir)
			}
		}
	}

	// Step 4: upload new sidecar files.
	uploaded := 0
	for _, p := range uploads {
		if _, err := client.Upload(p.LocalPath, targetDir, p.RemoteName); err != nil {
			logger.Warn("organizer: upload failed", "file", p.RemoteName, "error", err)
		} else {
			logger.Debug("organizer: uploaded sidecar", "file", p.RemoteName, "dir", targetDir)
			uploaded++
		}
	}
	return uploaded
}

// PlanScrapeUpload finds local sidecar files for a single video operation and
// returns pairs of (local_path, remote_name).
//
// It searches under cacheRoot/scrape_output/<category>/<parent_underscored>/
// for NFO/image files matching the video stem.
//
// newVideoName is the renamed target filename (used to derive the remote NFO
// name); if empty, the original filename stem is used.
func PlanScrapeUpload(op Op, newVideoName string, cacheRoot string) []FilePair {
	// media115 scrape_output lives inside the media115 cache dir, not cloud115.
	media115CacheRoot := strings.Replace(cacheRoot, "cloud115", "media115", 1)
	scrapeDir := filepath.Join(media115CacheRoot, "scrape_output")

	if _, err := os.Stat(scrapeDir); err != nil {
		return nil
	}

	var nfoName string
	if newVideoName != "" {
		nfoName = stem(newVideoName) + ".nfo"
	}
	videoStem := stem(op.File)

	// The Python code converts the parent path into a dir name by replacing "/" with "_".
	searchName := strings.ReplaceAll(op.Parent, "/", "_")

	// Walk category dirs, then output dirs.
	catEntries, err := os.ReadDir(scrapeDir)
	if err != nil {
		return nil
	}

	for _, catEntry := range catEntries {
		if !catEntry.IsDir() {
			continue
		}
		catDir := filepath.Join(scrapeDir, catEntry.Name())
		outEntries, err := os.ReadDir(catDir)
		if err != nil {
			continue
		}
		for _, outEntry := range outEntries {
			if !outEntry.IsDir() || outEntry.Name() != searchName {
				continue
			}
			outDir := filepath.Join(catDir, outEntry.Name())
			files, err := os.ReadDir(outDir)
			if err != nil {
				continue
			}
			var result []FilePair
			for _, f := range files {
				if f.IsDir() {
					continue
				}
				fName := f.Name()
				ext := filepath.Ext(fName)
				switch ext {
				case ".nfo":
					fStem := stem(fName)
					if fStem == videoStem {
						// Per-video NFO: rename to new video name if applicable.
						remoteName := fName
						if nfoName != "" {
							remoteName = nfoName
						}
						result = append(result, FilePair{
							LocalPath:  filepath.Join(outDir, fName),
							RemoteName: remoteName,
						})
					} else if fName == "tvshow.nfo" {
						result = append(result, FilePair{
							LocalPath:  filepath.Join(outDir, fName),
							RemoteName: "tvshow.nfo",
						})
					}
					// Skip NFOs for other videos in the same dir.
				case ".jpg", ".png":
					// Shared artwork: poster.jpg, fanart.jpg etc.
					result = append(result, FilePair{
						LocalPath:  filepath.Join(outDir, fName),
						RemoteName: fName,
					})
				}
			}
			return result
		}
	}
	return nil
}
