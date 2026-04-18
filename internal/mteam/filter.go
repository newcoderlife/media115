package mteam

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/gg/gconv"
)

var (
	reRes4K  = regexp.MustCompile(`(?i)\b(2160p|4k|uhd)\b`)
	reTVPack = regexp.MustCompile(`(?i)(e\d{2,3}-e\d{2,3}|complete|完结|全\s*\d+\s*集)`)
)

// parseCreatedDate parses the M-Team createdDate format, assumed UTC+8.
func parseCreatedDate(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*3600)
	}
	t, err := time.ParseInLocation("2006-01-02 15:04:05", s, loc)
	if err != nil {
		// Fallback to RFC3339.
		if t2, err2 := time.Parse(time.RFC3339, s); err2 == nil {
			return t2, true
		}
		return time.Time{}, false
	}
	return t, true
}

// parseRating parses a float rating; empty / "-" / non-numeric return 0, false.
func parseRating(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// containsFold reports whether s contains any element of subs (case-insensitive).
func containsFold(s string, subs []string) (string, bool) {
	if len(subs) == 0 {
		return "", false
	}
	sl := strings.ToLower(s)
	for _, sub := range subs {
		sub = strings.TrimSpace(sub)
		if sub == "" {
			continue
		}
		if strings.Contains(sl, strings.ToLower(sub)) {
			return sub, true
		}
	}
	return "", false
}

// labelsContain reports whether labels contains any of targets (case-insensitive).
func labelsContain(labels, targets []string) (string, bool) {
	if len(labels) == 0 || len(targets) == 0 {
		return "", false
	}
	lset := make(map[string]struct{}, len(labels))
	for _, l := range labels {
		lset[strings.ToLower(strings.TrimSpace(l))] = struct{}{}
	}
	for _, t := range targets {
		if _, ok := lset[strings.ToLower(strings.TrimSpace(t))]; ok {
			return t, true
		}
	}
	return "", false
}

// isFree returns true when the discount grants 100% free leech.
func isFree(d string) bool {
	switch strings.ToUpper(d) {
	case DiscountFree, Discount2XFree:
		return true
	default:
		return false
	}
}

// is4K returns true when the torrent is 4K based on labelsNew or name regex.
func is4K(t *Torrent) bool {
	if _, ok := labelsContain(t.LabelsNew, []string{"4k", "2160p", "uhd"}); ok {
		return true
	}
	return reRes4K.MatchString(t.Name)
}

// isTVPack returns true when the name indicates a complete season pack.
func isTVPack(name string) bool {
	return reTVPack.MatchString(name)
}

// hqLabels are the labels that indicate high-quality HDR/DOVI content.
var hqLabels = []string{"HDR", "HDR10", "HDR10+", "DV", "DOVI", "Dolby Vision"}

// isHQ returns true when the torrent has any HDR/DOVI label.
func isHQ(t *Torrent) bool {
	_, ok := labelsContain(t.LabelsNew, hqLabels)
	return ok
}

// junkLabels are auto-injected into labels_deny when NoJunk is enabled.
var junkLabels = []string{"原盘", "BDMV", "REMUX"}

// videoExts are file extensions counted for junk-pack detection.
var videoExts = map[string]bool{
	".mkv": true, ".mp4": true, ".avi": true, ".ts": true, ".m2ts": true, ".wmv": true,
}

// JunkVideoThreshold returns the max-video-files threshold for a search mode.
// Returns 0 when no junk-pack check is needed (adult).
func JunkVideoThreshold(mode string) int {
	switch mode {
	case ModeMovie:
		return 20
	case ModeTVShow:
		return 500
	case ModeMusic:
		return 100
	case ModeAdult:
		return 0
	default:
		return 20
	}
}

// CheckFiles inspects the file list and rejects packs with too many video files (肉酱盘).
func CheckFiles(files []TorrentFile, maxVideoFiles int) MatchResult {
	if maxVideoFiles <= 0 {
		maxVideoFiles = JunkVideoThreshold(ModeMovie) // conservative default
	}
	count := 0
	for _, f := range files {
		ext := strings.ToLower(filepath.Ext(f.Name))
		if videoExts[ext] {
			count++
		}
	}
	if count > maxVideoFiles {
		return MatchResult{Reason: fmt.Sprintf("video files %d exceeds limit %d (junk pack)", count, maxVideoFiles)}
	}
	return MatchResult{Match: true}
}

// MatchResult is the outcome of Rule.Match on a single torrent.
type MatchResult struct {
	Match  bool
	Reason string // non-empty when Match is false
}

// Match evaluates rule against torrent t. Now must be provided for
// deterministic FreshHours checks (callers typically pass time.Now()).
func (r *Rule) Match(t *Torrent, now time.Time) MatchResult {
	if r == nil {
		return MatchResult{Match: true}
	}

	if r.RequireFree && !isFree(t.Status.Discount) {
		return MatchResult{Reason: "not free"}
	}

	if r.Require4K && !is4K(t) {
		return MatchResult{Reason: "not 4K"}
	}

	if r.RequireHQ && !isHQ(t) {
		return MatchResult{Reason: "not HQ (no HDR/DOVI label)"}
	}

	if r.MaxFiles > 0 && gconv.To[int64, string](t.NumFiles) > int64(r.MaxFiles) {
		return MatchResult{Reason: "numfiles exceeds max_files"}
	}

	size := gconv.To[int64, string](t.Size)
	if r.MinSize > 0 && size < r.MinSize {
		return MatchResult{Reason: "size below min_size"}
	}
	if r.MaxSize > 0 && size > r.MaxSize {
		return MatchResult{Reason: "size above max_size"}
	}

	if r.MinSeeders > 0 && gconv.To[int64, string](t.Status.Seeders) < int64(r.MinSeeders) {
		return MatchResult{Reason: "seeders below min_seeders"}
	}

	if hit, ok := containsFold(t.Name, r.ExcludeKeywords); ok {
		return MatchResult{Reason: "matches exclude keyword: " + hit}
	}

	if hit, ok := labelsContain(t.LabelsNew, r.LabelsDeny); ok {
		return MatchResult{Reason: "matches labels_deny: " + hit}
	}

	// NoJunk: auto-reject torrents with 原盘/BDMV/REMUX labels.
	if r.NoJunk {
		if hit, ok := labelsContain(t.LabelsNew, junkLabels); ok {
			return MatchResult{Reason: "junk label: " + hit}
		}
	}

	if len(r.LabelsAllow) > 0 {
		if _, ok := labelsContain(t.LabelsNew, r.LabelsAllow); !ok {
			return MatchResult{Reason: "labels_allow not satisfied"}
		}
	}

	if r.FreshHours > 0 {
		created, ok := parseCreatedDate(t.CreatedDate)
		if !ok {
			return MatchResult{Reason: "cannot parse createdDate for fresh_hours"}
		}
		age := now.Sub(created)
		if age > time.Duration(r.FreshHours)*time.Hour {
			return MatchResult{Reason: "older than fresh_hours window"}
		}
	}

	if r.MinRating > 0 {
		imdb, imdbOK := parseRating(t.IMDBRating)
		douban, doubanOK := parseRating(t.DoubanRating)
		if !imdbOK && !doubanOK {
			return MatchResult{Reason: "no rating available for min_rating check"}
		}
		best := imdb
		if douban > best {
			best = douban
		}
		if best < r.MinRating {
			return MatchResult{Reason: fmt.Sprintf("best rating %.1f below min_rating %.1f", best, r.MinRating)}
		}
	}

	if r.TVCompleteEpisodes && strings.EqualFold(r.Mode, ModeTVShow) {
		if !isTVPack(t.Name) {
			return MatchResult{Reason: "not a TV pack"}
		}
	}

	return MatchResult{Match: true}
}
