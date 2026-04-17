package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bytedance/mockey"

	"github.com/newcoderlife/media115/internal/cloud115"
	"github.com/newcoderlife/media115/internal/config"
	"github.com/newcoderlife/media115/internal/organizer"
	"github.com/newcoderlife/media115/internal/scraper"
)

// ── uploadMissingNFO edge cases ───────────────────────────────────────────────

// TestUploadMissingNFONoSkips verifies that an empty skip list is a no-op
// (does not panic, does not call any client methods).
func TestUploadMissingNFONoSkips(t *testing.T) {
	// A nil client is safe because uploadMissingNFO returns early when
	// len(skips) == 0.
	uploadMissingNFO(nil, "影音/AV", []organizer.Op{}, nil, nil)
}

// TestUploadMissingNFONilLogger verifies that a nil logger falls back to the
// default without panicking, when there are no skips.
func TestUploadMissingNFONilLoggerNoSkips(t *testing.T) {
	uploadMissingNFO(nil, "影音/电影", []organizer.Op{}, []cloud115.TreeEntry{}, nil)
}

// TestUploadMissingNFOSkipReasonFilter verifies that ops whose Reason is not
// "already correct" or "already in standard format" are silently ignored,
// so no client call is attempted (client is nil, would panic if called).
func TestUploadMissingNFOSkipReasonFilter(t *testing.T) {
	skips := []organizer.Op{
		{
			File:   "SomeMovie.2023.mkv",
			Parent: "影音/电影/SomeMovie.2023",
			Reason: "some other reason",
		},
		{
			File:   "Another.2020.mkv",
			Parent: "影音/电影/Another.2020",
			Reason: "unknown reason",
		},
	}
	// If the filter doesn't work, SyncSidecars would be called with a nil
	// client and would panic.
	uploadMissingNFO(nil, "影音/电影", skips, []cloud115.TreeEntry{}, nil)
}

// TestUploadMissingNFOSkipWhenNFOExists verifies that a skip op whose NFO
// already exists in the tree is correctly filtered out before any client call.
func TestUploadMissingNFOSkipWhenNFOExists(t *testing.T) {
	filename := "Good.Movie.2022.mkv"
	parent := "影音/电影/Good.Movie.2022"
	// Simulate an existing NFO in the tree for this file.
	treeEntries := []cloud115.TreeEntry{
		{
			Name:    "Good.Movie.2022.nfo",
			Path:    "影音/电影/Good.Movie.2022/Good.Movie.2022.nfo",
			Parent:  parent,
			IsNFO:   true,
			IsVideo: false,
		},
	}
	skips := []organizer.Op{
		{
			File:   filename,
			Parent: parent,
			Reason: "already correct",
		},
	}
	// The NFO exists in treeEntries, so the video should be filtered out
	// before any client call is made. Passing nil client is safe here.
	uploadMissingNFO(nil, "影音/电影", skips, treeEntries, nil)
}

// ── scrapeRun loop coverage (mockey) ─────────────────────────────────────────

func TestScrapeRunScrapeSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	m := mockey.Mock(scraper.Scrape).To(
		func(_, _, _, _ string, _ scraper.ScrapeOpts, _ *slog.Logger) *scraper.ScrapeResult {
			return &scraper.ScrapeResult{
				Status: "ok",
				Match:  "ABP-040",
				IDs:    map[string]string{"number": "ABP-040"},
				Meta:   &scraper.Metadata{Title: "Some Title"},
			}
		},
	).Build()
	defer m.UnPatch()

	var buf bytes.Buffer
	o := newTestScrapeOpts(&buf, tmpDir)
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			{Path: "影音/AV/test/ABP-040.mp4", Name: "ABP-040.mp4", Parent: "影音/AV/test", IsVideo: true},
		}, nil
	}

	if err := scrapeRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "✓") {
		t.Error("expected success marker in output")
	}
	if !strings.Contains(out, "日志保存至") {
		t.Error("expected log save message")
	}
	if !strings.Contains(out, "下一步") {
		t.Error("expected next-step hint")
	}
}

func TestScrapeRunScrapeNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	m := mockey.Mock(scraper.Scrape).To(
		func(_, _, _, _ string, _ scraper.ScrapeOpts, _ *slog.Logger) *scraper.ScrapeResult {
			return &scraper.ScrapeResult{Status: "not_found"}
		},
	).Build()
	defer m.UnPatch()

	var buf bytes.Buffer
	o := newTestScrapeOpts(&buf, tmpDir)
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			{Path: "影音/AV/test/UNKNOWN-001.mp4", Name: "UNKNOWN-001.mp4", Parent: "影音/AV/test", IsVideo: true},
		}, nil
	}

	if err := scrapeRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "✗") {
		t.Error("expected failure marker in output")
	}

	// Verify not_found was cached.
	cached := scraper.CacheGet("av", "UNKNOWN-001")
	if cached == nil {
		t.Error("expected not_found cache entry")
	}
}

