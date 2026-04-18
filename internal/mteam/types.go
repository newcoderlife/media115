// Package mteam implements a subscription downloader for the M-Team
// private tracker. It searches torrents via the public OpenAPI, applies
// a rule filter, and writes matched .torrent files into a local watch
// folder. It does not interact with 115 at all.
package mteam

import (
	"encoding/json"
)

// Discount values returned by the M-Team API.
const (
	DiscountFree        = "FREE"
	DiscountPercent50   = "PERCENT_50"
	DiscountPercent70   = "PERCENT_70"
	Discount2X          = "_2X"
	Discount2XFree      = "_2X_FREE"
	Discount2XPercent50 = "_2X_PERCENT_50"
	DiscountNormal      = "NORMAL"
)

// SearchMode maps to the M-Team `mode` request field.
const (
	ModeNormal    = "normal"
	ModeAdult     = "adult"
	ModeMovie     = "movie"
	ModeMusic     = "music"
	ModeTVShow    = "tvshow"
	ModeWaterfall = "waterfall"
	ModeRSS       = "rss"
	ModeRankings  = "rankings"
	ModeAll       = "all"
)

// Status holds the nested `status` block of a torrent.
type Status struct {
	Seeders         string `json:"seeders"`
	Leechers        string `json:"leechers"`
	TimesCompleted  string `json:"timesCompleted"`
	Discount        string `json:"discount"`
	DiscountEndTime string `json:"discountEndTime"`
}

// Torrent is a single item from /api/torrent/search.
type Torrent struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	SmallDescr   string   `json:"smallDescr"`
	Size         string   `json:"size"` // bytes
	NumFiles     string   `json:"numfiles"`
	CreatedDate  string   `json:"createdDate"` // "YYYY-MM-DD HH:MM:SS"
	LabelsNew    []string `json:"labelsNew"`
	Category     string   `json:"category"`
	IMDB         string   `json:"imdb"`
	IMDBRating   string   `json:"imdbRating"`
	Douban       string   `json:"douban"`
	DoubanRating string   `json:"doubanRating"`
	Status       Status   `json:"status"`
}

// SearchRequest is the POST body for /api/torrent/search.
type SearchRequest struct {
	Keyword       string   `json:"keyword,omitempty"`
	Mode          string   `json:"mode"`
	Categories    []int64  `json:"categories,omitempty"`
	Sources       []int64  `json:"sources,omitempty"`
	Mediums       []int64  `json:"mediums,omitempty"`
	Standards     []int64  `json:"standards,omitempty"`
	VideoCodecs   []int64  `json:"videoCodecs,omitempty"`
	AudioCodecs   []int64  `json:"audioCodecs,omitempty"`
	Teams         []int64  `json:"teams,omitempty"`
	Processings   []int64  `json:"processings,omitempty"`
	LabelsNew     []string `json:"labelsNew,omitempty"`
	Discount      string   `json:"discount,omitempty"`
	PageNumber    int      `json:"pageNumber"`
	PageSize      int      `json:"pageSize"`
	SortField     string   `json:"sortField,omitempty"`
	SortDirection string   `json:"sortDirection,omitempty"`
}

// SearchResponse is the typed outer envelope for /api/torrent/search.
type SearchResponse struct {
	Code    json.RawMessage `json:"code"`
	Message string          `json:"message"`
	Data    SearchData      `json:"data"`
}

// SearchData is the `data` object inside a search response.
type SearchData struct {
	PageNumber string    `json:"pageNumber"`
	PageSize   string    `json:"pageSize"`
	Total      string    `json:"total"`
	TotalPages string    `json:"totalPages"`
	List       []Torrent `json:"data"` // M-Team puts rows under data.data
}

// GenericResponse is used for detail / token endpoints.
type GenericResponse struct {
	Code    json.RawMessage `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// TorrentFile is a single file inside a torrent, from /torrent/files.
type TorrentFile struct {
	Name string `json:"name"`
	Size string `json:"size"` // bytes as string
}

// CatalogItem is a single item from a list endpoint (categoryList, sourceList, etc.).
type CatalogItem struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	NameCht string `json:"nameCht"`
	NameChs string `json:"nameChs"`
}

// Rule is the persisted filtering rule. All fields are optional; a zero
// value means "no constraint on this dimension".
type Rule struct {
	// Search
	Keyword string `json:"keyword,omitempty"`
	Mode    string `json:"mode,omitempty"` // defaults to "normal"

	// Promotion
	RequireFree bool `json:"require_free,omitempty"`

	// Quality presets
	RequireHQ bool `json:"require_hq,omitempty"` // 4K + HDR/DOVI
	NoJunk    bool `json:"no_junk,omitempty"`    // reject 原盘 + 肉酱盘

	// Resolution
	Require4K bool `json:"require_4k,omitempty"`

	// Files
	MaxFiles int `json:"max_files,omitempty"` // inclusive upper bound; 0 = no limit

	// Size (bytes, inclusive)
	MinSize int64 `json:"min_size,omitempty"`
	MaxSize int64 `json:"max_size,omitempty"`

	// Swarm
	MinSeeders int `json:"min_seeders,omitempty"`

	// Keyword black list
	ExcludeKeywords []string `json:"exclude_keywords,omitempty"` // case-insensitive substrings

	// Labels
	LabelsAllow []string `json:"labels_allow,omitempty"` // if non-empty, torrent must have ≥1
	LabelsDeny  []string `json:"labels_deny,omitempty"`  // if any match, reject

	// Freshness
	FreshHours int `json:"fresh_hours,omitempty"` // createdDate must be within N hours of now

	// Ratings: inclusive lower bound, OR logic.
	// max(imdb, douban) >= MinRating passes; both absent = reject.
	MinRating float64 `json:"min_rating,omitempty"`

	// TV pack completeness (only applied when mode == "tvshow"):
	// when true, torrent name must look like a complete pack (E01-EXX, 全X集,
	// Complete, 完结).
	TVCompleteEpisodes bool `json:"tv_complete_episodes,omitempty"`
}

// ToSearchRequest builds a SearchRequest from the rule for the given page.
func (r *Rule) ToSearchRequest(page, pageSize int) SearchRequest {
	mode := r.Mode
	if mode == "" {
		mode = ModeNormal
	}
	req := SearchRequest{
		Keyword:       r.Keyword,
		Mode:          mode,
		PageNumber:    page,
		PageSize:      pageSize,
		SortField:     "CREATED_DATE",
		SortDirection: "DESC",
	}
	return req
}
