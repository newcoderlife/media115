package main

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/newcoderlife/media115/internal/cloud115"
	"github.com/newcoderlife/media115/internal/config"
	"github.com/newcoderlife/media115/internal/scraper"
)

func newTestScrapeFixOpts(buf *bytes.Buffer, tmpDir string) *scrapeFixOpts {
	return &scrapeFixOpts{
		Out:               buf,
		Filename:          "ABC-123.mp4",
		GetConfig:         func() (*config.Config, error) { return config.Default(), nil },
		RegisterProviders: func(*config.Config) {},
		GetTreeEntries: func(string) ([]cloud115.TreeEntry, error) {
			return nil, nil
		},
		CacheDir: func() string { return tmpDir },
		Scrape: func(string, string, string, string, scraper.ScrapeOpts, *slog.Logger) *scraper.ScrapeResult {
			return nil
		},
		SaveFileMap: func(string, string, string, *scraper.ScrapeResult) {},
		TmdbDetail:  func(string, string) (*scraper.Metadata, error) { return nil, errors.New("not implemented") },
	}
}

func TestScrapeFixRunConfigError(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScrapeFixOpts(&buf, t.TempDir())
	o.GetConfig = func() (*config.Config, error) {
		return nil, errors.New("config not found")
	}

	err := scrapeFixRun(o)
	if err == nil || !strings.Contains(err.Error(), "load config") {
		t.Fatalf("expected config error, got: %v", err)
	}
}

func TestScrapeFixRunAVAutoDetect(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScrapeFixOpts(&buf, t.TempDir())
	o.Filename = "ABC-123.mp4"
	o.Scrape = func(mediaType, query, filename, outDir string, opts scraper.ScrapeOpts, logger *slog.Logger) *scraper.ScrapeResult {
		return &scraper.ScrapeResult{
			Status: "ok",
			Match:  "ABC-123",
			IDs:    map[string]string{"number": "ABC-123"},
		}
	}

	if err := scrapeFixRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "分类: AV") {
		t.Error("expected AV category auto-detect")
	}
	if !strings.Contains(out, "✓ ABC-123") {
		t.Error("expected success output")
	}
}

func TestScrapeFixRunAVWithNumber(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScrapeFixOpts(&buf, t.TempDir())
	o.Filename = "random_file.mp4"
	o.Number = "XYZ-456"
	o.Scrape = func(mediaType, query, filename, outDir string, opts scraper.ScrapeOpts, logger *slog.Logger) *scraper.ScrapeResult {
		if query != "XYZ-456" {
			t.Errorf("expected query XYZ-456, got %s", query)
		}
		return &scraper.ScrapeResult{
			Status: "ok",
			Match:  "XYZ-456",
			IDs:    map[string]string{"number": "XYZ-456"},
		}
	}

	if err := scrapeFixRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "分类: AV") {
		t.Error("expected AV category when --number is set")
	}
}

func TestScrapeFixRunMovieAutoDetect(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScrapeFixOpts(&buf, t.TempDir())
	o.Filename = "Inception.2010.mkv"
	o.SearchQuery = "Inception"
	o.Scrape = func(mediaType, query, filename, outDir string, opts scraper.ScrapeOpts, logger *slog.Logger) *scraper.ScrapeResult {
		return &scraper.ScrapeResult{
			Status: "ok",
			Match:  "Inception",
			IDs:    map[string]string{"tmdb": "27205"},
		}
	}

	if err := scrapeFixRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "分类: 电影") {
		t.Error("expected 电影 category")
	}
	if !strings.Contains(out, "✓ Inception (27205)") {
		t.Error("expected success with tmdb ID")
	}
}

func TestScrapeFixRunMovieNoQuery(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScrapeFixOpts(&buf, t.TempDir())
	o.Filename = "file.mkv" // no recognizable title

	err := scrapeFixRun(o)
	if err == nil || !strings.Contains(err.Error(), "无法确定电影标题") {
		t.Fatalf("expected movie title error, got: %v", err)
	}
}

