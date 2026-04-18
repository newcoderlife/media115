package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/newcoderlife/media115/internal/cloud115"
)

type mockFindClient struct {
	searchFn func(keyword, path string) ([]cloud115.Entry, error)
}

func (m *mockFindClient) Search(keyword, path string) ([]cloud115.Entry, error) {
	return m.searchFn(keyword, path)
}
func (m *mockFindClient) Close() error { return nil }

func TestFindRun(t *testing.T) {
	tests := []struct {
		name    string
		opts    *findOpts
		wantErr string
		wantOut string
	}{
		{
			name: "client error",
			opts: &findOpts{
				Keyword:   "test",
				Path:      "/",
				GetClient: func() (findClient, error) { return nil, errors.New("no auth") },
			},
			wantErr: "no auth",
		},
		{
			name: "search error",
			opts: &findOpts{
				Keyword: "test",
				Path:    "/",
				GetClient: func() (findClient, error) {
					return &mockFindClient{searchFn: func(_, _ string) ([]cloud115.Entry, error) {
						return nil, errors.New("network error")
					}}, nil
				},
			},
			wantErr: "搜索失败",
		},
		{
			name: "no results",
			opts: &findOpts{
				Keyword: "nothing",
				Path:    "/",
				GetClient: func() (findClient, error) {
					return &mockFindClient{searchFn: func(_, _ string) ([]cloud115.Entry, error) {
						return nil, nil
					}}, nil
				},
			},
		},
		{
			name: "results found",
			opts: &findOpts{
				Keyword: "movie",
				Path:    "/videos",
				GetClient: func() (findClient, error) {
					return &mockFindClient{searchFn: func(keyword, path string) ([]cloud115.Entry, error) {
						return []cloud115.Entry{
							{Name: "movie1.mkv"},
							{Name: "movie2.mp4"},
						}, nil
					}}, nil
				},
			},
			wantOut: "movie1.mkv",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			tt.opts.Out = &buf
			err := findRun(tt.opts)
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
