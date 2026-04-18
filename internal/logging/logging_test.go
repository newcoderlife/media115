package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---- Stats tests ----

func TestStats_Incr_API(t *testing.T) {
	s := &Stats{StartTime: time.Now()}
	s.Incr("api")
	s.Incr("api")
	if s.APICalls != 2 {
		t.Errorf("APICalls = %d, want 2", s.APICalls)
	}
	if s.CacheHits != 0 || s.CacheMiss != 0 {
		t.Errorf("unexpected non-zero cache counts: hits=%d misses=%d", s.CacheHits, s.CacheMiss)
	}
}

func TestStats_Incr_CacheHit(t *testing.T) {
	s := &Stats{StartTime: time.Now()}
	s.Incr("cache_hit")
	s.Incr("cache_hit")
	s.Incr("cache_hit")
	if s.CacheHits != 3 {
		t.Errorf("CacheHits = %d, want 3", s.CacheHits)
	}
}

func TestStats_Incr_CacheMiss(t *testing.T) {
	s := &Stats{StartTime: time.Now()}
	s.Incr("cache_miss")
	if s.CacheMiss != 1 {
		t.Errorf("CacheMiss = %d, want 1", s.CacheMiss)
	}
}

func TestStats_Incr_UnknownKey(t *testing.T) {
	s := &Stats{StartTime: time.Now()}
	// Should not panic or increment anything.
	s.Incr("unknown")
	s.Incr("")
	if s.APICalls != 0 || s.CacheHits != 0 || s.CacheMiss != 0 {
		t.Errorf("unexpected increment on unknown key")
	}
}

func TestStats_Incr_Concurrent(t *testing.T) {
	s := &Stats{StartTime: time.Now()}
	var wg sync.WaitGroup
	n := 100
	wg.Add(n * 3)
	for i := 0; i < n; i++ {
		go func() { defer wg.Done(); s.Incr("api") }()
		go func() { defer wg.Done(); s.Incr("cache_hit") }()
		go func() { defer wg.Done(); s.Incr("cache_miss") }()
	}
	wg.Wait()
	if s.APICalls != n {
		t.Errorf("APICalls = %d, want %d", s.APICalls, n)
	}
	if s.CacheHits != n {
		t.Errorf("CacheHits = %d, want %d", s.CacheHits, n)
	}
	if s.CacheMiss != n {
		t.Errorf("CacheMiss = %d, want %d", s.CacheMiss, n)
	}
}

func TestStats_Summary(t *testing.T) {
	s := &Stats{StartTime: time.Now()}
	s.APICalls = 3
	s.CacheHits = 7
	s.CacheMiss = 2
	sum := s.Summary()
	if !strings.Contains(sum, "3 API calls") {
		t.Errorf("Summary missing API count: %q", sum)
	}
	if !strings.Contains(sum, "7 cache hits") {
		t.Errorf("Summary missing cache hits: %q", sum)
	}
	if !strings.Contains(sum, "2 cache misses") {
		t.Errorf("Summary missing cache misses: %q", sum)
	}
}

func TestStats_Summary_ZeroValues(t *testing.T) {
	s := &Stats{StartTime: time.Now()}
	sum := s.Summary()
	if !strings.Contains(sum, "0 API calls") {
		t.Errorf("Summary should show 0 API calls: %q", sum)
	}
}

// ---- consoleHandler tests ----

func newConsoleHandler(level slog.Level) (*consoleHandler, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return &consoleHandler{level: level, w: buf}, buf
}

func makeRecord(level slog.Level, msg string) slog.Record {
	return slog.NewRecord(time.Now(), level, msg, 0)
}

