package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/newcoderlife/media115/internal/cloud115"
)

type mockRapidClient struct {
	rapidFn func(localPath, remoteDir string) (*cloud115.RapidResult, error)
}

func (m *mockRapidClient) RapidUpload(l, r string) (*cloud115.RapidResult, error) {
	return m.rapidFn(l, r)
}
func (m *mockRapidClient) Close() error { return nil }

func TestRapidRun(t *testing.T) {
	tests := []struct {
		name    string
		opts    *rapidOpts
		wantErr string
		wantOut string
	}{
		{
			name: "client error",
			opts: &rapidOpts{
				LocalPath: "/tmp/f.mkv",
				RemoteDir: "/remote",
				GetClient: func() (rapidClient, error) { return nil, errors.New("no auth") },
			},
			wantErr: "no auth",
		},
		{
			name: "rapid upload error",
			opts: &rapidOpts{
				LocalPath: "/tmp/f.mkv",
				RemoteDir: "/remote",
				GetClient: func() (rapidClient, error) {
					return &mockRapidClient{rapidFn: func(_, _ string) (*cloud115.RapidResult, error) {
						return nil, errors.New("hash error")
					}}, nil
				},
			},
			wantErr: "秒传失败",
		},
		{
			name: "status not 2",
			opts: &rapidOpts{
				LocalPath: "/tmp/f.mkv",
				RemoteDir: "/remote",
				GetClient: func() (rapidClient, error) {
					return &mockRapidClient{rapidFn: func(_, _ string) (*cloud115.RapidResult, error) {
						return &cloud115.RapidResult{Status: 0}, nil
					}}, nil
				},
			},
			wantErr: "115 上没有此文件",
		},
		{
			name: "success",
			opts: &rapidOpts{
				LocalPath: "/tmp/movie.mkv",
				RemoteDir: "/remote",
				GetClient: func() (rapidClient, error) {
					return &mockRapidClient{rapidFn: func(_, _ string) (*cloud115.RapidResult, error) {
						return &cloud115.RapidResult{Status: 2, PickCode: "xyz"}, nil
					}}, nil
				},
			},
			wantOut: "秒传成功: movie.mkv (pickcode=xyz)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			tt.opts.Out = &buf
			err := rapidRun(tt.opts)
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
