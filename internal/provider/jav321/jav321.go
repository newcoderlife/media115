// Package jav321 implements a scraper Provider backed by jav321.com HTML scraping.
package jav321

import (
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/antchfx/htmlquery"
	"golang.org/x/net/html"

	"github.com/newcoderlife/media115/internal/scraper"
)

const (
	defaultBaseURL = "https://www.jav321.com"
)

// Provider implements scraper.Provider for jav321.
type Provider struct {
	baseURL  string
	client   *http.Client
	throttle *scraper.Throttle
}

// New creates a new jav321 provider.
func New() *Provider {
	return NewWithBaseURL(defaultBaseURL)
}

// NewWithBaseURL creates a jav321 provider with a custom base URL (for testing).
func NewWithBaseURL(baseURL string) *Provider {
	jar := newNoCookieJar()
	return &Provider{
		baseURL: strings.TrimRight(baseURL, "/"),
		client: &http.Client{
			Timeout: 20 * time.Second,
			Jar:     jar,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse // manual redirect handling
			},
		},
		throttle: scraper.NewThrottle(0.8),
	}
}

// Name returns the provider name.
func (p *Provider) Name() string { return "jav321" }

// SupportedTypes returns the media types this provider handles.
func (p *Provider) SupportedTypes() []string { return []string{"av", "gravure"} }

// Priority returns 0 — jav321 runs before javfree.
func (p *Provider) Priority() int { return 0 }

// Search searches jav321 for the given number.
func (p *Provider) Search(query string, opts scraper.SearchOpts) ([]scraper.SearchResult, error) {
	meta, err := p.FetchMetadata(query)
	if err != nil || meta == nil {
		return nil, nil
	}
	number, _ := meta["number"].(string)
	title, _ := meta["title"].(string)
	return []scraper.SearchResult{{
		ID:    number,
		Title: title,
	}}, nil
}

// Detail returns metadata for a jav321 number (used as ID).
func (p *Provider) Detail(id string) (*scraper.Metadata, error) {
	meta, err := p.FetchMetadata(id)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, fmt.Errorf("jav321: not found: %s", id)
	}
	return buildMetadata(meta), nil
}

// Scrape fetches metadata for the given AV number, writes NFO + poster.
func (p *Provider) Scrape(query, filename, outDir string, opts scraper.ScrapeOpts) (*scraper.ScrapeResult, error) {
	meta, err := p.FetchMetadata(query)
	if err != nil {
		return &scraper.ScrapeResult{Status: "error", Error: err.Error()}, err
	}
	if meta == nil {
		return &scraper.ScrapeResult{Status: "not_found"}, nil
	}

	m := buildMetadata(meta)
	stem := scraper.StemFilename(filename)
	nfoPath := filepath.Join(outDir, stem+".nfo")
	if err := scraper.GenerateMovieNFO(m, nfoPath); err != nil {
		return &scraper.ScrapeResult{Status: "error", Error: err.Error()}, err
	}

	if coverURL, _ := meta["cover_url"].(string); coverURL != "" {
		_ = scraper.DownloadImage(coverURL, filepath.Join(outDir, "poster.jpg"), 0, false)
	}

	return &scraper.ScrapeResult{
		Status: "ok",
		Match:  m.Title,
		IDs:    m.UniqueIDs,
		Meta:   m,
	}, nil
}

