package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/newcoderlife/media115/internal/cloud115"
)

func newTestOrganizeOpts(buf *bytes.Buffer, tmpDir string) *organizeOpts {
	return &organizeOpts{
		Out:      buf,
		Category: "AV",
		Execute:  false,
		GetClient: func() (*cloud115.Client, error) {
			return nil, errors.New("not available in test")
		},
		GetTreeEntries: func(string) ([]cloud115.TreeEntry, error) {
			return nil, nil
		},
		CacheDir: func() string { return tmpDir },
		Now:      time.Now,
	}
}

func TestOrganizeRunTreeError(t *testing.T) {
	var buf bytes.Buffer
	o := newTestOrganizeOpts(&buf, t.TempDir())
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return nil, errors.New("db locked")
	}

	err := organizeRun(o)
	if err == nil || !strings.Contains(err.Error(), "读取树缓存失败") {
		t.Fatalf("expected tree cache error, got: %v", err)
	}
}

func TestOrganizeRunEmptyTree(t *testing.T) {
	var buf bytes.Buffer
	o := newTestOrganizeOpts(&buf, t.TempDir())
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return nil, nil
	}

	if err := organizeRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "无树缓存") {
		t.Error("expected empty tree message")
	}
}

func TestOrganizeRunNoRenames(t *testing.T) {
	var buf bytes.Buffer
	o := newTestOrganizeOpts(&buf, t.TempDir())
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			// Video with no file_map cache → skip with "no scrape result cached"
			{Path: "影音/AV/test/video.mp4", Name: "video.mp4", Parent: "影音/AV/test", IsVideo: true},
		}, nil
	}

	if err := organizeRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "0 个重命名") {
		t.Errorf("expected 0 renames, got: %s", out)
	}
	if !strings.Contains(out, "没有需要重命名的文件") {
		t.Error("expected no rename message")
	}
}

func TestOrganizeRunNoVideosInCategory(t *testing.T) {
	var buf bytes.Buffer
	o := newTestOrganizeOpts(&buf, t.TempDir())
	o.Category = "电影"
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			// Video in a different category
			{Path: "影音/AV/test/video.mp4", Name: "video.mp4", Parent: "影音/AV/test", IsVideo: true},
		}, nil
	}

	if err := organizeRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "'电影' 分类整理计划: 0 个重命名, 0 个不变") {
		t.Errorf("expected zero plan for 电影, got: %s", out)
	}
}

func TestOrganizeRunDryRunDefault(t *testing.T) {
	var buf bytes.Buffer
	o := newTestOrganizeOpts(&buf, t.TempDir())
	// Verify execute is false by default in the opts we created
	if o.Execute {
		t.Fatal("expected Execute to default to false")
	}
}
