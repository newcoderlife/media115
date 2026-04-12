package bangumi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/newcoderlife/media115/internal/provider/bangumi"
	"github.com/newcoderlife/media115/internal/scraper"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// subjectResponse wraps subject data into the /v0/subjects/{id} shape.
func subjectResponse() map[string]any {
	return map[string]any{
		"id":       float64(12345),
		"name":     "テスト アニメ",
		"name_cn":  "测试动画",
		"summary":  "A test anime summary.",
		"air_date": "2023-04-01",
		"rating": map[string]any{
			"score": float64(8.5),
			"total": float64(1200),
		},
		"images": map[string]any{
			"large":  "https://example.com/large.jpg",
			"medium": "https://example.com/medium.jpg",
		},
		"tags": []any{
			map[string]any{"name": "Action"},
			map[string]any{"name": "Comedy"},
		},
	}
}

// searchResponse wraps a slice of subjects into the /v0/search/subjects shape.
func searchResponse(subjects []any) map[string]any {
	return map[string]any{
		"data":  subjects,
		"total": float64(len(subjects)),
	}
}

// newTestServer starts an httptest.Server that handles both search and subject
// detail endpoints.  Pass an empty subjects slice to simulate "no results".
func newTestServer(t *testing.T, subjects []any, subject map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/v0/search/subjects":
			json.NewEncoder(w).Encode(searchResponse(subjects))
		case len(r.URL.Path) > len("/v0/subjects/") &&
			r.URL.Path[:len("/v0/subjects/")] == "/v0/subjects/":
			if subject != nil {
				json.NewEncoder(w).Encode(subject)
			} else {
				http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			}
		case r.URL.Path == "/v0/episodes":
			json.NewEncoder(w).Encode(map[string]any{
				"data":  []any{},
				"total": float64(0),
			})
		default:
			http.NotFound(w, r)
		}
	}))
	return srv
}

// ── Provider metadata ─────────────────────────────────────────────────────────

func TestProviderMetadata(t *testing.T) {
	p := bangumi.New("")
	if p.Name() != "bangumi" {
		t.Errorf("Name = %q, want bangumi", p.Name())
	}
	types := p.SupportedTypes()
	if len(types) == 0 {
		t.Fatal("SupportedTypes is empty")
	}
	found := false
	for _, ty := range types {
		if ty == "anime" {
			found = true
		}
	}
	if !found {
		t.Errorf("SupportedTypes = %v, want to contain anime", types)
	}
	if p.Priority() != 0 {
		t.Errorf("Priority = %d, want 0", p.Priority())
	}
}

// ── Search ────────────────────────────────────────────────────────────────────

