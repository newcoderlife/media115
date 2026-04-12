package cloud115

// Entry represents a single file or directory returned by the cloud API.
type Entry struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // "dir" or "file"
	NodeID   string `json:"node_id"`
	Size     int64  `json:"size,omitempty"`
	PickCode string `json:"pick_code,omitempty"`
	CID      string `json:"cid,omitempty"` // convenience: same as NodeID for dirs
	FID      string `json:"fid,omitempty"` // convenience: same as NodeID for files
}

// TreeEntry represents a single node in the cloud file tree snapshot.
type TreeEntry struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	Parent  string `json:"parent"`
	IsVideo bool   `json:"is_video"`
	IsNFO   bool   `json:"is_nfo"`
}

// CacheStats summarises the current state of the cache database.
type CacheStats struct {
	PathCount   int                       `json:"path_count"`
	DirCount    int                       `json:"dir_count"`
	EntryCount  int                       `json:"entry_count"`
	TreeEntries int                       `json:"tree_entries"`
	DBSizeBytes int64                     `json:"db_size_bytes"`
	RateLimit   map[string]RateLimitState `json:"rate_limit"`
}

// RateLimitState holds per-endpoint rate-limit counters.
type RateLimitState struct {
	CooldownUntil float64 `json:"cooldown_until"`
	LastRequest   float64 `json:"last_request"`
	MinuteStart   float64 `json:"minute_start"`
	MinuteCount   int     `json:"minute_count"`
}

// SnapshotMeta holds metadata recorded alongside a tree snapshot.
type SnapshotMeta struct {
	RootPath   string `json:"root_path"`
	ExportedAt string `json:"exported_at"`
	EntryCount string `json:"entry_count"`
}