func TestScrapeRunScrapeError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	m := mockey.Mock(scraper.Scrape).To(
		func(_, _, _, _ string, _ scraper.ScrapeOpts, _ *slog.Logger) *scraper.ScrapeResult {
			return &scraper.ScrapeResult{Status: "error", Error: "network timeout"}
		},
	).Build()
	defer m.UnPatch()

	var buf bytes.Buffer
	o := newTestScrapeOpts(&buf, tmpDir)
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			{Path: "影音/AV/test/ERR-001.mp4", Name: "ERR-001.mp4", Parent: "影音/AV/test", IsVideo: true},
		}, nil
	}

	if err := scrapeRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "network timeout") {
		t.Error("expected error message in output")
	}
}

func TestScrapeRunCachedHit(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	// Write a cached scrape result.
	scraper.CachePut("av", "ABP-040", map[string]any{
		"title":      "Cached Title",
		"number":     "ABP-040",
		"_cached_at": float64(time.Now().Unix()),
	})

	// Scrape should NOT be called since it's cached.
	m := mockey.Mock(scraper.Scrape).To(
		func(_, _, _, _ string, _ scraper.ScrapeOpts, _ *slog.Logger) *scraper.ScrapeResult {
			t.Error("Scrape should not be called for cached result")
			return &scraper.ScrapeResult{Status: "error"}
		},
	).Build()
	defer m.UnPatch()

	var buf bytes.Buffer
	o := newTestScrapeOpts(&buf, tmpDir)
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			{Path: "影音/AV/test/ABP-040.mp4", Name: "ABP-040.mp4", Parent: "影音/AV/test", IsVideo: true},
		}, nil
	}

	if err := scrapeRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "✓") {
		t.Error("expected success marker for cached hit")
	}
}

func TestScrapeRunNotFoundCached(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	// Write a not_found cache entry.
	scraper.CachePut("av", "MISS-001", map[string]any{
		"_not_found": true,
		"_cached_at": float64(time.Now().Unix()),
	})

	// Scrape should NOT be called.
	m := mockey.Mock(scraper.Scrape).To(
		func(_, _, _, _ string, _ scraper.ScrapeOpts, _ *slog.Logger) *scraper.ScrapeResult {
			t.Error("Scrape should not be called for not_found cache")
			return &scraper.ScrapeResult{Status: "error"}
		},
	).Build()
	defer m.UnPatch()

	var buf bytes.Buffer
	o := newTestScrapeOpts(&buf, tmpDir)
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			{Path: "影音/AV/test/MISS-001.mp4", Name: "MISS-001.mp4", Parent: "影音/AV/test", IsVideo: true},
		}, nil
	}

	if err := scrapeRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "✗") {
		t.Error("expected failure marker for cached not_found")
	}
}

func TestScrapeRunWithLimitAndMultipleFiles(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	callCount := 0
	m := mockey.Mock(scraper.Scrape).To(
		func(_, _, _, _ string, _ scraper.ScrapeOpts, _ *slog.Logger) *scraper.ScrapeResult {
			callCount++
			return &scraper.ScrapeResult{
				Status: "ok",
				Match:  "Match",
				IDs:    map[string]string{"number": "ID"},
			}
		},
	).Build()
	defer m.UnPatch()

	var buf bytes.Buffer
	o := newTestScrapeOpts(&buf, tmpDir)
	o.Limit = 1
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			{Path: "影音/AV/test/A-001.mp4", Name: "A-001.mp4", Parent: "影音/AV/test", IsVideo: true},
			{Path: "影音/AV/test/A-002.mp4", Name: "A-002.mp4", Parent: "影音/AV/test", IsVideo: true},
			{Path: "影音/AV/test/A-003.mp4", Name: "A-003.mp4", Parent: "影音/AV/test", IsVideo: true},
		}, nil
	}

	if err := scrapeRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected Scrape called once with limit=1, got %d", callCount)
	}
}

