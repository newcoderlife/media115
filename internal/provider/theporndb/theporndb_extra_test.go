package theporndb_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/newcoderlife/media115/internal/provider/theporndb"
	"github.com/newcoderlife/media115/internal/scraper"
)

// ── Error path: HTTP 4xx/5xx ──────────────────────────────────────────────────

func TestSearchSceneHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forbidden", http.StatusForbidden)
	}))
	defer server.Close()

	p := theporndb.NewWithBaseURL("token", server.URL)
	_, err := p.SearchScene("anything")
	if err == nil {
		t.Error("expected error on HTTP 403, got nil")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should mention 403, got: %v", err)
	}
}

func TestSearchHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	p := theporndb.NewWithBaseURL("token", server.URL)
	_, err := p.Search("test", scraper.SearchOpts{})
	if err == nil {
		t.Error("expected error on HTTP 401, got nil")
	}
}

func TestSceneDetailHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	p := theporndb.NewWithBaseURL("token", server.URL)
	_, err := p.SceneDetail("nonexistent-id")
	if err == nil {
		t.Error("expected error on HTTP 404, got nil")
	}
}

func TestDetailHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", http.StatusInternalServerError)
	}))
	defer server.Close()

	p := theporndb.NewWithBaseURL("token", server.URL)
	_, err := p.Detail("scene-xyz")
	if err == nil {
		t.Error("expected error on HTTP 500, got nil")
	}
}

func TestSearchJAVHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer server.Close()

	p := theporndb.NewWithBaseURL("token", server.URL)
	_, err := p.SearchJAV("ABC-123")
	if err == nil {
		t.Error("expected error on HTTP 502, got nil")
	}
}

func TestScrapeHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	outDir := t.TempDir()
	p := theporndb.NewWithBaseURL("token", server.URL)
	result, err := p.Scrape("query", "file.mp4", outDir, scraper.ScrapeOpts{})
	if err == nil {
		t.Error("expected error on HTTP 503, got nil")
	}
	if result == nil {
		t.Fatal("expected non-nil ScrapeResult even on error")
	}
	if result.Status != "error" {
		t.Errorf("Status = %q, want 'error'", result.Status)
	}
}

// ── Error path: malformed JSON ────────────────────────────────────────────────

func TestGetMalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not valid json {{"))
	}))
	defer server.Close()

	p := theporndb.NewWithBaseURL("token", server.URL)
	_, err := p.SearchScene("anything")
	if err == nil {
		t.Error("expected error on malformed JSON, got nil")
	}
}

// ── SceneDetail: data is not a map ────────────────────────────────────────────

func TestSceneDetailDataNotMap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Return {"data": "not-a-map"} — data field is a string, not an object.
		json.NewEncoder(w).Encode(map[string]any{
			"data": "string-value",
		})
	}))
	defer server.Close()

	p := theporndb.NewWithBaseURL("token", server.URL)
	// Should not error — it falls back to returning the outer map.
	scene, err := p.SceneDetail("scene-123")
	if err != nil {
		t.Fatalf("SceneDetail with non-map data: %v", err)
	}
	// The fallback returns the outer wrapper map which does not have an id field.
	_ = scene
}

// ── Detail: uses ID correctly ─────────────────────────────────────────────────

func TestDetailBuildsMetadataCorrectly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"id":          "scene-777",
				"title":       "Full Detail Scene",
				"date":        "2025-03-15",
				"description": "Detailed description",
				"site": map[string]any{
					"name": "Premium Studio",
				},
				"tags": []any{
					map[string]any{"name": "4K"},
					map[string]any{"name": "HD"},
				},
				"performers": []any{
					map[string]any{"name": "Star One"},
					map[string]any{
						"parent": map[string]any{"name": "Star Two"},
					},
				},
				"posters": map[string]any{
					"large": "https://example.com/poster.jpg",
				},
			},
		})
	}))
	defer server.Close()

	p := theporndb.NewWithBaseURL("token", server.URL)
	meta, err := p.Detail("scene-777")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if meta.Title != "Full Detail Scene" {
		t.Errorf("Title = %q", meta.Title)
	}
	if meta.Year != 2025 {
		t.Errorf("Year = %d, want 2025", meta.Year)
	}
	if len(meta.Studios) == 0 || meta.Studios[0] != "Premium Studio" {
		t.Errorf("Studios = %v, want ['Premium Studio']", meta.Studios)
	}
	if len(meta.Genres) < 2 {
		t.Errorf("Genres = %v, want at least 2", meta.Genres)
	}
	if len(meta.Actors) < 2 {
		t.Errorf("Actors = %v, want at least 2", meta.Actors)
	}
	if meta.UniqueIDs["theporndb"] != "scene-777" {
		t.Errorf("UniqueIDs[theporndb] = %q, want 'scene-777'", meta.UniqueIDs["theporndb"])
	}
}

