package organizer

import (
	"log/slog"
	"regexp"
	"strings"

	"github.com/newcoderlife/media115/internal/cloud115"
	"github.com/newcoderlife/media115/internal/config"
)

// Execute runs the 6-phase organize plan against the live 115 API.
//
// Phases:
//  1. Resolve files — list each parent dir, verify files exist
//  2. Create dirs   — pre-list category, only mkdir missing subdirs
//  3. Batch move    — group by target folder, call client.Move
//  4. Batch rename  — client.BatchRename
//  5. Upload sidecar— SyncSidecars per target dir
//  6. Cleanup       — delete source dirs that no longer have video files
func Execute(client *cloud115.Client, ops []Op, categoryPath string, logger *slog.Logger) ([]OpResult, error) {
	if logger == nil {
		logger = slog.Default()
	}

	catPath := "/" + categoryPath
	cacheRoot := config.CacheDir()

	var results []OpResult

	// Collect skip ops immediately.
	var activeOps []Op
	for _, op := range ops {
		if op.Action == "skip" {
			results = append(results, OpResult{Op: op, Status: "skipped"})
		} else {
			activeOps = append(activeOps, op)
		}
	}
	if len(activeOps) == 0 {
		return results, nil
	}

	// Verify category path exists.
	if _, err := client.ResolvePath(catPath); err != nil {
		for _, op := range activeOps {
			results = append(results, OpResult{Op: op, Status: "error", Error: "category dir not found: " + categoryPath})
		}
		return results, nil
	}

	// ── Phase 1: Resolve files ────────────────────────────────────────────────
	logger.Info("organizer: resolving files", "count", len(activeOps))

	type resolved struct {
		op         Op
		parentPath string
	}
	dirListingCache := map[string]map[string]bool{} // parentPath → set of filenames
	var resolvedOps []resolved

	for _, op := range activeOps {
		parentPath := op.Parent
		if _, ok := dirListingCache[parentPath]; !ok {
			entries, err := client.ListDir("/" + parentPath)
			if err != nil {
				results = append(results, OpResult{Op: op, Status: "not_found", Error: "dir not found: " + parentPath})
				dirListingCache[parentPath] = nil // mark as failed
				continue
			}
			fileSet := map[string]bool{}
			for _, e := range entries {
				if e.Type == "file" {
					fileSet[e.Name] = true
				}
			}
			dirListingCache[parentPath] = fileSet
		}

		fileSet := dirListingCache[parentPath]
		if fileSet == nil || !fileSet[op.File] {
			results = append(results, OpResult{Op: op, Status: "not_found", Error: "file not in dir: " + op.File})
			continue
		}
		resolvedOps = append(resolvedOps, resolved{op, parentPath})
	}
	logger.Info("organizer: resolved", "ok", len(resolvedOps), "total", len(activeOps))

	// ── Phase 2: Create target directories ───────────────────────────────────
	targetFolders := map[string]bool{}
	for _, r := range resolvedOps {
		if r.op.NewFolder != "" {
			targetFolders[r.op.NewFolder] = true
		}
	}
	logger.Info("organizer: creating dirs", "count", len(targetFolders))

	existingItems, _ := client.ListDir(catPath)
	existingDirs := map[string]bool{}
	for _, e := range existingItems {
		if e.Type == "dir" {
			existingDirs[e.Name] = true
		}
	}

	createdFolders := map[string]bool{}
	newlyCreated := map[string]bool{}
	for folder := range targetFolders {
		if existingDirs[folder] {
			createdFolders[folder] = true
			continue
		}
		if _, err := client.Mkdir(catPath+"/"+folder, false); err != nil {
			// Last-resort: check existence.
			if _, err2 := client.ResolvePath(catPath + "/" + folder); err2 == nil {
				createdFolders[folder] = true
			}
			// Otherwise skip — move will fail, op gets error status later.
			continue
		}
		createdFolders[folder] = true
		newlyCreated[folder] = true
	}

	// ── Phase 3: Batch move ───────────────────────────────────────────────────
	type moveGroup struct {
		srcPaths []string
		srcOps   []int // indices into resolvedOps
	}
	moveGroups := map[string]*moveGroup{} // target folder → group
	for i, r := range resolvedOps {
		if r.op.NewFolder != "" && createdFolders[r.op.NewFolder] {
			g := moveGroups[r.op.NewFolder]
			if g == nil {
				g = &moveGroup{}
				moveGroups[r.op.NewFolder] = g
			}
			g.srcPaths = append(g.srcPaths, "/"+r.parentPath+"/"+r.op.File)
			g.srcOps = append(g.srcOps, i)
		}
	}

	failedIdx := map[int]bool{}
	totalMoves := 0
	for _, g := range moveGroups {
		totalMoves += len(g.srcPaths)
	}
	logger.Info("organizer: moving files", "moves", totalMoves, "dirs", len(moveGroups))

	for folder, g := range moveGroups {
		targetDir := catPath + "/" + folder
		if err := client.Move(g.srcPaths, targetDir); err != nil {
			logger.Error("organizer: move failed", "target", folder, "error", err)
			for _, idx := range g.srcOps {
				failedIdx[idx] = true
			}
		}
	}

	// ── Phase 4: Batch rename ─────────────────────────────────────────────────
	var renameItems []cloud115.BatchRenameItem
	var renameIdx []int
	for i, r := range resolvedOps {
		if failedIdx[i] {
			continue
		}
		if r.op.NewName == "" || r.op.NewName == r.op.File {
			continue
		}
		var filePath string
		if r.op.NewFolder != "" && createdFolders[r.op.NewFolder] {
			filePath = catPath + "/" + r.op.NewFolder + "/" + r.op.File
		} else {
			filePath = "/" + r.parentPath + "/" + r.op.File
		}
		renameItems = append(renameItems, cloud115.BatchRenameItem{Path: filePath, NewName: r.op.NewName})
		renameIdx = append(renameIdx, i)
	}
	if len(renameItems) > 0 {
		logger.Info("organizer: renaming files", "count", len(renameItems))
		if err := client.BatchRename(renameItems); err != nil {
			logger.Error("organizer: batch rename failed", "error", err)
			for _, idx := range renameIdx {
				failedIdx[idx] = true
			}
		}
	}

	// ── Phase 4.5: Update file_map for renamed files ─────────────────────────
	for i, r := range resolvedOps {
		if failedIdx[i] {
			continue
		}
		if r.op.NewName == "" || r.op.NewName == r.op.File {
			continue
		}
		oldStem := stemFilename(r.op.File)
		newStem := stemFilename(r.op.NewName)
		if oldStem != newStem {
			if oldData := fileMapCacheGet("file_map", oldStem); oldData != nil {
				fileMapCachePut("file_map", newStem, oldData)
			}
		}
	}

	// ── Phase 5: Upload sidecars ──────────────────────────────────────────────
	// Group video_file dicts by target upload directory.
	targetGroups := map[string][]VideoFile{} // target dir → []VideoFile
	for i, r := range resolvedOps {
		if failedIdx[i] {
			results = append(results, OpResult{Op: r.op, Status: "error", Error: "move or rename failed"})
			continue
		}
		var uploadDir string
		if r.op.NewFolder != "" && createdFolders[r.op.NewFolder] {
			uploadDir = catPath + "/" + r.op.NewFolder
		} else {
			uploadDir = "/" + r.parentPath
		}
		targetGroups[uploadDir] = append(targetGroups[uploadDir], VideoFile{
			File:    r.op.File,
			Parent:  r.op.Parent,
			NewName: r.op.NewName,
		})
		results = append(results, OpResult{Op: r.op, Status: "ok"})
	}

	logger.Info("organizer: uploading sidecars", "dirs", len(targetGroups))
	totalUploaded := 0
	for uploadDir, vfList := range targetGroups {
		dirName := leafName(uploadDir)
		isNew := newlyCreated[dirName]
		totalUploaded += SyncSidecars(client, uploadDir, vfList, isNew, logger, cacheRoot)
	}
	logger.Info("organizer: sidecar uploads complete", "uploaded", totalUploaded)

	// ── Phase 6: Cleanup empty source dirs ───────────────────────────────────
	sourceDirs := map[string]bool{}
	for _, r := range resolvedOps {
		if r.op.NewFolder == "" {
			continue
		}
		parentLeaf := leafName(r.op.Parent)
		if parentLeaf != r.op.NewFolder {
			sourceDirs[parentLeaf] = true
		}
	}

	if len(sourceDirs) > 0 {
		logger.Info("organizer: cleaning up old dirs", "count", len(sourceDirs))
		catItems, _ := client.ListDir(catPath)
		for _, item := range catItems {
			if item.Type != "dir" || !sourceDirs[item.Name] {
				continue
			}
			contents, err := client.ListDir(catPath + "/" + item.Name)
			if err != nil {
				continue
			}
			hasVideo := false
			for _, f := range contents {
				if f.Type == "file" && isVideoFile(f.Name) {
					hasVideo = true
					break
				}
			}
			if !hasVideo {
				if err := client.Delete([]string{catPath + "/" + item.Name}); err != nil {
					logger.Error("organizer: delete dir failed", "name", item.Name, "error", err)
				} else {
					logger.Debug("organizer: deleted empty dir", "name", item.Name)
				}
			}
		}
	}

	return results, nil
}

var reVideoExt = regexp.MustCompile(`(?i)\.(mkv|mp4|avi|ts|rmvb|flv|wmv|mov|m4v|iso)$`)

func isVideoFile(name string) bool { return reVideoExt.MatchString(name) }

// stemFilename returns the filename without its extension.
func stemFilename(name string) string {
	base := leafName(name)
	if idx := strings.LastIndex(base, "."); idx > 0 {
		return base[:idx]
	}
	return base
}

// leafName returns the last path component (everything after the last "/").
func leafName(p string) string {
	if idx := strings.LastIndex(p, "/"); idx >= 0 {
		return p[idx+1:]
	}
	return p
}
