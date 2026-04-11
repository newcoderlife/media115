// Package tmdb implements a scraper Provider backed by The Movie Database (TMDB) REST API.
package tmdb

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/newcoderlife/media115/internal/scraper"
)

const (
	defaultBaseURL  = "https://api.themoviedb.org/3"
	defaultLanguage = "zh-CN"
	imageBase       = "https://image.tmdb.org/t/p"
)

// Provider implements scraper.Provider for TMDB.
type Provider struct {
	token    string
	baseURL  string
	language string
	client   *http.Client
	throttle *scraper.Throttle
}

// New creates a TMDB provider with the given read-access token.
func New(token string) *Provider {
	return NewWithBaseURL(token, defaultBaseURL)
}

// NewWithBaseURL creates a TMDB provider using a custom base URL (for testing).
func NewWithBaseURL(token, baseURL string) *Provider {
	return &Provider{
		token:    token,
		baseURL:  strings.TrimRight(baseURL, "/"),
		language: defaultLanguage,
		client:   &http.Client{Timeout: 15 * time.Second},
		throttle: scraper.NewThrottle(0.8),
	}
}

// Name returns the provider name.
func (p *Provider) Name() string { return "tmdb" }

// SupportedTypes returns the media types this provider handles.
func (p *Provider) SupportedTypes() []string { return []string{"movie", "tv", "anime"} }

// Priority returns 1 for anime (bangumi runs first at 0).
func (p *Provider) Priority() int { return 1 }

// Search searches TMDB for the given query.
func (p *Provider) Search(query string, opts scraper.SearchOpts) ([]scraper.SearchResult, error) {
	movies, err := p.SearchMovie(query, "")
	if err != nil {
		return nil, err
	}
	var results []scraper.SearchResult
	for _, m := range movies {
		yr := yearFromDate(m["release_date"])
		results = append(results, scraper.SearchResult{
			ID:    fmt.Sprintf("movie:%v", m["id"]),
			Title: stringVal(m, "title", "original_title"),
			Year:  yr,
		})
	}
	tvs, err := p.SearchTV(query, "")
	if err == nil {
		for _, t := range tvs {
			results = append(results, scraper.SearchResult{
				ID:    fmt.Sprintf("tv:%v", t["id"]),
				Title: stringVal(t, "name", "original_name"),
				Year:  yearFromDate(t["first_air_date"]),
			})
		}
	}
	return results, nil
}

// Detail returns full metadata for a provider-specific ID (format: "movie:12345" or "tv:67890").
func (p *Provider) Detail(id string) (*scraper.Metadata, error) {
	parts := strings.SplitN(id, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("tmdb: invalid id format %q", id)
	}
	switch parts[0] {
	case "movie":
		return p.movieMetadata(parts[1])
	case "tv":
		return p.tvMetadata(parts[1], 0, 0)
	default:
		return nil, fmt.Errorf("tmdb: unknown type %q", parts[0])
	}
}

// Scrape performs search → best match → NFO + poster download.
func (p *Provider) Scrape(query, filename, outDir string, opts scraper.ScrapeOpts) (*scraper.ScrapeResult, error) {
	if opts.Season > 0 {
		return p.scrapeTV(query, filename, outDir, opts)
	}
	return p.scrapeMovie(query, filename, outDir, opts)
}