func TestScrapeFixRunTVNoSearchOrID(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScrapeFixOpts(&buf, t.TempDir())
	o.Filename = "Show.S01E01.mkv"
	o.Category = "剧目"

	err := scrapeFixRun(o)
	if err == nil || !strings.Contains(err.Error(), "TV 需要 --tmdb-id 或 --search") {
		t.Fatalf("expected TV requires tmdb-id or search error, got: %v", err)
	}
}

func TestScrapeFixRunTVWithSearch(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScrapeFixOpts(&buf, t.TempDir())
	o.Filename = "Show.S01E01.mkv"
	o.Category = "剧目"
	o.SearchQuery = "My Show"
	o.Season = 1
	o.Episode = 1
	o.Scrape = func(mediaType, query, filename, outDir string, opts scraper.ScrapeOpts, logger *slog.Logger) *scraper.ScrapeResult {
		if mediaType != "tv" {
			t.Errorf("expected tv mediaType, got %s", mediaType)
		}
		return &scraper.ScrapeResult{
			Status: "ok",
			Match:  "My Show",
			IDs:    map[string]string{"tmdb": "99999"},
		}
	}

	if err := scrapeFixRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "刮削 TV") {
		t.Error("expected TV scrape output")
	}
}

func TestScrapeFixRunScrapeReturnsNil(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScrapeFixOpts(&buf, t.TempDir())
	o.Filename = "ABC-123.mp4"
	// Scrape returns nil by default

	err := scrapeFixRun(o)
	if err == nil || !strings.Contains(err.Error(), "刮削返回 nil") {
		t.Fatalf("expected nil result error, got: %v", err)
	}
}

func TestScrapeFixRunScrapeNotFound(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScrapeFixOpts(&buf, t.TempDir())
	o.Filename = "ABC-123.mp4"
	o.Scrape = func(string, string, string, string, scraper.ScrapeOpts, *slog.Logger) *scraper.ScrapeResult {
		return &scraper.ScrapeResult{
			Status: "not_found",
			Error:  "no match",
		}
	}

	if err := scrapeFixRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "✗ 状态: not_found") {
		t.Error("expected not_found status output")
	}
	if !strings.Contains(out, "no match") {
		t.Error("expected error message in output")
	}
}

func TestScrapeFixRunExplicitCategory(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScrapeFixOpts(&buf, t.TempDir())
	o.Filename = "ABC-123.mp4"
	o.Category = "AV"
	o.Scrape = func(mediaType, query, filename, outDir string, opts scraper.ScrapeOpts, logger *slog.Logger) *scraper.ScrapeResult {
		return &scraper.ScrapeResult{
			Status: "ok",
			Match:  "ABC-123",
			IDs:    map[string]string{"number": "ABC-123"},
		}
	}

	if err := scrapeFixRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "分类: AV") {
		t.Error("expected AV category")
	}
}

func TestScrapeFixRunWithTreeEntry(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScrapeFixOpts(&buf, t.TempDir())
	o.Filename = "ABC-123.mp4"
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			{Path: "影音/AV/ABC-123/ABC-123.mp4", Name: "ABC-123.mp4", Parent: "影音/AV/ABC-123"},
		}, nil
	}
	o.Scrape = func(string, string, string, string, scraper.ScrapeOpts, *slog.Logger) *scraper.ScrapeResult {
		return &scraper.ScrapeResult{
			Status: "ok",
			Match:  "ABC-123",
			IDs:    map[string]string{"number": "ABC-123"},
		}
	}

	if err := scrapeFixRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	// When tree entry found, searchName uses parent path with slashes replaced
	if !strings.Contains(out, "影音_AV_ABC-123") {
		t.Error("expected parent-based output dir name")
	}
}

func TestScrapeFixRunMovieWithTmdbID(t *testing.T) {
	var buf bytes.Buffer
	tmpDir := t.TempDir()
	o := newTestScrapeFixOpts(&buf, tmpDir)
	o.Filename = "Inception.2010.mkv"
	o.Category = "电影"
	o.TmdbID = 27205
	o.TmdbDetail = func(token, query string) (*scraper.Metadata, error) {
		return &scraper.Metadata{
			Title:     "Inception",
			UniqueIDs: map[string]string{"tmdb": "27205"},
		}, nil
	}

	if err := scrapeFixRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "刮削 Movie (TMDB ID=27205)") {
		t.Error("expected movie TMDB ID output")
	}
	if !strings.Contains(out, "✓ Inception") {
		t.Error("expected success output")
	}
}

