package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/newcoderlife/media115/internal/mteam"
)

// withTempConfig points XDG_CONFIG_HOME + XDG_CACHE_HOME at temp dirs so
// the sub commands operate on an isolated environment.
func withTempConfig(t *testing.T) {
	t.Helper()
	cfgDir := t.TempDir()
	cacheDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfgDir)
	t.Setenv("XDG_CACHE_HOME", cacheDir)
}

// saveTickFlags snapshots all package-level tick* flag vars and returns a
// cleanup function that restores them. Call via defer in every test that
// modifies flag vars to prevent inter-test pollution.
func saveTickFlags(t *testing.T) {
	t.Helper()
	saved := struct {
		keyword         string
		mode            string
		watchDir        string
		requireFree     bool
		require4K       bool
		requireHQ       bool
		noJunk          bool
		maxFiles        int
		minSize         string
		maxSize         string
		minSeeders      int
		excludeKeywords []string
		labelsAllow     []string
		labelsDeny      []string
		freshHours      int
		minRating       float64
		tvComplete      bool
		dryRun          bool
		pages           int
	}{
		tickKeyword, tickMode, tickWatchDir, tickRequireFree,
		tickRequire4K, tickRequireHQ, tickNoJunk, tickMaxFiles,
		tickMinSize, tickMaxSize, tickMinSeeders, tickExcludeKeywords,
		tickLabelsAllow, tickLabelsDeny, tickFreshHours, tickMinRating,
		tickTVComplete, tickDryRun, tickPages,
	}
	t.Cleanup(func() {
		tickKeyword = saved.keyword
		tickMode = saved.mode
		tickWatchDir = saved.watchDir
		tickRequireFree = saved.requireFree
		tickRequire4K = saved.require4K
		tickRequireHQ = saved.requireHQ
		tickNoJunk = saved.noJunk
		tickMaxFiles = saved.maxFiles
		tickMinSize = saved.minSize
		tickMaxSize = saved.maxSize
		tickMinSeeders = saved.minSeeders
		tickExcludeKeywords = saved.excludeKeywords
		tickLabelsAllow = saved.labelsAllow
		tickLabelsDeny = saved.labelsDeny
		tickFreshHours = saved.freshHours
		tickMinRating = saved.minRating
		tickTVComplete = saved.tvComplete
		tickDryRun = saved.dryRun
		tickPages = saved.pages
	})
}

