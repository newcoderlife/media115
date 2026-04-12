package main

import "testing"

func TestServeCommandExists(t *testing.T) {
	// Just verify the command is registered
	cmd := rootCmd
	serve, _, err := cmd.Find([]string{"serve"})
	if err != nil {
		t.Fatal("serve command not found")
	}
	if serve.Use != "serve" {
		t.Fatalf("expected 'serve', got %q", serve.Use)
	}
}
