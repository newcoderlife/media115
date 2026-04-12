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

// TestCacheCmdRegistered verifies the cache command is registered on root.
func TestCacheCmdRegistered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"cache"})
	if err != nil {
		t.Fatalf("cache command not found: %v", err)
	}
	if cmd == rootCmd {
		t.Fatal("cache command returned root")
	}
	if !strings.HasPrefix(cmd.Use, "cache") {
		t.Errorf("cache cmd.Use = %q, want prefix 'cache'", cmd.Use)
	}
}

// TestCacheCmdHasSubcommands verifies status and clear subcommands exist.
func TestCacheCmdHasSubcommands(t *testing.T) {
	cacheCmd, _, _ := rootCmd.Find([]string{"cache"})

	statusCmd, _, err := cacheCmd.Find([]string{"status"})
	if err != nil || statusCmd == cacheCmd {
		t.Error("cache status subcommand not found")
	}

	clearCmd, _, err := cacheCmd.Find([]string{"clear"})
	if err != nil || clearCmd == cacheCmd {
		t.Error("cache clear subcommand not found")
	}
}
