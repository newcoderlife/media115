// Package organizer builds and executes rename/move plans for 115 media libraries.
package organizer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/newcoderlife/media115/internal/cloud115"
	"github.com/newcoderlife/media115/internal/config"
)

// Op is one entry in the organize plan.
type Op struct {
	File      string // original filename (e.g. "ABP-040.FHD.mp4")
	Path      string // full tree path (e.g. "影音/AV/ABP-040/ABP-040.FHD.mp4")
	Parent    string // parent dir path (e.g. "影音/AV/ABP-040")
	Type      string // media type from scrape: "movie", "av", "tv", "unknown"
	Action    string // "skip" or "rename"
	NewFolder string // target folder name (leaf only); empty = stay in place
	NewName   string // new filename; empty = no rename
	Reason    string // human-readable note for skipped ops
	SourceID  string // tmdb_id or AV number
}

// OpResult augments an Op with execution status.
type OpResult struct {
	Op
	Status string
	Error  string
}

// VideoFile is passed to SyncSidecars per video in a target directory.
type VideoFile struct {
	File    string
	Parent  string
	NewName string
}

// reStdNameExt matches files already in "Title (Year).ext" standard format.
var reStdNameExt = regexp.MustCompile(`^.+\(\d{4}\)\.\w{2,4}$`)

// reExtract extracts the file extension including the dot.
var reExtract = regexp.MustCompile(`(\.\w{2,4})$`)

// sanitize removes characters not allowed in filenames.
func sanitize(name string) string {
	return strings.TrimSpace(regexp.MustCompile(`[<>:"/\\|?*]`).ReplaceAllString(name, ""))
}

// stem returns the filename without extension.
func stem(name string) string {
	ext := filepath.Ext(name)
	if ext == "" {
		return name
	}
	return name[:len(name)-len(ext)]
}

// ExtractAVSuffix extracts a content-discriminating suffix from the part of a
// filename that follows the AV number.
//
// Preserves: Part1/Part2, A/B/C/D (disc), -C (cut version), CD1/CD2.
// Strips: HD, FHD, 4K, quality markers, codec info, team names.
//
// Examples:
//
//	"-C.mp4 stuff" → "-C"
//	"A.FHD"        → ".A"
//	".Part1"       → ".Part1"
//	".FHD"         → ""
//	"B_4K^WM"      → ".B"
func ExtractAVSuffix(afterNumber string) string {
	s := afterNumber
	if s == "" {
		return ""
	}

	// Pattern 1: -C (cut/censored version)
	if strings.HasPrefix(s, "-C") && (len(s) == 2 || !isAlpha(rune(s[2]))) {
		return "-C"
	}

	// Pattern 2: single letter A-D (multi-disc)
	// Matches "A.FHD", "B_4K", ".A", ".B"
	if len(s) >= 1 && isDiscLetter(rune(s[0])) {
		if len(s) == 1 || (!isAlphaNum(rune(s[1])) || isDigit(rune(s[1]))) {
			return "." + strings.ToUpper(string(s[0]))
		}
	}
	// Also match ".A", ".B" etc (previously organised)
	if len(s) >= 2 && (s[0] == '.' || s[0] == '_') && isDiscLetter(rune(s[1])) {
		if len(s) == 2 || !isAlphaNum(rune(s[2])) {
			return "." + strings.ToUpper(string(s[1]))
		}
	}

	// Pattern 3: .Part1, _Part2, Part1 …
	if m := regexp.MustCompile(`(?i)[._-]?(Part\d+)`).FindStringSubmatch(s); m != nil {
		return "." + m[1]
	}

	// Pattern 4: .CD1, _CD2
	if m := regexp.MustCompile(`(?i)[._-]?(CD\d+)`).FindStringSubmatch(s); m != nil {
		return "." + m[1]
	}

	return ""
}

