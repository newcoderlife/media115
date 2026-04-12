// Package theporndb implements a scraper Provider backed by the ThePornDB REST API.
package theporndb

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/bytedance/gg/gconv"

	"github.com/newcoderlife/media115/internal/scraper"
)

const defaultBaseURL = "https://api.theporndb.net"

// Provider implements scraper.Provider for ThePornDB.
type Provider struct {
	token    string
	baseURL  string
	client   *http.Client
	throttle *scraper.Throttle
}

// New creates a new ThePornDB provider.
func New(token string) *Provider {
	return NewWithBaseURL(token, defaultBaseURL)
}

// NewWithBaseURL creates a ThePornDB provider with a custom base URL (for testing).
func NewWithBaseURL(token, baseURL string) *Provider {
	return &Provider{
		token:    token,
		baseURL:  strings.TrimRight(baseURL, "/"),
		client:   &http.Client{Timeout: 15 * time.Second},
		throttle: scraper.NewThrottle(1.5),
	}
}

// Name returns the provider name.
func (p *Provider) Name() string { return "theporndb" }

// SupportedTypes returns the media types this provider handles.
func (p *Provider) SupportedTypes() []string { return []string{"av_west"} }

// Priority returns 0 — ThePornDB runs before StashDB.
func (p *Provider) Priority() int { return 0 }

// Search searches ThePornDB scenes by query.
func (p *Provider) Search(query string, opts scraper.SearchOpts) ([]scraper.SearchResult, error) {
	scenes, err := p.SearchScene(query)
	if err != nil {
		return nil, err
	}
	var results []scraper.SearchResult
	for _, s := range scenes {
		id := gconv.To[string, any](s["id"])
		title := stringVal(s, "title")
		yr := yearFromDate(stringVal(s, "date"))
		results = append(results, scraper.SearchResult{ID: id, Title: title, Year: yr})
	}
	return results, nil
}

// Detail returns full metadata for a ThePornDB scene ID.
func (p *Provider) Detail(id string) (*scraper.Metadata, error) {
	scene, err := p.SceneDetail(id)
	if err != nil {
		return nil, err
	}
	return buildMetadata(scene), nil
}

// Scrape searches for the query, picks the first result, generates NFO + downloads poster.
func (p *Provider) Scrape(query, filename, outDir string, opts scraper.ScrapeOpts) (*scraper.ScrapeResult, error) {
	scenes, err := p.SearchScene(query)
	if err != nil {
		return &scraper.ScrapeResult{Status: "error", Error: err.Error()}, err
	}
	if len(scenes) == 0 {
		return &scraper.ScrapeResult{Status: "not_found"}, nil
	}

	match := scenes[0]
	m := buildMetadata(match)

	stem := scraper.StemFilename(filename)
	nfoPath := filepath.Join(outDir, stem+".nfo")
	if err := scraper.GenerateMovieNFO(m, nfoPath); err != nil {
		return &scraper.ScrapeResult{Status: "error", Error: err.Error()}, err
	}

	// Download poster
	posterURL := extractPosterURL(match)
	if posterURL != "" {
		_ = scraper.DownloadImage(posterURL, filepath.Join(outDir, "poster.jpg"), 0, false)
	}

	return &scraper.ScrapeResult{
		Status: "ok",
		Match:  m.Title,
		IDs:    m.UniqueIDs,
		Meta:   m,
	}, nil
}

// ── ThePornDB API methods ─────────────────────────────────────────────────────

// SearchScene searches scenes by title/filename.
func (p *Provider) SearchScene(query string) ([]map[string]any, error) {
	data, err := p.get("/scenes", url.Values{"parse": {query}})
	if err != nil {
		return nil, err
	}
	return sliceMapVal(data, "data"), nil
}

// SceneDetail returns full scene details by ID.
func (p *Provider) SceneDetail(id string) (map[string]any, error) {
	data, err := p.get("/scenes/"+id, nil)
	if err != nil {
		return nil, err
	}
	if v, ok := data["data"].(map[string]any); ok {
		return v, nil
	}
	return data, nil
}

// SearchJAV searches JAV by number.
func (p *Provider) SearchJAV(query string) ([]map[string]any, error) {
	data, err := p.get("/jav", url.Values{"parse": {query}})
	if err != nil {
		return nil, err
	}
	return sliceMapVal(data, "data"), nil
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────

func (p *Provider) get(path string, params url.Values) (map[string]any, error) {
	reqURL := p.baseURL + path
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	p.throttle.Wait()
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("theporndb: HTTP %d for %s", resp.StatusCode, path)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

// ── Metadata builder ──────────────────────────────────────────────────────────

func buildMetadata(scene map[string]any) *scraper.Metadata {
	title := stringVal(scene, "title")
	date := stringVal(scene, "date")
	yr := yearFromDate(date)

	m := &scraper.Metadata{
		Title:     title,
		Year:      yr,
		Plot:      stringVal(scene, "description"),
		Premiered: date,
		UniqueIDs: map[string]string{},
	}

	if id := gconv.To[string, any](scene["id"]); id != "" {
		m.UniqueIDs["theporndb"] = id
	}

	// Studio
	if site, ok := scene["site"].(map[string]any); ok {
		if name := stringVal(site, "name"); name != "" {
			m.Studios = []string{name}
		}
	}

	// Tags → genres
	for _, t := range asSlice(scene["tags"]) {
		if tm, ok := t.(map[string]any); ok {
			if name := stringVal(tm, "name"); name != "" {
				m.Genres = append(m.Genres, name)
			}
		}
	}

	// Performers → actors
	for _, perf := range asSlice(scene["performers"]) {
		pm, ok := perf.(map[string]any)
		if !ok {
			continue
		}
		// TPDB nesting: performer may be under "parent" key
		parent, hasParent := pm["parent"].(map[string]any)
		if hasParent {
			if name := stringVal(parent, "name"); name != "" {
				m.Actors = append(m.Actors, scraper.Actor{Name: name})
			}
		} else if name := stringVal(pm, "name"); name != "" {
			m.Actors = append(m.Actors, scraper.Actor{Name: name})
		}
	}

	return m
}

func extractPosterURL(scene map[string]any) string {
	// Try posters field (dict with large/medium/small)
	if posters, ok := scene["posters"].(map[string]any); ok {
		for _, key := range []string{"large", "medium", "small"} {
			if v, ok := posters[key].(string); ok && v != "" {
				return v
			}
		}
	}
	// Try background field
	if bg, ok := scene["background"].(map[string]any); ok {
		for _, key := range []string{"large", "medium", "small"} {
			if v, ok := bg[key].(string); ok && v != "" {
				return v
			}
		}
	}
	// Try posters as slice
	for _, p := range asSlice(scene["posters"]) {
		switch v := p.(type) {
		case string:
			return v
		case map[string]any:
			if u := stringVal(v, "url"); u != "" {
				return u
			}
		}
	}
	return ""
}

// ── Utility helpers ───────────────────────────────────────────────────────────

func stringVal(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	return ""
}

func yearFromDate(date string) int {
	if len(date) < 4 {
		return 0
	}
	n := 0
	_, _ = fmt.Sscanf(date[:4], "%d", &n)
	return n
}

func asSlice(v any) []any {
	if s, ok := v.([]any); ok {
		return s
	}
	return nil
}

func sliceMapVal(m map[string]any, key string) []map[string]any {
	raw, ok := m[key]
	if !ok {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if mm, ok := item.(map[string]any); ok {
			out = append(out, mm)
		}
	}
	return out
}
