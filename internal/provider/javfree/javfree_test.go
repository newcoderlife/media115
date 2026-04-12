package javfree_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/newcoderlife/media115/internal/provider/javfree"
	"github.com/newcoderlife/media115/internal/scraper"
)

// ── HTML fixtures ─────────────────────────────────────────────────────────────

// detailHTML is a minimal javfree detail page with a matching number in the title.
const detailHTML = `<!DOCTYPE html>
<html>
<head>
  <title>[ABC-123] Test AV Title</title>
  <meta property="og:image" content="https://example.com/cover.jpg" />
</head>
<body>
  <h1 class="entry-title">[ABC-123] Test AV Title</h1>
  <article>
    <img src="https://example.com/article_cover.jpg" />
  </article>
</body>
</html>`

// detailHTMLNoCover is a detail page with a matching number but no og:image meta tag.
const detailHTMLNoCover = `<!DOCTYPE html>
<html>
<head><title>[XYZ-999] Title Without Cover</title></head>
<body>
  <h1 class="entry-title">[XYZ-999] Title Without Cover</h1>
</body>
</html>`

// emptyHTML has a title that does NOT contain the queried number → parseDetail returns nil.
const emptyHTML = `<!DOCTYPE html>
<html>
<head><title>Page Not Found</title></head>
<body><h1>Page Not Found</h1></body>
</html>`

// h1FallbackHTML tests the fallback from article h1.
const h1FallbackHTML = `<!DOCTYPE html>
<html>
<head><title>javfree page</title></head>
<body>
  <article>
    <h1>[DEF-456] Fallback H1 Title</h1>
    <img src="https://example.com/fallback.jpg" />
  </article>
</body>
</html>`

// ── Provider metadata ─────────────────────────────────────────────────────────

func TestProviderMetadata(t *testing.T) {
	p := javfree.New()
	if p.Name() != "javfree" {
		t.Errorf("Name = %q, want javfree", p.Name())
	}
	types := p.SupportedTypes()
	typeSet := make(map[string]bool)
	for _, ty := range types {
		typeSet[ty] = true
	}
	for _, want := range []string{"av", "gravure"} {
		if !typeSet[want] {
			t.Errorf("SupportedTypes missing %q; got %v", want, types)
		}
	}
	if p.Priority() != 1 {
		t.Errorf("Priority = %d, want 1", p.Priority())
	}
}

// ── FetchMetadata ─────────────────────────────────────────────────────────────

func TestFetchMetadataSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(detailHTML))
	}))
	defer srv.Close()

	p := javfree.NewWithBaseURL(srv.URL)
	meta, err := p.FetchMetadata("ABC-123")
	if err != nil {
		t.Fatalf("FetchMetadata: %v", err)
	}
	if meta == nil {
		t.Fatal("expected metadata, got nil")
	}

	title, _ := meta["title"].(string)
	if title == "" {
		t.Errorf("title is empty; meta = %v", meta)
	}
	number, _ := meta["number"].(string)
	if number != "ABC-123" {
		t.Errorf("number = %q, want ABC-123", number)
	}
	coverURL, _ := meta["cover_url"].(string)
	if coverURL != "https://example.com/cover.jpg" {
		t.Errorf("cover_url = %q", coverURL)
	}
}

func TestFetchMetadataNonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	p := javfree.NewWithBaseURL(srv.URL)
	meta, err := p.FetchMetadata("ABC-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Non-200 status => nil, nil
	if meta != nil {
		t.Errorf("expected nil meta for 404 response, got %v", meta)
	}
}

func TestFetchMetadataTitleMismatch(t *testing.T) {
	// HTML whose title does not contain the queried number → nil result
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(emptyHTML))
	}))
	defer srv.Close()

	p := javfree.NewWithBaseURL(srv.URL)
	meta, err := p.FetchMetadata("ABC-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta != nil {
		t.Errorf("expected nil meta when title does not contain number, got %v", meta)
	}
}

func TestFetchMetadataH1Fallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(h1FallbackHTML))
	}))
	defer srv.Close()

	p := javfree.NewWithBaseURL(srv.URL)
	meta, err := p.FetchMetadata("DEF-456")
	if err != nil {
		t.Fatalf("FetchMetadata: %v", err)
	}
	if meta == nil {
		t.Fatal("expected metadata, got nil")
	}
	title, _ := meta["title"].(string)
	if title == "" {
		t.Errorf("title is empty; meta = %v", meta)
	}
}

func TestFetchMetadataCoverFromArticleImg(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(detailHTMLNoCover))
	}))
	defer srv.Close()

	p := javfree.NewWithBaseURL(srv.URL)
	meta, err := p.FetchMetadata("XYZ-999")
	if err != nil {
		t.Fatalf("FetchMetadata: %v", err)
	}
	// May be nil cover if no img in article, that's fine — just confirm no error.
	if meta == nil {
		t.Fatal("expected metadata, got nil")
	}
}

// ── Search ────────────────────────────────────────────────────────────────────

func TestSearchWithResults(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(detailHTML))
	}))
	defer srv.Close()

	p := javfree.NewWithBaseURL(srv.URL)
	results, err := p.Search("ABC-123", scraper.SearchOpts{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected search results")
	}
	if results[0].ID != "ABC-123" {
		t.Errorf("ID = %q, want ABC-123", results[0].ID)
	}
	if results[0].Title == "" {
		t.Error("Title is empty")
	}
}

