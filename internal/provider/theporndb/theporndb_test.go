package theporndb_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/newcoderlife/media115/internal/provider/theporndb"
	"github.com/newcoderlife/media115/internal/scraper"
)

func sceneResponse(scenes ...map[string]any) map[string]any {
	return map[string]any{"data": scenes}
}

func TestSearchScene(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/scenes" {
			json.NewEncoder(w).Encode(sceneResponse(map[string]any{
				"id":    "scene-001",
				"title": "Test Scene",
				"date":  "2023-08-20",
				"site": map[string]any{
					"name": "Test Studio",
				},
				"performers": []any{
					map[string]any{"name": "Performer One"},
				},
			}))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	p := theporndb.NewWithBaseURL("token", server.URL)
	results, err := p.Search("Test Scene", scraper.SearchOpts{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	if results[0].Title != "Test Scene" {
		t.Errorf("Title = %q", results[0].Title)
	}
	if results[0].Year != 2023 {
		t.Errorf("Year = %d", results[0].Year)
	}
}

func TestSceneDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/scenes/") {
			json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"id":          "scene-001",
					"title":       "Detail Scene",
					"date":        "2024-01-10",
					"description": "A great scene",
					"tags":        []any{map[string]any{"name": "HD"}},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	p := theporndb.NewWithBaseURL("token", server.URL)
	meta, err := p.Detail("scene-001")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if meta.Title != "Detail Scene" {
		t.Errorf("Title = %q", meta.Title)
	}
	if meta.Plot != "A great scene" {
		t.Errorf("Plot = %q", meta.Plot)
	}
}

func TestScrapeWritesNFO(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/scenes" {
			json.NewEncoder(w).Encode(sceneResponse(map[string]any{
				"id":    "s-42",
				"title": "Scrape Scene",
				"date":  "2023-12-01",
				"performers": []any{
					map[string]any{"name": "Star A"},
				},
				"tags": []any{},
			}))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	outDir := t.TempDir()
	p := theporndb.NewWithBaseURL("token", server.URL)
	result, err := p.Scrape("Scrape Scene", "scrape_scene.mp4", outDir, scraper.ScrapeOpts{})
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("Status = %q, error = %s", result.Status, result.Error)
	}

	nfoPath := filepath.Join(outDir, "scrape_scene.nfo")
	if _, err := os.Stat(nfoPath); os.IsNotExist(err) {
		t.Errorf("NFO not created at %s", nfoPath)
	}
}

func TestScrapeNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	}))
	defer server.Close()

	outDir := t.TempDir()
	p := theporndb.NewWithBaseURL("token", server.URL)
	result, err := p.Scrape("Unknown Scene", "unknown.mp4", outDir, scraper.ScrapeOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "not_found" {
		t.Errorf("Status = %q, want not_found", result.Status)
	}
}

func TestProviderMetadata(t *testing.T) {
	p := theporndb.New("token")
	if p.Name() != "theporndb" {
		t.Errorf("Name = %q", p.Name())
	}
	types := p.SupportedTypes()
	if len(types) == 0 || types[0] != "av_west" {
		t.Errorf("SupportedTypes = %v", types)
	}
	if p.Priority() != 0 {
		t.Errorf("Priority = %d, want 0", p.Priority())
	}
}

func TestSearchJAV(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/jav" {
			json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "jav-001", "title": "JAV Title"},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	p := theporndb.NewWithBaseURL("token", server.URL)
	results, err := p.SearchJAV("JAV-001")
	if err != nil {
		t.Fatalf("SearchJAV: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected JAV results")
	}
}

func TestAuthHeaderSet(t *testing.T) {
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
	}))
	defer server.Close()

	p := theporndb.NewWithBaseURL("my-secret-token", server.URL)
	p.Search("anything", scraper.SearchOpts{})
	if gotAuth != "Bearer my-secret-token" {
		t.Errorf("Authorization = %q", gotAuth)
	}
}
