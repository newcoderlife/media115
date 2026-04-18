package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/newcoderlife/media115/internal/cloud115"
)

func newTestScanOpts(buf *bytes.Buffer) *scanOpts {
	return &scanOpts{
		Out:      buf,
		Category: "",
		ShowAll:  false,
		GetTreeEntries: func(string) ([]cloud115.TreeEntry, error) {
			return nil, nil
		},
	}
}

func TestScanRunTreeError(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScanOpts(&buf)
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return nil, errors.New("db locked")
	}

	err := scanRun(o)
	if err == nil || !strings.Contains(err.Error(), "读取树缓存失败") {
		t.Fatalf("expected tree cache error, got: %v", err)
	}
}

func TestScanRunEmptyTree(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScanOpts(&buf)

	if err := scanRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "无树缓存") {
		t.Error("expected empty tree message")
	}
}

func TestScanRunVideosWithNFO(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScanOpts(&buf)
	o.ShowAll = true
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			{Path: "影音/AV/ABC-123/ABC-123.mp4", Name: "ABC-123.mp4", Parent: "影音/AV/ABC-123", IsVideo: true},
			{Path: "影音/AV/ABC-123/ABC-123.nfo", Name: "ABC-123.nfo", Parent: "影音/AV/ABC-123", IsNFO: true},
		}, nil
	}

	if err := scanRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "找到 1 个视频文件") {
		t.Error("expected 1 video found message")
	}
	if !strings.Contains(out, "skip") {
		t.Error("expected skip action for video with NFO")
	}
	if !strings.Contains(out, "1 个跳过") {
		t.Error("expected 1 skip in summary")
	}
}

func TestScanRunVideosWithoutNFO(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScanOpts(&buf)
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			{Path: "影音/AV/ABC-123/ABC-123.mp4", Name: "ABC-123.mp4", Parent: "影音/AV/ABC-123", IsVideo: true},
		}, nil
	}

	if err := scanRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "1 个待刮削") {
		t.Error("expected 1 scrape in summary")
	}
}

func TestScanRunCategoryFilter(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScanOpts(&buf)
	o.Category = "电影"
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			{Path: "影音/AV/ABC-123/ABC-123.mp4", Name: "ABC-123.mp4", Parent: "影音/AV/ABC-123", IsVideo: true},
			{Path: "影音/电影/Inception (2010)/Inception.mkv", Name: "Inception.mkv", Parent: "影音/电影/Inception (2010)", IsVideo: true},
		}, nil
	}

	if err := scanRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "找到 1 个视频文件") {
		t.Error("expected 1 video (only 电影 category)")
	}
}

func TestScanRunShowAllFalseHidesSkip(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScanOpts(&buf)
	o.ShowAll = false
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			{Path: "影音/AV/ABC-123/ABC-123.mp4", Name: "ABC-123.mp4", Parent: "影音/AV/ABC-123", IsVideo: true},
			{Path: "影音/AV/ABC-123/ABC-123.nfo", Name: "ABC-123.nfo", Parent: "影音/AV/ABC-123", IsNFO: true},
		}, nil
	}

	if err := scanRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	// With showAll=false, skip rows should not appear in table
	if strings.Contains(out, "skip") {
		t.Error("expected skip row to be hidden when showAll=false")
	}
	if !strings.Contains(out, "1 个跳过") {
		t.Error("summary should still show skip count")
	}
}

func TestScanRunAnomalyNaming(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScanOpts(&buf)
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			// AV video in non-standard folder name
			{Path: "影音/AV/bad folder/ABC-123.mp4", Name: "ABC-123.mp4", Parent: "影音/AV/bad folder", IsVideo: true},
			{Path: "影音/AV/bad folder/ABC-123.nfo", Name: "ABC-123.nfo", Parent: "影音/AV/bad folder", IsNFO: true},
		}, nil
	}

	if err := scanRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Non-standard name") {
		t.Error("expected naming anomaly")
	}
}

func TestScanRunAnomalyMovieNaming(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScanOpts(&buf)
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			// Movie video in non-standard folder name (missing year)
			{Path: "影音/电影/BadName/movie.mkv", Name: "movie.mkv", Parent: "影音/电影/BadName", IsVideo: true},
			{Path: "影音/电影/BadName/movie.nfo", Name: "movie.nfo", Parent: "影音/电影/BadName", IsNFO: true},
		}, nil
	}

	if err := scanRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Non-standard name") {
		t.Error("expected naming anomaly for movie")
	}
}

