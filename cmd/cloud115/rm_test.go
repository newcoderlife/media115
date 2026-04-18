package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/newcoderlife/media115/internal/cloud115"
)

type mockRmClient struct {
	statFn   func(path string) (*cloud115.Entry, error)
	deleteFn func(paths []string) error
}

func (m *mockRmClient) Stat(path string) (*cloud115.Entry, error) { return m.statFn(path) }
func (m *mockRmClient) Delete(paths []string) error               { return m.deleteFn(paths) }
func (m *mockRmClient) Close() error                              { return nil }

func TestRmRun(t *testing.T) {
	tests := []struct {
		name    string
		opts    *rmOpts
		wantErr string
		wantOut string
	}{
		{
			name: "client error",
			opts: &rmOpts{
				Paths:     []string{"/file"},
				GetClient: func() (rmClient, error) { return nil, errors.New("no auth") },
			},
			wantErr: "no auth",
		},
		{
			name: "path not found",
			opts: &rmOpts{
				Paths: []string{"/missing"},
				GetClient: func() (rmClient, error) {
					return &mockRmClient{
						statFn: func(_ string) (*cloud115.Entry, error) { return nil, errors.New("not found") },
					}, nil
				},
			},
			wantErr: "路径不存在",
		},
		{
			name: "dir without recursive",
			opts: &rmOpts{
				Paths:     []string{"/mydir"},
				Recursive: false,
				GetClient: func() (rmClient, error) {
					return &mockRmClient{
						statFn: func(_ string) (*cloud115.Entry, error) {
							return &cloud115.Entry{Type: "dir", Name: "mydir"}, nil
						},
					}, nil
				},
			},
			wantErr: "是目录，需要 -r",
		},
		{
			name: "delete error",
			opts: &rmOpts{
				Paths:     []string{"/file.txt"},
				Recursive: false,
				GetClient: func() (rmClient, error) {
					return &mockRmClient{
						statFn:   func(_ string) (*cloud115.Entry, error) { return &cloud115.Entry{Type: "file"}, nil },
						deleteFn: func(_ []string) error { return errors.New("api error") },
					}, nil
				},
			},
			wantErr: "删除失败",
		},
		{
			name: "success file",
			opts: &rmOpts{
				Paths: []string{"/file.txt"},
				GetClient: func() (rmClient, error) {
					return &mockRmClient{
						statFn:   func(_ string) (*cloud115.Entry, error) { return &cloud115.Entry{Type: "file"}, nil },
						deleteFn: func(_ []string) error { return nil },
					}, nil
				},
			},
			wantOut: "已删除 1 个项目",
		},
		{
			name: "success dir recursive",
			opts: &rmOpts{
				Paths:     []string{"/dir1", "/dir2"},
				Recursive: true,
				GetClient: func() (rmClient, error) {
					return &mockRmClient{
						statFn:   func(_ string) (*cloud115.Entry, error) { return &cloud115.Entry{Type: "dir"}, nil },
						deleteFn: func(_ []string) error { return nil },
					}, nil
				},
			},
			wantOut: "已删除 2 个项目",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			tt.opts.Out = &buf
			err := rmRun(tt.opts)
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
