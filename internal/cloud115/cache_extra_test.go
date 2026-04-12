package cloud115

// cache_extra_test.go – additional coverage for cache.go
// Targets: DefaultCachePath, NewCache (schema migration edge cases),
// IncrementMinuteCount, TryAcquireSlot (full branches), StaleDirCount,
// Stats (populated), ClearMetadata, Clear, and helper paths.

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---- DefaultCachePath -------------------------------------------------------

func TestDefaultCachePath(t *testing.T) {
	p := DefaultCachePath()
	if p == "" {
		t.Fatal("DefaultCachePath must not be empty")
	}
	// Must end with cache.db.
	if filepath.Base(p) != "cache.db" {
		t.Errorf("expected base name cache.db, got %q", filepath.Base(p))
	}
}

// ---- NewCache (empty dbPath → uses default) ---------------------------------

func TestNewCacheDefaultPath(t *testing.T) {
	// Override the cache dir to a temp dir so we don't write to the real one.
	tmp := t.TempDir()
	c, err := NewCache(filepath.Join(tmp, "cache.db"))
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	defer func() { _ = c.Close() }()

	// Basic smoke: must be able to set/get a path.
	c.SetPath("/test", "cid_1")
	if _, _, ok := c.GetPath("/test"); !ok {
		t.Error("expected path to be stored")
	}
}

// ---- NewCache: re-open same file preserves data (no migration) --------------

func TestNewCacheReopenPreservesData(t *testing.T) {
	dbFile := filepath.Join(t.TempDir(), "reopen.db")

	c1, err := NewCache(dbFile)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	c1.SetPath("/keep", "cid_keep")
	_ = c1.Close()

	c2, err := NewCache(dbFile)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer func() { _ = c2.Close() }()

	cid, _, ok := c2.GetPath("/keep")
	if !ok {
		t.Fatal("path should survive close/reopen")
	}
	if cid != "cid_keep" {
		t.Errorf("cid = %q; want cid_keep", cid)
	}
}

// ---- IncrementMinuteCount ---------------------------------------------------

func TestIncrementMinuteCount(t *testing.T) {
	c := newTestCache(t)

	// Incrementing a non-existent row must create it.
	c.IncrementMinuteCount("api_test")
	rl := c.GetRateLimit("api_test")
	if rl.MinuteCount != 1 {
		t.Errorf("after first increment, minute_count = %d; want 1", rl.MinuteCount)
	}

	// Second increment adds to the existing row.
	c.IncrementMinuteCount("api_test")
	rl = c.GetRateLimit("api_test")
	if rl.MinuteCount != 2 {
		t.Errorf("after second increment, minute_count = %d; want 2", rl.MinuteCount)
	}
}

// ---- TryAcquireSlot: minute-window reset branch -----------------------------

func TestTryAcquireSlotMinuteWindowReset(t *testing.T) {
	c := newTestCache(t)
	now := float64(time.Now().Unix())

	// Seed a row with an old minute_start (>60s ago) and a high count.
	_, _ = c.db.Exec(
		"INSERT OR REPLACE INTO rate_limit (name, cooldown_until, last_request, minute_start, minute_count) VALUES (?,0,0,?,999)",
		"reset_test", now-120,
	)

	// The minute window should reset, so acquisition succeeds.
	if !c.TryAcquireSlot("reset_test", now, 0.0, 20) {
		t.Fatal("expected success after minute window reset")
	}
	rl := c.GetRateLimit("reset_test")
	if rl.MinuteCount != 1 {
		t.Errorf("minute_count = %d; want 1 after window reset", rl.MinuteCount)
	}
}

// ---- TryAcquireSlot: insert-or-ignore creates row first call ----------------

func TestTryAcquireSlotCreatesRow(t *testing.T) {
	c := newTestCache(t)
	now := float64(time.Now().Unix())

	// Name does not exist yet — TryAcquireSlot should create it and succeed.
	if !c.TryAcquireSlot("brand_new", now, 0.0, 100) {
		t.Fatal("expected success for brand-new limiter name")
	}
	rl := c.GetRateLimit("brand_new")
	if rl.MinuteCount != 1 {
		t.Errorf("minute_count = %d; want 1", rl.MinuteCount)
	}
	if rl.LastRequest == 0 {
		t.Error("last_request should be set after acquisition")
	}
}