func TestScrapeFixRunTVWithTmdbID(t *testing.T) {
	var buf bytes.Buffer
	tmpDir := t.TempDir()
	o := newTestScrapeFixOpts(&buf, tmpDir)
	o.Filename = "Show.S02E03.mkv"
	o.Category = "剧目"
	o.TmdbID = 12345
	o.Season = 2
	o.Episode = 3
	o.TmdbDetail = func(token, query string) (*scraper.Metadata, error) {
		return &scraper.Metadata{
			Title:     "Test Show",
			UniqueIDs: map[string]string{"tmdb": "12345"},
		}, nil
	}

	if err := scrapeFixRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "刮削 TV (TMDB ID=12345): S02E03") {
		t.Error("expected TV TMDB ID output with season/episode")
	}
}

func TestScrapeFixRunTmdbDetailError(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScrapeFixOpts(&buf, t.TempDir())
	o.Filename = "Movie.mkv"
	o.Category = "电影"
	o.TmdbID = 99999
	o.TmdbDetail = func(token, query string) (*scraper.Metadata, error) {
		return nil, errors.New("TMDB API error")
	}

	err := scrapeFixRun(o)
	if err == nil || !strings.Contains(err.Error(), "TMDB API error") {
		t.Fatalf("expected TMDB error, got: %v", err)
	}
}

func TestScrapeFixRunSaveFileMapCalled(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScrapeFixOpts(&buf, t.TempDir())
	o.Filename = "ABC-123.mp4"
	saved := false
	o.SaveFileMap = func(filename, mediaType, query string, sr *scraper.ScrapeResult) {
		saved = true
		if mediaType != "av" {
			t.Errorf("expected av type, got %s", mediaType)
		}
	}
	o.Scrape = func(string, string, string, string, scraper.ScrapeOpts, *slog.Logger) *scraper.ScrapeResult {
		return &scraper.ScrapeResult{
			Status: "ok",
			Match:  "ABC-123",
			IDs:    map[string]string{"number": "ABC-123"},
		}
	}

	if err := scrapeFixRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !saved {
		t.Error("expected SaveFileMap to be called")
	}
}

func TestScrapeFixRunAVEmptyNumber(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScrapeFixOpts(&buf, t.TempDir())
	o.Filename = "file.mkv" // no recognizable AV number
	o.Category = "AV"

	err := scrapeFixRun(o)
	if err == nil || !strings.Contains(err.Error(), "无法确定AV番号") {
		t.Fatalf("expected AV number error, got: %v", err)
	}
}

func TestScrapeFixRunTVTmdbIDDefaultSeason(t *testing.T) {
	var buf bytes.Buffer
	tmpDir := t.TempDir()
	o := newTestScrapeFixOpts(&buf, tmpDir)
	o.Filename = "Show.mkv" // no S/E in filename
	o.Category = "剧目"
	o.TmdbID = 55555
	// Season=0, Episode=0 → should default to S01E00
	o.TmdbDetail = func(token, query string) (*scraper.Metadata, error) {
		if query != "tv:55555" {
			t.Errorf("expected query tv:55555, got %s", query)
		}
		return &scraper.Metadata{
			Title:     "Test Show",
			UniqueIDs: map[string]string{"tmdb": "55555"},
		}, nil
	}

	if err := scrapeFixRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "S01E00") {
		t.Error("expected default season 1, episode 0")
	}
	if !strings.Contains(out, "✓ Test Show") {
		t.Error("expected success output")
	}
}

func TestScrapeFixRunTVTmdbIDWithPoster(t *testing.T) {
	var buf bytes.Buffer
	tmpDir := t.TempDir()
	o := newTestScrapeFixOpts(&buf, tmpDir)
	o.Filename = "Show.S01E01.mkv"
	o.Category = "剧目"
	o.TmdbID = 11111
	o.Season = 1
	o.Episode = 1
	o.TmdbDetail = func(token, query string) (*scraper.Metadata, error) {
		return &scraper.Metadata{
			Title:     "Poster Show",
			PosterURL: "http://invalid.test/poster.jpg", // will fail silently
			UniqueIDs: map[string]string{"tmdb": "11111"},
		}, nil
	}

	if err := scrapeFixRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "✓ Poster Show") {
		t.Error("expected success output")
	}
}

