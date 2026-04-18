package main

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/newcoderlife/media115/internal/cloud115"
)

func TestTrimSlash(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no slash", "foo", "foo"},
		{"single trailing slash", "foo/", "foo"},
		{"multiple trailing slashes", "foo///", "foo"},
		{"root slash kept", "/", "/"},
		{"nested path trailing slash", "/a/b/c/", "/a/b/c"},
		{"nested path multiple trailing slashes", "/a/b/c///", "/a/b/c"},
		{"empty string", "", ""},
		{"single slash stays", "/", "/"},
		{"path no trailing slash unchanged", "/a/b", "/a/b"},
		{"only slashes single char", "/", "/"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := trimSlash(tc.input)
			if got != tc.want {
				t.Errorf("trimSlash(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestTrimSlashPreservesRoot(t *testing.T) {
	result := trimSlash("/")
	if result != "/" {
		t.Errorf("trimSlash('/') = %q, want '/'", result)
	}
}

func TestTrimSlashIdempotent(t *testing.T) {
	inputs := []string{"/foo/bar", "baz", "/", "hello/world"}
	for _, in := range inputs {
		first := trimSlash(in)
		second := trimSlash(first)
		if first != second {
			t.Errorf("trimSlash not idempotent for %q: first=%q second=%q", in, first, second)
		}
	}
}

// ── dedupRun tests ───────────────────────────────────────────────────────────

type mockDedupClient struct {
	listDirFunc         func(path string) ([]cloud115.Entry, error)
	listDirUncachedFunc func(path string) ([]cloud115.Entry, error)
	deleteByIDsFunc     func(fids []string, refreshDirs []string) error
	refreshPathsFunc    func(paths []string) error
}

func (m *mockDedupClient) ListDir(path string) ([]cloud115.Entry, error) {
	return m.listDirFunc(path)
}

func (m *mockDedupClient) ListDirUncached(path string) ([]cloud115.Entry, error) {
	return m.listDirUncachedFunc(path)
}

func (m *mockDedupClient) DeleteByIDs(fids []string, refreshDirs []string) error {
	return m.deleteByIDsFunc(fids, refreshDirs)
}

func (m *mockDedupClient) RefreshPaths(paths []string) error {
	return m.refreshPathsFunc(paths)
}
func (m *mockDedupClient) Close() error { return nil }

func TestDedupRunClientError(t *testing.T) {
	var buf bytes.Buffer
	o := &dedupOpts{
		Out:       &buf,
		Path:      "/test",
		GetClient: func() (dedupClient, error) { return nil, errors.New("no auth") },
	}

	err := dedupRun(o)
	if err == nil || !strings.Contains(err.Error(), "no auth") {
		t.Fatalf("expected auth error, got: %v", err)
	}
}

func TestDedupRunListDirError(t *testing.T) {
	var buf bytes.Buffer
	o := &dedupOpts{
		Out:  &buf,
		Path: "/test",
		GetClient: func() (dedupClient, error) {
			return &mockDedupClient{
				listDirFunc: func(string) ([]cloud115.Entry, error) {
					return nil, errors.New("list failed")
				},
			}, nil
		},
	}

	err := dedupRun(o)
	if err == nil || !strings.Contains(err.Error(), "列出目录失败") {
		t.Fatalf("expected list dir error, got: %v", err)
	}
}

func TestDedupRunNoSubdirs(t *testing.T) {
	var buf bytes.Buffer
	o := &dedupOpts{
		Out:  &buf,
		Path: "/test",
		GetClient: func() (dedupClient, error) {
			return &mockDedupClient{
				listDirFunc: func(string) ([]cloud115.Entry, error) {
					return []cloud115.Entry{
						{Name: "file.txt", Type: "file"},
					}, nil
				},
			}, nil
		},
	}

	if err := dedupRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "没有子目录") {
		t.Error("expected no subdirs message")
	}
}

func TestDedupRunNoDuplicates(t *testing.T) {
	var buf bytes.Buffer
	o := &dedupOpts{
		Out:  &buf,
		Path: "/test",
		GetClient: func() (dedupClient, error) {
			return &mockDedupClient{
				listDirFunc: func(string) ([]cloud115.Entry, error) {
					return []cloud115.Entry{
						{Name: "subdir", Type: "dir"},
					}, nil
				},
				listDirUncachedFunc: func(string) ([]cloud115.Entry, error) {
					return []cloud115.Entry{
						{Name: "a.mp4", Type: "file", FID: "1"},
						{Name: "b.mp4", Type: "file", FID: "2"},
					}, nil
				},
			}, nil
		},
	}

	if err := dedupRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "共 0 个重复文件") {
		t.Error("expected 0 duplicates")
	}
}

func TestDedupRunDryRun(t *testing.T) {
	var buf bytes.Buffer
	o := &dedupOpts{
		Out:     &buf,
		Path:    "/test",
		Execute: false,
		GetClient: func() (dedupClient, error) {
			return &mockDedupClient{
				listDirFunc: func(string) ([]cloud115.Entry, error) {
					return []cloud115.Entry{
						{Name: "subdir", Type: "dir"},
					}, nil
				},
				listDirUncachedFunc: func(string) ([]cloud115.Entry, error) {
					return []cloud115.Entry{
						{Name: "dup.mp4", Type: "file", FID: "1"},
						{Name: "dup.mp4", Type: "file", FID: "2"},
					}, nil
				},
			}, nil
		},
	}

	if err := dedupRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "1 个重复文件") {
		t.Error("expected 1 duplicate")
	}
	if !strings.Contains(out, "dry-run") {
		t.Error("expected dry-run message")
	}
}

