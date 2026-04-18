package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/newcoderlife/media115/internal/cloud115"
	"github.com/newcoderlife/media115/internal/config"
)

type mockCloudDoctorClient struct {
	loginOK  bool
	stats    *cloud115.CacheStats
	statsErr error
}

func (m *mockCloudDoctorClient) CheckLogin() bool { return m.loginOK }
func (m *mockCloudDoctorClient) CacheStatus() (*cloud115.CacheStats, error) {
	return m.stats, m.statsErr
}
func (m *mockCloudDoctorClient) Close() error { return nil }

func newTestCloudDoctorOpts(buf *bytes.Buffer) *cloudDoctorOpts {
	return &cloudDoctorOpts{
		Out:        buf,
		ConfigPath: func() string { return "/tmp/test/config.toml" },
		LoadConfig: func() (*config.Config, error) { return config.Default(), nil },
		GetClient:  func() (cloudDoctorClient, error) { return nil, errors.New("no cookies") },
		CacheDir:   func() string { return "/tmp/test/cache" },
	}
}

func TestCloudDoctorRunConfigOK(t *testing.T) {
	var buf bytes.Buffer
	o := newTestCloudDoctorOpts(&buf)

	if err := cloudDoctorRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "配置加载:      ✓") {
		t.Error("expected config load success")
	}
	if !strings.Contains(out, "/tmp/test/config.toml") {
		t.Error("expected config path")
	}
}

func TestCloudDoctorRunConfigError(t *testing.T) {
	var buf bytes.Buffer
	o := newTestCloudDoctorOpts(&buf)
	o.LoadConfig = func() (*config.Config, error) {
		return nil, errors.New("bad config")
	}

	if err := cloudDoctorRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "配置加载:      ✗ 失败") {
		t.Error("expected config load failure")
	}
}

func TestCloudDoctorRunNoCookies(t *testing.T) {
	var buf bytes.Buffer
	o := newTestCloudDoctorOpts(&buf)

	if err := cloudDoctorRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "115 Cookies:   ✗ 未配置") {
		t.Error("expected no cookies message")
	}
	if !strings.Contains(out, "115 登录:      ✗ 未配置 cookies") {
		t.Error("expected login status message")
	}
}

func TestCloudDoctorRunClientError(t *testing.T) {
	var buf bytes.Buffer
	o := newTestCloudDoctorOpts(&buf)
	cfg := config.Default()
	cfg.Auth.Cookies = "test_cookie"
	o.LoadConfig = func() (*config.Config, error) { return cfg, nil }
	o.GetClient = func() (cloudDoctorClient, error) {
		return nil, errors.New("client creation failed")
	}

	if err := cloudDoctorRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "配置但无法创建客户端") {
		t.Error("expected client error message")
	}
}

func TestCloudDoctorRunLoggedIn(t *testing.T) {
	var buf bytes.Buffer
	o := newTestCloudDoctorOpts(&buf)
	cfg := config.Default()
	cfg.Auth.Cookies = "test_cookie"
	cfg.Auth.TMDB.Token = "tmdb_token"
	o.LoadConfig = func() (*config.Config, error) { return cfg, nil }
	o.GetClient = func() (cloudDoctorClient, error) {
		return &mockCloudDoctorClient{
			loginOK: true,
			stats: &cloud115.CacheStats{
				PathCount:   100,
				DirCount:    10,
				EntryCount:  500,
				TreeEntries: 450,
				DBSizeBytes: 1048576,
			},
		}, nil
	}

	if err := cloudDoctorRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "115 Cookies:   ✓ 已登录") {
		t.Error("expected logged in")
	}
	if !strings.Contains(out, "TMDB Token:    ✓ 已配置") {
		t.Error("expected TMDB configured")
	}
	if !strings.Contains(out, "115 登录:      ✓ 有效") {
		t.Error("expected login valid")
	}
	if !strings.Contains(out, "100 条目") {
		t.Error("expected path count")
	}
}

