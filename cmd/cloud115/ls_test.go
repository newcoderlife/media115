package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/newcoderlife/media115/internal/cloud115"
)

// capturePrint captures stdout during the execution of fn.
func capturePrint(fn func()) string {
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	os.Stdout = w
	fn()
	w.Close()
	os.Stdout = oldStdout
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

func TestLsDirPrintShortMode(t *testing.T) {
	items := []cloud115.Entry{
		{Name: "folderA", Type: "dir"},
		{Name: "movie.mkv", Type: "file", Size: 1024 * 1024 * 100}, // 100 MB
		{Name: "show.mp4", Type: "file", Size: 2048},
	}

	output := capturePrint(func() {
		// nil client is safe when recursive=false
		lsDirPrint(nil, items, "/", false, false, 0, 0)
	})

	if !strings.Contains(output, "folderA/") {
		t.Errorf("expected 'folderA/' in output, got: %q", output)
	}
	if !strings.Contains(output, "movie.mkv") {
		t.Errorf("expected 'movie.mkv' in output, got: %q", output)
	}
	if !strings.Contains(output, "show.mp4") {
		t.Errorf("expected 'show.mp4' in output, got: %q", output)
	}
}

func TestLsDirPrintLongMode(t *testing.T) {
	items := []cloud115.Entry{
		{Name: "docs", Type: "dir"},
		{Name: "readme.txt", Type: "file", Size: 512},
	}

	output := capturePrint(func() {
		lsDirPrint(nil, items, "/", true, false, 0, 0)
	})

	// Dirs show type "d", files show type "f" with size.
	if !strings.Contains(output, "d") || !strings.Contains(output, "docs/") {
		t.Errorf("long mode: expected 'd  ... docs/' in output, got: %q", output)
	}
	if !strings.Contains(output, "f") || !strings.Contains(output, "readme.txt") {
		t.Errorf("long mode: expected 'f  ... readme.txt' in output, got: %q", output)
	}
	// Size should be formatted (512 bytes = 512B)
	if !strings.Contains(output, "512B") {
		t.Errorf("long mode: expected '512B' in output, got: %q", output)
	}
}

func TestLsDirPrintIndent(t *testing.T) {
	items := []cloud115.Entry{
		{Name: "sub", Type: "dir"},
	}

	output0 := capturePrint(func() {
		lsDirPrint(nil, items, "/", false, false, 0, 0)
	})
	output2 := capturePrint(func() {
		lsDirPrint(nil, items, "/", false, false, 0, 2)
	})

	// With indent=2, four spaces prefix (2 * "  ").
	if strings.HasPrefix(output0, " ") {
		t.Errorf("indent=0 should not have leading space, got: %q", output0)
	}
	if !strings.HasPrefix(output2, "    ") {
		t.Errorf("indent=2 should have 4-space prefix, got: %q", output2)
	}
}

func TestLsDirPrintEmpty(t *testing.T) {
	output := capturePrint(func() {
		lsDirPrint(nil, []cloud115.Entry{}, "/", false, false, 0, 0)
	})
	if output != "" {
		t.Errorf("empty items should produce no output, got: %q", output)
	}
}

func TestLsDirPrintLongDirNoSize(t *testing.T) {
	items := []cloud115.Entry{
		{Name: "mydir", Type: "dir"},
	}

	output := capturePrint(func() {
		lsDirPrint(nil, items, "/", true, false, 0, 0)
	})

	// Directories in long mode show size field as empty (8 spaces).
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	// Should contain "d" type indicator.
	if !strings.Contains(lines[0], "d") {
		t.Errorf("expected 'd' type in long listing, got: %q", lines[0])
	}
	// Should end with "/".
	if !strings.HasSuffix(strings.TrimSpace(lines[0]), "mydir/") {
		t.Errorf("dir in long mode should end with '/', got: %q", lines[0])
	}
}

func TestLsDirPrintMultipleFiles(t *testing.T) {
	items := []cloud115.Entry{
		{Name: "a.txt", Type: "file", Size: 100},
		{Name: "b.txt", Type: "file", Size: 200},
		{Name: "c.txt", Type: "file", Size: 300},
	}

	output := capturePrint(func() {
		lsDirPrint(nil, items, "/", false, false, 0, 0)
	})

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d: %q", len(lines), output)
	}
}