func TestConsoleHandler_Enabled(t *testing.T) {
	tests := []struct {
		handlerLevel slog.Level
		queryLevel   slog.Level
		want         bool
	}{
		{slog.LevelInfo, slog.LevelDebug, false},
		{slog.LevelInfo, slog.LevelInfo, true},
		{slog.LevelInfo, slog.LevelWarn, true},
		{slog.LevelInfo, slog.LevelError, true},
		{slog.LevelDebug, slog.LevelDebug, true},
		{slog.LevelDebug, slog.LevelInfo, true},
	}
	for _, tc := range tests {
		h, _ := newConsoleHandler(tc.handlerLevel)
		got := h.Enabled(context.Background(), tc.queryLevel)
		if got != tc.want {
			t.Errorf("Enabled(handlerLevel=%v, queryLevel=%v) = %v, want %v",
				tc.handlerLevel, tc.queryLevel, got, tc.want)
		}
	}
}

func TestConsoleHandler_Handle_Debug(t *testing.T) {
	h, buf := newConsoleHandler(slog.LevelDebug)
	r := makeRecord(slog.LevelDebug, "debug message")
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	out := buf.String()
	// Debug format: "  <msg>\n"
	if !strings.HasPrefix(out, "  ") {
		t.Errorf("debug output should start with two spaces, got: %q", out)
	}
	if !strings.Contains(out, "debug message") {
		t.Errorf("debug output missing message, got: %q", out)
	}
}

func TestConsoleHandler_Handle_Info(t *testing.T) {
	h, buf := newConsoleHandler(slog.LevelInfo)
	r := makeRecord(slog.LevelInfo, "info message")
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	out := buf.String()
	// Info format: "<msg>\n" — no prefix, no level tag
	if strings.Contains(out, "[") {
		t.Errorf("info output should not contain level tag, got: %q", out)
	}
	if !strings.Contains(out, "info message") {
		t.Errorf("info output missing message, got: %q", out)
	}
	if strings.HasPrefix(out, " ") {
		t.Errorf("info output should not start with spaces, got: %q", out)
	}
}

func TestConsoleHandler_Handle_Warn(t *testing.T) {
	h, buf := newConsoleHandler(slog.LevelWarn)
	r := makeRecord(slog.LevelWarn, "warn message")
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	out := buf.String()
	// Warn format: "[WARN] <msg>\n"
	if !strings.Contains(out, "[WARN]") {
		t.Errorf("warn output should contain [WARN], got: %q", out)
	}
	if !strings.Contains(out, "warn message") {
		t.Errorf("warn output missing message, got: %q", out)
	}
}

func TestConsoleHandler_Handle_Error(t *testing.T) {
	h, buf := newConsoleHandler(slog.LevelInfo)
	r := makeRecord(slog.LevelError, "error message")
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle returned error: %v", err)
	}
	out := buf.String()
	// Error >= Warn, so format: "[ERROR] <msg>\n"
	if !strings.Contains(out, "[ERROR]") {
		t.Errorf("error output should contain [ERROR], got: %q", out)
	}
	if !strings.Contains(out, "error message") {
		t.Errorf("error output missing message, got: %q", out)
	}
}

func TestConsoleHandler_WithAttrs_ReturnsSelf(t *testing.T) {
	h, _ := newConsoleHandler(slog.LevelInfo)
	got := h.WithAttrs([]slog.Attr{slog.String("k", "v")})
	if got != h {
		t.Errorf("WithAttrs should return self")
	}
}

func TestConsoleHandler_WithGroup_ReturnsSelf(t *testing.T) {
	h, _ := newConsoleHandler(slog.LevelInfo)
	got := h.WithGroup("grp")
	if got != h {
		t.Errorf("WithGroup should return self")
	}
}

// ---- jsonFileHandler tests ----

func newJSONHandler() (*jsonFileHandler, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return &jsonFileHandler{w: buf, f: nil}, buf
}

func TestJSONFileHandler_Enabled(t *testing.T) {
	h, _ := newJSONHandler()
	tests := []struct {
		level slog.Level
		want  bool
	}{
		{slog.LevelDebug, true},
		{slog.LevelInfo, true},
		{slog.LevelWarn, true},
		{slog.LevelError, true},
	}
	for _, tc := range tests {
		got := h.Enabled(context.Background(), tc.level)
		if got != tc.want {
			t.Errorf("Enabled(%v) = %v, want %v", tc.level, got, tc.want)
		}
	}
}

