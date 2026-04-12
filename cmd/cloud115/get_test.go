package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestDownloadToFileSuccess tests that downloadToFile saves content correctly.
func TestDownloadToFileSuccess(t *testing.T) {
	content := "hello from the download"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, content)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "download.txt")
	err := downloadToFile(srv.URL+"/file.txt", dest)
	if err != nil {
		t.Fatalf("downloadToFile returned error: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("could not read dest file: %v", err)
	}
	if string(data) != content {
		t.Errorf("file content = %q, want %q", string(data), content)
	}
}

// TestDownloadToFileHTTPError tests that a non-200 response returns an error.
func TestDownloadToFileHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "should_not_exist.txt")
	err := downloadToFile(srv.URL+"/missing", dest)
	if err == nil {
		t.Error("expected error for HTTP 404, got nil")
	}
}

// TestDownloadToFileConnectionError tests that a bad URL returns an error.
func TestDownloadToFileConnectionError(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "no_file.txt")
	// Port 1 should refuse connections.
	err := downloadToFile("http://127.0.0.1:1/file", dest)
	if err == nil {
		t.Error("expected connection error, got nil")
	}
}

// TestDownloadToFileInvalidDestination tests that an invalid destination path fails.
func TestDownloadToFileInvalidDestination(t *testing.T) {
	content := "data"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, content)
	}))
	defer srv.Close()

	// Use a path inside a non-existent directory.
	err := downloadToFile(srv.URL+"/file.txt", "/nonexistent-dir/subdir/file.txt")
	if err == nil {
		t.Error("expected error for invalid destination, got nil")
	}
}

// TestDownloadToFile500 tests that HTTP 500 returns an error.
func TestDownloadToFile500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	dest := filepath.Join(t.TempDir(), "err.txt")
	err := downloadToFile(srv.URL+"/", dest)
	if err == nil {
		t.Error("expected error for HTTP 500, got nil")
	}
}
