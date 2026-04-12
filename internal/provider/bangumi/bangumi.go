// Package bangumi implements a scraper Provider backed by the Bangumi API (bgm.tv).
package bangumi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/bytedance/gg/gconv"
	"github.com/newcoderlife/media115/internal/scraper"
)

const (
	defaultBaseURL = "https://api.bgm.tv"
	userAgent      = "media115/0.1"
)

// Provider implements scraper.Provider for Bangumi.
type Provider struct {
	token    string
	baseURL  string
	client   *http.Client
	throttle *scraper.Throttle
}

// New creates a new Bangumi provider. token may be empty for unauthenticated access.
func New(token string) *Provider {
	return NewWithBaseURL(token, defaultBaseURL)
}

// NewWithBaseURL creates a Bangumi provider with a custom base URL (for testing).
func NewWithBaseURL(token, baseURL string) *Provider {
	return &Provider{
		token:    token,
		baseURL:  strings.TrimRight(baseURL, "/"),
		client:   &http.Client{Timeout: 10 * time.Second},
		throttle: scraper.NewThrottle(0.8),
	}
}

// Name returns the provider name.
func (p *Provider) Name() string { return "bangumi" }

// SupportedTypes returns the media types this provider handles.
func (p *Provider) SupportedTypes() []string { return []string{"anime"} }

// Priority returns 0 — bangumi runs before TMDB for anime.
func (p *Provider) Priority() int { return 0 }

// Search searches Bangumi for the given keyword (anime subject type=2).
func (p *Provider) Search(query string, opts scraper.SearchOpts) ([]scraper.SearchResult, error) {
	subjects, err := p.SearchSubjects(query, 2, 10)
	if err != nil {
		return nil, err
	}
	var results []scraper.SearchResult
	for _, s := range subjects {
		yr := 0
		if airDate, ok := s["air_date"].(string); ok && len(airDate) >= 4 {
			yr, _ = strconv.Atoi(airDate[:4])
		}
		id := gconv.To[string, any](s["id"])
		title := stringVal(s, "name_cn", "name")
		results = append(results, scraper.SearchResult{ID: id, Title: title, Year: yr})
	}
	return results, nil
}

// Detail returns full metadata for the given Bangumi subject ID.
func (p *Provider) Detail(id string) (*scraper.Metadata, error) {
	subID, err := strconv.Atoi(id)
	if err != nil {
		return nil, fmt.Errorf("bangumi: invalid subject id %q", id)
	}
	subject, err := p.Subject(subID)
	if err != nil {
		return nil, err
	}
	return buildMetadata(subject, 0, 0), nil
}

// Scrape performs search → match → NFO generation.
func (p *Provider) Scrape(query, filename, outDir string, opts scraper.ScrapeOpts) (*scraper.ScrapeResult, error) {
	subjects, err := p.SearchSubjects(query, 2, 10)
	if err != nil {
		return &scraper.ScrapeResult{Status: "error", Error: err.Error()}, err
	}
	if len(subjects) == 0 {
		return &scraper.ScrapeResult{Status: "not_found"}, nil
	}

	best := subjects[0]
	bgmID := gconv.To[int, any](best["id"])
	if bgmID == 0 {
		return &scraper.ScrapeResult{Status: "not_found"}, nil
	}

	// Fetch full subject detail
	subject, err := p.Subject(bgmID)
	if err != nil {
		// Fall back to search result
		subject = best
	}

	m := buildMetadata(subject, opts.Season, opts.Episode)

	stem := scraper.StemFilename(filename)
	nfoPath := filepath.Join(outDir, stem+".nfo")
	if err := scraper.GenerateEpisodeNFO(m, nfoPath); err != nil {
		return &scraper.ScrapeResult{Status: "error", Error: err.Error()}, err
	}

	// Generate tvshow.nfo if it doesn't already exist in outDir.
	tvshowPath := filepath.Join(outDir, "tvshow.nfo")
	if _, err := os.Stat(tvshowPath); os.IsNotExist(err) {
		tvshowMeta := &scraper.Metadata{
			Title:     m.Title,
			ShowTitle: m.ShowTitle,
			Year:      m.Year,
			Premiered: m.Premiered,
			Aired:     m.Aired,
			Plot:      m.Plot,
			Rating:    m.Rating,
			Votes:     m.Votes,
			PosterURL: m.PosterURL,
			Tags:      m.Tags,
			UniqueIDs: m.UniqueIDs,
		}
		_ = scraper.GenerateTVShowNFO(tvshowMeta, tvshowPath)
	}

	// Download poster if available
	if m.PosterURL != "" {
		destPath := filepath.Join(outDir, "poster.jpg")
		_ = scraper.DownloadImage(m.PosterURL, destPath, 0, false)
	}

	return &scraper.ScrapeResult{
		Status: "ok",
		Match:  m.Title,
		IDs:    m.UniqueIDs,
		Meta:   m,
	}, nil
}

