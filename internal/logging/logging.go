package logging

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
)

// Setup creates a logger that writes to both stderr and a file.
// Console level: Info by default, Debug if verbose is true.
// File level: always Debug, written to ~/.cache/cloud115/logs.
func Setup(verbose bool) *slog.Logger {
	// Console handler: Info by default, Debug if verbose
	consoleLevel := slog.LevelInfo
	if verbose {
		consoleLevel = slog.LevelDebug
	}
	consoleHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: consoleLevel})

	// File handler: always Debug
	logPath := logFilePath()
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		// Fall back to console only
		return slog.New(consoleHandler)
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		// Fall back to console only
		return slog.New(consoleHandler)
	}
	fileHandler := slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug})

	// Multi-handler: write to both console and file
	return slog.New(multiHandler{consoleHandler, fileHandler})
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

// multiHandler writes to multiple slog.Handler instances.
type multiHandler struct {
	console slog.Handler
	file    slog.Handler
}

func (m multiHandler) Enabled(_ context.Context, level slog.Level) bool {
	return m.console.Enabled(nil, level) || m.file.Enabled(nil, level)
}

func (m multiHandler) Handle(ctx context.Context, r slog.Record) error {
	_ = m.console.Handle(ctx, r)
	_ = m.file.Handle(ctx, r)
	return nil
}

func (m multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return multiHandler{m.console.WithAttrs(attrs), m.file.WithAttrs(attrs)}
}

func (m multiHandler) WithGroup(name string) slog.Handler {
	return multiHandler{m.console.WithGroup(name), m.file.WithGroup(name)}
}
