package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/newcoderlife/media115/internal/cloud115"
)

type mockGetClient struct {
	findFileFn func(path string) (*cloud115.Entry, error)
	downloadFn func(pickCode, ua string) (string, error)
}

func (m *mockGetClient) FindFile(path string) (*cloud115.Entry, error) { return m.findFileFn(path) }
func (m *mockGetClient) DownloadURL(pc, ua string) (string, error)     { return m.downloadFn(pc, ua) }
func (m *mockGetClient) Close() error                                  { return nil }

func TestGetRun(t *testing.T) {
	tests := []struct {
		name    string
		opts    *getOpts
		wantErr string
		wantOut string
	}{
		{
			name: "client error",
			opts: &getOpts{
				RemotePath: "/movie.mkv",
				LocalDir:   "/tmp",
				GetClient:  func() (getFileClient, error) { return nil, errors.New("no auth") },
				Downloader: func(_, _ string) error { return nil },
			},
			wantErr: "no auth",
		},
		{
			name: "file not found",
			opts: &getOpts{
				RemotePath: "/missing.mkv",
				LocalDir:   "/tmp",
				GetClient: func() (getFileClient, error) {
					return &mockGetClient{
						findFileFn: func(_ string) (*cloud115.Entry, error) { return nil, errors.New("not found") },
					}, nil
				},
				Downloader: func(_, _ string) error { return nil },
			},
			wantErr: "远程文件不存在",
		},
		{
			name: "no pick code",
			opts: &getOpts{
				RemotePath: "/movie.mkv",
				LocalDir:   "/tmp",
				GetClient: func() (getFileClient, error) {
					return &mockGetClient{
						findFileFn: func(_ string) (*cloud115.Entry, error) {
							return &cloud115.Entry{Name: "movie.mkv", PickCode: ""}, nil
						},
					}, nil
				},
				Downloader: func(_, _ string) error { return nil },
			},
			wantErr: "文件缺少 pick_code",
		},
		{
			name: "download url error",
			opts: &getOpts{
				RemotePath: "/movie.mkv",
				LocalDir:   "/tmp",
				GetClient: func() (getFileClient, error) {
					return &mockGetClient{
						findFileFn: func(_ string) (*cloud115.Entry, error) {
							return &cloud115.Entry{Name: "movie.mkv", PickCode: "abc"}, nil
						},
						downloadFn: func(_, _ string) (string, error) { return "", errors.New("expired") },
					}, nil
				},
				Downloader: func(_, _ string) error { return nil },
			},
			wantErr: "获取下载链接失败",
		},
		{
			name: "download file error",
			opts: &getOpts{
				RemotePath: "/movie.mkv",
				LocalDir:   "/tmp",
				GetClient: func() (getFileClient, error) {
					return &mockGetClient{
						findFileFn: func(_ string) (*cloud115.Entry, error) {
							return &cloud115.Entry{Name: "movie.mkv", PickCode: "abc"}, nil
						},
						downloadFn: func(_, _ string) (string, error) { return "http://example.com/file", nil },
					}, nil
				},
				Downloader: func(_, _ string) error { return errors.New("disk full") },
			},
			wantErr: "下载失败",
		},
		{
			name: "success",
			opts: &getOpts{
				RemotePath: "/movie.mkv",
				LocalDir:   "/tmp",
				GetClient: func() (getFileClient, error) {
					return &mockGetClient{
						findFileFn: func(_ string) (*cloud115.Entry, error) {
							return &cloud115.Entry{Name: "movie.mkv", PickCode: "abc"}, nil
						},
						downloadFn: func(_, _ string) (string, error) { return "http://example.com/file", nil },
					}, nil
				},
				Downloader: func(_, _ string) error { return nil },
			},
			wantOut: "已下载",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			tt.opts.Out = &buf
			err := getRun(tt.opts)
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
