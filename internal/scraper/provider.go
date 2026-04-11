package scraper

// SearchOpts holds optional search parameters.
type SearchOpts struct {
	Year    int
	Season  int
	Episode int
}

// ScrapeOpts holds optional scrape parameters.
type ScrapeOpts struct {
	Year    int
	Season  int
	Episode int
}

// SearchResult is a candidate match returned by Provider.Search.
type SearchResult struct {
	ID    string
	Title string
	Year  int
}

// Actor holds actor metadata.
type Actor struct {
	Name  string
	Role  string
	Thumb string
}

// Metadata holds all scraped metadata for a media item.
type Metadata struct {
	Title         string
	OriginalTitle string
	Year          int
	Plot          string
	Tagline       string
	Runtime       int
	Rating        float64
	Votes         int
	Premiered     string
	Genres        []string
	Directors     []string
	Credits       []string // writers
	Actors        []Actor
	Studios       []string
	Countries     []string
	Tags          []string
	UniqueIDs     map[string]string
	PosterURL     string
	FanartURL     string
	Set           string // collection/franchise name

	// TV / episode specific
	ShowTitle string
	Season    int
	Episode   int
	Aired     string
}

// ScrapeResult is returned by Provider.Scrape.
type ScrapeResult struct {
	Status string // "ok", "not_found", "error"
	Match  string
	Source string
	Error  string
	IDs    map[string]string // e.g. {"tmdb": "12345"}
	Meta   *Metadata         // full metadata when available (used for file_map caching)
}

// Provider is the interface every scraper backend must implement.
type Provider interface {
	// Name returns the provider's unique name (e.g. "tmdb", "javbus").
	Name() string

	// SupportedTypes returns the media types this provider handles
	// (e.g. ["movie", "tv"], ["av"], ["gravure"]).
	SupportedTypes() []string

	// Priority controls ordering when multiple providers handle the same type.
	// Lower numbers run first.
	Priority() int

	// Search returns a ranked list of candidates for the given query.
	Search(query string, opts SearchOpts) ([]SearchResult, error)

	// Detail returns full metadata for the given provider-specific ID.
	Detail(id string) (*Metadata, error)

	// Scrape is the all-in-one entry point: search → match → write NFO +
	// artwork.  Returns a non-nil *ScrapeResult on both success and failure.
	Scrape(query, filename, outDir string, opts ScrapeOpts) (*ScrapeResult, error)
}
