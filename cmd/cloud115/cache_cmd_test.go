package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/newcoderlife/media115/internal/cloud115"
)

func TestShowCacheStatusNoLogin(t *testing.T) {
	var buf bytes.Buffer
	result := showCacheStatusNoLogin(&buf)
	output := buf.String()

	if result != nil {
		t.Errorf("showCacheStatusNoLogin() returned error: %v", result)
	}

	if !strings.Contains(output, "cloud115 缓存") {
		t.Errorf("expected output to contain 'cloud115 缓存', got: %q", output)
	}
	if !strings.Contains(output, "需要登录") {
		t.Errorf("expected output to mention login requirement, got: %q", output)
	}
}

type mockCacheStatusClient struct {
	statusFn func() (*cloud115.CacheStats, error)
}

func (m *mockCacheStatusClient) CacheStatus() (*cloud115.CacheStats, error) { return m.statusFn() }
func (m *mockCacheStatusClient) Close() error                               { return nil }

type mockCacheClearable struct {
	cleared bool
}

func (m *mockCacheClearable) ClearMetadata() { m.cleared = true }
func (m *mockCacheClearable) Close() error   { return nil }

func TestCacheStatusRun(t *testing.T) {
	tests := []struct {
		name    string
		opts    *cacheStatusOpts
		wantErr string
		wantOut string
	}{
		{
			name: "client error falls back to no-login",
			opts: &cacheStatusOpts{
				GetClient: func() (cacheStatusClient, error) { return nil, errors.New("no auth") },
			},
			wantOut: "需要登录",
		},
		{
			name: "cache status error",
			opts: &cacheStatusOpts{
				GetClient: func() (cacheStatusClient, error) {
					return &mockCacheStatusClient{
						statusFn: func() (*cloud115.CacheStats, error) { return nil, errors.New("db err") },
					}, nil
				},
			},
			wantErr: "获取缓存状态失败",
		},
		{
			name: "success",
			opts: &cacheStatusOpts{
				GetClient: func() (cacheStatusClient, error) {
					return &mockCacheStatusClient{
						statusFn: func() (*cloud115.CacheStats, error) {
							return &cloud115.CacheStats{
								PathCount:   10,
								DirCount:    5,
								EntryCount:  100,
								TreeEntries: 50,
								DBSizeBytes: 1024,
								RateLimit: map[string]cloud115.RateLimitState{
									"list": {MinuteCount: 3},
								},
							}, nil
						},
					}, nil
				},
			},
			wantOut: "路径映射",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			tt.opts.Out = &buf
			err := cacheStatusRun(tt.opts)
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

func TestCacheClearRun(t *testing.T) {
	tests := []struct {
		name    string
		opts    *cacheClearOpts
		wantErr string
		wantOut string
	}{
		{
			name: "cache open error",
			opts: &cacheClearOpts{
				NewCache: func() (cacheClearable, error) { return nil, errors.New("perm denied") },
			},
			wantErr: "打开缓存失败",
		},
		{
			name: "success",
			opts: &cacheClearOpts{
				NewCache: func() (cacheClearable, error) { return &mockCacheClearable{}, nil },
			},
			wantOut: "缓存已清除",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			tt.opts.Out = &buf
			err := cacheClearRun(tt.opts)
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
