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

// ── helper ────────────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// ── Provider basics ───────────────────────────────────────────────────────────

func TestPriority(t *testing.T) {
	p := tmdb.New("token")
	if p.Priority() != 1 {
		t.Errorf("Priority() = %d, want 1", p.Priority())
	}
}

// ── Detail error cases ────────────────────────────────────────────────────────

func TestDetailInvalidIDFormat(t *testing.T) {
	p := tmdb.New("token")
	_, err := p.Detail("nocolon")
	if err == nil {
		t.Error("expected error for invalid id format, got nil")
	}
}

func TestDetailUnknownType(t *testing.T) {
	p := tmdb.New("token")
	_, err := p.Detail("unknown:123")
	if err == nil {
		t.Error("expected error for unknown type, got nil")
	}
}

func TestDetailInvalidMovieID(t *testing.T) {
	p := tmdb.New("token")
	_, err := p.Detail("movie:notanumber")
	if err == nil {
		t.Error("expected error for non-numeric movie id, got nil")
	}
}

func TestDetailInvalidTVID(t *testing.T) {
	p := tmdb.New("token")
	_, err := p.Detail("tv:notanumber")
	if err == nil {
		t.Error("expected error for non-numeric tv id, got nil")
	}
}

func TestDetailMovieHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	p := tmdb.NewWithBaseURL("token", server.URL)
	_, err := p.Detail("movie:12345")
	if err == nil {
		t.Error("expected error for HTTP 401, got nil")
	}
}

func TestDetailTVSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/tv/555/credits"):
			writeJSON(w, map[string]any{
				"crew": []map[string]any{
					{"name": "TV Director", "job": "Director"},
				},
				"cast": []map[string]any{
					{"name": "TV Actor", "character": "Lead"},
					{"name": "TV Actor 2", "character": "Side"},
				},
			})
		case strings.Contains(r.URL.Path, "/tv/555"):
			writeJSON(w, map[string]any{
				"id":             555,
				"name":           "Great Show",
				"original_name":  "Great Show Original",
				"first_air_date": "2020-03-01",
				"overview":       "A great show.",
				"vote_average":   8.5,
				"vote_count":     2000,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	p := tmdb.NewWithBaseURL("token", server.URL)
	meta, err := p.Detail("tv:555")
	if err != nil {
		t.Fatalf("Detail tv: %v", err)
	}
	if meta.Title != "Great Show" {
		t.Errorf("Title = %q, want Great Show", meta.Title)
	}
	if meta.Year != 2020 {
		t.Errorf("Year = %d, want 2020", meta.Year)
	}
	if meta.Rating != 8.5 {
		t.Errorf("Rating = %f, want 8.5", meta.Rating)
	}
}

// ── buildMovieMetadata coverage ───────────────────────────────────────────────

func TestMovieDetailFullFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/movie/77/credits"):
			writeJSON(w, map[string]any{
				"crew": []map[string]any{
					{"name": "Director A", "job": "Director"},
					{"name": "Writer B", "job": "Screenplay"},
					{"name": "Writer C", "job": "Writer"},
					{"name": "Editor D", "job": "Editor"}, // not captured
				},
				"cast": func() []map[string]any {
					// 16 actors - only first 15 should be taken
					actors := make([]map[string]any, 16)
					for i := range actors {
						actors[i] = map[string]any{
							"name":         "Actor " + string(rune('A'+i)),
							"character":    "Role " + string(rune('A'+i)),
							"profile_path": "/actor.jpg",
						}
					}
					return actors
				}(),
			})
		case strings.Contains(r.URL.Path, "/movie/77"):
			writeJSON(w, map[string]any{
				"id":             77,
				"title":          "Full Movie",
				"original_title": "Full Movie Original",
				"release_date":   "2022-07-04",
				"overview":       "Plot here.",
				"tagline":        "A tagline.",
				"runtime":        135,
				"vote_average":   7.9,
				"vote_count":     3000,
				"imdb_id":        "tt0077777",
				"poster_path":    "/poster77.jpg",
				"backdrop_path":  "/backdrop77.jpg",
				"belongs_to_collection": map[string]any{
					"id":   999,
					"name": "Full Collection",
				},
				"genres": []map[string]any{
					{"id": 28, "name": "Action"},
					{"id": 18, "name": "Drama"},
				},
				"production_companies": []map[string]any{
					{"name": "Studio X"},
				},
				"production_countries": []map[string]any{
					{"name": "United States of America"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	p := tmdb.NewWithBaseURL("token", server.URL)
	meta, err := p.Detail("movie:77")
	if err != nil {
		t.Fatalf("Detail movie:77: %v", err)
	}

	if meta.Title != "Full Movie" {
		t.Errorf("Title = %q", meta.Title)
	}
	if meta.OriginalTitle != "Full Movie Original" {
		t.Errorf("OriginalTitle = %q", meta.OriginalTitle)
	}
	if meta.Year != 2022 {
		t.Errorf("Year = %d, want 2022", meta.Year)
	}
	if meta.Tagline != "A tagline." {
		t.Errorf("Tagline = %q", meta.Tagline)
	}
	if meta.Runtime != 135 {
		t.Errorf("Runtime = %d, want 135", meta.Runtime)
	}
	if meta.UniqueIDs["imdb"] != "tt0077777" {
		t.Errorf("imdb id = %q, want tt0077777", meta.UniqueIDs["imdb"])
	}
	if meta.Set != "Full Collection" {
		t.Errorf("Set = %q, want Full Collection", meta.Set)
	}
	if len(meta.Genres) != 2 {
		t.Errorf("Genres = %v, want 2", meta.Genres)
	}
	if len(meta.Studios) != 1 || meta.Studios[0] != "Studio X" {
		t.Errorf("Studios = %v", meta.Studios)
	}
	if len(meta.Countries) != 1 {
		t.Errorf("Countries = %v", meta.Countries)
	}
	if !strings.HasSuffix(meta.PosterURL, "/poster77.jpg") {
		t.Errorf("PosterURL = %q", meta.PosterURL)
	}
	if !strings.HasSuffix(meta.FanartURL, "/backdrop77.jpg") {
		t.Errorf("FanartURL = %q", meta.FanartURL)
	}
	if len(meta.Directors) != 1 || meta.Directors[0] != "Director A" {
		t.Errorf("Directors = %v", meta.Directors)
	}
	if len(meta.Credits) != 2 {
		t.Errorf("Credits = %v, want 2 writers", meta.Credits)
	}
	// Only first 15 actors
	if len(meta.Actors) != 15 {
		t.Errorf("Actors count = %d, want 15", len(meta.Actors))
	}
	// Actor thumbs should be populated from profile_path
	if !strings.HasSuffix(meta.Actors[0].Thumb, "/actor.jpg") {
		t.Errorf("Actor[0].Thumb = %q", meta.Actors[0].Thumb)
	}
}

// ── buildTVMetadata coverage ──────────────────────────────────────────────────

func TestScrapeTVWritesNFO(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/search/tv"):
			writeJSON(w, map[string]any{
				"results": []map[string]any{
					{"id": 777, "name": "TV Scrape Show", "first_air_date": "2021-09-15"},
				},
			})
		case strings.Contains(r.URL.Path, "/tv/777/season/1"):
			writeJSON(w, map[string]any{
				"episodes": []map[string]any{
					{
						"episode_number": 1,
						"name":           "Pilot Episode",
						"overview":       "The first episode.",
						"air_date":       "2021-09-15",
					},
				},
			})
		case strings.Contains(r.URL.Path, "/tv/777/credits"):
			writeJSON(w, map[string]any{
				"crew": []map[string]any{
					{"name": "Show Director", "job": "Director"},
				},
				"cast": []map[string]any{
					{"name": "Lead", "character": "Main"},
				},
			})
		case strings.Contains(r.URL.Path, "/tv/777"):
			writeJSON(w, map[string]any{
				"id":                   777,
				"name":                 "TV Scrape Show",
				"original_name":        "TV Scrape Show",
				"first_air_date":       "2021-09-15",
				"overview":             "A show about scraping.",
				"vote_average":         7.0,
				"vote_count":           500,
				"poster_path":          "/tvposter.jpg",
				"backdrop_path":        "/tvbackdrop.jpg",
				"genres":               []map[string]any{{"name": "Drama"}},
				"production_companies": []map[string]any{{"name": "Netflix"}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	outDir := t.TempDir()
	p := tmdb.NewWithBaseURL("token", server.URL)
	result, err := p.Scrape("TV Scrape Show", "tv_scrape_s01e01.mkv", outDir, scraper.ScrapeOpts{Season: 1, Episode: 1})
	if err != nil {
		t.Fatalf("Scrape TV: %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("Status = %q (error: %s), want ok", result.Status, result.Error)
	}

	// Episode NFO
	nfoPath := filepath.Join(outDir, "tv_scrape_s01e01.nfo")
	if _, err := os.Stat(nfoPath); os.IsNotExist(err) {
		t.Errorf("episode NFO not created at %s", nfoPath)
	}

	// tvshow.nfo
	tvshowPath := filepath.Join(outDir, "tvshow.nfo")
	if _, err := os.Stat(tvshowPath); os.IsNotExist(err) {
		t.Errorf("tvshow.nfo not created at %s", tvshowPath)
	}
}

func TestScrapeTVNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"results": []any{}})
	}))
	defer server.Close()

	outDir := t.TempDir()
	p := tmdb.NewWithBaseURL("token", server.URL)
	result, err := p.Scrape("Unknown Show XYZ", "unknown_s01e01.mkv", outDir, scraper.ScrapeOpts{Season: 1, Episode: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "not_found" {
		t.Errorf("Status = %q, want not_found", result.Status)
	}
}

func TestScrapeTVFallbackSingleWord(t *testing.T) {
	// First search returns empty; single-word fallback also returns empty
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		writeJSON(w, map[string]any{"results": []any{}})
	}))
	defer server.Close()

	outDir := t.TempDir()
	p := tmdb.NewWithBaseURL("token", server.URL)
	result, _ := p.Scrape("Multi Word Show Title", "show_s02e03.mkv", outDir, scraper.ScrapeOpts{Season: 2, Episode: 3})
	if result.Status != "not_found" {
		t.Errorf("Status = %q, want not_found", result.Status)
	}
}

func TestScrapeTVSearchError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	outDir := t.TempDir()
	p := tmdb.NewWithBaseURL("token", server.URL)
	result, err := p.Scrape("Error Show", "error_s01e01.mkv", outDir, scraper.ScrapeOpts{Season: 1, Episode: 1})
	if err == nil {
		t.Error("expected error, got nil")
	}
	if result.Status != "error" {
		t.Errorf("Status = %q, want error", result.Status)
	}
}