func isAlpha(r rune) bool      { return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') }
func isDigit(r rune) bool      { return r >= '0' && r <= '9' }
func isAlphaNum(r rune) bool   { return isAlpha(r) || isDigit(r) }
func isDiscLetter(r rune) bool { return r == 'A' || r == 'B' || r == 'C' || r == 'D' || r == 'a' || r == 'b' || r == 'c' || r == 'd' }

// fileMapCacheGet reads a JSON cache entry from
// ~/.cache/media115/scrape/{source}/{key}.json.
// Returns nil if absent or unreadable.
func fileMapCacheGet(source, key string) map[string]any {
	cacheDir := config.CacheDir()
	// Python media115 cache lives in ~/.cache/media115/, not ~/.cache/cloud115/
	// Replace the cloud115-specific suffix with media115.
	cacheDir = strings.Replace(cacheDir, "cloud115", "media115", 1)
	path := filepath.Join(cacheDir, "scrape", source, key+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	// Treat not_found entries as absent.
	if nf, _ := m["_not_found"].(bool); nf {
		return nil
	}
	return m
}

// BuildPlan builds a rename/move plan from the file_map cache.
//
// category filters tree entries (e.g. "AV", "电影").
// cacheGet may be nil; when nil, fileMapCacheGet is used.
func BuildPlan(category string, treeEntries []cloud115.TreeEntry, cacheGet func(source, key string) map[string]any) []Op {
	if cacheGet == nil {
		cacheGet = fileMapCacheGet
	}

	var ops []Op

	// Filter to videos in this category.
	var videos []cloud115.TreeEntry
	for _, e := range treeEntries {
		if !e.IsVideo {
			continue
		}
		// Match if category appears as a path component.
		pathSlash := "/" + e.Path + "/"
		catSlash := "/" + category + "/"
		if strings.Contains(pathSlash, catSlash) {
			videos = append(videos, e)
		}
	}

	for _, item := range videos {
		name := item.Name
		parent := item.Parent

		// parent leaf = last component of parent path
		parentLeaf := parent
		if idx := strings.LastIndex(parent, "/"); idx >= 0 {
			parentLeaf = parent[idx+1:]
		}

		scrapeInfo := cacheGet("file_map", stem(name))

		op := Op{
			File:   name,
			Path:   item.Path,
			Parent: parent,
			Type:   "unknown",
			Action: "skip",
		}

		if scrapeInfo != nil {
			if t, ok := scrapeInfo["type"].(string); ok {
				op.Type = t
			}
			// source_id: prefer tmdb_id, fallback to number
			if tmdbID, ok := scrapeInfo["tmdb_id"]; ok && tmdbID != nil {
				op.SourceID = formatID(tmdbID)
			} else if num, ok := scrapeInfo["number"].(string); ok {
				op.SourceID = num
			}
		}

		if scrapeInfo == nil {
			if reStdNameExt.MatchString(name) {
				op.Reason = "already in standard format"
			} else {
				op.Reason = "no scrape result cached"
			}
			ops = append(ops, op)
			continue
		}

		mediaType := op.Type
		ext := ".mkv"
		if m := reExtract.FindStringSubmatch(name); m != nil {
			ext = m[1]
		}

		switch mediaType {
		case "movie":
			title, _ := scrapeInfo["title"].(string)
			yearVal := scrapeInfo["year"]
			year := toInt(yearVal)
			if title != "" && year != 0 {
				newFolder := sanitize(title) + " (" + itoa(year) + ")"
				newName := sanitize(title) + " (" + itoa(year) + ")" + ext
				if newFolder != parentLeaf || newName != name {
					op.NewFolder = newFolder
					op.NewName = newName
					op.Action = "rename"
				}
			}

		case "av":
			number, _ := scrapeInfo["number"].(string)
			if number != "" {
				newFolder := number
				// Strip extension from name stem to search for number
				nameStem := reExtract.ReplaceAllString(name, "")
				// Build flexible regex from number (tolerates optional separators)
				numParts := regexp.MustCompile(`[-_.]`).Split(number, -1)
				escaped := make([]string, len(numParts))
				for i, p := range numParts {
					escaped[i] = regexp.QuoteMeta(p)
				}
				numRe := regexp.MustCompile(`(?i)` + strings.Join(escaped, `[-_.]?`) + `(.*)`)
				afterNumber := ""
				if m := numRe.FindStringSubmatch(nameStem); m != nil {
					afterNumber = m[1]
				}
				suffix := ExtractAVSuffix(afterNumber)
				newName := number + suffix + ext
				if newFolder != parentLeaf || newName != name {
					if newFolder != parentLeaf {
						op.NewFolder = newFolder
					}
					if newName != name {
						op.NewName = newName
					}
					op.Action = "rename"
				}
			}

		case "tv":
			showTitle := ""
			if st, ok := scrapeInfo["showtitle"].(string); ok && st != "" {
				showTitle = st
			} else if t, ok := scrapeInfo["title"].(string); ok {
				showTitle = t
			}
			year := toInt(scrapeInfo["year"])
			season := toInt(scrapeInfo["season"])
			episode := toInt(scrapeInfo["episode"])

			var newFolder string
			if showTitle != "" && year != 0 {
				newFolder = sanitize(showTitle) + " (" + itoa(year) + ")"
			} else if showTitle != "" {
				newFolder = sanitize(showTitle)
			}

			var newName string
			if showTitle != "" && season > 0 && episode > 0 {
				newName = sanitize(showTitle) + " " + fmtSE(season, episode) + ext
			}

			needsRename := false
			if newFolder != "" && newFolder != parentLeaf {
				needsRename = true
			} else {
				newFolder = "" // already correct
			}
			if newName != "" && newName != name {
				needsRename = true
			} else {
				newName = ""
			}

			if needsRename {
				op.NewFolder = newFolder
				op.NewName = newName
				op.Action = "rename"
			}
		}

		if op.Action == "skip" && op.Reason == "" {
			op.Reason = "already correct"
		}

		ops = append(ops, op)
	}

	return ops
}

// ── helpers ───────────────────────────────────────────────────────────────────

func toInt(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case int64:
		return int(x)
	case json.Number:
		n, _ := x.Int64()
		return int(n)
	}
	return 0
}

func itoa(n int) string {
	return fmt.Sprintf("%d", n)
}

func fmtSE(season, episode int) string {
	return fmt.Sprintf("S%02dE%02d", season, episode)
}

func formatID(v any) string {
	switch x := v.(type) {
	case float64:
		return fmt.Sprintf("%.0f", x)
	case int:
		return fmt.Sprintf("%d", x)
	case string:
		return x
	}
	return ""
}
