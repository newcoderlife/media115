package tmdb_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/newcoderlife/media115/internal/provider/tmdb"
	"github.com/newcoderlife/media115/internal/scraper"
)

func TestSearchMovie(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/search/movie") {
			json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{
					{"id": 12345, "title": "Test Movie", "release_date": "2024-01-15"},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	p := tmdb.NewWithBaseURL("test-token", server.URL)
	results, err := p.Search("Test Movie", scraper.SearchOpts{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	if results[0].Title != "Test Movie" {
		t.Errorf("Title = %q, want %q", results[0].Title, "Test Movie")
	}
	if results[0].Year != 2024 {
		t.Errorf("Year = %d, want 2024", results[0].Year)
	}
}

func TestSearchMovieNoResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
	}))
	defer server.Close()

	p := tmdb.NewWithBaseURL("token", server.URL)
	results, err := p.Search("NonExistent", scraper.SearchOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

func TestMovieDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/movie/12345/credits"):
			json.NewEncoder(w).Encode(map[string]any{
				"crew": []map[string]any{
					{"name": "Jane Director", "job": "Director"},
				},
				"cast": []map[string]any{
					{"name": "Actor One", "character": "Hero", "profile_path": "/abc.jpg"},
				},
			})
		case strings.HasPrefix(r.URL.Path, "/movie/12345"):
			json.NewEncoder(w).Encode(map[string]any{
				"id":           12345,
				"title":        "Test Movie",
				"release_date": "2024-06-01",
				"overview":     "A great film",
				"runtime":      120,
				"vote_average": 7.5,
				"vote_count":   1000,
				"genres":       []map[string]any{{"name": "Action"}},
				"poster_path":  "/poster.jpg",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	p := tmdb.NewWithBaseURL("token", server.URL)
	meta, err := p.Detail("movie:12345")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if meta.Title != "Test Movie" {
		t.Errorf("Title = %q", meta.Title)
	}
	if meta.Runtime != 120 {
		t.Errorf("Runtime = %d, want 120", meta.Runtime)
	}
	if len(meta.Genres) == 0 || meta.Genres[0] != "Action" {
		t.Errorf("Genres = %v", meta.Genres)
	}
	if len(meta.Directors) == 0 || meta.Directors[0] != "Jane Director" {
		t.Errorf("Directors = %v", meta.Directors)
	}
	if len(meta.Actors) == 0 || meta.Actors[0].Name != "Actor One" {
		t.Errorf("Actors = %v", meta.Actors)
	}
}

func TestScrapeMovieWritesNFO(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/search/movie"):
			json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{
					{"id": 99, "title": "Scrape Test", "release_date": "2023-03-10"},
				},
			})
		case strings.Contains(r.URL.Path, "/movie/99/images"):
			json.NewEncoder(w).Encode(map[string]any{"posters": []any{}})
		case strings.Contains(r.URL.Path, "/movie/99/credits"):
			json.NewEncoder(w).Encode(map[string]any{"crew": []any{}, "cast": []any{}})
		case strings.Contains(r.URL.Path, "/movie/99"):
			json.NewEncoder(w).Encode(map[string]any{
				"id":           99,
				"title":        "Scrape Test",
				"release_date": "2023-03-10",
				"overview":     "Test plot",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	outDir := t.TempDir()
	p := tmdb.NewWithBaseURL("token", server.URL)
	result, err := p.Scrape("Scrape Test", "scrape_test.mkv", outDir, scraper.ScrapeOpts{})
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("Status = %q, want ok (error: %s)", result.Status, result.Error)
	}
	if result.Match != "Scrape Test" {
		t.Errorf("Match = %q", result.Match)
	}

	nfoPath := filepath.Join(outDir, "scrape_test.nfo")
	if _, err := os.Stat(nfoPath); os.IsNotExist(err) {
		t.Errorf("NFO not created at %s", nfoPath)
	}
}

func TestScrapeMovieNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
	}))
	defer server.Close()

	outDir := t.TempDir()
	p := tmdb.NewWithBaseURL("token", server.URL)
	result, err := p.Scrape("Unknown Film", "unknown.mkv", outDir, scraper.ScrapeOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "not_found" {
		t.Errorf("Status = %q, want not_found", result.Status)
	}
}

func TestProviderMetadata(t *testing.T) {
	p := tmdb.New("token")
	if p.Name() != "tmdb" {
		t.Errorf("Name = %q", p.Name())
	}
	types := p.SupportedTypes()
	typeSet := make(map[string]bool)
	for _, typ := range types {
		typeSet[typ] = true
	}
	for _, want := range []string{"movie", "tv", "anime"} {
		if !typeSet[want] {
			t.Errorf("SupportedTypes missing %q", want)
		}
	}
}

func TestSearchTV(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/search/tv") {
			json.NewEncoder(w).Encode(map[string]any{
				"results": []map[string]any{
					{"id": 555, "name": "Test Show", "first_air_date": "2022-09-01"},
				},
			})
			return
		}
		if strings.Contains(r.URL.Path, "/search/movie") {
			json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	p := tmdb.NewWithBaseURL("token", server.URL)
	results, err := p.Search("Test Show", scraper.SearchOpts{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	found := false
	for _, r := range results {
		if r.Title == "Test Show" {
			found = true
		}
	}
	if !found {
		t.Errorf("TV show not found in results: %v", results)
	}
}

func TestRateLimitRetry(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"results": []any{}})
	}))
	defer server.Close()

	p := tmdb.NewWithBaseURL("token", server.URL)
	_, err := p.Search("anything", scraper.SearchOpts{})
	if err != nil {
		t.Fatalf("unexpected error on retry: %v", err)
	}
	if callCount < 2 {
		t.Errorf("expected retry, got %d calls", callCount)
	}
}
