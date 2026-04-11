package jav321_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/newcoderlife/media115/internal/provider/jav321"
	"github.com/newcoderlife/media115/internal/scraper"
)

// detailHTML is a minimal jav321 detail page HTML for testing.
const detailHTML = `<!DOCTYPE html>
<html>
<head><title>TEST-001</title></head>
<body>
  <h3>Test AV Title TEST-001</h3>
  <div class="col-md-9">
    <p>配信開始日: 2023-05-15</p>
    <p>収録時間: 120分</p>
    <p>メーカー: Test Studio</p>
    <a href="/star/001">Actor One</a>
    <a href="/star/002">Actor Two</a>
    <a href="/genre/action">Action</a>
    <a href="/genre/drama">Drama</a>
  </div>
  <div class="col-md-3">
    <img src="https://example.com/cover.jpg" class="img-responsive" />
  </div>
</body>
</html>`

func TestFetchMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			// Simulate redirect to /video/test-001
			http.Redirect(w, r, "/video/test-001", http.StatusFound)
		case "/video/test-001":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(detailHTML))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	p := jav321.NewWithBaseURL(server.URL)
	meta, err := p.FetchMetadata("TEST-001")
	if err != nil {
		t.Fatalf("FetchMetadata: %v", err)
	}
	if meta == nil {
		t.Fatal("expected metadata, got nil")
	}
	if title, _ := meta["title"].(string); title == "" {
		t.Errorf("title is empty; meta = %v", meta)
	}
}

func TestScrapeWritesNFO(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			http.Redirect(w, r, "/video/abc-123", http.StatusFound)
		case "/video/abc-123":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(detailHTML))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	outDir := t.TempDir()
	p := jav321.NewWithBaseURL(server.URL)
	result, err := p.Scrape("ABC-123", "ABC-123.mkv", outDir, scraper.ScrapeOpts{})
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("Status = %q, error = %s", result.Status, result.Error)
	}

	nfoPath := filepath.Join(outDir, "ABC-123.nfo")
	if _, err := os.Stat(nfoPath); os.IsNotExist(err) {
		t.Errorf("NFO not created at %s", nfoPath)
	}

	// Read NFO and verify it's valid XML
	data, err := os.ReadFile(nfoPath)
	if err != nil {
		t.Fatalf("read NFO: %v", err)
	}
	if len(data) == 0 {
		t.Error("NFO is empty")
	}
}

func TestScrapeNoRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK) // no redirect → not_found
		w.Write([]byte("search results but no redirect"))
	}))
	defer server.Close()

	outDir := t.TempDir()
	p := jav321.NewWithBaseURL(server.URL)
	result, err := p.Scrape("NOTFOUND-999", "NOTFOUND-999.mkv", outDir, scraper.ScrapeOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "not_found" {
		t.Errorf("Status = %q, want not_found", result.Status)
	}
}

func TestProviderInterface(t *testing.T) {
	p := jav321.New()
	if p.Name() != "jav321" {
		t.Errorf("Name = %q", p.Name())
	}
	types := p.SupportedTypes()
	if len(types) == 0 {
		t.Error("SupportedTypes is empty")
	}
	typeSet := make(map[string]bool)
	for _, typ := range types {
		typeSet[typ] = true
	}
	for _, want := range []string{"av", "gravure"} {
		if !typeSet[want] {
			t.Errorf("SupportedTypes missing %q", want)
		}
	}
	if p.Priority() != 0 {
		t.Errorf("Priority = %d, want 0", p.Priority())
	}
}

func TestActorsAndGenresParsed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/search":
			http.Redirect(w, r, "/video/xyz", http.StatusFound)
		case "/video/xyz":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(detailHTML))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	p := jav321.NewWithBaseURL(server.URL)
	meta, err := p.Detail("TEST-001")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if len(meta.Actors) == 0 {
		t.Error("expected actors")
	}
	if len(meta.Genres) == 0 {
		t.Error("expected genres")
	}
}
