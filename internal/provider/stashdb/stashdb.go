// Package stashdb implements a scraper Provider backed by the StashDB GraphQL API.
package stashdb

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/bytedance/gg/gconv"
	"github.com/newcoderlife/media115/internal/scraper"
)

const defaultEndpoint = "https://stashdb.org/graphql"

// Provider implements scraper.Provider for StashDB.
type Provider struct {
	apiKey   string
	endpoint string
	client   *http.Client
	throttle *scraper.Throttle
}

// New creates a new StashDB provider.
func New(apiKey string) *Provider {
	return NewWithEndpoint(apiKey, defaultEndpoint)
}

// NewWithEndpoint creates a StashDB provider with a custom GraphQL endpoint (for testing).
func NewWithEndpoint(apiKey, endpoint string) *Provider {
	return &Provider{
		apiKey:   apiKey,
		endpoint: endpoint,
		client:   &http.Client{Timeout: 15 * time.Second},
		throttle: scraper.NewThrottle(1.0),
	}
}

// Name returns the provider name.
func (p *Provider) Name() string { return "stashdb" }

// SupportedTypes returns the media types this provider handles.
func (p *Provider) SupportedTypes() []string { return []string{"av_west"} }

// Priority returns 1 — StashDB runs after ThePornDB.
func (p *Provider) Priority() int { return 1 }

// Search searches StashDB scenes by query.
func (p *Provider) Search(query string, opts scraper.SearchOpts) ([]scraper.SearchResult, error) {
	scenes, err := p.SearchScenes(query, 10)
	if err != nil {
		return nil, err
	}
	var results []scraper.SearchResult
	for _, s := range scenes {
		results = append(results, scraper.SearchResult{
			ID:    stringVal(s, "id"),
			Title: stringVal(s, "title"),
			Year:  yearFromDate(stringVal(s, "date")),
		})
	}
	return results, nil
}

// Detail returns full metadata for a StashDB scene ID.
func (p *Provider) Detail(id string) (*scraper.Metadata, error) {
	scene, err := p.SceneDetail(id)
	if err != nil {
		return nil, err
	}
	return buildMetadata(scene), nil
}

// Scrape searches, picks the first result, generates NFO + downloads poster.
func (p *Provider) Scrape(query, filename, outDir string, opts scraper.ScrapeOpts) (*scraper.ScrapeResult, error) {
	scenes, err := p.SearchScenes(query, 10)
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

	// Download poster from first image
	if imgURL := firstImageURL(match); imgURL != "" {
		_ = scraper.DownloadImage(imgURL, filepath.Join(outDir, "poster.jpg"), 0, false)
	}

	return &scraper.ScrapeResult{
		Status: "ok",
		Match:  m.Title,
		IDs:    m.UniqueIDs,
		Meta:   m,
	}, nil
}

// ── StashDB GraphQL methods ───────────────────────────────────────────────────

const searchScenesQuery = `
query ($term: String!, $per_page: Int!) {
    searchScene(term: $term, limit: $per_page) {
        id
        title
        date
        studio { id name }
        performers { performer { id name } as_role }
        urls { url type }
        images { url }
    }
}
`

const sceneDetailQuery = `
query ($id: ID!) {
    findScene(id: $id) {
        id
        title
        date
        details
        duration
        studio { id name }
        performers { performer { id name } as_role }
        urls { url type }
        images { url }
        tags { id name }
    }
}
`

// SearchScenes searches scenes by term.
func (p *Provider) SearchScenes(term string, perPage int) ([]map[string]any, error) {
	data, err := p.query(searchScenesQuery, map[string]any{
		"term":     term,
		"per_page": perPage,
	})
	if err != nil {
		return nil, err
	}
	return sliceMapVal(data, "searchScene"), nil
}

// SceneDetail returns full scene detail by ID.
func (p *Provider) SceneDetail(id string) (map[string]any, error) {
	data, err := p.query(sceneDetailQuery, map[string]any{"id": id})
	if err != nil {
		return nil, err
	}
	if v, ok := data["findScene"].(map[string]any); ok {
		return v, nil
	}
	return nil, fmt.Errorf("stashdb: scene %s not found", id)
}

// ── HTTP / GraphQL helpers ────────────────────────────────────────────────────

func (p *Provider) query(gql string, variables map[string]any) (map[string]any, error) {
	payload := map[string]any{
		"query":     gql,
		"variables": variables,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	p.throttle.Wait()
	req, err := http.NewRequest(http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if p.apiKey != "" {
		req.Header.Set("ApiKey", p.apiKey)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("stashdb: HTTP %d", resp.StatusCode)
	}

	var result struct {
		Data   map[string]any `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	if len(result.Errors) > 0 {
		msgs := make([]string, len(result.Errors))
		for i, e := range result.Errors {
			msgs[i] = e.Message
		}
		return nil, fmt.Errorf("stashdb: GraphQL errors: %s", strings.Join(msgs, "; "))
	}
	if result.Data == nil {
		return map[string]any{}, nil
	}
	return result.Data, nil
}

// ── Metadata builder ──────────────────────────────────────────────────────────

func buildMetadata(scene map[string]any) *scraper.Metadata {
	title := stringVal(scene, "title")
	date := stringVal(scene, "date")

	m := &scraper.Metadata{
		Title:     title,
		Year:      yearFromDate(date),
		Plot:      stringVal(scene, "details"),
		Premiered: date,
		UniqueIDs: map[string]string{},
	}

	if id := stringVal(scene, "id"); id != "" {
		m.UniqueIDs["stashdb"] = id
	}

	// Runtime (duration in seconds → minutes)
	if dur := gconv.To[float64, any](scene["duration"]); dur > 0 {
		m.Runtime = int(dur / 60)
	}

	// Studio
	if studio, ok := scene["studio"].(map[string]any); ok {
		if name := stringVal(studio, "name"); name != "" {
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
		role := stringVal(pm, "as_role")
		performer, ok := pm["performer"].(map[string]any)
		if !ok {
			continue
		}
		name := stringVal(performer, "name")
		if name != "" {
			m.Actors = append(m.Actors, scraper.Actor{Name: name, Role: role})
		}
	}

	return m
}

func firstImageURL(scene map[string]any) string {
	images := asSlice(scene["images"])
	if len(images) == 0 {
		return ""
	}
	switch img := images[0].(type) {
	case string:
		return img
	case map[string]any:
		return stringVal(img, "url")
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
	fmt.Sscanf(date[:4], "%d", &n)
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