func (p *Provider) scrapeMovie(query, filename, outDir string, opts scraper.ScrapeOpts) (*scraper.ScrapeResult, error) {
	results, err := p.SearchMovie(query, "")
	if err != nil {
		return &scraper.ScrapeResult{Status: "error", Error: err.Error()}, err
	}
	if len(results) == 0 {
		simplified := strings.Split(query, ".")[0]
		if simplified != query {
			results, err = p.SearchMovie(simplified, "")
		}
		if err != nil || len(results) == 0 {
			return &scraper.ScrapeResult{Status: "not_found"}, nil
		}
	}

	best := bestMatch(results, opts.Year, "release_date")
	tmdbID := intVal(best, "id")
	if tmdbID == 0 {
		return &scraper.ScrapeResult{Status: "not_found"}, nil
	}

	detail, err := p.MovieDetail(tmdbID, "")
	if err != nil {
		return &scraper.ScrapeResult{Status: "error", Error: err.Error()}, err
	}
	images, _ := p.MovieImages(tmdbID)
	if images == nil {
		images = map[string]any{}
	}
	credits, _ := p.MovieCredits(tmdbID, "")
	if credits == nil {
		credits = map[string]any{}
	}

	m := buildMovieMetadata(detail, credits)
	stem := scraper.StemFilename(filename)
	nfoPath := filepath.Join(outDir, stem+".nfo")
	if err := scraper.GenerateMovieNFO(m, nfoPath); err != nil {
		return &scraper.ScrapeResult{Status: "error", Error: err.Error()}, err
	}

	_ = saveTMDBPoster(images, outDir)

	return &scraper.ScrapeResult{
		Status: "ok",
		Match:  m.Title,
		IDs:    m.UniqueIDs,
		Meta:   m,
	}, nil
}

func (p *Provider) scrapeTV(query, filename, outDir string, opts scraper.ScrapeOpts) (*scraper.ScrapeResult, error) {
	cleanRe := regexp.MustCompile(`^\[.*?\]\s*`)
	clean := cleanRe.ReplaceAllString(query, "")
	clean = strings.ReplaceAll(clean, ".", " ")
	clean = strings.TrimSpace(clean)

	results, err := p.SearchTV(clean, "")
	if err != nil {
		return &scraper.ScrapeResult{Status: "error", Error: err.Error()}, err
	}
	if len(results) == 0 {
		words := strings.Fields(clean)
		if len(words) > 1 {
			results, _ = p.SearchTV(words[0], "")
		}
		if len(results) == 0 {
			return &scraper.ScrapeResult{Status: "not_found"}, nil
		}
	}

	best := results[0]
	tmdbID := intVal(best, "id")
	if tmdbID == 0 {
		return &scraper.ScrapeResult{Status: "not_found"}, nil
	}

	detail, err := p.TVDetail(tmdbID, "")
	if err != nil {
		return &scraper.ScrapeResult{Status: "error", Error: err.Error()}, err
	}
	credits, _ := p.TVCredits(tmdbID, "")
	if credits == nil {
		credits = map[string]any{}
	}

	m := buildTVMetadata(detail, credits, opts.Season, opts.Episode)

	if opts.Season > 0 && opts.Episode > 0 {
		if seasonData, err := p.SeasonDetail(tmdbID, opts.Season, ""); err == nil {
			enrichEpisodeMetadata(m, seasonData, opts.Episode)
		}
	}

	stem := scraper.StemFilename(filename)
	nfoPath := filepath.Join(outDir, stem+".nfo")
	if err := scraper.GenerateEpisodeNFO(m, nfoPath); err != nil {
		return &scraper.ScrapeResult{Status: "error", Error: err.Error()}, err
	}

	// Generate tvshow.nfo if it doesn't already exist in outDir.
	tvshowPath := filepath.Join(outDir, "tvshow.nfo")
	if _, err := os.Stat(tvshowPath); os.IsNotExist(err) {
		tvshowMeta := &scraper.Metadata{
			Title:         stringVal(detail, "name", "original_name"),
			OriginalTitle: stringVal(detail, "original_name"),
			ShowTitle:     stringVal(detail, "name", "original_name"),
			Year:          yearFromDate(detail["first_air_date"]),
			Premiered:     stringVal(detail, "first_air_date"),
			Plot:          stringVal(detail, "overview"),
			Rating:        floatVal(detail, "vote_average"),
			Votes:         intVal(detail, "vote_count"),
			UniqueIDs:     m.UniqueIDs,
		}
		if pp := stringVal(detail, "poster_path"); pp != "" {
			tvshowMeta.PosterURL = imageBase + "/original" + pp
		}
		if bp := stringVal(detail, "backdrop_path"); bp != "" {
			tvshowMeta.FanartURL = imageBase + "/original" + bp
		}
		for _, g := range asSlice(detail["genres"]) {
			if gm, ok := g.(map[string]any); ok {
				tvshowMeta.Genres = append(tvshowMeta.Genres, stringVal(gm, "name"))
			}
		}
		for _, c := range asSlice(detail["production_companies"]) {
			if cm, ok := c.(map[string]any); ok {
				tvshowMeta.Studios = append(tvshowMeta.Studios, stringVal(cm, "name"))
			}
		}
		_ = scraper.GenerateTVShowNFO(tvshowMeta, tvshowPath)
	}

	return &scraper.ScrapeResult{
		Status: "ok",
		Match:  m.Title,
		IDs:    m.UniqueIDs,
		Meta:   m,
	}, nil
}

