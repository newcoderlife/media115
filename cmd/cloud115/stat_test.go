package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/newcoderlife/media115/internal/cloud115"
)

type mockStatClient struct {
	statFn func(path string) (*cloud115.Entry, error)
}

func (m *mockStatClient) Stat(path string) (*cloud115.Entry, error) { return m.statFn(path) }
func (m *mockStatClient) Close() error                              { return nil }

func TestStatRun(t *testing.T) {
	tests := []struct {
		name    string
		opts    *statOpts
		wantErr string
		wantOut string
	}{
		{
			name: "client error",
			opts: &statOpts{
				Path:      "/test",
				GetClient: func() (statClient, error) { return nil, errors.New("no auth") },
			},
			wantErr: "no auth",
		},
		{
			name: "path not found",
			opts: &statOpts{
				Path: "/missing",
				GetClient: func() (statClient, error) {
					return &mockStatClient{statFn: func(_ string) (*cloud115.Entry, error) {
						return nil, errors.New("not found")
					}}, nil
				},
			},
			wantErr: "路径不存在",
		},
		{
			name: "success file",
			opts: &statOpts{
				Path: "/test/movie.mkv",
				GetClient: func() (statClient, error) {
					return &mockStatClient{statFn: func(_ string) (*cloud115.Entry, error) {
						return &cloud115.Entry{Name: "movie.mkv", Type: "file", Size: 1024, PickCode: "abc123"}, nil
					}}, nil
				},
			},
			wantOut: "movie.mkv",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			tt.opts.Out = &buf
			err := statRun(tt.opts)
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