// ── Bangumi API methods ───────────────────────────────────────────────────────

// SearchSubjects searches Bangumi subjects by keyword and type.
// subjectType: 1=book, 2=anime, 3=music, 4=game, 6=real.
func (p *Provider) SearchSubjects(keyword string, subjectType, limit int) ([]map[string]any, error) {
	body := map[string]any{
		"keyword": keyword,
		"filter":  map[string]any{"type": []int{subjectType}},
	}
	params := url.Values{"limit": {strconv.Itoa(limit)}}
	data, err := p.post("/v0/search/subjects", params, body)
	if err != nil {
		return nil, err
	}
	raw, _ := data["data"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out, nil
}

// Subject returns full metadata for a subject by ID.
func (p *Provider) Subject(id int) (map[string]any, error) {
	return p.get(fmt.Sprintf("/v0/subjects/%d", id), nil)
}

// Episodes returns episodes for a subject.
func (p *Provider) Episodes(subjectID, episodeType, limit, offset int) ([]map[string]any, error) {
	params := url.Values{
		"subject_id": {strconv.Itoa(subjectID)},
		"limit":      {strconv.Itoa(limit)},
		"offset":     {strconv.Itoa(offset)},
	}
	if episodeType >= 0 {
		params.Set("type", strconv.Itoa(episodeType))
	}
	data, err := p.get("/v0/episodes", params)
	if err != nil {
		return nil, err
	}
	raw, _ := data["data"].([]any)
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if m, ok := item.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out, nil
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
	p.addHeaders(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("bangumi: HTTP %d for %s", resp.StatusCode, path)
	}
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func (p *Provider) post(path string, params url.Values, body any) (map[string]any, error) {
	reqURL := p.baseURL + path
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	p.throttle.Wait()
	req, err := http.NewRequest(http.MethodPost, reqURL, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	p.addHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("bangumi: HTTP %d for %s", resp.StatusCode, path)
	}
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func (p *Provider) addHeaders(req *http.Request) {
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}
}

// ── Metadata builder ──────────────────────────────────────────────────────────

func buildMetadata(subject map[string]any, season, episode int) *scraper.Metadata {
	originalTitle := stringVal(subject, "name")   // Japanese name
	chineseTitle := stringVal(subject, "name_cn") // Chinese name
	title := chineseTitle
	if title == "" {
		title = originalTitle
	}
	m := &scraper.Metadata{
		Title:         title,
		OriginalTitle: originalTitle,
		ShowTitle:     title,
		Season:        season,
		Episode:       episode,
		Plot:          stringVal(subject, "summary"),
		UniqueIDs:     map[string]string{},
	}

	if id := gconv.To[int, any](subject["id"]); id != 0 {
		m.UniqueIDs["bangumi"] = strconv.Itoa(id)
	}

	// Air date / year
	if airDate := stringVal(subject, "air_date"); airDate != "" {
		m.Aired = airDate
		if len(airDate) >= 4 {
			m.Year, _ = strconv.Atoi(airDate[:4])
			m.Premiered = airDate
		}
	}

	// Rating
	if rating, ok := subject["rating"].(map[string]any); ok {
		m.Rating = gconv.To[float64, any](rating["score"])
		m.Votes = gconv.To[int, any](rating["total"])
	}

	// Poster
	if images, ok := subject["images"].(map[string]any); ok {
		m.PosterURL = stringVal(images, "large", "medium", "small")
	}

	// Tags
	for _, t := range asSlice(subject["tags"]) {
		if tm, ok := t.(map[string]any); ok {
			if name := stringVal(tm, "name"); name != "" {
				m.Tags = append(m.Tags, name)
			}
		}
	}

	return m
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

func asSlice(v any) []any {
	if s, ok := v.([]any); ok {
		return s
	}
	return nil
}