func TestScrapeRunMovieType(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	m := mockey.Mock(scraper.Scrape).To(
		func(mediaType, _, _, _ string, _ scraper.ScrapeOpts, _ *slog.Logger) *scraper.ScrapeResult {
			if mediaType != "movie" {
				t.Errorf("expected mediaType=movie, got %s", mediaType)
			}
			return &scraper.ScrapeResult{
				Status: "ok",
				Match:  "Inception",
				IDs:    map[string]string{"tmdb": "27205"},
				Meta:   &scraper.Metadata{Title: "Inception", Year: 2010},
			}
		},
	).Build()
	defer m.UnPatch()

	var buf bytes.Buffer
	o := newTestScrapeOpts(&buf, tmpDir)
	o.Category = "电影"
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			{Path: "影音/电影/test/Inception.2010.mp4", Name: "Inception.2010.mp4", Parent: "影音/电影/test", IsVideo: true},
		}, nil
	}

	if err := scrapeRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "27205") {
		t.Errorf("expected tmdb ID in output, got: %s", out)
	}

	// Verify file_map was written.
	fmPath := filepath.Join(tmpDir, "media115", "scrape", "file_map", "Inception.2010.json")
	data, err := os.ReadFile(fmPath)
	if err != nil {
		t.Fatalf("expected file_map cache: %v", err)
	}
	var fm map[string]any
	if err := json.Unmarshal(data, &fm); err != nil {
		t.Fatalf("invalid file_map JSON: %v", err)
	}
	if fm["type"] != "movie" {
		t.Errorf("expected type=movie, got %v", fm["type"])
	}
}

func TestScrapeRunTVType(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	m := mockey.Mock(scraper.Scrape).To(
		func(mediaType, _, _, _ string, _ scraper.ScrapeOpts, _ *slog.Logger) *scraper.ScrapeResult {
			return &scraper.ScrapeResult{
				Status: "ok",
				Match:  "Breaking Bad",
				IDs:    map[string]string{"tmdb": "1396"},
				Meta:   &scraper.Metadata{Title: "Pilot", ShowTitle: "Breaking Bad", Year: 2008, Season: 1, Episode: 1},
			}
		},
	).Build()
	defer m.UnPatch()

	var buf bytes.Buffer
	o := newTestScrapeOpts(&buf, tmpDir)
	o.Category = "剧目"
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			{Path: "影音/剧目/test/Breaking.Bad.S01E01.mp4", Name: "Breaking.Bad.S01E01.mp4", Parent: "影音/剧目/test", IsVideo: true},
		}, nil
	}

	if err := scrapeRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Breaking Bad") {
		t.Error("expected show title in output")
	}

	// Verify file_map was written as tv type.
	fmPath := filepath.Join(tmpDir, "media115", "scrape", "file_map", "Breaking.Bad.S01E01.json")
	data, err := os.ReadFile(fmPath)
	if err != nil {
		t.Fatalf("expected file_map cache: %v", err)
	}
	var fm map[string]any
	if err := json.Unmarshal(data, &fm); err != nil {
		t.Fatalf("invalid file_map JSON: %v", err)
	}
	if fm["type"] != "tv" {
		t.Errorf("expected type=tv, got %v", fm["type"])
	}
}

func TestScrapeRunMkdirOutputError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	// Create a file where the output dir would be — so MkdirAll fails.
	outPath := filepath.Join(tmpDir, "media115", "scrape_output", "AV")
	_ = os.MkdirAll(filepath.Dir(outPath), 0o755)
	_ = os.WriteFile(outPath, []byte("block"), 0o644)

	var buf bytes.Buffer
	o := newTestScrapeOpts(&buf, tmpDir)
	o.CacheDir = func() string { return filepath.Join(tmpDir, "media115") }
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			{Path: "影音/AV/test/ABP-040.mp4", Name: "ABP-040.mp4", Parent: "影音/AV/test", IsVideo: true},
		}, nil
	}

	err := scrapeRun(o)
	if err == nil || !strings.Contains(err.Error(), "创建输出目录失败") {
		t.Fatalf("expected mkdir error, got: %v", err)
	}
}

// ── doctor newDoctorCmd RunE coverage ─────────────────────────────────────────

func TestDoctorRunCacheStatusError(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.Default()
	cfg.Auth.Cookies = "test-cookies"

	o := newTestDoctorOpts(&buf, t.TempDir())
	o.LoadConfig = func() (*config.Config, error) { return cfg, nil }
	o.GetClient = func() (doctorClient, error) {
		return &mockDoctorClient{
			loginOK:  true,
			statsErr: os.ErrPermission,
		}, nil
	}

	if err := doctorRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "缓存状态:      ✗ 获取失败") {
		t.Error("expected cache status error")
	}
}

func TestDoctorRunRateLimitCooldown(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.Default()
	cfg.Auth.Cookies = "test-cookies"

	o := newTestDoctorOpts(&buf, t.TempDir())
	o.LoadConfig = func() (*config.Config, error) { return cfg, nil }
	o.GetClient = func() (doctorClient, error) {
		return &mockDoctorClient{
			loginOK: true,
			stats: &cloud115.CacheStats{
				RateLimit: map[string]cloud115.RateLimitState{
					"list": {CooldownUntil: float64(time.Now().Unix() + 60)},
				},
			},
		}, nil
	}

	if err := doctorRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "限流 (list)") {
		t.Error("expected rate limit cooldown message")
	}
}
