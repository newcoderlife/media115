package scraper

import (
	"log/slog"
	"sort"
	"sync"
)

var (
	mu        sync.RWMutex
	providers = map[string][]Provider{} // mediaType → sorted by priority
)

// Register adds p to the registry for every media type it declares.
// Providers with a lower Priority() value run first.
func Register(p Provider) {
	mu.Lock()
	defer mu.Unlock()
	for _, t := range p.SupportedTypes() {
		providers[t] = append(providers[t], p)
		sort.Slice(providers[t], func(i, j int) bool {
			return providers[t][i].Priority() < providers[t][j].Priority()
		})
	}
}

// Reset clears all registered providers. Intended for use in tests.
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	providers = make(map[string][]Provider)
}

// GetProviders returns the registered providers for the given media type,
// ordered by priority (lowest first).  Returns nil when none are registered.
func GetProviders(mediaType string) []Provider {
	mu.RLock()
	defer mu.RUnlock()
	return providers[mediaType]
}

// Scrape tries providers for mediaType in priority order and returns the first
// successful result.  If no provider succeeds it returns Status "not_found".
func Scrape(mediaType, query, filename, outDir string, opts ScrapeOpts, logger *slog.Logger) *ScrapeResult {
	if logger == nil {
		logger = slog.Default()
	}

	prov := GetProviders(mediaType)
	if len(prov) == 0 {
		return &ScrapeResult{Status: "error", Error: "no providers for type: " + mediaType}
	}

	for _, p := range prov {
		logger.Debug("SCRAPE trying", "provider", p.Name(), "type", mediaType, "query", query)
		result, err := p.Scrape(query, filename, outDir, opts)
		if err != nil {
			logger.Warn("SCRAPE failed", "provider", p.Name(), "error", err)
			continue
		}
		if result != nil && result.Status == "ok" {
			result.Source = p.Name()
			return result
		}
		if result != nil {
			logger.Debug("SCRAPE not matched", "provider", p.Name(), "status", result.Status)
		}
	}

	return &ScrapeResult{Status: "not_found"}
}