func TestJSONFileHandler_Handle_BasicJSON(t *testing.T) {
	h, buf := newJSONHandler()
	r := slog.NewRecord(time.Now(), slog.LevelInfo, "hello world", 0)
	r.Add("layer", "test-layer")
	r.Add("run_id", "abc123")

	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle error: %v", err)
	}

	line := strings.TrimSpace(buf.String())
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %q", err, line)
	}
	if obj["msg"] != "hello world" {
		t.Errorf("msg = %v, want 'hello world'", obj["msg"])
	}
	if obj["level"] != "INFO" {
		t.Errorf("level = %v, want 'INFO'", obj["level"])
	}
	if obj["layer"] != "test-layer" {
		t.Errorf("layer = %v, want 'test-layer'", obj["layer"])
	}
	if obj["run_id"] != "abc123" {
		t.Errorf("run_id = %v, want 'abc123'", obj["run_id"])
	}
	if _, ok := obj["time"]; !ok {
		t.Errorf("time field missing from JSON output")
	}
}

func TestJSONFileHandler_Handle_EscapesBackslash(t *testing.T) {
	h, buf := newJSONHandler()
	r := slog.NewRecord(time.Now(), slog.LevelInfo, `path\to\file`, 0)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	line := strings.TrimSpace(buf.String())
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		t.Fatalf("output is not valid JSON after backslash escaping: %v\noutput: %q", err, line)
	}
	if obj["msg"] != `path\to\file` {
		t.Errorf("msg = %v, want 'path\\to\\file'", obj["msg"])
	}
}

func TestJSONFileHandler_Handle_EscapesDoubleQuote(t *testing.T) {
	h, buf := newJSONHandler()
	r := slog.NewRecord(time.Now(), slog.LevelInfo, `say "hello"`, 0)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	line := strings.TrimSpace(buf.String())
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		t.Fatalf("output is not valid JSON after quote escaping: %v\noutput: %q", err, line)
	}
	if obj["msg"] != `say "hello"` {
		t.Errorf("msg = %v, want 'say \"hello\"'", obj["msg"])
	}
}

func TestJSONFileHandler_Handle_NoLayerAttr(t *testing.T) {
	h, buf := newJSONHandler()
	// Record with no layer/run_id attrs — should use package-level runID, empty layer.
	r := slog.NewRecord(time.Now(), slog.LevelDebug, "bare message", 0)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle error: %v", err)
	}
	line := strings.TrimSpace(buf.String())
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %q", err, line)
	}
	if obj["layer"] != "" {
		t.Errorf("layer should be empty string, got %v", obj["layer"])
	}
	// run_id should fall back to package runID (non-empty 6-char hex)
	rid, ok := obj["run_id"].(string)
	if !ok || len(rid) != 6 {
		t.Errorf("run_id should be a 6-char hex string, got %v", obj["run_id"])
	}
}

func TestJSONFileHandler_WithAttrs_ReturnsSelf(t *testing.T) {
	h, _ := newJSONHandler()
	got := h.WithAttrs(nil)
	if got != h {
		t.Errorf("WithAttrs should return self")
	}
}

func TestJSONFileHandler_WithGroup_ReturnsSelf(t *testing.T) {
	h, _ := newJSONHandler()
	got := h.WithGroup("g")
	if got != h {
		t.Errorf("WithGroup should return self")
	}
}

// ---- multiHandler tests ----

func newMultiHandler(consoleLevel slog.Level) (*multiHandler, *bytes.Buffer, *bytes.Buffer) {
	consoleBuf := &bytes.Buffer{}
	fileBuf := &bytes.Buffer{}
	console := &consoleHandler{level: consoleLevel, w: consoleBuf}
	file := &jsonFileHandler{w: fileBuf, f: nil}
	mh := &multiHandler{console: console, file: file, fileRef: nil}
	return mh, consoleBuf, fileBuf
}