// FetchMetadata performs the jav321 POST search and parses the detail page.
func (p *Provider) FetchMetadata(number string) (map[string]any, error) {
	p.throttle.Wait()
	// POST to /search with sn=number
	formData := url.Values{"sn": {number}}
	req, err := http.NewRequest(http.MethodPost, p.baseURL+"/search",
		strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Expect a redirect to /video/...
	if resp.StatusCode != http.StatusMovedPermanently &&
		resp.StatusCode != http.StatusFound &&
		resp.StatusCode != http.StatusSeeOther {
		return nil, nil
	}

	location := resp.Header.Get("Location")
	if !strings.Contains(location, "/video/") {
		return nil, nil
	}

	detailURL := location
	if !strings.HasPrefix(location, "http") {
		detailURL = p.baseURL + location
	}

	// GET the detail page
	detailReq, err := http.NewRequest(http.MethodGet, detailURL, nil)
	if err != nil {
		return nil, err
	}
	detailReq.Header.Set("User-Agent", "Mozilla/5.0")

	// Use a non-redirect-blocking client for the detail page
	plainClient := &http.Client{Timeout: 20 * time.Second}
	detailResp, err := plainClient.Do(detailReq)
	if err != nil {
		return nil, err
	}
	defer detailResp.Body.Close()

	if detailResp.StatusCode != http.StatusOK {
		return nil, nil
	}

	doc, err := html.Parse(detailResp.Body)
	if err != nil {
		return nil, err
	}
	return parseDetail(doc, number), nil
}

// parseDetail extracts AV metadata from the jav321 detail page.
func parseDetail(doc *html.Node, number string) map[string]any {
	result := map[string]any{
		"number": number,
	}

	// Title: //h3/text()
	if nodes := htmlquery.Find(doc, "//h3"); len(nodes) > 0 {
		result["title"] = strings.TrimSpace(htmlquery.InnerText(nodes[0]))
	}
	if result["title"] == "" {
		result["title"] = number
	}

	// Info div
	infoDivs := htmlquery.Find(doc, `//div[@class="col-md-9"]`)
	if len(infoDivs) > 0 {
		infoText := htmlquery.InnerText(infoDivs[0])
		result["release_date"] = extractField(infoText, "配信開始日", "發行日期", "发行日期")
		result["runtime"] = extractDigits(extractField(infoText, "収録時間", "長度"))
		result["studio"] = extractField(infoText, "メーカー", "製作商")
		result["label"] = extractField(infoText, "レーベル", "發行商")
		result["series"] = extractField(infoText, "シリーズ", "系列")
	}

	// Actors: //div[@class="col-md-9"]//a[contains(@href, "/star/")]
	var actors []string
	for _, a := range htmlquery.Find(doc, `//div[@class="col-md-9"]//a[contains(@href, "/star/")]`) {
		if name := strings.TrimSpace(htmlquery.InnerText(a)); name != "" {
			actors = append(actors, name)
		}
	}
	result["actors"] = actors

	// Genres: //div[@class="col-md-9"]//a[contains(@href, "/genre/")]
	var genres []string
	for _, g := range htmlquery.Find(doc, `//div[@class="col-md-9"]//a[contains(@href, "/genre/")]`) {
		if name := strings.TrimSpace(htmlquery.InnerText(g)); name != "" {
			genres = append(genres, name)
		}
	}
	result["genres"] = genres

	// Cover image
	coverURL := ""
	covers := htmlquery.Find(doc, `//img[contains(@class, "img-responsive")]/@src`)
	if len(covers) > 0 {
		coverURL = htmlquery.InnerText(covers[0])
	}
	if coverURL == "" {
		covers = htmlquery.Find(doc, `//div[@class="col-md-3"]//img/@src`)
		if len(covers) > 0 {
			coverURL = htmlquery.InnerText(covers[0])
		}
	}
	result["cover_url"] = coverURL

	return result
}

// ── Metadata builder ──────────────────────────────────────────────────────────

func buildMetadata(meta map[string]any) *scraper.Metadata {
	title, _ := meta["title"].(string)
	number, _ := meta["number"].(string)
	studio, _ := meta["studio"].(string)
	actors, _ := meta["actors"].([]string)
	genres, _ := meta["genres"].([]string)
	coverURL, _ := meta["cover_url"].(string)

	m := &scraper.Metadata{
		Title:         title,
		OriginalTitle: title,
		Plot:          "",
		Genres:        genres,
		PosterURL:     coverURL,
		UniqueIDs:     map[string]string{"jav321": number},
	}

	if studio != "" {
		m.Studios = []string{studio}
	}

	releaseDate, _ := meta["release_date"].(string)
	if releaseDate != "" {
		m.Premiered = releaseDate
		if len(releaseDate) >= 4 {
			if yr, err := parseInt(releaseDate[:4]); err == nil {
				m.Year = yr
			}
		}
	}

	if runtimeStr, _ := meta["runtime"].(string); runtimeStr != "" {
		if rt, err := parseInt(runtimeStr); err == nil {
			m.Runtime = rt
		}
	}

	for _, a := range actors {
		m.Actors = append(m.Actors, scraper.Actor{Name: a})
	}

	return m
}

// ── Text helpers ──────────────────────────────────────────────────────────────

// extractField extracts the value following any of the given labels in text.
func extractField(text string, labels ...string) string {
	for _, label := range labels {
		if idx := strings.Index(text, label); idx >= 0 {
			rest := text[idx+len(label):]
			// trim separator chars
			rest = strings.TrimLeftFunc(rest, func(r rune) bool {
				return r == ':' || r == '：' || unicode.IsSpace(r)
			})
			// take up to newline
			if nl := strings.IndexByte(rest, '\n'); nl >= 0 {
				rest = rest[:nl]
			}
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// extractDigits returns only the digit characters from s.
func extractDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func parseInt(s string) (int, error) {
	n := 0
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// noCookieJar is a minimal http.CookieJar that discards all cookies.
type noCookieJar struct{}

func newNoCookieJar() http.CookieJar { return noCookieJar{} }

func (noCookieJar) SetCookies(*url.URL, []*http.Cookie) {}
func (noCookieJar) Cookies(*url.URL) []*http.Cookie     { return nil }
