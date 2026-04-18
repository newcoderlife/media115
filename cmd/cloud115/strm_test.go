package main

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/newcoderlife/media115/internal/cloud115"
)

type mockStrmClient struct {
	treeEntriesFn func(category string) ([]cloud115.TreeEntry, error)
}

func (m *mockStrmClient) TreeEntries(cat string) ([]cloud115.TreeEntry, error) {
	return m.treeEntriesFn(cat)
}
func (m *mockStrmClient) Close() error { return nil }

func TestStrmRun(t *testing.T) {
	tests := []struct {
		name    string
		opts    *strmOpts
		wantErr string
		wantOut string
	}{
		{
			name: "client error",
			opts: &strmOpts{
				Path:      "videos",
				Host:      "localhost",
				Port:      8080,
				GetClient: func() (strmClient, error) { return nil, errors.New("no auth") },
			},
			wantErr: "no auth",
		},
		{
			name: "tree entries error",
			opts: &strmOpts{
				Path: "videos",
				Host: "localhost",
				Port: 8080,
				GetClient: func() (strmClient, error) {
					return &mockStrmClient{
						treeEntriesFn: func(_ string) ([]cloud115.TreeEntry, error) {
							return nil, errors.New("db error")
						},
					}, nil
				},
			},
			wantErr: "db error",
		},
		{
			name: "no matching videos",
			opts: &strmOpts{
				Path: "videos",
				Host: "localhost",
				Port: 8080,
				GetClient: func() (strmClient, error) {
					return &mockStrmClient{
						treeEntriesFn: func(_ string) ([]cloud115.TreeEntry, error) {
							return []cloud115.TreeEntry{
								{Path: "other/movie.mkv", IsVideo: true},
							}, nil
						},
					}, nil
				},
			},
			wantOut: "No video files found",
		},
		{
			name: "mkdir error",
			opts: &strmOpts{
				Path:   "videos",
				Output: "/tmp/strm_test",
				Host:   "localhost",
				Port:   8080,
				GetClient: func() (strmClient, error) {
					return &mockStrmClient{
						treeEntriesFn: func(_ string) ([]cloud115.TreeEntry, error) {
							return []cloud115.TreeEntry{
								{Path: "videos/movie.mkv", IsVideo: true},
							}, nil
						},
					}, nil
				},
				MkdirAll:  func(_ string, _ os.FileMode) error { return errors.New("perm denied") },
				WriteFile: os.WriteFile,
			},
			wantErr: "create dir",
		},
		{
			name: "write error",
			opts: &strmOpts{
				Path:   "videos",
				Output: "/tmp/strm_test",
				Host:   "localhost",
				Port:   8080,
				GetClient: func() (strmClient, error) {
					return &mockStrmClient{
						treeEntriesFn: func(_ string) ([]cloud115.TreeEntry, error) {
							return []cloud115.TreeEntry{
								{Path: "videos/movie.mkv", IsVideo: true},
							}, nil
						},
					}, nil
				},
				MkdirAll:  func(_ string, _ os.FileMode) error { return nil },
				WriteFile: func(_ string, _ []byte, _ os.FileMode) error { return errors.New("disk full") },
			},
			wantErr: "write strm",
		},
		{
			name: "success",
			opts: &strmOpts{
				Path:   "videos",
				Output: "/tmp/strm_test",
				Host:   "localhost",
				Port:   8080,
				GetClient: func() (strmClient, error) {
					return &mockStrmClient{
						treeEntriesFn: func(_ string) ([]cloud115.TreeEntry, error) {
							return []cloud115.TreeEntry{
								{Path: "videos/movie.mkv", IsVideo: true},
								{Path: "videos/sub/show.mp4", IsVideo: true},
								{Path: "videos/doc.nfo", IsVideo: false},
							}, nil
						},
					}, nil
				},
				MkdirAll: func(_ string, _ os.FileMode) error { return nil },
				WriteFile: func(name string, data []byte, _ os.FileMode) error {
					if !strings.HasSuffix(name, ".strm") {
						return errors.New("unexpected file extension")
					}
					return nil
				},
			},
			wantOut: "Generated 2 .strm files",
		},
		{
			name: "exact path match",
			opts: &strmOpts{
				Path:   "videos/movie.mkv",
				Output: "/tmp/strm_test",
				Host:   "localhost",
				Port:   8080,
				GetClient: func() (strmClient, error) {
					return &mockStrmClient{
						treeEntriesFn: func(_ string) ([]cloud115.TreeEntry, error) {
							return []cloud115.TreeEntry{
								{Path: "videos/movie.mkv", IsVideo: true},
							}, nil
						},
					}, nil
				},
				MkdirAll:  func(_ string, _ os.FileMode) error { return nil },
				WriteFile: func(_ string, _ []byte, _ os.FileMode) error { return nil },
			},
			wantOut: "Generated 1 .strm files",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			tt.opts.Out = &buf
			err := strmRun(tt.opts)
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
