package main

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// TestShowCacheStatusNoLogin verifies that showCacheStatusNoLogin prints the
// expected message to stdout and returns nil.
func TestShowCacheStatusNoLogin(t *testing.T) {
	// Capture stdout.
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	result := showCacheStatusNoLogin()

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
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