// ── TMDB API methods ──────────────────────────────────────────────────────────

// SearchMovie searches the TMDB movie database.
func (p *Provider) SearchMovie(query, language string) ([]map[string]any, error) {
	params := url.Values{"query": {query}, "language": {coalesce(language, p.language)}}
	data, err := p.get("/search/movie", params)
	if err != nil {
		return nil, err
	}
	return sliceVal(data, "results"), nil
}

// MovieDetail fetches full movie detail.
func (p *Provider) MovieDetail(id int, language string) (map[string]any, error) {
	params := url.Values{"language": {coalesce(language, p.language)}}
	return p.get(fmt.Sprintf("/movie/%d", id), params)
}

// MovieImages fetches movie images.
func (p *Provider) MovieImages(id int) (map[string]any, error) {
	params := url.Values{"include_image_language": {"en,zh,null"}}
	return p.get(fmt.Sprintf("/movie/%d/images", id), params)
}

// MovieCredits fetches movie credits.
func (p *Provider) MovieCredits(id int, language string) (map[string]any, error) {
	params := url.Values{"language": {coalesce(language, p.language)}}
	return p.get(fmt.Sprintf("/movie/%d/credits", id), params)
}

// SearchTV searches the TMDB TV database.
func (p *Provider) SearchTV(query, language string) ([]map[string]any, error) {
	params := url.Values{"query": {query}, "language": {coalesce(language, p.language)}}
	data, err := p.get("/search/tv", params)
	if err != nil {
		return nil, err
	}
	return sliceVal(data, "results"), nil
}

// TVDetail fetches full TV show detail.
func (p *Provider) TVDetail(id int, language string) (map[string]any, error) {
	params := url.Values{"language": {coalesce(language, p.language)}}
	return p.get(fmt.Sprintf("/tv/%d", id), params)
}

// SeasonDetail fetches season detail including episodes.
func (p *Provider) SeasonDetail(tvID, season int, language string) (map[string]any, error) {
	params := url.Values{"language": {coalesce(language, p.language)}}
	return p.get(fmt.Sprintf("/tv/%d/season/%d", tvID, season), params)
}

// TVCredits fetches TV show credits.
func (p *Provider) TVCredits(id int, language string) (map[string]any, error) {
	params := url.Values{"language": {coalesce(language, p.language)}}
	return p.get(fmt.Sprintf("/tv/%d/credits", id), params)
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────

func (p *Provider) get(path string, params url.Values) (map[string]any, error) {
	reqURL := p.baseURL + path
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}

	p.throttle.Wait()
	for attempt := 0; attempt < 3; attempt++ {
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

		if resp.StatusCode == http.StatusTooManyRequests {
			retryAfter := 5
			if v := resp.Header.Get("Retry-After"); v != "" {
				if n, e := strconv.Atoi(v); e == nil {
					retryAfter = n
				}
			}
			resp.Body.Close()
			time.Sleep(time.Duration(retryAfter) * time.Second)
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			resp.Body.Close()
			return nil, fmt.Errorf("tmdb: HTTP %d for %s", resp.StatusCode, path)
		}

		var result map[string]any
		err = json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		return result, nil
	}
	return nil, fmt.Errorf("tmdb: rate limit exceeded after 3 retries for %s", path)
}

