package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/newcoderlife/media115/internal/cloud115"
	"github.com/newcoderlife/media115/internal/config"
)

type mockDoctorClient struct {
	loginOK  bool
	stats    *cloud115.CacheStats
	statsErr error
}

func (m *mockDoctorClient) CheckLogin() bool                           { return m.loginOK }
func (m *mockDoctorClient) CacheStatus() (*cloud115.CacheStats, error) { return m.stats, m.statsErr }
func (m *mockDoctorClient) Close() error                               { return nil }

func newTestDoctorOpts(buf *bytes.Buffer, tmpDir string) *doctorOpts {
	return &doctorOpts{
		Out:        buf,
		ConfigPath: func() string { return "/tmp/test/config.toml" },
		LoadConfig: func() (*config.Config, error) { return config.Default(), nil },
		GetClient:  func() (doctorClient, error) { return nil, errors.New("no cookies") },
		CacheDir:   func() string { return tmpDir },
	}
}

func TestDoctorRunConfigOK(t *testing.T) {
	var buf bytes.Buffer
	o := newTestDoctorOpts(&buf, t.TempDir())

	if err := doctorRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "配置加载:      ✓") {
		t.Error("expected successful config load indicator")
	}
	if !strings.Contains(out, "/tmp/test/config.toml") {
		t.Error("expected config path in output")
	}
}

func TestDoctorRunConfigFail(t *testing.T) {
	var buf bytes.Buffer
	o := newTestDoctorOpts(&buf, t.TempDir())
	o.LoadConfig = func() (*config.Config, error) { return nil, errors.New("file not found") }

	if err := doctorRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "配置加载:      ✗ 失败") {
		t.Error("expected config load failure indicator")
	}
}

func TestDoctorRunLoginValid(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.Default()
	cfg.Auth.Cookies = "test-cookies"

	o := newTestDoctorOpts(&buf, t.TempDir())
	o.LoadConfig = func() (*config.Config, error) { return cfg, nil }
	o.GetClient = func() (doctorClient, error) {
		return &mockDoctorClient{
			loginOK: true,
			stats: &cloud115.CacheStats{
				PathCount:   10,
				DirCount:    3,
				EntryCount:  50,
				TreeEntries: 100,
				DBSizeBytes: 1024 * 1024,
			},
		}, nil
	}

	if err := doctorRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "115 登录:      ✓ 有效") {
		t.Error("expected valid login indicator")
	}
	if !strings.Contains(out, "路径映射:      10 条目") {
		t.Error("expected cache stats in output")
	}
}

func TestDoctorRunLoginExpired(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.Default()
	cfg.Auth.Cookies = "expired-cookies"

	o := newTestDoctorOpts(&buf, t.TempDir())
	o.LoadConfig = func() (*config.Config, error) { return cfg, nil }
	o.GetClient = func() (doctorClient, error) {
		return &mockDoctorClient{loginOK: false, stats: &cloud115.CacheStats{}}, nil
	}

	if err := doctorRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "cookie 已过期") {
		t.Error("expected expired login indicator")
	}
}

func TestDoctorRunClientError(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.Default()
	cfg.Auth.Cookies = "bad-cookies"

	o := newTestDoctorOpts(&buf, t.TempDir())
	o.LoadConfig = func() (*config.Config, error) { return cfg, nil }
	o.GetClient = func() (doctorClient, error) { return nil, errors.New("connection refused") }

	if err := doctorRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(buf.String(), "初始化失败") {
		t.Error("expected client init failure indicator")
	}
}

func TestDoctorRunCacheDir(t *testing.T) {
	var buf bytes.Buffer
	tmpDir := t.TempDir()

	// Create scrape dir with JSON files.
	scrapeDir := filepath.Join(tmpDir, "scrape")
	_ = os.MkdirAll(scrapeDir, 0o755)
	_ = os.WriteFile(filepath.Join(scrapeDir, "file1.json"), []byte("{}"), 0o644)
	_ = os.WriteFile(filepath.Join(scrapeDir, "file2.json"), []byte("{}"), 0o644)

	// Create scrape_output dir with one category.
	outDir := filepath.Join(tmpDir, "scrape_output", "AV")
	_ = os.MkdirAll(outDir, 0o755)

	o := newTestDoctorOpts(&buf, tmpDir)
	if err := doctorRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "2 个JSON文件") {
		t.Errorf("expected '2 个JSON文件' in output, got: %s", out)
	}
	if !strings.Contains(out, "1 个分类") {
		t.Errorf("expected '1 个分类' in output, got: %s", out)
	}
}

func TestDoctorRunNoCacheDir(t *testing.T) {
	var buf bytes.Buffer
	tmpDir := filepath.Join(t.TempDir(), "nonexistent")

	o := newTestDoctorOpts(&buf, tmpDir)
	if err := doctorRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "scrape目录不存在") {
		t.Error("expected missing scrape dir message")
	}
	if !strings.Contains(out, "刮削输出:      不存在") {
		t.Error("expected missing output dir message")
	}
}

func TestDoctorRunTokenDisplay(t *testing.T) {
	var buf bytes.Buffer
	cfg := config.Default()
	cfg.Auth.TMDB.Token = "test-tmdb-token"
	cfg.Auth.StashDB.APIKey = "test-stashdb-key"

	o := newTestDoctorOpts(&buf, t.TempDir())
	o.LoadConfig = func() (*config.Config, error) { return cfg, nil }

	if err := doctorRun(o); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "TMDB Token") || !strings.Contains(out, "✓ 已配置") {
		t.Error("expected TMDB token configured")
	}
	if !strings.Contains(out, "Bangumi Token") || !strings.Contains(out, "✗ 未配置") {
		t.Error("expected Bangumi token unconfigured")
	}
}