func TestScanRunAnomalyResidual(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScanOpts(&buf)
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			// NFO without matching video
			{Path: "影音/AV/OLD-001/OLD-001.nfo", Name: "OLD-001.nfo", Parent: "影音/AV/OLD-001", IsNFO: true},
		}, nil
	}

	if err := scanRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Residual dir") {
		t.Error("expected residual anomaly")
	}
}

func TestScanRunAnomalyDupNFO(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScanOpts(&buf)
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			{Path: "影音/AV/ABC-123/ABC-123.mp4", Name: "ABC-123.mp4", Parent: "影音/AV/ABC-123", IsVideo: true},
			{Path: "影音/AV/ABC-123/ABC-123.nfo", Name: "ABC-123.nfo", Parent: "影音/AV/ABC-123", IsNFO: true},
			{Path: "影音/AV/ABC-123/ABC-123.nfo", Name: "ABC-123.nfo", Parent: "影音/AV/ABC-123", IsNFO: true},
		}, nil
	}

	if err := scanRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Duplicate NFO") {
		t.Error("expected duplicate NFO anomaly")
	}
}

func TestScanRunNoNFOIsScrape(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScanOpts(&buf)
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			{Path: "影音/AV/unknown/random_file.mp4", Name: "random_file.mp4", Parent: "影音/AV/unknown", IsVideo: true},
		}, nil
	}

	if err := scanRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "scrape") || !strings.Contains(out, "1 个待刮削") {
		t.Error("expected scrape action for file without NFO")
	}
}

func TestScanRunNoAnomalies(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScanOpts(&buf)
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			// Standard AV naming with NFO
			{Path: "影音/AV/ABC-123/ABC-123.mp4", Name: "ABC-123.mp4", Parent: "影音/AV/ABC-123", IsVideo: true},
			{Path: "影音/AV/ABC-123/ABC-123.nfo", Name: "ABC-123.nfo", Parent: "影音/AV/ABC-123", IsNFO: true},
		}, nil
	}

	if err := scanRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if strings.Contains(out, "异常") {
		t.Error("expected no anomalies")
	}
}

func TestScanRunStandardMovieNaming(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScanOpts(&buf)
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			{Path: "影音/电影/Inception (2010)/Inception.mkv", Name: "Inception.mkv", Parent: "影音/电影/Inception (2010)", IsVideo: true},
			{Path: "影音/电影/Inception (2010)/Inception.nfo", Name: "Inception.nfo", Parent: "影音/电影/Inception (2010)", IsNFO: true},
		}, nil
	}

	if err := scanRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if strings.Contains(out, "Non-standard name") {
		t.Error("standard movie naming should not trigger anomaly")
	}
}

func TestScanRunCategoryFilterNFO(t *testing.T) {
	var buf bytes.Buffer
	o := newTestScanOpts(&buf)
	o.Category = "AV"
	o.ShowAll = true
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			{Path: "AV/ABC-123/ABC-123.mp4", Name: "ABC-123.mp4", Parent: "AV/ABC-123", IsVideo: true},
			{Path: "AV/ABC-123/ABC-123.nfo", Name: "ABC-123.nfo", Parent: "AV/ABC-123", IsNFO: true},
			{Path: "AV/ABC-123/ABC-123.nfo", Name: "ABC-123.nfo", Parent: "AV/ABC-123", IsNFO: true},
			// This NFO is in 电影, should be filtered out by category=AV
			{Path: "电影/Movie (2020)/Movie.nfo", Name: "Movie.nfo", Parent: "电影/Movie (2020)", IsNFO: true},
			{Path: "电影/Movie (2020)/Movie.nfo", Name: "Movie.nfo", Parent: "电影/Movie (2020)", IsNFO: true},
			// Residual dir in 电影 — should also be filtered out
			{Path: "电影/OldMovie/OldMovie.nfo", Name: "OldMovie.nfo", Parent: "电影/OldMovie", IsNFO: true},
		}, nil
	}

	if err := scanRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	// Should detect dup NFO in AV
	if !strings.Contains(out, "Duplicate NFO") {
		t.Error("expected duplicate NFO anomaly in AV category")
	}
	// Should NOT report 电影 anomalies because category filter is AV
	if strings.Contains(out, "OldMovie") {
		t.Error("should not show 电影 anomalies when category=AV")
	}
}