// ── Metadata builders ─────────────────────────────────────────────────────────

func buildMovieMetadata(detail, credits map[string]any) *scraper.Metadata {
	m := &scraper.Metadata{
		Title:         stringVal(detail, "title", "original_title"),
		OriginalTitle: stringVal(detail, "original_title"),
		Year:          yearFromDate(detail["release_date"]),
		Plot:          stringVal(detail, "overview"),
		Tagline:       stringVal(detail, "tagline"),
		Runtime:       intVal(detail, "runtime"),
		Rating:        floatVal(detail, "vote_average"),
		Votes:         intVal(detail, "vote_count"),
		Premiered:     stringVal(detail, "release_date"),
		UniqueIDs:     map[string]string{},
	}

	if id := intVal(detail, "id"); id != 0 {
		m.UniqueIDs["tmdb"] = strconv.Itoa(id)
	}
	if imdb := stringVal(detail, "imdb_id"); imdb != "" {
		m.UniqueIDs["imdb"] = imdb
	}
	if coll, ok := detail["belongs_to_collection"].(map[string]any); ok {
		m.Set = stringVal(coll, "name")
	}

	for _, g := range asSlice(detail["genres"]) {
		if gm, ok := g.(map[string]any); ok {
			m.Genres = append(m.Genres, stringVal(gm, "name"))
		}
	}
	for _, c := range asSlice(detail["production_companies"]) {
		if cm, ok := c.(map[string]any); ok {
			m.Studios = append(m.Studios, stringVal(cm, "name"))
		}
	}
	for _, c := range asSlice(detail["production_countries"]) {
		if cm, ok := c.(map[string]any); ok {
			m.Countries = append(m.Countries, stringVal(cm, "name"))
		}
	}

	if pp := stringVal(detail, "poster_path"); pp != "" {
		m.PosterURL = imageBase + "/original" + pp
	}
	if bp := stringVal(detail, "backdrop_path"); bp != "" {
		m.FanartURL = imageBase + "/original" + bp
	}

	for _, c := range asSlice(credits["crew"]) {
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		switch stringVal(cm, "job") {
		case "Director":
			m.Directors = append(m.Directors, stringVal(cm, "name"))
		case "Screenplay", "Writer":
			m.Credits = append(m.Credits, stringVal(cm, "name"))
		}
	}
	for i, c := range asSlice(credits["cast"]) {
		if i >= 15 {
			break
		}
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		thumb := ""
		if pp := stringVal(cm, "profile_path"); pp != "" {
			thumb = imageBase + "/w185" + pp
		}
		m.Actors = append(m.Actors, scraper.Actor{
			Name:  stringVal(cm, "name"),
			Role:  stringVal(cm, "character"),
			Thumb: thumb,
		})
	}
	return m
}

func buildTVMetadata(detail, credits map[string]any, season, episode int) *scraper.Metadata {
	m := &scraper.Metadata{
		Title:     stringVal(detail, "name", "original_name"),
		ShowTitle: stringVal(detail, "name", "original_name"),
		Season:    season,
		Episode:   episode,
		Plot:      stringVal(detail, "overview"),
		Aired:     stringVal(detail, "first_air_date"),
		Rating:    floatVal(detail, "vote_average"),
		Votes:     intVal(detail, "vote_count"),
		UniqueIDs: map[string]string{},
	}

	if id := intVal(detail, "id"); id != 0 {
		m.UniqueIDs["tmdb"] = strconv.Itoa(id)
	}

	for i, c := range asSlice(credits["crew"]) {
		if i >= 3 {
			break
		}
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		if stringVal(cm, "job") == "Director" {
			m.Directors = append(m.Directors, stringVal(cm, "name"))
		}
	}
	for i, c := range asSlice(credits["cast"]) {
		if i >= 10 {
			break
		}
		cm, ok := c.(map[string]any)
		if !ok {
			continue
		}
		m.Actors = append(m.Actors, scraper.Actor{
			Name: stringVal(cm, "name"),
			Role: stringVal(cm, "character"),
		})
	}
	return m
}

