package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/newcoderlife/media115/internal/cloud115"
	"github.com/newcoderlife/media115/internal/config"
)

func newTestScrapeOpts(buf *bytes.Buffer, tmpDir string) *scrapeOpts {
	return &scrapeOpts{
		Out:      buf,
		Category: "AV",
		Force:    false,
		Limit:    0,
		GetConfig: func() (*config.Config, error) {
			return config.Default(), nil
		},
		RegisterProviders: func(*config.Config) {},
		GetTreeEntries: func(string) ([]cloud115.TreeEntry, error) {
			return nil, nil
		},
		CacheDir: func() string { return tmpDir },
		Now:      time.Now,
	}
}

func TestScrapeRunConfigError(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScrapeOpts(&buf, t.TempDir())
	o.GetConfig = func() (*config.Config, error) {
		return nil, errors.New("config not found")
	}

	err := scrapeRun(o)
	if err == nil || !strings.Contains(err.Error(), "load config") {
		t.Fatalf("expected config error, got: %v", err)
	}
}

func TestScrapeRunTreeError(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScrapeOpts(&buf, t.TempDir())
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return nil, errors.New("db locked")
	}

	err := scrapeRun(o)
	if err == nil || !strings.Contains(err.Error(), "读取树缓存失败") {
		t.Fatalf("expected tree cache error, got: %v", err)
	}
}

func TestScrapeRunEmptyTree(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScrapeOpts(&buf, t.TempDir())

	if err := scrapeRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "无树缓存") {
		t.Error("expected empty tree message")
	}
}

func TestScrapeRunNoVideos(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScrapeOpts(&buf, t.TempDir())
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			// NFO file, not a video
			{Path: "影音/AV/test/info.nfo", Name: "info.nfo", Parent: "影音/AV/test", IsNFO: true},
		}, nil
	}

	if err := scrapeRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "没有需要刮削的文件") {
		t.Error("expected no videos message")
	}
}

func TestScrapeRunSkipWithNFO(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScrapeOpts(&buf, t.TempDir())
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			{Path: "影音/AV/test/ABP-040.mp4", Name: "ABP-040.mp4", Parent: "影音/AV/test", IsVideo: true},
			{Path: "影音/AV/test/ABP-040.nfo", Name: "ABP-040.nfo", Parent: "影音/AV/test", IsNFO: true},
		}, nil
	}

	if err := scrapeRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "没有需要刮削的文件") {
		t.Error("expected all videos skipped due to existing NFO")
	}
}

func TestScrapeRunForceIgnoresNFO(t *testing.T) {
	var buf bytes.Buffer
	tmpDir := t.TempDir()
	o := newTestScrapeOpts(&buf, tmpDir)
	o.Force = true
	o.Limit = 0
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			{Path: "影音/AV/test/ABP-040.mp4", Name: "ABP-040.mp4", Parent: "影音/AV/test", IsVideo: true},
			{Path: "影音/AV/test/ABP-040.nfo", Name: "ABP-040.nfo", Parent: "影音/AV/test", IsNFO: true},
		}, nil
	}

	// With force=true, the video should not be skipped even with NFO present.
	// It will reach the scrape loop, which calls external APIs.
	// We only verify it doesn't print "没有需要刮削的文件".
	_ = scrapeRun(o)
	if strings.Contains(buf.String(), "没有需要刮削的文件") {
		t.Error("force should not skip videos with existing NFO")
	}
}

func TestScrapeRunLimit(t *testing.T) {
	var buf bytes.Buffer
	tmpDir := t.TempDir()
	o := newTestScrapeOpts(&buf, tmpDir)
	o.Limit = 1
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			{Path: "影音/AV/test/ABP-040.mp4", Name: "ABP-040.mp4", Parent: "影音/AV/test", IsVideo: true},
			{Path: "影音/AV/test/ABP-041.mp4", Name: "ABP-041.mp4", Parent: "影音/AV/test", IsVideo: true},
		}, nil
	}

	// With limit=1, only 1 file processed. We just verify it doesn't say "没有需要刮削的文件".
	_ = scrapeRun(o)
	out := buf.String()
	if strings.Contains(out, "没有需要刮削的文件") {
		t.Error("should have videos to scrape")
	}
}

func TestScrapeRunDifferentCategory(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScrapeOpts(&buf, t.TempDir())
	o.Category = "电影"
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			// Video only in AV category, not 电影
			{Path: "影音/AV/test/ABP-040.mp4", Name: "ABP-040.mp4", Parent: "影音/AV/test", IsVideo: true},
		}, nil
	}

	if err := scrapeRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "没有需要刮削的文件") {
		t.Error("expected no videos for mismatched category")
	}
}
