// Package javfree implements a scraper Provider backed by javfree.me HTML scraping.
package javfree

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/antchfx/htmlquery"
	"github.com/newcoderlife/media115/internal/scraper"
	"golang.org/x/net/html"
)

const defaultBaseURL = "https://javfree.me"

// Provider implements scraper.Provider for javfree.
type Provider struct {
	baseURL string
	client  *http.Client
}

// New creates a new javfree provider.
func New() *Provider {
	return NewWithBaseURL(defaultBaseURL)
}

// NewWithBaseURL creates a javfree provider with a custom base URL (for testing).
func NewWithBaseURL(baseURL string) *Provider {
	return &Provider{
		baseURL: strings.TrimRight(baseURL, "/"),
		client: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

// Name returns the provider name.
func (p *Provider) Name() string { return "javfree" }

// SupportedTypes returns the media types this provider handles.
func (p *Provider) SupportedTypes() []string { return []string{"av", "gravure"} }

// Priority returns 1 — javfree is secondary to jav321.
func (p *Provider) Priority() int { return 1 }

// Search searches javfree for the given number.
func (p *Provider) Search(query string, opts scraper.SearchOpts) ([]scraper.SearchResult, error) {
	meta, err := p.FetchMetadata(query)
	if err != nil || meta == nil {
		return nil, nil
	}
	title, _ := meta["title"].(string)
	number, _ := meta["number"].(string)
	return []scraper.SearchResult{{ID: number, Title: title}}, nil
}

// Detail returns metadata for a javfree number (used as ID).
func (p *Provider) Detail(id string) (*scraper.Metadata, error) {
	meta, err := p.FetchMetadata(id)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		return nil, fmt.Errorf("javfree: not found: %s", id)
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

// FetchMetadata retrieves AV metadata from javfree by number.
func (p *Provider) FetchMetadata(number string) (map[string]any, error) {
	// GET /{number}/ (lowercase)
	reqURL := p.baseURL + "/" + strings.ToLower(number) + "/"
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil
	}

	doc, err := html.Parse(resp.Body)
	if err != nil {
		return nil, err
	}

	return parseDetail(doc, number), nil
}

// parseDetail extracts metadata from the javfree detail page.
func parseDetail(doc *html.Node, number string) map[string]any {
	// Title: try entry-title class, then article h1, then h1
	title := ""
	for _, xpath := range []string{
		`//*[contains(@class,"entry-title")]`,
		`//article//h1`,
		`//h1`,
	} {
		nodes := htmlquery.Find(doc, xpath)
		if len(nodes) > 0 {
			t := strings.TrimSpace(htmlquery.InnerText(nodes[0]))
			if t != "" {
				title = t
				break
			}
		}
	}

	if title == "" {
		return nil
	}
	// Verify number is in title
	if !strings.Contains(strings.ToUpper(title), strings.ToUpper(number)) {
		return nil
	}

	// Clean title: strip leading "[NUMBER] " prefix
	cleanTitle := title
	prefix := "[" + strings.ToUpper(number) + "]"
	if strings.HasPrefix(strings.ToUpper(title), strings.ToUpper(prefix)) {
		cleanTitle = strings.TrimSpace(title[len(prefix):])
	}
	if cleanTitle == "" {
		cleanTitle = title
	}

	// Cover: try og:image meta, then article img
	coverURL := ""
	for _, xpath := range []string{
		`//meta[@property="og:image"]/@content`,
		`//article//img/@src`,
	} {
		nodes := htmlquery.Find(doc, xpath)
		if len(nodes) > 0 {
			v := strings.TrimSpace(htmlquery.InnerText(nodes[0]))
			if v != "" {
				coverURL = v
				break
			}
		}
	}

	return map[string]any{
		"title":        cleanTitle,
		"number":       strings.ToUpper(number),
		"cover_url":    coverURL,
		"actors":       []string{},
		"genres":       []string{},
		"release_date": "",
		"runtime":      "",
		"studio":       "",
		"director":     "",
	}
}

// ── Metadata builder ──────────────────────────────────────────────────────────

func buildMetadata(meta map[string]any) *scraper.Metadata {
	title, _ := meta["title"].(string)
	number, _ := meta["number"].(string)
	coverURL, _ := meta["cover_url"].(string)

	return &scraper.Metadata{
		Title:         title,
		OriginalTitle: title,
		PosterURL:     coverURL,
		UniqueIDs:     map[string]string{"javfree": number},
	}
}