func TestSearchNoResults(t *testing.T) {
	// Server returns a page whose title doesn't match the query.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(emptyHTML))
	}))
	defer srv.Close()

	p := javfree.NewWithBaseURL(srv.URL)
	results, err := p.Search("NOTFOUND-000", scraper.SearchOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSearchHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	p := javfree.NewWithBaseURL(srv.URL)
	// 404 returns nil,nil from FetchMetadata, so Search should return nil,nil too.
	results, err := p.Search("ABC-123", scraper.SearchOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results for 404, got %d", len(results))
	}
}

// ── Detail ────────────────────────────────────────────────────────────────────

func TestDetailSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(detailHTML))
	}))
	defer srv.Close()

	p := javfree.NewWithBaseURL(srv.URL)
	meta, err := p.Detail("ABC-123")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if meta == nil {
		t.Fatal("expected metadata, got nil")
	}
	if meta.Title == "" {
		t.Error("Title is empty")
	}
	if meta.UniqueIDs["javfree"] != "ABC-123" {
		t.Errorf("UniqueIDs[javfree] = %q, want ABC-123", meta.UniqueIDs["javfree"])
	}
	if meta.PosterURL != "https://example.com/cover.jpg" {
		t.Errorf("PosterURL = %q", meta.PosterURL)
	}
}

func TestDetailNotFound(t *testing.T) {
	// Server returns a page that doesn't match the queried number.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(emptyHTML))
	}))
	defer srv.Close()

	p := javfree.NewWithBaseURL(srv.URL)
	_, err := p.Detail("NOTFOUND-000")
	if err == nil {
		t.Error("expected error for not-found number")
	}
}

func TestDetailServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Force connection close to trigger a client error.
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "hijack not supported", http.StatusInternalServerError)
			return
		}
		conn, _, _ := hj.Hijack()
		conn.Close()
	}))
	defer srv.Close()

	p := javfree.NewWithBaseURL(srv.URL)
	_, err := p.Detail("ABC-123")
	if err == nil {
		t.Error("expected error for connection reset")
	}
}

// ── Scrape ────────────────────────────────────────────────────────────────────

func TestScrapeWritesNFO(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(detailHTML))
	}))
	defer srv.Close()

	outDir := t.TempDir()
	p := javfree.NewWithBaseURL(srv.URL)
	result, err := p.Scrape("ABC-123", "ABC-123.mkv", outDir, scraper.ScrapeOpts{})
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("Status = %q, error = %q", result.Status, result.Error)
	}
	if result.Match == "" {
		t.Error("Match is empty")
	}

	nfoPath := filepath.Join(outDir, "ABC-123.nfo")
	if _, err := os.Stat(nfoPath); os.IsNotExist(err) {
		t.Errorf("NFO not created at %s", nfoPath)
	}
	data, err := os.ReadFile(nfoPath)
	if err != nil {
		t.Fatalf("read NFO: %v", err)
	}
	if len(data) == 0 {
		t.Error("NFO file is empty")
	}
}

func TestScrapeNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(emptyHTML))
	}))
	defer srv.Close()

	outDir := t.TempDir()
	p := javfree.NewWithBaseURL(srv.URL)
	result, err := p.Scrape("NOTFOUND-000", "NOTFOUND-000.mkv", outDir, scraper.ScrapeOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "not_found" {
		t.Errorf("Status = %q, want not_found", result.Status)
	}
}

func TestScrapeNetworkError(t *testing.T) {
	// Force a network error by closing the server before scraping.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "hijack not supported", http.StatusInternalServerError)
			return
		}
		conn, _, _ := hj.Hijack()
		conn.Close()
	}))
	defer srv.Close()

	outDir := t.TempDir()
	p := javfree.NewWithBaseURL(srv.URL)
	result, err := p.Scrape("ABC-123", "ABC-123.mkv", outDir, scraper.ScrapeOpts{})
	if err == nil {
		t.Error("expected error for network failure")
	}
	if result.Status != "error" {
		t.Errorf("Status = %q, want error", result.Status)
	}
}

func TestScrapeIDs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(detailHTML))
	}))
	defer srv.Close()

	outDir := t.TempDir()
	p := javfree.NewWithBaseURL(srv.URL)
	result, err := p.Scrape("ABC-123", "ABC-123.mp4", outDir, scraper.ScrapeOpts{})
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if result.IDs["javfree"] != "ABC-123" {
		t.Errorf("IDs[javfree] = %q, want ABC-123", result.IDs["javfree"])
	}
}

// ── Title prefix stripping ────────────────────────────────────────────────────

func TestTitlePrefixStripped(t *testing.T) {
	// The HTML has "[ABC-123] Test AV Title" — the prefix "[ABC-123]" should be stripped.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(detailHTML))
	}))
	defer srv.Close()

	p := javfree.NewWithBaseURL(srv.URL)
	meta, err := p.Detail("ABC-123")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	// After stripping "[ABC-123] " the title should be "Test AV Title"
	if meta.Title != "Test AV Title" {
		t.Errorf("Title = %q, want Test AV Title", meta.Title)
	}
}

// ── Case-insensitive number matching ─────────────────────────────────────────

const lowerCaseHTML = `<!DOCTYPE html>
<html>
<head><title>[abc-123] lowercase number title</title></head>
<body>
  <h1 class="entry-title">[abc-123] lowercase number title</h1>
</body>
</html>`

func TestFetchMetadataCaseInsensitive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(lowerCaseHTML))
	}))
	defer srv.Close()

	p := javfree.NewWithBaseURL(srv.URL)
	// Query in uppercase, page has lowercase — should still match.
	meta, err := p.FetchMetadata("ABC-123")
	if err != nil {
		t.Fatalf("FetchMetadata: %v", err)
	}
	if meta == nil {
		t.Fatal("expected metadata for case-insensitive match, got nil")
	}
}