func TestCloudDoctorRunLoginExpired(t *testing.T) {
	var buf bytes.Buffer
	o := newTestCloudDoctorOpts(&buf)
	cfg := config.Default()
	cfg.Auth.Cookies = "test_cookie"
	o.LoadConfig = func() (*config.Config, error) { return cfg, nil }
	o.GetClient = func() (cloudDoctorClient, error) {
		return &mockCloudDoctorClient{
			loginOK: false,
			stats:   &cloud115.CacheStats{},
		}, nil
	}

	if err := cloudDoctorRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "115 Cookies:   ✗ Cookie 过期") {
		t.Error("expected cookie expired")
	}
	if !strings.Contains(out, "cookie 已过期") {
		t.Error("expected expired login message")
	}
}

func TestCloudDoctorRunCacheStatusError(t *testing.T) {
	var buf bytes.Buffer
	o := newTestCloudDoctorOpts(&buf)
	cfg := config.Default()
	cfg.Auth.Cookies = "test_cookie"
	o.LoadConfig = func() (*config.Config, error) { return cfg, nil }
	o.GetClient = func() (cloudDoctorClient, error) {
		return &mockCloudDoctorClient{
			loginOK:  true,
			statsErr: errors.New("db error"),
		}, nil
	}

	if err := cloudDoctorRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "缓存状态:      ✗ 获取失败") {
		t.Error("expected cache status error")
	}
}

func TestCloudDoctorRunWithTMDBNoCookies(t *testing.T) {
	var buf bytes.Buffer
	o := newTestCloudDoctorOpts(&buf)
	cfg := config.Default()
	cfg.Auth.TMDB.Token = "test_token"
	o.LoadConfig = func() (*config.Config, error) { return cfg, nil }

	if err := cloudDoctorRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "TMDB Token:    ✓ 已配置") {
		t.Error("expected TMDB configured")
	}
}

func TestCloudDoctorRunProxyConfig(t *testing.T) {
	var buf bytes.Buffer
	o := newTestCloudDoctorOpts(&buf)

	if err := cloudDoctorRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "代理配置:") {
		t.Error("expected proxy config section")
	}
	if !strings.Contains(out, "strm-proxy:") {
		t.Error("expected strm-proxy config")
	}
}

func TestCloudDoctorRunClientErrorNoTMDB(t *testing.T) {
	var buf bytes.Buffer
	o := newTestCloudDoctorOpts(&buf)
	cfg := config.Default()
	cfg.Auth.Cookies = "test_cookie"
	// No TMDB token set
	o.LoadConfig = func() (*config.Config, error) { return cfg, nil }
	o.GetClient = func() (cloudDoctorClient, error) {
		return nil, errors.New("client failed")
	}

	if err := cloudDoctorRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "配置但无法创建客户端") {
		t.Error("expected client error message")
	}
	if !strings.Contains(out, "TMDB Token:    ✗ 未配置") {
		t.Error("expected TMDB not configured message")
	}
}

func TestCloudDoctorRunRateLimitCooldown(t *testing.T) {
	var buf bytes.Buffer
	o := newTestCloudDoctorOpts(&buf)
	cfg := config.Default()
	cfg.Auth.Cookies = "test_cookie"
	o.LoadConfig = func() (*config.Config, error) { return cfg, nil }
	o.GetClient = func() (cloudDoctorClient, error) {
		futureTime := float64(time.Now().Unix()) + 60
		return &mockCloudDoctorClient{
			loginOK: true,
			stats: &cloud115.CacheStats{
				RateLimit: map[string]cloud115.RateLimitState{
					"list": {CooldownUntil: futureTime, MinuteCount: 20},
				},
			},
		}, nil
	}

	if err := cloudDoctorRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "cooldown 中") {
		t.Error("expected cooldown message")
	}
}

func TestCloudDoctorRunRateLimitNormal(t *testing.T) {
	var buf bytes.Buffer
	o := newTestCloudDoctorOpts(&buf)
	cfg := config.Default()
	cfg.Auth.Cookies = "test_cookie"
	o.LoadConfig = func() (*config.Config, error) { return cfg, nil }
	o.GetClient = func() (cloudDoctorClient, error) {
		return &mockCloudDoctorClient{
			loginOK: true,
			stats: &cloud115.CacheStats{
				RateLimit: map[string]cloud115.RateLimitState{
					"list": {CooldownUntil: 0, MinuteCount: 5},
				},
			},
		}, nil
	}

	if err := cloudDoctorRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "正常 (5/20 QPM)") {
		t.Error("expected normal rate limit message")
	}
}