func TestScrapeFixRunTVTmdbIDDetailError(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScrapeFixOpts(&buf, t.TempDir())
	o.Filename = "Show.S01E01.mkv"
	o.Category = "剧目"
	o.TmdbID = 99999
	o.TmdbDetail = func(string, string) (*scraper.Metadata, error) {
		return nil, errors.New("TV detail failed")
	}

	err := scrapeFixRun(o)
	if err == nil || !strings.Contains(err.Error(), "TV detail failed") {
		t.Fatalf("expected TV detail error, got: %v", err)
	}
}

func TestScrapeFixRunMovieTmdbIDWithPoster(t *testing.T) {
	var buf bytes.Buffer
	tmpDir := t.TempDir()
	o := newTestScrapeFixOpts(&buf, tmpDir)
	o.Filename = "Movie.mkv"
	o.Category = "电影"
	o.TmdbID = 22222
	o.TmdbDetail = func(token, query string) (*scraper.Metadata, error) {
		if query != "movie:22222" {
			t.Errorf("expected query movie:22222, got %s", query)
		}
		return &scraper.Metadata{
			Title:     "Test Movie",
			PosterURL: "http://invalid.test/poster.jpg",
			UniqueIDs: map[string]string{"tmdb": "22222"},
		}, nil
	}

	if err := scrapeFixRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "✓ Test Movie") {
		t.Error("expected success output")
	}
}

func TestScrapeFixRunMovieAutoTitle(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScrapeFixOpts(&buf, t.TempDir())
	o.Filename = "Inception.2010.mkv" // analysis.Title = "Inception"
	// No SearchQuery, no TmdbID → should use analysis.Title
	o.Scrape = func(mediaType, query, filename, outDir string, opts scraper.ScrapeOpts, logger *slog.Logger) *scraper.ScrapeResult {
		if query != "Inception" {
			t.Errorf("expected query Inception, got %s", query)
		}
		return &scraper.ScrapeResult{
			Status: "ok",
			Match:  "Inception",
			IDs:    map[string]string{"tmdb": "27205"},
		}
	}

	if err := scrapeFixRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestScrapeFixRunMkdirError(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScrapeFixOpts(&buf, "/dev/null/impossible")
	o.Filename = "ABC-123.mp4"

	err := scrapeFixRun(o)
	if err == nil || !strings.Contains(err.Error(), "创建输出目录失败") {
		t.Fatalf("expected mkdir error, got: %v", err)
	}
}

func TestScrapeFixRunTVAutoDetect(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScrapeFixOpts(&buf, t.TempDir())
	o.Filename = "Show.S01E05.mkv" // AnalyzeFilename → mediaType "tv"
	o.SearchQuery = "My Show"
	o.Scrape = func(mediaType, query, filename, outDir string, opts scraper.ScrapeOpts, logger *slog.Logger) *scraper.ScrapeResult {
		return &scraper.ScrapeResult{Status: "ok", Match: "My Show", IDs: map[string]string{"tmdb": "123"}}
	}

	if err := scrapeFixRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "分类: 剧目") {
		t.Error("expected 剧目 category auto-detect for TV filename")
	}
}

func TestScrapeFixRunTVSearchDefaultSeasonEpisode(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScrapeFixOpts(&buf, t.TempDir())
	o.Filename = "Show.mkv" // no S/E in filename → analysis.Season=0, analysis.Episode=0
	o.Category = "剧目"
	o.SearchQuery = "My Show"
	// Season=0, Episode=0 → defaults to S01E00
	o.Scrape = func(mediaType, query, filename, outDir string, opts scraper.ScrapeOpts, logger *slog.Logger) *scraper.ScrapeResult {
		if opts.Season != 1 {
			t.Errorf("expected season default 1, got %d", opts.Season)
		}
		return &scraper.ScrapeResult{Status: "ok", Match: "My Show", IDs: map[string]string{"tmdb": "123"}}
	}

	if err := scrapeFixRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "S01E00") {
		t.Error("expected default S01E00")
	}
}