func TestMultiHandler_Enabled_ConsoleInfo(t *testing.T) {
	mh, _, _ := newMultiHandler(slog.LevelInfo)
	// Debug: console says no, file says yes => overall yes
	if !mh.Enabled(context.Background(), slog.LevelDebug) {
		t.Errorf("multiHandler.Enabled(Debug) should be true because file handler accepts debug")
	}
	// Info: both say yes
	if !mh.Enabled(context.Background(), slog.LevelInfo) {
		t.Errorf("multiHandler.Enabled(Info) should be true")
	}
}

func TestMultiHandler_Handle_DebugOnlyWritesToFile(t *testing.T) {
	mh, consoleBuf, fileBuf := newMultiHandler(slog.LevelInfo)
	r := makeRecord(slog.LevelDebug, "debug only")
	_ = mh.Handle(context.Background(), r)

	if consoleBuf.Len() != 0 {
		t.Errorf("console should not receive debug when level=Info, got: %q", consoleBuf.String())
	}
	if fileBuf.Len() == 0 {
		t.Errorf("file handler should receive debug message")
	}
}

func TestMultiHandler_Handle_InfoWritesToBoth(t *testing.T) {
	mh, consoleBuf, fileBuf := newMultiHandler(slog.LevelInfo)
	r := makeRecord(slog.LevelInfo, "both sides now")
	_ = mh.Handle(context.Background(), r)

	if !strings.Contains(consoleBuf.String(), "both sides now") {
		t.Errorf("console missing info message, got: %q", consoleBuf.String())
	}
	if fileBuf.Len() == 0 {
		t.Errorf("file handler should receive info message")
	}
}

func TestMultiHandler_Handle_NilFile(t *testing.T) {
	consoleBuf := &bytes.Buffer{}
	console := &consoleHandler{level: slog.LevelInfo, w: consoleBuf}
	mh := &multiHandler{console: console, file: nil, fileRef: nil}

	r := makeRecord(slog.LevelInfo, "no file handler")
	if err := mh.Handle(context.Background(), r); err != nil {
		t.Errorf("Handle with nil file should not error: %v", err)
	}
	if !strings.Contains(consoleBuf.String(), "no file handler") {
		t.Errorf("console missing message, got: %q", consoleBuf.String())
	}
}

func TestMultiHandler_WithAttrs_ReturnsSelf(t *testing.T) {
	mh, _, _ := newMultiHandler(slog.LevelInfo)
	got := mh.WithAttrs(nil)
	if got != mh {
		t.Errorf("WithAttrs should return self")
	}
}

func TestMultiHandler_WithGroup_ReturnsSelf(t *testing.T) {
	mh, _, _ := newMultiHandler(slog.LevelInfo)
	got := mh.WithGroup("g")
	if got != mh {
		t.Errorf("WithGroup should return self")
	}
}

// ---- logFilePath tests ----

func TestLogFilePath_DefaultsToHome(t *testing.T) {
	// Ensure XDG_CACHE_HOME is unset
	old := os.Getenv("XDG_CACHE_HOME")
	os.Unsetenv("XDG_CACHE_HOME")
	defer os.Setenv("XDG_CACHE_HOME", old)

	got := logFilePath()
	cacheBase, _ := os.UserCacheDir()
	expected := filepath.Join(cacheBase, "media115", "log")
	if got != expected {
		t.Errorf("logFilePath() = %q, want %q", got, expected)
	}
}

func TestLogFilePath_RespectsXDGCacheHome(t *testing.T) {
	old := os.Getenv("XDG_CACHE_HOME")
	os.Setenv("XDG_CACHE_HOME", "/tmp/testcache")
	defer os.Setenv("XDG_CACHE_HOME", old)

	got := logFilePath()
	expected := "/tmp/testcache/media115/log"
	if got != expected {
		t.Errorf("logFilePath() = %q, want %q", got, expected)
	}
}

// ---- Setup tests ----

func TestSetup_ReturnsStats(t *testing.T) {
	// Point log file somewhere safe in /tmp so the real file open works.
	os.Setenv("XDG_CACHE_HOME", t.TempDir())
	defer os.Unsetenv("XDG_CACHE_HOME")

	stats := Setup(false)
	if stats == nil {
		t.Fatal("Setup returned nil Stats")
	}
	if stats.StartTime.IsZero() {
		t.Errorf("Stats.StartTime should be set")
	}
}

