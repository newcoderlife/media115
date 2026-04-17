package main

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bytedance/mockey"

	"github.com/newcoderlife/media115/internal/cloud115"
	"github.com/newcoderlife/media115/internal/organizer"
)

func TestOrganizeRunExecuteSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	writeFileMap(tmpDir, "Good.Movie.2022", map[string]any{
		"type": "movie", "title": "Good Movie", "year": 2022, "tmdb_id": "12345",
	})

	client, err := cloud115.NewClient("test=cookie", cloud115.WithCacheDir(tmpDir))
	if err != nil {
		t.Fatalf("failed to create test client: %v", err)
	}
	defer client.Close()

	mExec := mockey.Mock(organizer.Execute).To(
		func(_ *cloud115.Client, ops []organizer.Op, _ string, _ *slog.Logger) ([]organizer.OpResult, error) {
			var results []organizer.OpResult
			for _, op := range ops {
				results = append(results, organizer.OpResult{Op: op, Status: "ok"})
			}
			return results, nil
		},
	).Build()
	defer mExec.UnPatch()

	mVerify := mockey.Mock(organizer.Verify).Return(1, 0).Build()
	defer mVerify.UnPatch()

	var buf bytes.Buffer
	o := &organizeOpts{
		Out:      &buf,
		Category: "电影",
		Execute:  true,
		GetClient: func() (*cloud115.Client, error) {
			return client, nil
		},
		GetTreeEntries: func(string) ([]cloud115.TreeEntry, error) {
			return []cloud115.TreeEntry{
				{Path: "影音/电影/wrong/Good.Movie.2022.mp4", Name: "Good.Movie.2022.mp4", Parent: "影音/电影/wrong", IsVideo: true},
			}, nil
		},
		CacheDir: func() string { return tmpDir },
		Now:      time.Now,
	}

	if err := organizeRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "执行 1 个重命名") {
		t.Errorf("expected execute message, got: %s", out)
	}
	if !strings.Contains(out, "1 个成功, 0 个失败") {
		t.Error("expected success count")
	}
	if !strings.Contains(out, "验证通过") {
		t.Error("expected verification success")
	}
	if !strings.Contains(out, "日志保存至") {
		t.Error("expected log save message")
	}

	// Verify log file was created.
	logDir := filepath.Join(tmpDir, "history")
	entries, _ := os.ReadDir(logDir)
	if len(entries) == 0 {
		t.Error("expected log file in history dir")
	}
}

func TestOrganizeRunExecutePartialFailure(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	writeFileMap(tmpDir, "Good.Movie.2022", map[string]any{
		"type": "movie", "title": "Good Movie", "year": 2022, "tmdb_id": "12345",
	})
	writeFileMap(tmpDir, "Bad.Movie.2021", map[string]any{
		"type": "movie", "title": "Bad Movie", "year": 2021, "tmdb_id": "99999",
	})

	client, err := cloud115.NewClient("test=cookie", cloud115.WithCacheDir(tmpDir))
	if err != nil {
		t.Fatalf("failed to create test client: %v", err)
	}
	defer client.Close()

	mExec := mockey.Mock(organizer.Execute).To(
		func(_ *cloud115.Client, ops []organizer.Op, _ string, _ *slog.Logger) ([]organizer.OpResult, error) {
			var results []organizer.OpResult
			for i, op := range ops {
				if i == 0 {
					results = append(results, organizer.OpResult{Op: op, Status: "ok"})
				} else {
					results = append(results, organizer.OpResult{Op: op, Status: "error", Error: "file not found"})
				}
			}
			return results, nil
		},
	).Build()
	defer mExec.UnPatch()

	mVerify := mockey.Mock(organizer.Verify).Return(1, 1).Build()
	defer mVerify.UnPatch()

	var buf bytes.Buffer
	o := &organizeOpts{
		Out:      &buf,
		Category: "电影",
		Execute:  true,
		GetClient: func() (*cloud115.Client, error) {
			return client, nil
		},
		GetTreeEntries: func(string) ([]cloud115.TreeEntry, error) {
			return []cloud115.TreeEntry{
				{Path: "影音/电影/a/Good.Movie.2022.mp4", Name: "Good.Movie.2022.mp4", Parent: "影音/电影/a", IsVideo: true},
				{Path: "影音/电影/b/Bad.Movie.2021.mp4", Name: "Bad.Movie.2021.mp4", Parent: "影音/电影/b", IsVideo: true},
			}, nil
		},
		CacheDir: func() string { return tmpDir },
		Now:      time.Now,
	}

	if err := organizeRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "1 个成功, 1 个失败") {
		t.Errorf("expected 1 ok + 1 fail, got: %s", out)
	}
	if !strings.Contains(out, "验证未通过") {
		t.Error("expected verification failures")
	}
}

func TestOrganizeRunExecuteError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	writeFileMap(tmpDir, "Good.Movie.2022", map[string]any{
		"type": "movie", "title": "Good Movie", "year": 2022, "tmdb_id": "12345",
	})

	client, err := cloud115.NewClient("test=cookie", cloud115.WithCacheDir(tmpDir))
	if err != nil {
		t.Fatalf("failed to create test client: %v", err)
	}
	defer client.Close()

	mExec := mockey.Mock(organizer.Execute).Return(nil, os.ErrPermission).Build()
	defer mExec.UnPatch()

	var buf bytes.Buffer
	o := &organizeOpts{
		Out:      &buf,
		Category: "电影",
		Execute:  true,
		GetClient: func() (*cloud115.Client, error) {
			return client, nil
		},
		GetTreeEntries: func(string) ([]cloud115.TreeEntry, error) {
			return []cloud115.TreeEntry{
				{Path: "影音/电影/wrong/Good.Movie.2022.mp4", Name: "Good.Movie.2022.mp4", Parent: "影音/电影/wrong", IsVideo: true},
			}, nil
		},
		CacheDir: func() string { return tmpDir },
		Now:      time.Now,
	}

	gErr := organizeRun(o)
	if gErr == nil || !strings.Contains(gErr.Error(), "执行失败") {
		t.Fatalf("expected execute error, got: %v", gErr)
	}
}