func TestScrapeTVSkipsExistingTvshowNFO(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/search/tv"):
			writeJSON(w, map[string]any{
				"results": []map[string]any{
					{"id": 888, "name": "Existing Show", "first_air_date": "2019-01-01"},
				},
			})
		case strings.Contains(r.URL.Path, "/tv/888/season/1"):
			writeJSON(w, map[string]any{"episodes": []any{}})
		case strings.Contains(r.URL.Path, "/tv/888/credits"):
			writeJSON(w, map[string]any{"crew": []any{}, "cast": []any{}})
		case strings.Contains(r.URL.Path, "/tv/888"):
			writeJSON(w, map[string]any{
				"id": 888, "name": "Existing Show", "first_air_date": "2019-01-01",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	outDir := t.TempDir()
	// Pre-create tvshow.nfo
	existingContent := []byte("existing tvshow nfo")
	tvshowPath := filepath.Join(outDir, "tvshow.nfo")
	if err := os.WriteFile(tvshowPath, existingContent, 0o644); err != nil {
		t.Fatal(err)
	}

	p := tmdb.NewWithBaseURL("token", server.URL)
	_, err := p.Scrape("Existing Show", "existing_s01e01.mkv", outDir, scraper.ScrapeOpts{Season: 1, Episode: 1})
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}

	// tvshow.nfo should not be overwritten
	data, _ := os.ReadFile(tvshowPath)
	if string(data) != string(existingContent) {
		t.Error("tvshow.nfo was overwritten when it should have been preserved")
	}
}

// ── bestMatch coverage ────────────────────────────────────────────────────────

func TestScrapeMovieBestMatchByYear(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/search/movie"):
			writeJSON(w, map[string]any{
				"results": []map[string]any{
					{"id": 101, "title": "Same Title", "release_date": "2010-01-01"},
					{"id": 202, "title": "Same Title", "release_date": "2023-06-15"},
				},
			})
		case strings.Contains(r.URL.Path, "/movie/202/images"):
			writeJSON(w, map[string]any{"posters": []any{}})
		case strings.Contains(r.URL.Path, "/movie/202/credits"):
			writeJSON(w, map[string]any{"crew": []any{}, "cast": []any{}})
		case strings.Contains(r.URL.Path, "/movie/202"):
			writeJSON(w, map[string]any{
				"id": 202, "title": "Same Title", "release_date": "2023-06-15",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	outDir := t.TempDir()
	p := tmdb.NewWithBaseURL("token", server.URL)
	result, err := p.Scrape("Same Title", "same_title_2023.mkv", outDir, scraper.ScrapeOpts{Year: 2023})
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("Status = %q, want ok (error: %s)", result.Status, result.Error)
	}
}

// ── saveTMDBPoster with poster ────────────────────────────────────────────────

func TestScrapeMovieWithPoster(t *testing.T) {
	posterBody := []byte("fake poster image")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/search/movie"):
			writeJSON(w, map[string]any{
				"results": []map[string]any{
					{"id": 300, "title": "Poster Movie", "release_date": "2023-01-01"},
				},
			})
		case strings.Contains(r.URL.Path, "/movie/300/images"):
			// Return a poster that points back to this test server
			writeJSON(w, map[string]any{
				"posters": []map[string]any{
					{"file_path": "/t/p/w500/poster300.jpg"},
				},
			})
		case strings.Contains(r.URL.Path, "/movie/300/credits"):
			writeJSON(w, map[string]any{"crew": []any{}, "cast": []any{}})
		case strings.Contains(r.URL.Path, "/movie/300"):
			writeJSON(w, map[string]any{
				"id": 300, "title": "Poster Movie", "release_date": "2023-01-01",
			})
		case strings.Contains(r.URL.Path, "/w500/poster300.jpg"):
			w.Write(posterBody)
		default:
			// Return empty so poster download gracefully fails (TMDB URL not the test server)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	outDir := t.TempDir()
	p := tmdb.NewWithBaseURL("token", server.URL)
	result, err := p.Scrape("Poster Movie", "poster_movie.mkv", outDir, scraper.ScrapeOpts{})
	if err != nil {
		t.Fatalf("Scrape: %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("Status = %q", result.Status)
	}
}

// ── scrapeMovie simplified query fallback ─────────────────────────────────────

func TestScrapeMovieSimplifiedFallback(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		q := r.URL.Query().Get("query")
		switch {
		case strings.Contains(r.URL.Path, "/search/movie"):
			if q == "Movie" {
				// simplified (dot-split) query succeeds
				writeJSON(w, map[string]any{
					"results": []map[string]any{
						{"id": 400, "title": "Movie", "release_date": "2024-01-01"},
					},
				})
			} else {
				writeJSON(w, map[string]any{"results": []any{}})
			}
		case strings.Contains(r.URL.Path, "/movie/400/images"):
			writeJSON(w, map[string]any{"posters": []any{}})
		case strings.Contains(r.URL.Path, "/movie/400/credits"):
			writeJSON(w, map[string]any{"crew": []any{}, "cast": []any{}})
		case strings.Contains(r.URL.Path, "/movie/400"):
			writeJSON(w, map[string]any{
				"id": 400, "title": "Movie", "release_date": "2024-01-01",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	outDir := t.TempDir()
	p := tmdb.NewWithBaseURL("token", server.URL)
	// "Movie.2024.1080p" → first search empty, simplified to "Movie" succeeds
	result, err := p.Scrape("Movie.2024.1080p", "movie_2024_1080p.mkv", outDir, scraper.ScrapeOpts{})
	if err != nil {
		t.Fatalf("Scrape simplified: %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("Status = %q (error: %s), want ok", result.Status, result.Error)
	}
}

// ── scrapeMovie - zero id after best-match ────────────────────────────────────

func TestScrapeMovieZeroID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/search/movie") {
			// Return a result with no "id" field
			writeJSON(w, map[string]any{
				"results": []map[string]any{
					{"title": "No ID Movie", "release_date": "2023-01-01"},
				},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	outDir := t.TempDir()
	p := tmdb.NewWithBaseURL("token", server.URL)
	result, err := p.Scrape("No ID Movie", "noid.mkv", outDir, scraper.ScrapeOpts{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "not_found" {
		t.Errorf("Status = %q, want not_found", result.Status)
	}
}

// ── scrapeMovie error from MovieDetail ───────────────────────────────────────

func TestScrapeMovieDetailError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/search/movie") {
			writeJSON(w, map[string]any{
				"results": []map[string]any{
					{"id": 500, "title": "Detail Error Movie", "release_date": "2020-01-01"},
				},
			})
			return
		}
		// movie/500 returns error
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	outDir := t.TempDir()
	p := tmdb.NewWithBaseURL("token", server.URL)
	result, err := p.Scrape("Detail Error Movie", "detail_error.mkv", outDir, scraper.ScrapeOpts{})
	if err == nil {
		t.Error("expected error from MovieDetail, got nil")
	}
	if result.Status != "error" {
		t.Errorf("Status = %q, want error", result.Status)
	}
}

// ── enrichEpisodeMetadata ─────────────────────────────────────────────────────

func TestScrapeTVEnrichEpisode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/search/tv"):
			writeJSON(w, map[string]any{
				"results": []map[string]any{
					{"id": 600, "name": "Enrich Show", "first_air_date": "2020-01-01"},
				},
			})
		case strings.Contains(r.URL.Path, "/tv/600/season/2"):
			writeJSON(w, map[string]any{
				"episodes": []map[string]any{
					{"episode_number": 3, "name": "Enriched Title", "overview": "Enriched plot.", "air_date": "2020-05-01"},
					{"episode_number": 4, "name": "Other Episode", "overview": "Other plot.", "air_date": "2020-05-08"},
				},
			})
		case strings.Contains(r.URL.Path, "/tv/600/credits"):
			writeJSON(w, map[string]any{
				"crew": []map[string]any{
					{"name": "Dir1", "job": "Director"},
					{"name": "Dir2", "job": "Director"},
					{"name": "Dir3", "job": "Director"},
					{"name": "Dir4", "job": "Director"}, // 4th should be ignored (limit 3)
				},
				"cast": func() []map[string]any {
					// 12 actors → only first 10 should be taken
					actors := make([]map[string]any, 12)
					for i := range actors {
						actors[i] = map[string]any{"name": "Cast " + string(rune('A'+i)), "character": "Role"}
					}
					return actors
				}(),
			})
		case strings.Contains(r.URL.Path, "/tv/600"):
			writeJSON(w, map[string]any{
				"id":             600,
				"name":           "Enrich Show",
				"first_air_date": "2020-01-01",
				"overview":       "Show overview.",
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	outDir := t.TempDir()
	p := tmdb.NewWithBaseURL("token", server.URL)
	result, err := p.Scrape("Enrich Show", "enrich_s02e03.mkv", outDir, scraper.ScrapeOpts{Season: 2, Episode: 3})
	if err != nil {
		t.Fatalf("Scrape enriched: %v", err)
	}
	if result.Status != "ok" {
		t.Errorf("Status = %q", result.Status)
	}

	// Check the episode NFO was enriched
	nfoPath := filepath.Join(outDir, "enrich_s02e03.nfo")
	data, err := os.ReadFile(nfoPath)
	if err != nil {
		t.Fatalf("read NFO: %v", err)
	}
	if !strings.Contains(string(data), "Enriched Title") {
		t.Errorf("expected enriched title in NFO, got:\n%s", data)
	}
	if !strings.Contains(string(data), "Enriched plot.") {
		t.Errorf("expected enriched plot in NFO, got:\n%s", data)
	}

	// buildTVMetadata limits: 3 directors, 10 cast
	meta := result.Meta
	if len(meta.Directors) != 3 {
		t.Errorf("Directors = %d, want 3 (limit)", len(meta.Directors))
	}
	if len(meta.Actors) != 10 {
		t.Errorf("Actors = %d, want 10 (limit)", len(meta.Actors))
	}
}

// ── HTTP error cases ──────────────────────────────────────────────────────────

func TestSearchMovieHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	p := tmdb.NewWithBaseURL("token", server.URL)
	_, err := p.Search("anything", scraper.SearchOpts{})
	if err == nil {
		t.Error("expected error for HTTP 500, got nil")
	}
}

func TestScrapeMovieSearchError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	outDir := t.TempDir()
	p := tmdb.NewWithBaseURL("token", server.URL)
	result, err := p.Scrape("Error Movie", "error.mkv", outDir, scraper.ScrapeOpts{})
	if err == nil {
		t.Error("expected error, got nil")
	}
	if result.Status != "error" {
		t.Errorf("Status = %q, want error", result.Status)
	}
}

// ── TVDetail / SeasonDetail / TVCredits direct calls ─────────────────────────

func TestTVDetailDirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/tv/123") {
			writeJSON(w, map[string]any{"id": 123, "name": "Direct TV"})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	p := tmdb.NewWithBaseURL("token", server.URL)
	detail, err := p.TVDetail(123, "")
	if err != nil {
		t.Fatalf("TVDetail: %v", err)
	}
	if detail["name"] != "Direct TV" {
		t.Errorf("name = %v, want Direct TV", detail["name"])
	}
}

func TestSeasonDetailDirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/tv/123/season/2") {
			writeJSON(w, map[string]any{"season_number": 2, "episodes": []any{}})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	p := tmdb.NewWithBaseURL("token", server.URL)
	detail, err := p.SeasonDetail(123, 2, "")
	if err != nil {
		t.Fatalf("SeasonDetail: %v", err)
	}
	if v, _ := detail["season_number"].(float64); int(v) != 2 {
		t.Errorf("season_number = %v, want 2", detail["season_number"])
	}
}

func TestTVCreditsDirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/tv/123/credits") {
			writeJSON(w, map[string]any{"cast": []any{}, "crew": []any{}})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	p := tmdb.NewWithBaseURL("token", server.URL)
	credits, err := p.TVCredits(123, "")
	if err != nil {
		t.Fatalf("TVCredits: %v", err)
	}
	if credits == nil {
		t.Error("TVCredits returned nil")
	}
}

// ── Rate limit max retries ────────────────────────────────────────────────────

func TestRateLimitMaxRetries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	p := tmdb.NewWithBaseURL("token", server.URL)
	_, err := p.SearchMovie("anything", "")
	if err == nil {
		t.Error("expected rate limit error after max retries, got nil")
	}
	if !strings.Contains(err.Error(), "rate limit") {
		t.Errorf("error = %v, want to contain 'rate limit'", err)
	}
}

// ── Search TV error (partial coverage of Search) ─────────────────────────────

func TestSearchPartialTVError(t *testing.T) {
	// movie search succeeds, tv search fails → should still return movie results
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/search/movie") {
			writeJSON(w, map[string]any{
				"results": []map[string]any{
					{"id": 1, "title": "Movie Result", "release_date": "2023-01-01"},
				},
			})
			return
		}
		// TV search errors
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	p := tmdb.NewWithBaseURL("token", server.URL)
	results, err := p.Search("Something", scraper.SearchOpts{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Error("expected movie results even when TV search fails")
	}
}
