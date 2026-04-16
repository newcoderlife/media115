package stashdb_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/newcoderlife/media115/internal/provider/stashdb"
	"github.com/newcoderlife/media115/internal/scraper"
)

func graphqlResponse(data map[string]any) map[string]any {
	return map[string]any{"data": data}
}

func TestSearchScenes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		json.NewEncoder(w).Encode(graphqlResponse(map[string]any{
			"searchScene": []any{
				map[string]any{
					"id":    "scene-abc",
					"title": "StashDB Scene",
					"date":  "2024-02-14",
					"studio": map[string]any{
						"id":   "studio-1",
						"name": "Hot Studio",
					},
					"performers": []any{},
					"images":     []any{map[string]any{"url": "https://example.com/img.jpg"}},
					"tags":       []any{},
				},
			},
		}))
	}))
	defer server.Close()

	p := stashdb.NewWithEndpoint("api-key", server.URL)
	results, err := p.Search("StashDB Scene", scraper.SearchOpts{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	if results[0].ID != "scene-abc" {
		t.Errorf("ID = %q", results[0].ID)
	}
	if results[0].Title != "StashDB Scene" {
		t.Errorf("Title = %q", results[0].Title)
	}
	if results[0].Year != 2024 {
		t.Errorf("Year = %d", results[0].Year)
	}
}

func TestSceneDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(graphqlResponse(map[string]any{
			"findScene": map[string]any{
				"id":       "scene-xyz",
				"title":    "Detailed Scene",
				"date":     "2023-11-01",
				"details":  "Full scene description",
				"duration": float64(2700), // 45 minutes in seconds
				"studio":   map[string]any{"name": "Premium Studio"},
				"performers": []any{
					map[string]any{
						"performer": map[string]any{"id": "p1", "name": "Star One"},
						"as_role":   "Lead",
					},
				},
				"tags":   []any{map[string]any{"id": "t1", "name": "Category A"}},
				"images": []any{map[string]any{"url": "https://example.com/cover.jpg"}},
			},
		}))
	}))
	defer server.Close()

	p := stashdb.NewWithEndpoint("key", server.URL)
	meta, err := p.Detail("scene-xyz")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if meta.Title != "Detailed Scene" {
		t.Errorf("Title = %q", meta.Title)
	}
	if meta.Plot != "Full scene description" {
		t.Errorf("Plot = %q", meta.Plot)
	}
	if meta.Runtime != 45 {
		t.Errorf("Runtime = %d, want 45", meta.Runtime)
	}
	if len(meta.Studios) == 0 || meta.Studios[0] != "Premium Studio" {
		t.Errorf("Studios = %v", meta.Studios)
	}
	if len(meta.Actors) == 0 || meta.Actors[0].Name != "Star One" {
		t.Errorf("Actors = %v", meta.Actors)
	}
	if len(meta.Genres) == 0 || meta.Genres[0] != "Category A" {
		t.Errorf("Genres = %v", meta.Genres)
	}
	if meta.UniqueIDs["stashdb"] != "scene-xyz" {
		t.Errorf("UniqueIDs[stashdb] = %q", meta.UniqueIDs["stashdb"])
	}
}

func TestScrapeWritesNFO(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(graphqlResponse(map[string]any{
			"searchScene": []any{
				map[string]any{
					"id":         "s-99",
					"title":      "NFO Scene",
					"date":       "2024-06-15",
					"studio":     map[string]any{"name": "NFO Studio"},
					"performers": []any{},
					"images":     []any{},
					"tags":       []any{},
				},
			},
		}))
	}))
	defer server.Close()

	outDir := t.TempDir()
	p := stashdb.NewWithEndpoint("key", server.URL)
	result, err := p.Scrape("NFO Scene", "nfo_scene.mp4", outDir, scraper.ScrapeOpts{})
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("Status = %q, error = %s", result.Status, result.Error)
	}
	if result.Match != "NFO Scene" {
		t.Errorf("Match = %q", result.Match)
	}

	nfoPath := filepath.Join(outDir, "nfo_scene.nfo")
	if _, err := os.Stat(nfoPath); os.IsNotExist(err) {
		t.Errorf("NFO not created at %s", nfoPath)
	}
}

func TestScrapeNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(graphqlResponse(map[string]any{
			"searchScene": []any{},
		}))
	}))
	defer server.Close()

	outDir := t.TempDir()
	p := stashdb.NewWithEndpoint("key", server.URL)
	result, err := p.Scrape("Not Found", "unknown.mp4", outDir, scraper.ScrapeOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "not_found" {
		t.Errorf("Status = %q, want not_found", result.Status)
	}
}

func TestProviderMetadata(t *testing.T) {
	p := stashdb.New("api-key")
	if p.Name() != "stashdb" {
		t.Errorf("Name = %q", p.Name())
	}
	types := p.SupportedTypes()
	if len(types) == 0 || types[0] != "av_west" {
		t.Errorf("SupportedTypes = %v", types)
	}
	if p.Priority() != 1 {
		t.Errorf("Priority = %d, want 1", p.Priority())
	}
}

func TestGraphQLErrorHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"errors": []any{
				map[string]any{"message": "field not found"},
			},
		})
	}))
	defer server.Close()

	p := stashdb.NewWithEndpoint("key", server.URL)
	_, err := p.Search("anything", scraper.SearchOpts{})
	if err == nil {
		t.Error("expected error for GraphQL errors response")
	}
}

func TestAPIKeyHeader(t *testing.T) {
	var gotKey string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("ApiKey")
		json.NewEncoder(w).Encode(graphqlResponse(map[string]any{
			"searchScene": []any{},
		}))
	}))
	defer server.Close()

	p := stashdb.NewWithEndpoint("my-api-key", server.URL)
	p.Search("test", scraper.SearchOpts{})
	if gotKey != "my-api-key" {
		t.Errorf("ApiKey header = %q, want my-api-key", gotKey)
	}
}

func TestSceneDetailNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(graphqlResponse(map[string]any{
			"findScene": nil, // scene not found
		}))
	}))
	defer server.Close()

	p := stashdb.NewWithEndpoint("key", server.URL)
	_, err := p.Detail("non-existent")
	if err == nil {
		t.Error("expected error for not found scene")
	}
}

func TestQueryHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	p := stashdb.NewWithEndpoint("key", server.URL)
	_, err := p.Search("test", scraper.SearchOpts{})
	if err == nil {
		t.Error("expected error for HTTP 500")
	}
}

func TestQueryHTTP502(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	p := stashdb.NewWithEndpoint("key", server.URL)
	_, err := p.Search("test", scraper.SearchOpts{})
	if err == nil {
		t.Error("expected error for HTTP 502")
	}
}

func TestQueryInvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not json {{{"))
	}))
	defer server.Close()

	p := stashdb.NewWithEndpoint("key", server.URL)
	_, err := p.Search("test", scraper.SearchOpts{})
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestQueryNullDataResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Respond with data: null (no errors)
		json.NewEncoder(w).Encode(map[string]any{
			"data": nil,
		})
	}))
	defer server.Close()

	p := stashdb.NewWithEndpoint("key", server.URL)
	results, err := p.Search("test", scraper.SearchOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// null data → empty searchScene → empty results
	if len(results) != 0 {
		t.Errorf("expected empty results, got %d", len(results))
	}
}

func TestBuildMetadataNoStudio(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(graphqlResponse(map[string]any{
			"findScene": map[string]any{
				"id":         "s1",
				"title":      "No Studio Scene",
				"date":       "2024-01-01",
				"studio":     nil,
				"performers": []any{},
				"tags":       []any{},
				"images":     []any{},
			},
		}))
	}))
	defer server.Close()

	p := stashdb.NewWithEndpoint("key", server.URL)
	meta, err := p.Detail("s1")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if len(meta.Studios) != 0 {
		t.Errorf("Studios = %v; want empty for nil studio", meta.Studios)
	}
}