func TestSearchWithResults(t *testing.T) {
	sub := subjectResponse()
	srv := newTestServer(t, []any{sub}, sub)
	defer srv.Close()

	p := bangumi.NewWithBaseURL("", srv.URL)
	results, err := p.Search("テスト アニメ", scraper.SearchOpts{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected search results, got 0")
	}
	if results[0].ID != "12345" {
		t.Errorf("ID = %q, want 12345", results[0].ID)
	}
	if results[0].Title != "测试动画" {
		t.Errorf("Title = %q, want 测试动画", results[0].Title)
	}
	if results[0].Year != 2023 {
		t.Errorf("Year = %d, want 2023", results[0].Year)
	}
}

func TestSearchFallbackToJapaneseName(t *testing.T) {
	// name_cn is absent; should fall back to "name".
	sub := map[string]any{
		"id":       float64(9999),
		"name":     "Japanese Only Title",
		"air_date": "2021-07-01",
	}
	srv := newTestServer(t, []any{sub}, sub)
	defer srv.Close()

	p := bangumi.NewWithBaseURL("", srv.URL)
	results, err := p.Search("Japanese Only Title", scraper.SearchOpts{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	if results[0].Title != "Japanese Only Title" {
		t.Errorf("Title = %q, want Japanese Only Title", results[0].Title)
	}
}

func TestSearchNoResults(t *testing.T) {
	srv := newTestServer(t, []any{}, nil)
	defer srv.Close()

	p := bangumi.NewWithBaseURL("", srv.URL)
	results, err := p.Search("nonexistent anime XYZ", scraper.SearchOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestSearchHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := bangumi.NewWithBaseURL("", srv.URL)
	_, err := p.Search("anything", scraper.SearchOpts{})
	if err == nil {
		t.Error("expected error for HTTP 500")
	}
}

func TestSearchMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not valid json{{"))
	}))
	defer srv.Close()

	p := bangumi.NewWithBaseURL("", srv.URL)
	_, err := p.Search("anything", scraper.SearchOpts{})
	if err == nil {
		t.Error("expected error for malformed JSON response")
	}
}

// ── Detail ────────────────────────────────────────────────────────────────────

func TestDetailFullSubject(t *testing.T) {
	sub := subjectResponse()
	srv := newTestServer(t, []any{}, sub)
	defer srv.Close()

	p := bangumi.NewWithBaseURL("", srv.URL)
	meta, err := p.Detail("12345")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if meta == nil {
		t.Fatal("expected metadata, got nil")
	}
	if meta.Title != "测试动画" {
		t.Errorf("Title = %q, want 测试动画", meta.Title)
	}
	if meta.OriginalTitle != "テスト アニメ" {
		t.Errorf("OriginalTitle = %q, want テスト アニメ", meta.OriginalTitle)
	}
	if meta.Year != 2023 {
		t.Errorf("Year = %d, want 2023", meta.Year)
	}
	if meta.Premiered != "2023-04-01" {
		t.Errorf("Premiered = %q, want 2023-04-01", meta.Premiered)
	}
	if meta.Rating != 8.5 {
		t.Errorf("Rating = %f, want 8.5", meta.Rating)
	}
	if meta.Votes != 1200 {
		t.Errorf("Votes = %d, want 1200", meta.Votes)
	}
	if meta.PosterURL != "https://example.com/large.jpg" {
		t.Errorf("PosterURL = %q", meta.PosterURL)
	}
	if len(meta.Tags) < 2 {
		t.Errorf("Tags = %v, expected at least 2", meta.Tags)
	}
	if meta.UniqueIDs["bangumi"] != "12345" {
		t.Errorf("UniqueIDs[bangumi] = %q, want 12345", meta.UniqueIDs["bangumi"])
	}
}

func TestDetailInvalidID(t *testing.T) {
	p := bangumi.NewWithBaseURL("", "http://localhost:9999")
	_, err := p.Detail("not-a-number")
	if err == nil {
		t.Error("expected error for non-numeric ID")
	}
}

func TestDetailSubjectNotFound(t *testing.T) {
	srv := newTestServer(t, []any{}, nil) // nil subject => 404
	defer srv.Close()

	p := bangumi.NewWithBaseURL("", srv.URL)
	_, err := p.Detail("99999")
	if err == nil {
		t.Error("expected error when subject not found")
	}
}

// ── SearchSubjects & Subject (API methods) ────────────────────────────────────

func TestSearchSubjectsReturnsSlice(t *testing.T) {
	sub := subjectResponse()
	srv := newTestServer(t, []any{sub, sub}, sub) // two results
	defer srv.Close()

	p := bangumi.NewWithBaseURL("", srv.URL)
	subjects, err := p.SearchSubjects("anime", 2, 10)
	if err != nil {
		t.Fatalf("SearchSubjects: %v", err)
	}
	if len(subjects) != 2 {
		t.Errorf("expected 2 subjects, got %d", len(subjects))
	}
}

func TestSubjectByID(t *testing.T) {
	sub := subjectResponse()
	srv := newTestServer(t, []any{}, sub)
	defer srv.Close()

	p := bangumi.NewWithBaseURL("", srv.URL)
	s, err := p.Subject(12345)
	if err != nil {
		t.Fatalf("Subject: %v", err)
	}
	if s["name"] != "テスト アニメ" {
		t.Errorf("name = %v", s["name"])
	}
}

func TestEpisodes(t *testing.T) {
	srv := newTestServer(t, []any{}, subjectResponse())
	defer srv.Close()

	p := bangumi.NewWithBaseURL("", srv.URL)
	eps, err := p.Episodes(12345, 0, 10, 0)
	if err != nil {
		t.Fatalf("Episodes: %v", err)
	}
	// The test server returns empty data array — just verify no error and slice returned.
	if eps == nil {
		t.Error("expected non-nil slice")
	}
}

func TestEpisodesNoTypeFilter(t *testing.T) {
	srv := newTestServer(t, []any{}, subjectResponse())
	defer srv.Close()

	p := bangumi.NewWithBaseURL("", srv.URL)
	// episodeType < 0 means no type filter
	eps, err := p.Episodes(12345, -1, 10, 0)
	if err != nil {
		t.Fatalf("Episodes (no type): %v", err)
	}
	if eps == nil {
		t.Error("expected non-nil slice")
	}
}

// ── Scrape ────────────────────────────────────────────────────────────────────

func TestScrapeWritesNFO(t *testing.T) {
	sub := subjectResponse()
	srv := newTestServer(t, []any{sub}, sub)
	defer srv.Close()

	outDir := t.TempDir()
	p := bangumi.NewWithBaseURL("", srv.URL)
	result, err := p.Scrape("测试动画", "test_anime.mkv", outDir, scraper.ScrapeOpts{Season: 1, Episode: 3})
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("Status = %q, error = %q", result.Status, result.Error)
	}
	if result.Match == "" {
		t.Error("Match is empty")
	}

	nfoPath := filepath.Join(outDir, "test_anime.nfo")
	if _, err := os.Stat(nfoPath); os.IsNotExist(err) {
		t.Errorf("NFO not created at %s", nfoPath)
	}

	// Verify tvshow.nfo is also created
	tvshowPath := filepath.Join(outDir, "tvshow.nfo")
	if _, err := os.Stat(tvshowPath); os.IsNotExist(err) {
		t.Errorf("tvshow.nfo not created at %s", tvshowPath)
	}
}

func TestScrapeNotFound(t *testing.T) {
	srv := newTestServer(t, []any{}, nil)
	defer srv.Close()

	outDir := t.TempDir()
	p := bangumi.NewWithBaseURL("", srv.URL)
	result, err := p.Scrape("no such anime", "not_found.mkv", outDir, scraper.ScrapeOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "not_found" {
		t.Errorf("Status = %q, want not_found", result.Status)
	}
}

func TestScrapeSearchError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	outDir := t.TempDir()
	p := bangumi.NewWithBaseURL("", srv.URL)
	result, err := p.Scrape("anything", "anime.mkv", outDir, scraper.ScrapeOpts{})
	if err == nil {
		t.Error("expected error")
	}
	if result.Status != "error" {
		t.Errorf("Status = %q, want error", result.Status)
	}
}

func TestScrapeSubjectDetailFallback(t *testing.T) {
	// Return a valid search result but make the detail endpoint fail (404).
	// Scrape should fall back to using the search result data.
	sub := subjectResponse()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v0/search/subjects" {
			json.NewEncoder(w).Encode(searchResponse([]any{sub}))
		} else {
			// All other requests (including /v0/subjects/{id}) return 500.
			http.Error(w, "detail error", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	outDir := t.TempDir()
	p := bangumi.NewWithBaseURL("", srv.URL)
	// Should not fail — falls back to search result
	result, err := p.Scrape("测试动画", "fallback.mkv", outDir, scraper.ScrapeOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("Status = %q, want ok (fallback)", result.Status)
	}
}

// ── Auth header ───────────────────────────────────────────────────────────────

func TestAuthorizationHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(searchResponse([]any{}))
	}))
	defer srv.Close()

	p := bangumi.NewWithBaseURL("my-secret-token", srv.URL)
	p.Search("anime", scraper.SearchOpts{})

	if gotAuth != "Bearer my-secret-token" {
		t.Errorf("Authorization header = %q, want Bearer my-secret-token", gotAuth)
	}
}

