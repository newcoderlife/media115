package organizer

import (
	"database/sql"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/newcoderlife/media115/internal/cloud115"
)

// seedCacheDB seeds the SQLite cache DB at dbPath with:
//   - A path_index row so ResolvePath(path) → cid without API call.
//   - dir_meta + dir_entry rows so ListDir(path) returns entries from cache.
func seedCacheDB(t *testing.T, dbPath string, path, cid string, entries []cloud115.Entry) {
	t.Helper()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("seedCacheDB open: %v", err)
	}
	defer db.Close()

	now := time.Now().Unix()

	// path_index
	if _, err := db.Exec(
		"INSERT OR REPLACE INTO path_index (path, cid, ts) VALUES (?, ?, ?)",
		path, cid, now,
	); err != nil {
		t.Fatalf("seedCacheDB path_index: %v", err)
	}

	// dir_meta (marks listing as fresh)
	if _, err := db.Exec(
		"INSERT OR REPLACE INTO dir_meta (cid, ts) VALUES (?, ?)",
		cid, now,
	); err != nil {
		t.Fatalf("seedCacheDB dir_meta: %v", err)
	}

	// dir_entry rows
	for i, e := range entries {
		nodeID := e.NodeID
		if nodeID == "" {
			nodeID = cid + "_entry_" + itoa(i)
		}
		if _, err := db.Exec(
			"INSERT OR REPLACE INTO dir_entry (parent_cid, name, type, node_id, size, pick_code) VALUES (?, ?, ?, ?, NULL, NULL)",
			cid, e.Name, e.Type, nodeID,
		); err != nil {
			t.Fatalf("seedCacheDB dir_entry: %v", err)
		}
	}
}

// TestExecuteAllSkipOps verifies that Execute returns immediately when all ops
// are marked skip, without touching the cloud115 client.
// (nil client is safe because the code returns before any client calls.)
func TestExecuteAllSkipOps(t *testing.T) {
	ops := []Op{
		{File: "already.mkv", Path: "AV/already/already.mkv", Parent: "AV/already", Action: "skip", Reason: "already correct"},
		{File: "standard (2020).mkv", Path: "电影/standard (2020)/standard (2020).mkv", Parent: "电影/standard (2020)", Action: "skip", Reason: "already in standard format"},
	}

	results, err := Execute(nil, ops, "AV", slog.Default())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Status != "skipped" {
			t.Errorf("expected status=skipped, got %q for file %q", r.Status, r.File)
		}
	}
}