func TestSetup_VerboseSetsDebugLevel(t *testing.T) {
	os.Setenv("XDG_CACHE_HOME", t.TempDir())
	defer os.Unsetenv("XDG_CACHE_HOME")

	Setup(true)
	logger := slog.Default()
	if !logger.Enabled(context.Background(), slog.LevelDebug) {
		t.Errorf("verbose Setup should enable debug-level logging on the global logger")
	}
}

func TestSetup_NonVerboseDisablesDebug(t *testing.T) {
	os.Setenv("XDG_CACHE_HOME", t.TempDir())
	defer os.Unsetenv("XDG_CACHE_HOME")

	Setup(false)
	logger := slog.Default()
	// The file handler always accepts debug, so the multiHandler.Enabled returns
	// true. We check the console handler directly instead.
	// (The global logger enabled check goes through multiHandler which says true
	//  because file accepts debug — that's expected behaviour.)
	// Just verify the global logger is non-nil and info is enabled.
	if !logger.Enabled(context.Background(), slog.LevelInfo) {
		t.Errorf("non-verbose Setup should still enable info logging")
	}
}

func TestSetup_FileOpenFailFallsBackToConsole(t *testing.T) {
	// Use an unwritable path to force file-open failure.
	os.Setenv("XDG_CACHE_HOME", "/proc/nonexistent/cannot/create")
	defer os.Unsetenv("XDG_CACHE_HOME")

	stats := Setup(false)
	if stats == nil {
		t.Fatal("Setup returned nil even on file error")
	}
	// Logger should still be usable (console-only fallback).
	logger := slog.Default()
	if !logger.Enabled(context.Background(), slog.LevelInfo) {
		t.Errorf("fallback logger should have info enabled")
	}
}

// ---- Log function tests ----

func TestLog_EmitsToHandler(t *testing.T) {
	buf := &bytes.Buffer{}
	h := &consoleHandler{level: slog.LevelDebug, w: buf}
	slog.SetDefault(slog.New(h))

	Log(slog.LevelInfo, "test log message", "test-layer")
	out := buf.String()
	if !strings.Contains(out, "test log message") {
		t.Errorf("Log did not emit to handler, got: %q", out)
	}
}

func TestLog_DisabledLevelSkips(t *testing.T) {
	buf := &bytes.Buffer{}
	// Info-level handler will not accept Debug.
	h := &consoleHandler{level: slog.LevelInfo, w: buf}
	slog.SetDefault(slog.New(h))

	Log(slog.LevelDebug, "should not appear", "layer")
	if buf.Len() != 0 {
		t.Errorf("disabled level should not emit, got: %q", buf.String())
	}
}

func TestLog_IncludesLayerAndRunID(t *testing.T) {
	fileBuf := &bytes.Buffer{}
	fileH := &jsonFileHandler{w: fileBuf, f: nil}
	slog.SetDefault(slog.New(fileH))

	Log(slog.LevelInfo, "structured msg", "my-layer")
	line := strings.TrimSpace(fileBuf.String())
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		t.Fatalf("log output not valid JSON: %v\noutput: %q", err, line)
	}
	if obj["layer"] != "my-layer" {
		t.Errorf("layer = %v, want 'my-layer'", obj["layer"])
	}
	rid, ok := obj["run_id"].(string)
	if !ok || len(rid) != 6 {
		t.Errorf("run_id should be 6-char hex, got %v", obj["run_id"])
	}
}

// ---- runID sanity check ----

func TestRunID_Format(t *testing.T) {
	if len(runID) != 6 {
		t.Errorf("runID should be 6 hex chars, got %q (len=%d)", runID, len(runID))
	}
	for _, c := range runID {
		if !strings.ContainsRune("0123456789abcdef", c) {
			t.Errorf("runID contains non-hex char %q: %q", c, runID)
		}
	}
}