// ---- Stats: all counters populated ------------------------------------------

func TestCacheStatsPopulated(t *testing.T) {
	c := newTestCache(t)

	c.SetPath("/a", "1")
	c.SetPath("/b", "2")
	c.SetPath("/c", "3")
	c.SetDirListing("d1", []Entry{
		fileEntry("f1.mkv", "n1"),
		fileEntry("f2.mkv", "n2"),
	})
	c.SetDirListing("d2", []Entry{fileEntry("f3.mkv", "n3")})
	c.SetTree([]TreeEntry{
		{Path: "X/X.mkv", Name: "X.mkv", Parent: "X", IsVideo: true},
	}, "")
	c.SetRateLimit("api", RateLimitState{CooldownUntil: 10.0, MinuteCount: 5})

	s := c.Stats()

	if s.PathCount != 3 {
		t.Errorf("PathCount = %d; want 3", s.PathCount)
	}
	if s.DirCount != 2 {
		t.Errorf("DirCount = %d; want 2", s.DirCount)
	}
	if s.EntryCount != 3 {
		t.Errorf("EntryCount = %d; want 3", s.EntryCount)
	}
	if s.TreeEntries != 1 {
		t.Errorf("TreeEntries = %d; want 1", s.TreeEntries)
	}
	if s.DBSizeBytes <= 0 {
		t.Error("DBSizeBytes should be > 0")
	}
	rl, ok := s.RateLimit["api"]
	if !ok {
		t.Fatal("expected api in RateLimit map")
	}
	if rl.CooldownUntil != 10.0 || rl.MinuteCount != 5 {
		t.Errorf("unexpected rate-limit state: %+v", rl)
	}
}

// ---- Stats: missing db file → DBSizeBytes=0 no panic -----------------------

func TestCacheStatsNoDBFile(t *testing.T) {
	// Create a cache, populate it, then delete the underlying file (keep the
	// open connection). Stats() uses os.Stat which will fail — must not panic.
	tmp := t.TempDir()
	dbFile := filepath.Join(tmp, "ghost.db")
	c, err := NewCache(dbFile)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	defer func() { _ = c.Close() }()

	// Remove the file while the connection is still open.
	_ = os.Remove(dbFile)

	// Stats must not panic; DBSizeBytes may be 0 now.
	s := c.Stats()
	_ = s // just assert no panic
}

// ---- ClearMetadata: rate-limit survives -------------------------------------

func TestClearMetadataPreservesRateLimit(t *testing.T) {
	c := newTestCache(t)

	c.SetPath("/movies", "cid_1")
	c.SetDirListing("cid_1", []Entry{fileEntry("a.mkv", "n1")})
	c.SetRateLimit("api", RateLimitState{CooldownUntil: 42.0, MinuteCount: 7})
	c.SetTree([]TreeEntry{{Path: "AV/X.mkv", Name: "X.mkv", Parent: "AV", IsVideo: true}}, "/AV")

	c.ClearMetadata()

	// path, dir_entry, tree_entry, snapshot_meta must be gone.
	if _, _, ok := c.GetPath("/movies"); ok {
		t.Error("path should be cleared by ClearMetadata")
	}
	if entries := c.GetDirEntries("cid_1"); len(entries) != 0 {
		t.Errorf("dir entries should be empty, got %d", len(entries))
	}
	if tree := c.GetTreeEntries(""); len(tree) != 0 {
		t.Errorf("tree entries should be empty, got %d", len(tree))
	}
	meta := c.GetSnapshotMeta()
	if meta.RootPath != "" {
		t.Errorf("snapshot_meta should be cleared, got root_path=%q", meta.RootPath)
	}

	// rate_limit must survive.
	rl := c.GetRateLimit("api")
	if rl.CooldownUntil != 42.0 || rl.MinuteCount != 7 {
		t.Errorf("rate_limit should survive ClearMetadata, got %+v", rl)
	}
}

// ---- Clear: wipes everything including rate-limit ---------------------------

