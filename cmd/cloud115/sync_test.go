package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/newcoderlife/media115/internal/cloud115"
)

type mockSyncClient struct {
	exportTreeFn  func(path string) (string, error)
	saveTreeFn    func(text string, videoExts []string, rootPath string) (int, error)
	treeStatsFn   func() (*cloud115.TreeStats, error)
	warmFn        func(path string, depth int, progress func(string, int)) error
	cacheStatusFn func() (*cloud115.CacheStats, error)
}

func (m *mockSyncClient) ExportTree(path string) (string, error) { return m.exportTreeFn(path) }
func (m *mockSyncClient) SaveTree(text string, videoExts []string, rootPath string) (int, error) {
	return m.saveTreeFn(text, videoExts, rootPath)
}
func (m *mockSyncClient) TreeStats() (*cloud115.TreeStats, error) { return m.treeStatsFn() }
func (m *mockSyncClient) Warm(path string, depth int, progress func(string, int)) error {
	return m.warmFn(path, depth, progress)
}
func (m *mockSyncClient) CacheStatus() (*cloud115.CacheStats, error) { return m.cacheStatusFn() }
func (m *mockSyncClient) Close() error                               { return nil }

func TestSyncRun(t *testing.T) {
	tests := []struct {
		name    string
		opts    *syncOpts
		wantErr string
		wantOut string
	}{
		{
			name: "client error",
			opts: &syncOpts{
				Path:      "/videos",
				GetClient: func() (syncClient, error) { return nil, errors.New("no auth") },
			},
			wantErr: "no auth",
		},
		{
			name: "export error",
			opts: &syncOpts{
				Path: "/videos",
				GetClient: func() (syncClient, error) {
					return &mockSyncClient{
						exportTreeFn: func(_ string) (string, error) { return "", errors.New("timeout") },
					}, nil
				},
			},
			wantErr: "导出失败",
		},
		{
			name: "export empty",
			opts: &syncOpts{
				Path: "/videos",
				GetClient: func() (syncClient, error) {
					return &mockSyncClient{
						exportTreeFn: func(_ string) (string, error) { return "", nil },
					}, nil
				},
			},
			wantErr: "导出失败: /videos",
		},
		{
			name: "save tree error",
			opts: &syncOpts{
				Path:      "/videos",
				VideoExts: videoExts,
				GetClient: func() (syncClient, error) {
					return &mockSyncClient{
						exportTreeFn: func(_ string) (string, error) { return "tree data", nil },
						saveTreeFn:   func(_ string, _ []string, _ string) (int, error) { return 0, errors.New("db error") },
					}, nil
				},
			},
			wantErr: "保存到 SQLite 失败",
		},
		{
			name: "tree stats error",
			opts: &syncOpts{
				Path:      "/videos",
				VideoExts: videoExts,
				GetClient: func() (syncClient, error) {
					return &mockSyncClient{
						exportTreeFn: func(_ string) (string, error) { return "tree data", nil },
						saveTreeFn:   func(_ string, _ []string, _ string) (int, error) { return 10, nil },
						treeStatsFn:  func() (*cloud115.TreeStats, error) { return nil, errors.New("stats err") },
					}, nil
				},
			},
			wantErr: "获取 tree stats 失败",
		},
		{
			name: "success no deep",
			opts: &syncOpts{
				Path:      "/videos",
				VideoExts: videoExts,
				GetClient: func() (syncClient, error) {
					return &mockSyncClient{
						exportTreeFn: func(_ string) (string, error) { return "tree data", nil },
						saveTreeFn:   func(_ string, _ []string, _ string) (int, error) { return 10, nil },
						treeStatsFn: func() (*cloud115.TreeStats, error) {
							return &cloud115.TreeStats{Total: 100, Videos: 50, NFOs: 10}, nil
						},
					}, nil
				},
			},
			wantOut: "100 条目",
		},
		{
			name: "deep warm success",
			opts: &syncOpts{
				Path:      "/videos",
				Deep:      true,
				Depth:     2,
				VideoExts: videoExts,
				GetClient: func() (syncClient, error) {
					return &mockSyncClient{
						exportTreeFn: func(_ string) (string, error) { return "tree data", nil },
						saveTreeFn:   func(_ string, _ []string, _ string) (int, error) { return 10, nil },
						treeStatsFn: func() (*cloud115.TreeStats, error) {
							return &cloud115.TreeStats{Total: 100, Videos: 50, NFOs: 10}, nil
						},
						warmFn: func(_ string, _ int, progress func(string, int)) error {
							progress("/dir1", 1)
							return nil
						},
						cacheStatusFn: func() (*cloud115.CacheStats, error) {
							return &cloud115.CacheStats{PathCount: 5, DirCount: 3, EntryCount: 20}, nil
						},
					}, nil
				},
			},
			wantOut: "预热完成",
		},
		{
			name: "deep warm error",
			opts: &syncOpts{
				Path:      "/videos",
				Deep:      true,
				Depth:     2,
				VideoExts: videoExts,
				GetClient: func() (syncClient, error) {
					return &mockSyncClient{
						exportTreeFn: func(_ string) (string, error) { return "tree data", nil },
						saveTreeFn:   func(_ string, _ []string, _ string) (int, error) { return 10, nil },
						treeStatsFn: func() (*cloud115.TreeStats, error) {
							return &cloud115.TreeStats{Total: 100, Videos: 50, NFOs: 10}, nil
						},
						warmFn: func(_ string, _ int, _ func(string, int)) error {
							return errors.New("warm error")
						},
					}, nil
				},
			},
			wantOut: "预热失败",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			tt.opts.Out = &buf
			err := syncRun(tt.opts)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got: %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantOut != "" && !strings.Contains(buf.String(), tt.wantOut) {
				t.Errorf("expected output containing %q, got: %q", tt.wantOut, buf.String())
			}
		})
	}
}
