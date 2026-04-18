package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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

// writeFileMap writes a file_map cache JSON file under the tmpDir.
func writeFileMap(tmpDir, stem string, data map[string]any) {
	dir := filepath.Join(tmpDir, "media115", "scrape", "file_map")
	_ = os.MkdirAll(dir, 0o755)
	b, _ := json.Marshal(data)
	_ = os.WriteFile(filepath.Join(dir, stem+".json"), b, 0o644)
}

func TestOrganizeRunWithRenamesDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	writeFileMap(tmpDir, "Good.Movie.2022", map[string]any{
		"type": "movie", "title": "Good Movie", "year": 2022, "tmdb_id": "12345",
	})

	var buf bytes.Buffer
	o := newTestOrganizeOpts(&buf, tmpDir)
	o.Category = "电影"
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			{Path: "影音/电影/wrong/Good.Movie.2022.mp4", Name: "Good.Movie.2022.mp4", Parent: "影音/电影/wrong", IsVideo: true},
		}, nil
	}

	if err := organizeRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "1 个重命名") {
		t.Errorf("expected 1 rename, got: %s", out)
	}
	if !strings.Contains(out, "Good Movie (2022)") {
		t.Error("expected new name in table")
	}
	if !strings.Contains(out, "tmdb:12345") {
		t.Error("expected source ID in table")
	}
	if !strings.Contains(out, "演练完成") {
		t.Error("expected dry-run message")
	}
}

func TestOrganizeRunWithConflicts(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	// Two videos that resolve to the same movie target.
	for _, stem := range []string{"Good.Movie.2022", "Good.Movie.2022.REPACK"} {
		writeFileMap(tmpDir, stem, map[string]any{
			"type": "movie", "title": "Good Movie", "year": 2022, "tmdb_id": "12345",
		})
	}

	var buf bytes.Buffer
	o := newTestOrganizeOpts(&buf, tmpDir)
	o.Category = "电影"
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			{Path: "影音/电影/a/Good.Movie.2022.mp4", Name: "Good.Movie.2022.mp4", Parent: "影音/电影/a", IsVideo: true},
			{Path: "影音/电影/b/Good.Movie.2022.REPACK.mp4", Name: "Good.Movie.2022.REPACK.mp4", Parent: "影音/电影/b", IsVideo: true},
		}, nil
	}

	if err := organizeRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "目标冲突") {
		t.Errorf("expected conflict warning, got: %s", out)
	}
	if !strings.Contains(out, "请在执行前解决冲突") {
		t.Error("expected conflict resolution hint")
	}
}

func TestOrganizeRunExecuteClientError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	writeFileMap(tmpDir, "Good.Movie.2022", map[string]any{
		"type": "movie", "title": "Good Movie", "year": 2022, "tmdb_id": "12345",
	})

	var buf bytes.Buffer
	o := newTestOrganizeOpts(&buf, tmpDir)
	o.Category = "电影"
	o.Execute = true
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			{Path: "影音/电影/wrong/Good.Movie.2022.mp4", Name: "Good.Movie.2022.mp4", Parent: "影音/电影/wrong", IsVideo: true},
		}, nil
	}

	err := organizeRun(o)
	if err == nil || !strings.Contains(err.Error(), "获取客户端失败") {
		t.Fatalf("expected client error, got: %v", err)
	}
}

func TestOrganizeRunNoRenamesExecuteClientFail(t *testing.T) {
	var buf bytes.Buffer
	o := newTestOrganizeOpts(&buf, t.TempDir())
	o.Execute = true
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			{Path: "影音/AV/test/video.mp4", Name: "video.mp4", Parent: "影音/AV/test", IsVideo: true},
		}, nil
	}

	if err := organizeRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "没有需要重命名的文件") {
		t.Error("expected no rename message")
	}
}

func TestOrganizeRunWithAVRenames(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	writeFileMap(tmpDir, "abp040", map[string]any{
		"type": "av", "number": "ABP-040", "title": "Some Title",
	})

	var buf bytes.Buffer
	o := newTestOrganizeOpts(&buf, tmpDir)
	o.Category = "AV"
	o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) {
		return []cloud115.TreeEntry{
			{Path: "影音/AV/misc/abp040.mp4", Name: "abp040.mp4", Parent: "影音/AV/misc", IsVideo: true},
		}, nil
	}

	if err := organizeRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "1 个重命名") {
		t.Errorf("expected 1 rename, got: %s", out)
	}
	if !strings.Contains(out, "演练完成") {
		t.Error("expected dry-run message")
	}
}

func TestOrganizeRunFileFilter(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	writeFileMap(tmpDir, "Good.Movie.2022", map[string]any{
		"type": "movie", "title": "Good Movie", "year": 2022, "tmdb_id": "111",
	})
	writeFileMap(tmpDir, "Bad.Movie.2023", map[string]any{
		"type": "movie", "title": "Bad Movie", "year": 2023, "tmdb_id": "222",
	})

	entries := []cloud115.TreeEntry{
		{Path: "影音/电影/a/Good.Movie.2022.mp4", Name: "Good.Movie.2022.mp4", Parent: "影音/电影/a", IsVideo: true},
		{Path: "影音/电影/b/Bad.Movie.2023.mp4", Name: "Bad.Movie.2023.mp4", Parent: "影音/电影/b", IsVideo: true},
	}

	t.Run("substring match", func(t *testing.T) {
		var buf bytes.Buffer
		o := newTestOrganizeOpts(&buf, tmpDir)
		o.Category = "电影"
		o.File = "Good"
		o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) { return entries, nil }

		if err := organizeRun(o); err != nil {
			t.Fatal(err)
		}
		out := buf.String()
		if !strings.Contains(out, "1 个重命名") {
			t.Errorf("expected 1 rename, got: %s", out)
		}
		if strings.Contains(out, "Bad Movie") {
			t.Error("Bad Movie should be filtered out")
		}
	})

	t.Run("glob match", func(t *testing.T) {
		var buf bytes.Buffer
		o := newTestOrganizeOpts(&buf, tmpDir)
		o.Category = "电影"
		o.File = "Bad.Movie.*"
		o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) { return entries, nil }

		if err := organizeRun(o); err != nil {
			t.Fatal(err)
		}
		out := buf.String()
		if !strings.Contains(out, "1 个重命名") {
			t.Errorf("expected 1 rename, got: %s", out)
		}
		if strings.Contains(out, "Good Movie") {
			t.Error("Good Movie should be filtered out")
		}
	})

	t.Run("no match", func(t *testing.T) {
		var buf bytes.Buffer
		o := newTestOrganizeOpts(&buf, tmpDir)
		o.Category = "电影"
		o.File = "NonExistent"
		o.GetTreeEntries = func(string) ([]cloud115.TreeEntry, error) { return entries, nil }

		if err := organizeRun(o); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "0 个重命名, 0 个不变") {
			t.Error("expected empty plan")
		}
	})
}