func TestDedupRunExecute(t *testing.T) {
	var buf bytes.Buffer
	var deletedFIDs []string
	var refreshedPaths []string
	o := &dedupOpts{
		Out:     &buf,
		Path:    "/test",
		Execute: true,
		GetClient: func() (dedupClient, error) {
			return &mockDedupClient{
				listDirFunc: func(string) ([]cloud115.Entry, error) {
					return []cloud115.Entry{
						{Name: "subdir", Type: "dir"},
					}, nil
				},
				listDirUncachedFunc: func(string) ([]cloud115.Entry, error) {
					return []cloud115.Entry{
						{Name: "dup.mp4", Type: "file", FID: "1"},
						{Name: "dup.mp4", Type: "file", FID: "2"},
					}, nil
				},
				deleteByIDsFunc: func(fids []string, _ []string) error {
					deletedFIDs = append(deletedFIDs, fids...)
					return nil
				},
				refreshPathsFunc: func(paths []string) error {
					refreshedPaths = paths
					return nil
				},
			}, nil
		},
	}

	if err := dedupRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "清理完成") {
		t.Error("expected cleanup complete message")
	}
	if len(deletedFIDs) != 1 || deletedFIDs[0] != "2" {
		t.Errorf("expected FID '2' deleted, got: %v", deletedFIDs)
	}
	if len(refreshedPaths) != 1 {
		t.Errorf("expected 1 refreshed path, got: %d", len(refreshedPaths))
	}
}

func TestDedupRunDeleteError(t *testing.T) {
	var buf bytes.Buffer
	o := &dedupOpts{
		Out:     &buf,
		Path:    "/test",
		Execute: true,
		GetClient: func() (dedupClient, error) {
			return &mockDedupClient{
				listDirFunc: func(string) ([]cloud115.Entry, error) {
					return []cloud115.Entry{
						{Name: "subdir", Type: "dir"},
					}, nil
				},
				listDirUncachedFunc: func(string) ([]cloud115.Entry, error) {
					return []cloud115.Entry{
						{Name: "dup.mp4", Type: "file", FID: "1"},
						{Name: "dup.mp4", Type: "file", FID: "2"},
					}, nil
				},
				deleteByIDsFunc: func([]string, []string) error {
					return errors.New("delete failed")
				},
				refreshPathsFunc: func([]string) error { return nil },
			}, nil
		},
	}

	err := dedupRun(o)
	if err == nil || !strings.Contains(err.Error(), "删除批次失败") {
		t.Fatalf("expected delete error, got: %v", err)
	}
}

func TestDedupRunUncachedListError(t *testing.T) {
	var buf bytes.Buffer
	o := &dedupOpts{
		Out:  &buf,
		Path: "/test",
		GetClient: func() (dedupClient, error) {
			return &mockDedupClient{
				listDirFunc: func(string) ([]cloud115.Entry, error) {
					return []cloud115.Entry{
						{Name: "subdir", Type: "dir"},
					}, nil
				},
				listDirUncachedFunc: func(string) ([]cloud115.Entry, error) {
					return nil, errors.New("uncached failed")
				},
			}, nil
		},
	}

	if err := dedupRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "警告: 无法列出") {
		t.Error("expected warning for uncached list failure")
	}
}

func TestDedupRunRefreshError(t *testing.T) {
	var buf bytes.Buffer
	o := &dedupOpts{
		Out:     &buf,
		Path:    "/test",
		Execute: true,
		GetClient: func() (dedupClient, error) {
			return &mockDedupClient{
				listDirFunc: func(string) ([]cloud115.Entry, error) {
					return []cloud115.Entry{
						{Name: "subdir", Type: "dir"},
					}, nil
				},
				listDirUncachedFunc: func(string) ([]cloud115.Entry, error) {
					return []cloud115.Entry{
						{Name: "dup.mp4", Type: "file", FID: "1"},
						{Name: "dup.mp4", Type: "file", FID: "2"},
					}, nil
				},
				deleteByIDsFunc: func([]string, []string) error { return nil },
				refreshPathsFunc: func([]string) error {
					return errors.New("refresh failed")
				},
			}, nil
		},
	}

	if err := dedupRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "警告: 刷新目录缓存失败") {
		t.Error("expected refresh warning")
	}
}

func TestDedupRunTruncatesDisplay(t *testing.T) {
	var buf bytes.Buffer
	// Create more than 5 unique duplicate filenames to trigger "..." truncation
	var files []cloud115.Entry
	for i := 0; i < 7; i++ {
		name := fmt.Sprintf("dup%d.mp4", i)
		files = append(files, cloud115.Entry{Name: name, Type: "file", FID: fmt.Sprintf("a%d", i)})
		files = append(files, cloud115.Entry{Name: name, Type: "file", FID: fmt.Sprintf("b%d", i)})
	}

	o := &dedupOpts{
		Out:  &buf,
		Path: "/test",
		GetClient: func() (dedupClient, error) {
			return &mockDedupClient{
				listDirFunc: func(string) ([]cloud115.Entry, error) {
					return []cloud115.Entry{{Name: "subdir", Type: "dir"}}, nil
				},
				listDirUncachedFunc: func(string) ([]cloud115.Entry, error) {
					return files, nil
				},
			}, nil
		},
	}

	if err := dedupRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "...") {
		t.Error("expected truncation with ...")
	}
	if !strings.Contains(out, "7 个重复文件") {
		t.Error("expected 7 duplicates")
	}
}