func TestParseSize(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		err  bool
	}{
		{"", 0, false},
		{"1024", 1024, false},
		{"1K", 1024, false},
		{"2M", 2 * 1024 * 1024, false},
		{"1.5G", int64(1.5 * float64(1<<30)), false},
		{"4T", 4 * (1 << 40), false},
		{"abc", 0, true},
		{"-5G", 0, true},
		{"G", 0, true},
		{"1.5", 1, false},
	}
	for _, c := range cases {
		got, err := parseSize(c.in)
		if c.err {
			if err == nil {
				t.Errorf("parseSize(%q) expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseSize(%q): %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("parseSize(%q)=%d want %d", c.in, got, c.want)
		}
	}
}

func writeMTeamConfig(t *testing.T, extra string) {
	t.Helper()
	cfgPath := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "media115", "config.toml")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "[mteam]\napi_key = \"K\"\n" + extra
	if err := os.WriteFile(cfgPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestSub_NoAPIKey(t *testing.T) {
	withTempConfig(t)
	saveTickFlags(t)
	tickWatchDir = t.TempDir()
	err := subCmd.RunE(subCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "api_key") {
		t.Fatalf("expected api_key error: %v", err)
	}
}

func TestSub_NoWatchDir(t *testing.T) {
	withTempConfig(t)
	saveTickFlags(t)
	writeMTeamConfig(t, "")
	tickWatchDir = ""
	err := subCmd.RunE(subCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "watch-dir") {
		t.Fatalf("expected watch-dir error: %v", err)
	}
}

func TestSub_BadMinSize(t *testing.T) {
	withTempConfig(t)
	saveTickFlags(t)
	writeMTeamConfig(t, "")
	tickWatchDir = t.TempDir()
	tickMinSize = "abc"
	err := subCmd.RunE(subCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "min-size") {
		t.Fatalf("expected min-size error: %v", err)
	}
}

func TestSub_BadMaxSize(t *testing.T) {
	withTempConfig(t)
	saveTickFlags(t)
	writeMTeamConfig(t, "")
	tickWatchDir = t.TempDir()
	tickMaxSize = "abc"
	err := subCmd.RunE(subCmd, nil)
	if err == nil || !strings.Contains(err.Error(), "max-size") {
		t.Fatalf("expected max-size error: %v", err)
	}
}

func TestSub_WithFakeServer(t *testing.T) {
	withTempConfig(t)
	saveTickFlags(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"total":"0","data":[]}}`))
	}))
	defer srv.Close()
	writeMTeamConfig(t, "base_url = \""+srv.URL+"\"\nmin_interval_seconds = 0\n")

	tickDryRun = true
	tickKeyword = "test"
	tickWatchDir = t.TempDir()
	tickPages = 1
	if err := subCmd.RunE(subCmd, nil); err != nil {
		t.Fatalf("sub: %v", err)
	}
}

func TestSub_WithResults_Rejected(t *testing.T) {
	withTempConfig(t)
	saveTickFlags(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"total":"1","data":[
			{"id":"1","name":"Movie","size":"20","status":{"seeders":"1","discount":"NORMAL"}}
		]}}`))
	}))
	defer srv.Close()
	writeMTeamConfig(t, "base_url = \""+srv.URL+"\"\nmin_interval_seconds = 0\n")

	tickDryRun = true
	tickKeyword = "test"
	tickWatchDir = t.TempDir()
	tickPages = 1
	tickRequireFree = true

	// Capture stdout to verify report output.
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	defer func() { os.Stdout = old }()

	if err := subCmd.RunE(subCmd, nil); err != nil {
		os.Stdout = old
		t.Fatalf("sub: %v", err)
	}
	w.Close()
	out, _ := io.ReadAll(r)
	os.Stdout = old
	output := string(out)

	// The torrent is NORMAL discount, so it should be rejected by require-free.
	if !strings.Contains(output, "扫描=1") {
		t.Errorf("expected 扫描=1 in output, got: %s", output)
	}
	if !strings.Contains(output, "下载=0") {
		t.Errorf("expected 下载=0 in output, got: %s", output)
	}
	if !strings.Contains(output, "跳过原因") {
		t.Errorf("expected 跳过原因 in output, got: %s", output)
	}
}

func TestPrintReport_Empty(t *testing.T) {
	r := &mteam.RunReport{}
	// Capture stdout.
	old := os.Stdout
	rd, w, _ := os.Pipe()
	os.Stdout = w
	printReport(r)
	w.Close()
	out, _ := io.ReadAll(rd)
	os.Stdout = old
	output := string(out)
	if !strings.Contains(output, "扫描=0") {
		t.Errorf("expected 扫描=0, got: %s", output)
	}
	// No rejections section when empty.
	if strings.Contains(output, "跳过原因") {
		t.Errorf("unexpected 跳过原因 in empty report: %s", output)
	}
}

func TestPrintReport_WithRejections(t *testing.T) {
	r := &mteam.RunReport{
		Scanned:    10,
		Matched:    3,
		Downloaded: 2,
		Skipped:    7,
		Errors:     1,
		Rejections: map[string]int{
			"not free":           3,
			"already downloaded": 2,
		},
	}
	// Capture stdout.
	old := os.Stdout
	rd, w, _ := os.Pipe()
	os.Stdout = w
	printReport(r)
	w.Close()
	out, _ := io.ReadAll(rd)
	os.Stdout = old
	output := string(out)
	if !strings.Contains(output, "扫描=10") {
		t.Errorf("expected 扫描=10, got: %s", output)
	}
	if !strings.Contains(output, "下载=2") {
		t.Errorf("expected 下载=2, got: %s", output)
	}
	if !strings.Contains(output, "跳过原因") {
		t.Errorf("expected 跳过原因, got: %s", output)
	}
	if !strings.Contains(output, "not free") {
		t.Errorf("expected 'not free' reason, got: %s", output)
	}
}

func TestPrintReport_TruncatesLongReason(t *testing.T) {
	longReason := strings.Repeat("x", 80)
	r := &mteam.RunReport{
		Scanned:    1,
		Rejections: map[string]int{longReason: 1},
	}
	old := os.Stdout
	rd, w, _ := os.Pipe()
	os.Stdout = w
	printReport(r)
	w.Close()
	out, _ := io.ReadAll(rd)
	os.Stdout = old
	output := string(out)
	// trunc(reason, 40) should cut the 80-char reason.
	if strings.Contains(output, longReason) {
		t.Errorf("expected reason to be truncated, got full: %s", output)
	}
}