func TestNoAuthorizationHeaderWhenTokenEmpty(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(searchResponse([]any{}))
	}))
	defer srv.Close()

	p := bangumi.NewWithBaseURL("", srv.URL)
	p.Search("anime", scraper.SearchOpts{})

	if gotAuth != "" {
		t.Errorf("expected no Authorization header, got %q", gotAuth)
	}
}

// ── Edge cases ────────────────────────────────────────────────────────────────

func TestScrapeZeroBgmID(t *testing.T) {
	// Search result has id=0 — should return not_found.
	sub := map[string]any{
		"id":       float64(0),
		"name":     "Zero ID Anime",
		"air_date": "2020-01-01",
	}
	srv := newTestServer(t, []any{sub}, sub)
	defer srv.Close()

	outDir := t.TempDir()
	p := bangumi.NewWithBaseURL("", srv.URL)
	result, err := p.Scrape("Zero ID Anime", "zero.mkv", outDir, scraper.ScrapeOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "not_found" {
		t.Errorf("Status = %q, want not_found", result.Status)
	}
}

func TestDetailSubjectNoChineseName(t *testing.T) {
	// Subject with no name_cn — title should be the Japanese name.
	sub := map[string]any{
		"id":      float64(777),
		"name":    "只有日文名",
		"summary": "summary text",
	}
	srv := newTestServer(t, []any{}, sub)
	defer srv.Close()

	p := bangumi.NewWithBaseURL("", srv.URL)
	meta, err := p.Detail("777")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if meta.Title != "只有日文名" {
		t.Errorf("Title = %q, want 只有日文名", meta.Title)
	}
}
