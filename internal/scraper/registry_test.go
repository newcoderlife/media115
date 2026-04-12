package scraper

import (
	"errors"
	"testing"
)

// mockProvider is a minimal Provider for testing.
type mockProvider struct {
	name     string
	types    []string
	priority int
	status   string
}

func (m *mockProvider) Name() string             { return m.name }
func (m *mockProvider) SupportedTypes() []string { return m.types }
func (m *mockProvider) Priority() int            { return m.priority }
func (m *mockProvider) Search(string, SearchOpts) ([]SearchResult, error) {
	return nil, nil
}
func (m *mockProvider) Detail(string) (*Metadata, error) { return nil, nil }
func (m *mockProvider) Scrape(query, filename, outDir string, opts ScrapeOpts) (*ScrapeResult, error) {
	if m.status == "error" {
		return nil, errors.New("mock error")
	}
	return &ScrapeResult{Status: m.status, Match: query}, nil
}

// resetProviders clears the global registry for test isolation.
func resetProviders() {
	mu.Lock()
	providers = map[string][]Provider{}
	mu.Unlock()
}

func TestRegisterAndGet(t *testing.T) {
	resetProviders()

	p1 := &mockProvider{name: "p1", types: []string{"movie"}, priority: 10, status: "ok"}
	p2 := &mockProvider{name: "p2", types: []string{"movie", "tv"}, priority: 5, status: "ok"}

	Register(p1)
	Register(p2)

	movie := GetProviders("movie")
	if len(movie) != 2 {
		t.Fatalf("expected 2 movie providers, got %d", len(movie))
	}
	// p2 has lower priority so it should be first.
	if movie[0].Name() != "p2" {
		t.Errorf("expected p2 first (priority 5), got %q", movie[0].Name())
	}

	tv := GetProviders("tv")
	if len(tv) != 1 || tv[0].Name() != "p2" {
		t.Errorf("expected [p2] for tv, got %v", tv)
	}

	none := GetProviders("av")
	if len(none) != 0 {
		t.Errorf("expected 0 providers for av, got %d", len(none))
	}
}

func TestScrapeNoProviders(t *testing.T) {
	resetProviders()

	result := Scrape("nonexistent", "query", "file.mp4", "/tmp", ScrapeOpts{}, nil)
	if result.Status != "error" {
		t.Errorf("expected error status, got %q", result.Status)
	}
	if result.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestScrapeFirstOKWins(t *testing.T) {
	resetProviders()

	p1 := &mockProvider{name: "slow", types: []string{"movie"}, priority: 10, status: "not_found"}
	p2 := &mockProvider{name: "fast", types: []string{"movie"}, priority: 5, status: "ok"}

	Register(p1)
	Register(p2)

	result := Scrape("movie", "Inception", "Inception.mkv", "/tmp", ScrapeOpts{}, nil)
	if result.Status != "ok" {
		t.Errorf("expected ok, got %q", result.Status)
	}
	if result.Source != "fast" {
		t.Errorf("expected source=fast, got %q", result.Source)
	}
}

func TestScrapeProviderError(t *testing.T) {
	resetProviders()

	pe := &mockProvider{name: "errprov", types: []string{"movie"}, priority: 1, status: "error"}
	Register(pe)

	result := Scrape("movie", "query", "file.mp4", "/tmp", ScrapeOpts{}, nil)
	// All providers failed/errored → not_found
	if result.Status != "not_found" {
		t.Errorf("expected not_found, got %q", result.Status)
	}
}