func enrichEpisodeMetadata(m *scraper.Metadata, seasonData map[string]any, episode int) {
	for _, ep := range asSlice(seasonData["episodes"]) {
		epm, ok := ep.(map[string]any)
		if !ok {
			continue
		}
		if intVal(epm, "episode_number") == episode {
			if v := stringVal(epm, "overview"); v != "" {
				m.Plot = v
			}
			if v := stringVal(epm, "air_date"); v != "" {
				m.Aired = v
			}
			if v := stringVal(epm, "name"); v != "" {
				m.Title = v
			}
			break
		}
	}
}

func (p *Provider) movieMetadata(idStr string) (*scraper.Metadata, error) {
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return nil, fmt.Errorf("tmdb: invalid movie id %q", idStr)
	}
	detail, err := p.MovieDetail(id, "")
	if err != nil {
		return nil, err
	}
	credits, _ := p.MovieCredits(id, "")
	if credits == nil {
		credits = map[string]any{}
	}
	return buildMovieMetadata(detail, credits), nil
}

func (p *Provider) tvMetadata(idStr string, season, episode int) (*scraper.Metadata, error) {
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return nil, fmt.Errorf("tmdb: invalid tv id %q", idStr)
	}
	detail, err := p.TVDetail(id, "")
	if err != nil {
		return nil, err
	}
	credits, _ := p.TVCredits(id, "")
	if credits == nil {
		credits = map[string]any{}
	}
	return buildTVMetadata(detail, credits, season, episode), nil
}

func saveTMDBPoster(images map[string]any, outDir string) error {
	posters := asSlice(images["posters"])
	if len(posters) == 0 {
		return nil
	}
	pm, ok := posters[0].(map[string]any)
	if !ok {
		return nil
	}
	filePath := stringVal(pm, "file_path")
	if filePath == "" {
		return nil
	}
	return scraper.SavePoster(filePath, outDir, "w500", "poster.jpg")
}

// ── Utility helpers ───────────────────────────────────────────────────────────

func bestMatch(results []map[string]any, year int, dateField string) map[string]any {
	if year == 0 || len(results) == 0 {
		return results[0]
	}
	yearStr := strconv.Itoa(year)
	for _, r := range results {
		d, _ := r[dateField].(string)
		if len(d) >= 4 && d[:4] == yearStr {
			return r
		}
	}
	return results[0]
}

func yearFromDate(v any) int {
	s, ok := v.(string)
	if !ok || len(s) < 4 {
		return 0
	}
	n, _ := strconv.Atoi(s[:4])
	return n
}

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

func intVal(m map[string]any, keys ...string) int {
	for _, k := range keys {
		v, ok := m[k]
		if !ok {
			continue
		}
		switch n := v.(type) {
		case int:
			return n
		case int64:
			return int(n)
		case float64:
			return int(n)
		case json.Number:
			i, _ := n.Int64()
			return int(i)
		}
	}
	return 0
}

func floatVal(m map[string]any, keys ...string) float64 {
	for _, k := range keys {
		v, ok := m[k]
		if !ok {
			continue
		}
		switch n := v.(type) {
		case float64:
			return n
		case json.Number:
			f, _ := n.Float64()
			return f
		}
	}
	return 0
}

func sliceVal(m map[string]any, key string) []map[string]any {
	v, ok := m[key]
	if !ok {
		return nil
	}
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if m2, ok := item.(map[string]any); ok {
			out = append(out, m2)
		}
	}
	return out
}

func asSlice(v any) []any {
	if v == nil {
		return nil
	}
	s, ok := v.([]any)
	if !ok {
		return nil
	}
	return s
}

func coalesce(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
