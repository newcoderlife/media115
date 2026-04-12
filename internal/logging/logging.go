package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ── Stats ─────────────────────────────────────────────────────────────────────

// Stats tracks API and cache call counts for end-of-command summaries.
type Stats struct {
	mu        sync.Mutex
	APICalls  int
	CacheHits int
	CacheMiss int
	StartTime time.Time
}

// Incr increments one of the tracked counters.
// Valid keys: "api", "cache_hit", "cache_miss".
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

// Summary returns a one-line human-readable summary.
func (s *Stats) Summary() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	elapsed := time.Since(s.StartTime).Round(time.Millisecond)
	return fmt.Sprintf("%d API calls, %d cache hits, %d cache misses, %s",
		s.APICalls, s.CacheHits, s.CacheMiss, elapsed)
}

// ── consoleHandler ────────────────────────────────────────────────────────────

// consoleHandler is a human-readable slog.Handler for terminal output.
// Info messages are printed as-is (no timestamp, no level prefix).
// Debug messages are indented with attrs appended.
// Warn/Error messages are prefixed with [WARN]/[ERROR].
type consoleHandler struct {
	level slog.Level
	w     io.Writer
}

func (h *consoleHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *consoleHandler) Handle(_ context.Context, r slog.Record) error {
	if r.Level < h.level {
		return nil
	}

	var attrs []string
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, fmt.Sprintf("%s=%v", a.Key, a.Value))
		return true
	})

	attrStr := ""
	if len(attrs) > 0 {
		attrStr = " " + strings.Join(attrs, " ")
	}

	switch {
	case r.Level == slog.LevelDebug:
		fmt.Fprintf(h.w, "  %s%s\n", r.Message, attrStr)
	case r.Level == slog.LevelInfo:
		fmt.Fprintf(h.w, "%s%s\n", r.Message, attrStr)
	default:
		fmt.Fprintf(h.w, "[%s] %s%s\n", r.Level.String(), r.Message, attrStr)
	}
	return nil
}

func (h *consoleHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	// consoleHandler is stateless beyond level/writer; attrs are handled per-record.
	return h
}

func (h *consoleHandler) WithGroup(name string) slog.Handler {
	return h
}

// ── multiHandler ──────────────────────────────────────────────────────────────

// multiHandler writes each record to both a console and a file handler.
type multiHandler struct {
	console slog.Handler
	file    slog.Handler
	f       *os.File // kept to sync after each write
}

func (m multiHandler) Enabled(_ context.Context, level slog.Level) bool {
	return m.console.Enabled(nil, level) || m.file.Enabled(nil, level)
}

func (m multiHandler) Handle(ctx context.Context, r slog.Record) error {
	if m.console.Enabled(ctx, r.Level) {
		_ = m.console.Handle(ctx, r)
	}
	if m.file != nil && m.file.Enabled(ctx, r.Level) {
		_ = m.file.Handle(ctx, r)
		// Flush after every write so records are visible even if the
		// slog.JSONHandler buffers internally.
		if m.f != nil {
			_ = m.f.Sync()
		}
	}
	return nil
}

func (m multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return multiHandler{m.console.WithAttrs(attrs), m.file.WithAttrs(attrs), m.f}
}

func (m multiHandler) WithGroup(name string) slog.Handler {
	return multiHandler{m.console.WithGroup(name), m.file.WithGroup(name), m.f}
}

// ── Setup ─────────────────────────────────────────────────────────────────────

// Setup creates a logger that writes human-readable output to stderr and JSON
// to ~/.cache/cloud115/logs. It also returns a Stats tracker.
// Console level: Info by default, Debug if verbose is true.
// File level: always Debug.
func Setup(verbose bool) (*slog.Logger, *Stats) {
	stats := &Stats{StartTime: time.Now()}

	consoleLevel := slog.LevelInfo
	if verbose {
		consoleLevel = slog.LevelDebug
	}
	console := &consoleHandler{level: consoleLevel, w: os.Stderr}

	logPath := logFilePath()
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return slog.New(console), stats
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return slog.New(console), stats
	}

	fileHandler := slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug})
	handler := multiHandler{console: console, file: fileHandler, f: f}
	return slog.New(handler), stats
}

// logFilePath returns the path to the log file.
// File is ~/.cache/cloud115/logs (a file, not a directory).
func logFilePath() string {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = filepath.Join(home, ".cache")
	}
	return filepath.Join(base, "cloud115", "logs")
}