func TestBuildMetadataNoPerformers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(graphqlResponse(map[string]any{
			"findScene": map[string]any{
				"id":    "s2",
				"title": "No Performers",
				"date":  "2024-01-01",
				"performers": []any{
					"invalid performer type", // not a map → should be skipped
				},
				"tags":   []any{},
				"images": []any{},
			},
		}))
	}))
	defer server.Close()

	p := stashdb.NewWithEndpoint("key", server.URL)
	meta, err := p.Detail("s2")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if len(meta.Actors) != 0 {
		t.Errorf("Actors = %v; want empty for invalid performer", meta.Actors)
	}
}

func TestBuildMetadataPerformerNoInnerPerformer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(graphqlResponse(map[string]any{
			"findScene": map[string]any{
				"id":    "s3",
				"title": "Missing Inner Performer",
				"date":  "2024-01-01",
				"performers": []any{
					map[string]any{
						"as_role":   "Lead",
						"performer": "not-a-map", // invalid type
					},
				},
				"tags":   []any{},
				"images": []any{},
			},
		}))
	}))
	defer server.Close()

	p := stashdb.NewWithEndpoint("key", server.URL)
	meta, err := p.Detail("s3")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if len(meta.Actors) != 0 {
		t.Errorf("Actors = %v; want empty for invalid inner performer", meta.Actors)
	}
}

func TestBuildMetadataZeroDuration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(graphqlResponse(map[string]any{
			"findScene": map[string]any{
				"id":         "s4",
				"title":      "Zero Duration",
				"date":       "2024-01-01",
				"duration":   float64(0),
				"performers": []any{},
				"tags":       []any{},
				"images":     []any{},
			},
		}))
	}))
	defer server.Close()

	p := stashdb.NewWithEndpoint("key", server.URL)
	meta, err := p.Detail("s4")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if meta.Runtime != 0 {
		t.Errorf("Runtime = %d; want 0 for zero duration", meta.Runtime)
	}
}

func TestFirstImageURLStringType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(graphqlResponse(map[string]any{
			"searchScene": []any{
				map[string]any{
					"id":         "img-str",
					"title":      "String Image",
					"date":       "2024-01-01",
					"images":     []any{"https://example.com/direct-string.jpg"},
					"performers": []any{},
					"tags":       []any{},
				},
			},
		}))
	}))
	defer server.Close()

	outDir := t.TempDir()
	p := stashdb.NewWithEndpoint("key", server.URL)
	// The scrape should succeed and attempt to download the poster
	result, err := p.Scrape("String Image", "test.mp4", outDir, scraper.ScrapeOpts{})
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("Status = %q", result.Status)
	}
}

func TestYearFromDateShortString(t *testing.T) {
	// Indirectly test yearFromDate with a short date string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(graphqlResponse(map[string]any{
			"findScene": map[string]any{
				"id":         "s5",
				"title":      "Short Date",
				"date":       "24", // < 4 chars
				"performers": []any{},
				"tags":       []any{},
				"images":     []any{},
			},
		}))
	}))
	defer server.Close()

	p := stashdb.NewWithEndpoint("key", server.URL)
	meta, err := p.Detail("s5")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if meta.Year != 0 {
		t.Errorf("Year = %d; want 0 for short date", meta.Year)
	}
}

func TestScrapeErrorPropagation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	p := stashdb.NewWithEndpoint("key", server.URL)
	result, err := p.Scrape("test", "test.mp4", t.TempDir(), scraper.ScrapeOpts{})
	if err == nil {
		t.Error("expected error from Scrape")
	}
	if result.Status != "error" {
		t.Errorf("Status = %q; want error", result.Status)
	}
}
