package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/newcoderlife/media115/internal/cloud115"
)

type mockPutClient struct {
	rapidFn  func(localPath, remoteDir string) (*cloud115.RapidResult, error)
	uploadFn func(localPath, remoteDir, filename string) (map[string]any, error)
}

func (m *mockPutClient) RapidUpload(l, r string) (*cloud115.RapidResult, error) {
	return m.rapidFn(l, r)
}
func (m *mockPutClient) Upload(l, r, f string) (map[string]any, error) { return m.uploadFn(l, r, f) }
func (m *mockPutClient) Close() error                                  { return nil }

func TestPutRun(t *testing.T) {
	tests := []struct {
		name    string
		opts    *putOpts
		wantErr string
		wantOut string
	}{
		{
			name: "client error",
			opts: &putOpts{
				LocalPath: "/tmp/file.txt",
				RemoteDir: "/remote",
				GetClient: func() (putClient, error) { return nil, errors.New("no auth") },
			},
			wantErr: "no auth",
		},
		{
			name: "rapid upload success",
			opts: &putOpts{
				LocalPath: "/tmp/movie.mkv",
				RemoteDir: "/remote",
				GetClient: func() (putClient, error) {
					return &mockPutClient{
						rapidFn: func(_, _ string) (*cloud115.RapidResult, error) {
							return &cloud115.RapidResult{Status: 2, PickCode: "abc"}, nil
						},
					}, nil
				},
			},
			wantOut: "已上传（秒传）: movie.mkv",
		},
		{
			name: "rapid fails fallback upload success",
			opts: &putOpts{
				LocalPath: "/tmp/movie.mkv",
				RemoteDir: "/remote",
				GetClient: func() (putClient, error) {
					return &mockPutClient{
						rapidFn:  func(_, _ string) (*cloud115.RapidResult, error) { return nil, errors.New("fail") },
						uploadFn: func(_, _, _ string) (map[string]any, error) { return nil, nil },
					}, nil
				},
			},
			wantOut: "已上传: movie.mkv",
		},
		{
			name: "rapid status not 2 fallback",
			opts: &putOpts{
				LocalPath: "/tmp/movie.mkv",
				RemoteDir: "/remote",
				GetClient: func() (putClient, error) {
					return &mockPutClient{
						rapidFn:  func(_, _ string) (*cloud115.RapidResult, error) { return &cloud115.RapidResult{Status: 0}, nil },
						uploadFn: func(_, _, _ string) (map[string]any, error) { return nil, nil },
					}, nil
				},
			},
			wantOut: "已上传: movie.mkv",
		},
		{
			name: "no-rapid mode",
			opts: &putOpts{
				LocalPath: "/tmp/movie.mkv",
				RemoteDir: "/remote",
				NoRapid:   true,
				GetClient: func() (putClient, error) {
					return &mockPutClient{
						uploadFn: func(_, _, _ string) (map[string]any, error) { return nil, nil },
					}, nil
				},
			},
			wantOut: "已上传: movie.mkv",
		},
		{
			name: "upload fails",
			opts: &putOpts{
				LocalPath: "/tmp/movie.mkv",
				RemoteDir: "/remote",
				NoRapid:   true,
				GetClient: func() (putClient, error) {
					return &mockPutClient{
						uploadFn: func(_, _, _ string) (map[string]any, error) { return nil, errors.New("disk full") },
					}, nil
				},
			},
			wantErr: "上传失败",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			tt.opts.Out = &buf
			err := putRun(tt.opts)
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