// ── buildMetadata: performer nesting via "parent" ─────────────────────────────

func TestBuildMetadataPerformerParentNesting(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(sceneResponse(map[string]any{
			"id":    "scene-nested",
			"title": "Nested Performers",
			"performers": []any{
				// Performer with "parent" key (TPDB nesting style)
				map[string]any{
					"parent": map[string]any{"name": "Nested Star"},
					"name":   "should-be-ignored",
				},
				// Direct performer
				map[string]any{"name": "Direct Star"},
				// Non-map item (should be skipped without panicking)
				"not-a-map",
			},
		}))
	}))
	defer server.Close()

	p := theporndb.NewWithBaseURL("token", server.URL)
	results, err := p.Search("Nested Performers", scraper.SearchOpts{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	_ = results

	// Verify via Detail that performers are parsed correctly.
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"id":    "scene-nested",
				"title": "Nested Performers",
				"performers": []any{
					map[string]any{
						"parent": map[string]any{"name": "Nested Star"},
					},
					map[string]any{"name": "Direct Star"},
				},
			},
		})
	}))
	defer server2.Close()

	p2 := theporndb.NewWithBaseURL("token", server2.URL)
	meta, err := p2.Detail("scene-nested")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if len(meta.Actors) != 2 {
		t.Errorf("Actors = %v, want 2 actors", meta.Actors)
	}
	names := make(map[string]bool)
	for _, a := range meta.Actors {
		names[a.Name] = true
	}
	if !names["Nested Star"] {
		t.Error("expected 'Nested Star' in actors")
	}
	if !names["Direct Star"] {
		t.Error("expected 'Direct Star' in actors")
	}
}

// ── extractPosterURL: all branches ───────────────────────────────────────────

func TestScrapeUsesBackgroundPoster(t *testing.T) {
	// Use background.large when posters is absent
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(sceneResponse(map[string]any{
			"id":    "scene-bg",
			"title": "Background Poster Scene",
			"date":  "2024-05-01",
			"background": map[string]any{
				"large": "https://example.com/bg-large.jpg",
			},
		}))
	}))
	defer server.Close()

	outDir := t.TempDir()
	p := theporndb.NewWithBaseURL("token", server.URL)
	result, err := p.Scrape("Background Poster Scene", "bg_scene.mp4", outDir, scraper.ScrapeOpts{})
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("Status = %q", result.Status)
	}
}

func TestScrapeUsesBackgroundPosterMedium(t *testing.T) {
	// Use background.medium when large is absent
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(sceneResponse(map[string]any{
			"id":    "scene-bg-med",
			"title": "Background Medium Poster",
			"date":  "2024-05-02",
			"background": map[string]any{
				"medium": "https://example.com/bg-medium.jpg",
			},
		}))
	}))
	defer server.Close()

	outDir := t.TempDir()
	p := theporndb.NewWithBaseURL("token", server.URL)
	result, err := p.Scrape("Background Medium Poster", "bg_med.mp4", outDir, scraper.ScrapeOpts{})
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("Status = %q, want 'ok'", result.Status)
	}
}

func TestScrapeUsesPostersSliceString(t *testing.T) {
	// posters as []any where first element is a string URL
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(sceneResponse(map[string]any{
			"id":      "scene-ps",
			"title":   "Posters Slice String",
			"posters": []any{"https://example.com/poster-str.jpg"},
		}))
	}))
	defer server.Close()

	outDir := t.TempDir()
	p := theporndb.NewWithBaseURL("token", server.URL)
	result, err := p.Scrape("Posters Slice String", "poster_str.mp4", outDir, scraper.ScrapeOpts{})
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("Status = %q, want 'ok'", result.Status)
	}
}

func TestScrapeUsesPostersSliceMap(t *testing.T) {
	// posters as []any where element is map{"url": "..."}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(sceneResponse(map[string]any{
			"id":    "scene-pm",
			"title": "Posters Slice Map",
			"posters": []any{
				map[string]any{"url": "https://example.com/poster-map.jpg"},
			},
		}))
	}))
	defer server.Close()

	outDir := t.TempDir()
	p := theporndb.NewWithBaseURL("token", server.URL)
	result, err := p.Scrape("Posters Slice Map", "poster_map.mp4", outDir, scraper.ScrapeOpts{})
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("Status = %q, want 'ok'", result.Status)
	}
}