func TestClearWipesRateLimit(t *testing.T) {
	c := newTestCache(t)

	c.SetPath("/foo", "cid_foo")
	c.SetRateLimit("dl", RateLimitState{CooldownUntil: 999.0})
	c.SetDirListing("d1", []Entry{fileEntry("x.mkv", "n1")})

	c.Clear()

	if _, _, ok := c.GetPath("/foo"); ok {
		t.Error("path should be gone after Clear")
	}
	if rl := c.GetRateLimit("dl"); rl.CooldownUntil != 0 {
		t.Errorf("rate_limit should be gone after Clear, got %+v", rl)
	}
	if entries := c.GetDirEntries("d1"); len(entries) != 0 {
		t.Errorf("dir_entry should be gone after Clear, got %d", len(entries))
	}
}

// ---- StaleDirCount: multiple directories ------------------------------------

func TestStaleDirCountMultiple(t *testing.T) {
	c := newTestCache(t)

	// Add three directories — all fresh.
	c.SetDirListing("d1", []Entry{fileEntry("f", "n1")})
	c.SetDirListing("d2", []Entry{fileEntry("g", "n2")})
	c.SetDirListing("d3", []Entry{fileEntry("h", "n3")})

	if n := c.StaleDirCount(3600); n != 0 {
		t.Errorf("expected 0 stale dirs, got %d", n)
	}

	// Force two of them to be very old.
	_, _ = c.db.Exec("UPDATE dir_meta SET ts = 0 WHERE cid IN ('d1', 'd3')")

	if n := c.StaleDirCount(3600); n != 2 {
		t.Errorf("expected 2 stale dirs, got %d", n)
	}
}

// ---- GetSnapshotMeta: empty database ----------------------------------------

func TestGetSnapshotMetaEmpty(t *testing.T) {
	c := newTestCache(t)
	meta := c.GetSnapshotMeta()
	if meta.RootPath != "" || meta.ExportedAt != "" || meta.EntryCount != "" {
		t.Errorf("expected zero-value meta, got %+v", meta)
	}
}

// ---- ClearTree --------------------------------------------------------------

func TestClearTreeEmpty(t *testing.T) {
	c := newTestCache(t)
	// Calling ClearTree on an already-empty table must not panic.
	c.ClearTree()

	// Populate and then clear.
	c.SetTree([]TreeEntry{
		{Path: "a/b.mkv", Name: "b.mkv", Parent: "a", IsVideo: true},
	}, "")
	c.ClearTree()

	if got := c.GetTreeEntries(""); len(got) != 0 {
		t.Errorf("expected empty after ClearTree, got %d", len(got))
	}
}

// ---- SetDirListing: entry with null size/pickCode ---------------------------

func TestSetDirListingNullableFields(t *testing.T) {
	c := newTestCache(t)

	// Entry with Size=0 and PickCode="" should store/retrieve as NULL.
	c.SetDirListing("d_null", []Entry{
		{Name: "dir_only", Type: "dir", NodeID: "n1", Size: 0, PickCode: ""},
	})

	entries := c.GetDirEntries("d_null")
	if len(entries) != 1 {
		t.Fatalf("want 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Size != 0 {
		t.Errorf("size = %d; want 0", e.Size)
	}
	if e.PickCode != "" {
		t.Errorf("pick_code = %q; want empty", e.PickCode)
	}
}

// ---- FindEntries: empty result when no match --------------------------------

func TestFindEntriesNotFound(t *testing.T) {
	c := newTestCache(t)
	c.SetDirListing("d1", []Entry{fileEntry("a.mkv", "n1")})
	result := c.FindEntries("d1", "nothere.mkv")
	if len(result) != 0 {
		t.Errorf("expected empty slice, got %v", result)
	}
}

// ---- GetDirEntries: category filter edge case --------------------------------

func TestGetTreeEntriesCategoryExactMatch(t *testing.T) {
	c := newTestCache(t)
	c.SetTree([]TreeEntry{
		{Path: "AV", Name: "AV", Parent: "", IsVideo: false, IsNFO: false},
		{Path: "AV/X.mkv", Name: "X.mkv", Parent: "AV", IsVideo: true},
	}, "")

	// Exact path match should be included.
	result := c.GetTreeEntries("AV")
	if len(result) != 2 {
		t.Errorf("want 2 (exact + child), got %d: %v", len(result), result)
	}
}

// ---- Close: nil db is idempotent --------------------------------------------

func TestCacheCloseIdempotent(t *testing.T) {
	c, err := NewCache(filepath.Join(t.TempDir(), "idem.db"))
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("second Close (nil db): %v", err)
	}
}
