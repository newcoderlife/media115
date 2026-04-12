package logging

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var runID string

func init() {
	b := make([]byte, 3)
	rand.Read(b)
	runID = hex.EncodeToString(b) // 6 chars
}

// Stats tracks API/cache metrics per command invocation.
type Stats struct {
	mu        sync.Mutex
	APICalls  int
	CacheHits int
	CacheMiss int
	StartTime time.Time
}

func (s *Stats) Incr(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch key {
	case "api":
		s.APICalls++
	case "cache_hit":
		s.CacheHits++
	case "cache_miss":
		s.CacheMiss++
	}
}

func (s *Stats) Summary() string {
	elapsed := time.Since(s.StartTime).Round(time.Millisecond)
	return fmt.Sprintf("%d API calls, %d cache hits, %d cache misses, %s",
		s.APICalls, s.CacheHits, s.CacheMiss, elapsed)
}

// Setup creates a logger with console + JSON file handlers and sets it as the
// global default via slog.SetDefault.
// Console: prints only msg to stderr (Info default, Debug if verbose).
// File: JSON with time/level/msg/layer/run_id to ~/.cache/cloud115/logs.
func Setup(verbose bool) *Stats {
	stats := &Stats{StartTime: time.Now()}

	consoleLevel := slog.LevelInfo
	if verbose {
		consoleLevel = slog.LevelDebug
	}

	console := &consoleHandler{level: consoleLevel, w: os.Stderr}

	// File handler
	logPath := logFilePath()
	os.MkdirAll(filepath.Dir(logPath), 0o755)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)

	var handler slog.Handler
	if err != nil {
		handler = console
	} else {
		file := &jsonFileHandler{w: f, f: f}
		handler = &multiHandler{console: console, file: file, fileRef: f}
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	return stats
}

// Log emits a structured log message using the global slog.Default() logger.
func Log(level slog.Level, msg string, layer string) {
	logger := slog.Default()
	if !logger.Enabled(context.Background(), level) {
		return
	}
	r := slog.NewRecord(time.Now(), level, msg, 0)
	r.Add("layer", layer)
	r.Add("run_id", runID)
	logger.Handler().Handle(context.Background(), r)
}

func logFilePath() string {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".cache")
	}
	return filepath.Join(base, "cloud115", "logs")
}

// ---- Console handler: only prints msg ----

type consoleHandler struct {
	level slog.Level
	w     io.Writer
}

func (h *consoleHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *consoleHandler) Handle(_ context.Context, r slog.Record) error {
	if r.Level == slog.LevelDebug {
		fmt.Fprintf(h.w, "  %s\n", r.Message)
	} else if r.Level >= slog.LevelWarn {
		fmt.Fprintf(h.w, "[%s] %s\n", r.Level.String(), r.Message)
	} else {
		// Info — clean output
		fmt.Fprintf(h.w, "%s\n", r.Message)
	}
	return nil
}

func (h *consoleHandler) WithAttrs(attrs []slog.Attr) slog.Handler { return h }
func (h *consoleHandler) WithGroup(name string) slog.Handler       { return h }

// ---- JSON file handler: fixed schema ----

type jsonFileHandler struct {
	w  io.Writer
	f  *os.File
	mu sync.Mutex
}

func (h *jsonFileHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelDebug
}

func (h *jsonFileHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Extract layer and run_id from attrs
	layer := ""
	rid := runID
	r.Attrs(func(a slog.Attr) bool {
		switch a.Key {
		case "layer":
			layer = a.Value.String()
		case "run_id":
			rid = a.Value.String()
		}
		return true
	})

	// Write fixed-schema JSON
	// Escape msg for JSON
	msgJSON := strings.ReplaceAll(r.Message, `\`, `\\`)
	msgJSON = strings.ReplaceAll(msgJSON, `"`, `\"`)

	line := fmt.Sprintf(`{"time":"%s","level":"%s","msg":"%s","layer":"%s","run_id":"%s"}`,
		r.Time.Format(time.RFC3339Nano),
		r.Level.String(),
		msgJSON,
		layer,
		rid,
	)
	fmt.Fprintln(h.w, line)

	if h.f != nil {
		_ = h.f.Sync()
	}
	return nil
}

func (h *jsonFileHandler) WithAttrs(attrs []slog.Attr) slog.Handler { return h }
func (h *jsonFileHandler) WithGroup(name string) slog.Handler       { return h }

// ---- Multi handler ----

type multiHandler struct {
	console *consoleHandler
	file    *jsonFileHandler
	fileRef *os.File
}

func (m *multiHandler) Enabled(_ context.Context, level slog.Level) bool {
	return m.console.Enabled(nil, level) || m.file.Enabled(nil, level)
}

func (m *multiHandler) Handle(ctx context.Context, r slog.Record) error {
	if m.console.Enabled(ctx, r.Level) {
		_ = m.console.Handle(ctx, r)
	}
	if m.file != nil && m.file.Enabled(ctx, r.Level) {
		_ = m.file.Handle(ctx, r)
	}
	return nil
}

func (m *multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler { return m }
func (m *multiHandler) WithGroup(name string) slog.Handler       { return m }