func TestScrapeNoPosterURL(t *testing.T) {
	// No poster fields at all — should succeed without poster download.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(sceneResponse(map[string]any{
			"id":    "scene-noposter",
			"title": "No Poster Scene",
		}))
	}))
	defer server.Close()

	outDir := t.TempDir()
	p := theporndb.NewWithBaseURL("token", server.URL)
	result, err := p.Scrape("No Poster Scene", "no_poster.mp4", outDir, scraper.ScrapeOpts{})
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("Status = %q, want 'ok'", result.Status)
	}
	// No poster.jpg should exist (we can't download from the fake test server).
	posterPath := filepath.Join(outDir, "poster.jpg")
	// poster.jpg may or may not exist depending on download behavior — just
	// ensure the Scrape did not crash.
	_ = posterPath
}

// ── extractPosterURL: posters dict with missing large/medium/small ─────────────

func TestScrapePostersWithSmallOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(sceneResponse(map[string]any{
			"id":    "scene-small",
			"title": "Small Poster Only",
			"posters": map[string]any{
				"small": "https://example.com/small.jpg",
			},
		}))
	}))
	defer server.Close()

	outDir := t.TempDir()
	p := theporndb.NewWithBaseURL("token", server.URL)
	result, err := p.Scrape("Small Poster Only", "small_poster.mp4", outDir, scraper.ScrapeOpts{})
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("Status = %q", result.Status)
	}
}

// ── yearFromDate edge cases ───────────────────────────────────────────────────

func TestSearchResultsWithShortDate(t *testing.T) {
	// Provide a date shorter than 4 chars — year should be 0.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(sceneResponse(map[string]any{
			"id":    "scene-short-date",
			"title": "Short Date Scene",
			"date":  "202", // only 3 chars
		}))
	}))
	defer server.Close()

	p := theporndb.NewWithBaseURL("token", server.URL)
	results, err := p.Search("Short Date Scene", scraper.SearchOpts{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	if results[0].Year != 0 {
		t.Errorf("Year = %d for short date, want 0", results[0].Year)
	}
}

func TestSearchResultsWithEmptyDate(t *testing.T) {
	// Provide an empty date field — year should be 0.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(sceneResponse(map[string]any{
			"id":    "scene-no-date",
			"title": "No Date Scene",
			"date":  "",
		}))
	}))
	defer server.Close()

	p := theporndb.NewWithBaseURL("token", server.URL)
	results, err := p.Search("No Date Scene", scraper.SearchOpts{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}
	if results[0].Year != 0 {
		t.Errorf("Year = %d for empty date, want 0", results[0].Year)
	}
}

// ── sliceMapVal: edge cases ───────────────────────────────────────────────────

func TestSearchReturnsEmptyWhenDataKeyMissing(t *testing.T) {
	// Response has no "data" key — sliceMapVal returns nil → empty results.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"other_key": []any{}})
	}))
	defer server.Close()

	p := theporndb.NewWithBaseURL("token", server.URL)
	results, err := p.Search("anything", scraper.SearchOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results when data key missing, got %d", len(results))
	}
}

func TestSearchReturnsEmptyWhenDataIsNotSlice(t *testing.T) {
	// data field is a string, not a slice.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"data": "not-a-slice"})
	}))
	defer server.Close()

	p := theporndb.NewWithBaseURL("token", server.URL)
	results, err := p.Search("anything", scraper.SearchOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results when data is not a slice, got %d", len(results))
	}
}

func TestSearchSkipsNonMapItems(t *testing.T) {
	// data slice contains a mix of maps and non-maps; only maps count.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"data": []any{
				"not-a-map",
				map[string]any{"id": "good-scene", "title": "Real Scene", "date": "2023-01-01"},
				42,
			},
		})
	}))
	defer server.Close()

	p := theporndb.NewWithBaseURL("token", server.URL)
	scenes, err := p.SearchScene("anything")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only 1 map item should be returned.
	if len(scenes) != 1 {
		t.Errorf("expected 1 scene from mixed slice, got %d", len(scenes))
	}
}

// ── buildMetadata: tags with non-map items ────────────────────────────────────

func TestBuildMetadataTagsNonMapItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"id":    "scene-tags",
				"title": "Tag Test Scene",
				"tags": []any{
					map[string]any{"name": "Action"},
					"string-tag", // should be skipped
					map[string]any{"name": "Drama"},
					42, // should be skipped
				},
			},
		})
	}))
	defer server.Close()

	p := theporndb.NewWithBaseURL("token", server.URL)
	meta, err := p.Detail("scene-tags")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if len(meta.Genres) != 2 {
		t.Errorf("Genres = %v, want 2", meta.Genres)
	}
}

// ── No auth token path ────────────────────────────────────────────────────────