// TestExecuteEmptyOps verifies that Execute with zero ops returns an empty
// result without panicking.
func TestExecuteEmptyOps(t *testing.T) {
	results, err := Execute(nil, nil, "AV", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}

// TestExecuteNilLoggerDefaultsToSlogDefault verifies that a nil logger does
// not panic (it defaults to slog.Default() internally).
func TestExecuteNilLoggerDefaultsToSlogDefault(t *testing.T) {
	ops := []Op{
		{File: "x.mkv", Path: "AV/x/x.mkv", Parent: "AV/x", Action: "skip", Reason: "test"},
	}
	results, err := Execute(nil, ops, "AV", nil) // nil logger
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

// newTestClient creates a real *cloud115.Client backed by a temp SQLite DB.
// API calls will fail (no valid cookies / network), but the client does not panic.
// Returns the client and the cache directory path (for cache seeding).
func newTestClient(t *testing.T) *cloud115.Client {
	t.Helper()
	tmpDir := t.TempDir()
	return newTestClientWithDir(t, tmpDir)
}

func newTestClientWithDir(t *testing.T, tmpDir string) *cloud115.Client {
	t.Helper()
	cacheDir := filepath.Join(tmpDir, "cache")
	client, err := cloud115.NewClient("", cloud115.WithCacheDir(cacheDir))
	if err != nil {
		t.Fatalf("newTestClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// newSeededTestClient creates a *cloud115.Client with a pre-seeded SQLite cache
// so that ResolvePath and ListDir work entirely from cache without API calls.
//
// paths maps path → (cid, entries).
func newSeededTestClient(t *testing.T, paths map[string]struct {
	CID     string
	Entries []cloud115.Entry
}) *cloud115.Client {
	t.Helper()
	tmpDir := t.TempDir()
	cacheDir := filepath.Join(tmpDir, "cache")
	dbPath := filepath.Join(cacheDir, "cache.db")

	// Create the client first (this creates the DB schema).
	client := newTestClientWithDir(t, tmpDir)

	// Now seed the cache DB.
	for path, info := range paths {
		seedCacheDB(t, dbPath, path, info.CID, info.Entries)
	}
	return client
}

// TestExecuteCategoryNotFound verifies that when the category path cannot be
// resolved (no live API), Execute marks all active ops as errors and returns
// gracefully — exercising the ResolvePath failure branch.
func TestExecuteCategoryNotFound(t *testing.T) {
	client := newTestClient(t)

	ops := []Op{
		// One skip op and one rename op.
		{File: "skip.mkv", Path: "AV/done/skip.mkv", Parent: "AV/done", Action: "skip", Reason: "already correct"},
		{File: "rename.mkv", Path: "AV/old/rename.mkv", Parent: "AV/old", Action: "rename", NewFolder: "NEW-001", NewName: "NEW-001.mkv"},
	}

	results, err := Execute(client, ops, "AV", slog.Default())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Skip op becomes "skipped"; active op becomes "error" (category dir not found).
	statusCounts := map[string]int{}
	for _, r := range results {
		statusCounts[r.Status]++
	}
	if statusCounts["skipped"] != 1 {
		t.Errorf("expected 1 skipped, got %d", statusCounts["skipped"])
	}
	if statusCounts["error"] != 1 {
		t.Errorf("expected 1 error (category not found), got %d", statusCounts["error"])
	}
}

// TestExecuteMixedSkipAndActive verifies that Execute processes a mix of skip
// and rename ops, marking rename ops as errors when the category is unreachable.
func TestExecuteMixedSkipAndActive(t *testing.T) {
	client := newTestClient(t)

	ops := []Op{
		{File: "a.mkv", Action: "skip", Reason: "already correct", Parent: "AV/a"},
		{File: "b.mkv", Action: "skip", Reason: "no scrape", Parent: "AV/b"},
		{File: "c.mkv", Action: "rename", NewFolder: "C-001", NewName: "C-001.mkv", Parent: "AV/old-c"},
		{File: "d.mkv", Action: "rename", NewFolder: "D-001", NewName: "D-001.mkv", Parent: "AV/old-d"},
	}

	results, err := Execute(client, ops, "AV", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	skipped := 0
	errored := 0
	for _, r := range results {
		switch r.Status {
		case "skipped":
			skipped++
		case "error":
			errored++
		}
	}
	if skipped != 2 {
		t.Errorf("expected 2 skipped, got %d", skipped)
	}
	if errored != 2 {
		t.Errorf("expected 2 errors (category not found), got %d", errored)
	}
}

// TestExecuteWithSeededCache exercises Execute phases 1-6 by pre-seeding the
// SQLite cache so ResolvePath and ListDir succeed without real API calls.
//
// Phase 3 (Move) and Phase 4 (BatchRename) will fail (no live API) and ops
// will be marked as "error". Phase 6 (Cleanup) runs with a failed catItems
// listing. All these code paths get exercised.
func TestExecuteWithSeededCache(t *testing.T) {
	const catCID = "cat001"
	const parentCID = "parent001"
	// Execute prepends "/" to categoryPath, so catPath = "/AV"
	const catPath = "/AV"
	const parentPath = "/影音/AV/ABP-040"

	type seedEntry = struct {
		CID     string
		Entries []cloud115.Entry
	}

	client := newSeededTestClient(t, map[string]seedEntry{
		// Category path: Execute uses "/" + categoryPath = "/AV"
		catPath: {CID: catCID, Entries: []cloud115.Entry{
			{Name: "ABP-040", Type: "dir", NodeID: parentCID},
		}},
		// Parent dir containing the video file: Execute uses "/" + op.Parent
		parentPath: {CID: parentCID, Entries: []cloud115.Entry{
			{Name: "ABP-040.FHD.mp4", Type: "file", NodeID: "file001"},
		}},
	})

	ops := []Op{
		{
			File:      "ABP-040.FHD.mp4",
			Path:      "影音/AV/ABP-040/ABP-040.FHD.mp4",
			Parent:    "影音/AV/ABP-040",
			Action:    "rename",
			NewFolder: "ABP-040",
			NewName:   "ABP-040.mp4",
			Type:      "av",
		},
	}

	results, err := Execute(client, ops, "AV", slog.Default())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// With seeded cache, Phase 1 resolves successfully. Phase 3 (Move) fails
	// because the real API has no valid session, so op ends as "error".
	// (The exact status depends on whether Mkdir succeeds.)
	t.Logf("result status=%q error=%q", results[0].Status, results[0].Error)
}

// TestExecuteWithSeededCacheFileNotFound exercises the Phase 1 "file not in dir"
// branch by seeding a parent listing that does NOT contain the op file.
func TestExecuteWithSeededCacheFileNotFound(t *testing.T) {
	const catCID = "cat002"
	const parentCID = "parent002"
	// Execute prepends "/" to categoryPath
	const catPath = "/AV"
	const parentPath = "/影音/AV/TST-999"

	type seedEntry = struct {
		CID     string
		Entries []cloud115.Entry
	}

	client := newSeededTestClient(t, map[string]seedEntry{
		catPath: {CID: catCID, Entries: []cloud115.Entry{}},
		// Parent dir exists but does NOT contain the expected file.
		parentPath: {CID: parentCID, Entries: []cloud115.Entry{
			{Name: "unrelated.txt", Type: "file", NodeID: "f002"},
		}},
	})

	ops := []Op{
		{
			File:      "TST-999.mp4",
			Path:      "影音/AV/TST-999/TST-999.mp4",
			Parent:    "影音/AV/TST-999",
			Action:    "rename",
			NewFolder: "TST-999",
			NewName:   "TST-999.mp4",
		},
	}

	results, err := Execute(client, ops, "AV", slog.Default())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Status != "not_found" {
		t.Errorf("expected not_found status, got %q", results[0].Status)
	}
}

// TestExecuteWithSeededCacheRenameOnly exercises the rename-only path (no NewFolder)
// in Execute: the file is in the right folder but needs a name change.
func TestExecuteWithSeededCacheRenameOnly(t *testing.T) {
	const catCID = "cat003"
	const parentCID = "parent003"
	// Execute uses "/" + categoryPath
	const catPath = "/电影"
	const parentPath = "/影音/电影/Inception (2010)"

	type seedEntry = struct {
		CID     string
		Entries []cloud115.Entry
	}

	client := newSeededTestClient(t, map[string]seedEntry{
		catPath: {CID: catCID, Entries: []cloud115.Entry{
			{Name: "Inception (2010)", Type: "dir", NodeID: parentCID},
		}},
		parentPath: {CID: parentCID, Entries: []cloud115.Entry{
			{Name: "Inception.2010.mkv", Type: "file", NodeID: "f003"},
		}},
	})

	ops := []Op{
		{
			File:      "Inception.2010.mkv",
			Path:      "影音/电影/Inception (2010)/Inception.2010.mkv",
			Parent:    "影音/电影/Inception (2010)",
			Action:    "rename",
			NewFolder: "", // no folder change
			NewName:   "Inception (2010).mkv",
			Type:      "movie",
		},
	}

	results, err := Execute(client, ops, "电影", slog.Default())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	// Phase 1 succeeds (file found in cache). Phase 4 (BatchRename) fails
	// (no live API). Result is either "ok" or "error".
	t.Logf("rename-only result: status=%q error=%q", results[0].Status, results[0].Error)
}