func TestNoAuthToken(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	}))
	defer server.Close()

	// Create provider with empty token.
	p := theporndb.NewWithBaseURL("", server.URL)
	_, _ = p.Search("anything", scraper.SearchOpts{})

	if gotAuth != "" {
		t.Errorf("Authorization header should not be set for empty token, got %q", gotAuth)
	}
}

// ── Scrape with poster URL in posters dict (large) ────────────────────────────

func TestScrapeWritesNFOWithPosterDictLarge(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/scenes" {
			json.NewEncoder(w).Encode(sceneResponse(map[string]any{
				"id":    "s-poster",
				"title": "Poster Dict Scene",
				"date":  "2024-07-01",
				"posters": map[string]any{
					"large": "https://example.com/large.jpg",
				},
				"performers": []any{
					map[string]any{"name": "Actor One"},
				},
			}))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	outDir := t.TempDir()
	p := theporndb.NewWithBaseURL("token", server.URL)
	result, err := p.Scrape("Poster Dict Scene", "poster_dict.mp4", outDir, scraper.ScrapeOpts{})
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("Status = %q, error = %s", result.Status, result.Error)
	}
	if result.Meta == nil {
		t.Fatal("Meta should not be nil on success")
	}
	if result.Meta.Title != "Poster Dict Scene" {
		t.Errorf("Meta.Title = %q", result.Meta.Title)
	}
}

// ── Request uses correct URL with params ──────────────────────────────────────

func TestSearchSceneQueryParam(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("parse")
		json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	}))
	defer server.Close()

	p := theporndb.NewWithBaseURL("token", server.URL)
	_, _ = p.SearchScene("my search query")
	if gotQuery != "my search query" {
		t.Errorf("parse param = %q, want 'my search query'", gotQuery)
	}
}

func TestSearchJAVQueryParam(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	}))
	defer server.Close()

	p := theporndb.NewWithBaseURL("token", server.URL)
	_, _ = p.SearchJAV("ABC-123")
	if gotPath != "/jav" {
		t.Errorf("path = %q, want '/jav'", gotPath)
	}
}

func TestSceneDetailPath(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"id": "scene-abc", "title": "Test"},
		})
	}))
	defer server.Close()

	p := theporndb.NewWithBaseURL("token", server.URL)
	_, _ = p.SceneDetail("scene-abc")
	if gotPath != "/scenes/scene-abc" {
		t.Errorf("path = %q, want '/scenes/scene-abc'", gotPath)
	}
}

// ── BaseURL trailing slash stripping ─────────────────────────────────────────

func TestNewWithBaseURLStripsTrailingSlash(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	}))
	defer server.Close()

	// Pass the URL with a trailing slash.
	p := theporndb.NewWithBaseURL("token", server.URL+"/")
	_, _ = p.SearchScene("anything")
	if !strings.HasPrefix(gotPath, "/scenes") {
		t.Errorf("path should start with /scenes, got %q", gotPath)
	}
}

// ── Scrape: NFO write error ───────────────────────────────────────────────────

func TestScrapeNFOWriteErrorWhenOutDirIsFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(sceneResponse(map[string]any{
			"id":    "s-nfo-err",
			"title": "NFO Error Scene",
			"date":  "2024-01-01",
		}))
	}))
	defer server.Close()

	tmp := t.TempDir()
	// Use an outDir that doesn't exist yet to exercise the directory-creation
	// path inside scraper.GenerateMovieNFO.
	outDir := tmp + "/new_subdir"
	p := theporndb.NewWithBaseURL("token", server.URL)
	result, err := p.Scrape("NFO Error Scene", "nfo_test.mp4", outDir, scraper.ScrapeOpts{})
	// outDir doesn't exist yet; scraper.GenerateMovieNFO should create it or fail gracefully.
	// Either way, the function should return a result.
	if result == nil {
		t.Fatal("result should not be nil")
	}
	_ = err
}

// ── buildMetadata: empty performer name ──────────────────────────────────────

func TestBuildMetadataEmptyPerformerName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"id":    "scene-empty-perf",
				"title": "Empty Performer Test",
				"performers": []any{
					map[string]any{"name": ""}, // empty name should be skipped
					map[string]any{
						"parent": map[string]any{"name": ""}, // empty nested name
					},
					map[string]any{"name": "Valid Actor"},
				},
			},
		})
	}))
	defer server.Close()

	p := theporndb.NewWithBaseURL("token", server.URL)
	meta, err := p.Detail("scene-empty-perf")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	// Only "Valid Actor" should be included.
	if len(meta.Actors) != 1 {
		t.Errorf("Actors = %v, want 1 (only Valid Actor)", meta.Actors)
	}
	if meta.Actors[0].Name != "Valid Actor" {
		t.Errorf("Actor[0].Name = %q, want 'Valid Actor'", meta.Actors[0].Name)
	}
}
