package cloud115

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/newcoderlife/media115/internal/config"
	_ "modernc.org/sqlite" // register "sqlite" driver
)

const schemaVersion = 3

const schema = `
CREATE TABLE IF NOT EXISTS path_index (
    path TEXT PRIMARY KEY,
    cid  TEXT NOT NULL,
    ts   INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS dir_meta (
    cid TEXT PRIMARY KEY,
    ts  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS dir_entry (
    parent_cid TEXT NOT NULL,
    name       TEXT NOT NULL,
    type       TEXT NOT NULL,
    node_id    TEXT NOT NULL,
    size       INTEGER,
    pick_code  TEXT,
    PRIMARY KEY (parent_cid, node_id)
);

CREATE INDEX IF NOT EXISTS idx_entry_parent ON dir_entry(parent_cid);
CREATE INDEX IF NOT EXISTS idx_entry_node   ON dir_entry(node_id);
CREATE INDEX IF NOT EXISTS idx_entry_name   ON dir_entry(parent_cid, name);

CREATE TABLE IF NOT EXISTS rate_limit (
    name           TEXT PRIMARY KEY,
    cooldown_until REAL    DEFAULT 0,
    last_request   REAL    DEFAULT 0,
    minute_start   REAL    DEFAULT 0,
    minute_count   INTEGER DEFAULT 0
);

CREATE TABLE IF NOT EXISTS tree_entry (
    path     TEXT NOT NULL,
    name     TEXT NOT NULL,
    parent   TEXT NOT NULL,
    is_video INTEGER NOT NULL DEFAULT 0,
    is_nfo   INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (path)
);

CREATE INDEX IF NOT EXISTS idx_tree_parent ON tree_entry(parent);

CREATE TABLE IF NOT EXISTS snapshot_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
`

// Cache is a SQLite-backed file/directory cache for cloud115.
// Every public method commits immediately (or runs in its own transaction).
type Cache struct {
	db     *sql.DB
	dbPath string
}

// DefaultCachePath returns the default path for the SQLite cache database.
func DefaultCachePath() string {
	return filepath.Join(config.CacheDir(), "cache.db")
}

// NewCache opens (or creates) the SQLite cache at dbPath.
// Pass an empty string to use the default path.
func NewCache(dbPath string) (*Cache, error) {
	if dbPath == "" {
		dbPath = DefaultCachePath()
	}
	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("cache: mkdir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("cache: open db: %w", err)
	}

	// WAL mode for better concurrent read/write performance.
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("cache: WAL mode: %w", err)
	}

	// Schema migration: if stored version < schemaVersion, drop cache tables and
	// recreate. Cache data is expendable — it rebuilds from API calls.
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("cache: user_version: %w", err)
	}
	if version < schemaVersion {
		for _, tbl := range []string{"dir_entry", "dir_meta", "path_index", "tree_entry", "snapshot_meta", "rate_limit"} {
			if _, err := db.Exec("DROP TABLE IF EXISTS " + tbl); err != nil {
				_ = db.Close()
				return nil, fmt.Errorf("cache: drop %s: %w", tbl, err)
			}
		}
		for _, idx := range []string{"idx_entry_parent", "idx_entry_node", "idx_entry_name", "idx_tree_parent"} {
			if _, err := db.Exec("DROP INDEX IF EXISTS " + idx); err != nil {
				_ = db.Close()
				return nil, fmt.Errorf("cache: drop index %s: %w", idx, err)
			}
		}
		if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", schemaVersion)); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("cache: set user_version: %w", err)
		}
	}

	// Apply schema (CREATE TABLE IF NOT EXISTS — idempotent).
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("cache: apply schema: %w", err)
	}

	return &Cache{db: db, dbPath: dbPath}, nil
}

// ---- path_index -----------------------------------------------------------

// GetPath returns the cid and timestamp for path, or ok=false if not found.
func (c *Cache) GetPath(path string) (cid string, ts int64, ok bool) {
	err := c.db.QueryRow(
		"SELECT cid, ts FROM path_index WHERE path = ?", path,
	).Scan(&cid, &ts)
	if err == sql.ErrNoRows {
		return "", 0, false
	}
	if err != nil {
		return "", 0, false
	}
	return cid, ts, true
}

// SetPath inserts or replaces a path→cid mapping with ts = now.
func (c *Cache) SetPath(path, cid string) {
	_, _ = c.db.Exec(
		"INSERT OR REPLACE INTO path_index (path, cid, ts) VALUES (?, ?, ?)",
		path, cid, time.Now().Unix(),
	)
}

// DeletePathPrefix deletes path itself and every entry whose path starts with
// prefix + "/".
func (c *Cache) DeletePathPrefix(prefix string) {
	prefix = strings.TrimRight(prefix, "/")
	_, _ = c.db.Exec(
		"DELETE FROM path_index WHERE path = ? OR path LIKE ?",
		prefix, prefix+"/%",
	)
}

// ---- dir listing ----------------------------------------------------------

// GetDirTS returns the freshness timestamp for directory cid, or ok=false.
func (c *Cache) GetDirTS(cid string) (ts int64, ok bool) {
	err := c.db.QueryRow("SELECT ts FROM dir_meta WHERE cid = ?", cid).Scan(&ts)
	if err == sql.ErrNoRows {
		return 0, false
	}
	if err != nil {
		return 0, false
	}
	return ts, true
}

// SetDirListing atomically replaces the cached listing for directory cid.
func (c *Cache) SetDirListing(cid string, entries []Entry) {
	tx, err := c.db.Begin()
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.Exec("DELETE FROM dir_entry WHERE parent_cid = ?", cid); err != nil {
		return
	}
	for _, e := range entries {
		var size sql.NullInt64
		if e.Size != 0 {
			size = sql.NullInt64{Int64: e.Size, Valid: true}
		}
		var pickCode sql.NullString
		if e.PickCode != "" {
			pickCode = sql.NullString{String: e.PickCode, Valid: true}
		}
		if _, err = tx.Exec(
			"INSERT OR REPLACE INTO dir_entry (parent_cid, name, type, node_id, size, pick_code) VALUES (?, ?, ?, ?, ?, ?)",
			cid, e.Name, e.Type, e.NodeID, size, pickCode,
		); err != nil {
			return
		}
	}
	if _, err = tx.Exec(
		"INSERT OR REPLACE INTO dir_meta (cid, ts) VALUES (?, ?)",
		cid, time.Now().Unix(),
	); err != nil {
		return
	}
	err = tx.Commit()
}

// GetDirEntries returns cached entries for cid, dirs first then by name.
func (c *Cache) GetDirEntries(cid string) []Entry {
	rows, err := c.db.Query(
		"SELECT name, type, node_id, size, pick_code FROM dir_entry WHERE parent_cid = ? "+
			"ORDER BY CASE WHEN type='dir' THEN 0 ELSE 1 END, name",
		cid,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	return scanEntries(rows)
}

// FindEntry returns the first entry matching name inside parentCID, or nil.
func (c *Cache) FindEntry(parentCID, name string) *Entry {
	row := c.db.QueryRow(
		"SELECT name, type, node_id, size, pick_code FROM dir_entry WHERE parent_cid = ? AND name = ? LIMIT 1",
		parentCID, name,
	)
	e, err := scanEntry(row)
	if err != nil {
		return nil
	}
	return e
}

// FindEntries returns all entries matching name inside parentCID.
func (c *Cache) FindEntries(parentCID, name string) []Entry {
	rows, err := c.db.Query(
		"SELECT name, type, node_id, size, pick_code FROM dir_entry WHERE parent_cid = ? AND name = ?",
		parentCID, name,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()
	return scanEntries(rows)
}

// AddEntry upserts a single entry into the cached listing.
func (c *Cache) AddEntry(parentCID string, entry Entry) {
	var size sql.NullInt64
	if entry.Size != 0 {
		size = sql.NullInt64{Int64: entry.Size, Valid: true}
	}
	var pickCode sql.NullString
	if entry.PickCode != "" {
		pickCode = sql.NullString{String: entry.PickCode, Valid: true}
	}
	_, _ = c.db.Exec(
		"INSERT OR REPLACE INTO dir_entry (parent_cid, name, type, node_id, size, pick_code) VALUES (?, ?, ?, ?, ?, ?)",
		parentCID, entry.Name, entry.Type, entry.NodeID, size, pickCode,
	)
}

// RemoveEntry deletes all entries with the given name from parentCID.
func (c *Cache) RemoveEntry(parentCID, name string) {
	_, _ = c.db.Exec(
		"DELETE FROM dir_entry WHERE parent_cid = ? AND name = ?",
		parentCID, name,
	)
}

// RemoveEntryByID deletes a specific entry identified by nodeID within parentCID.
func (c *Cache) RemoveEntryByID(parentCID, nodeID string) {
	_, _ = c.db.Exec(
		"DELETE FROM dir_entry WHERE parent_cid = ? AND node_id = ?",
		parentCID, nodeID,
	)
}

// RenameEntry renames the entry identified by nodeID within parentCID.
func (c *Cache) RenameEntry(parentCID, nodeID, newName string) {
	_, _ = c.db.Exec(
		"UPDATE dir_entry SET name = ? WHERE parent_cid = ? AND node_id = ?",
		newName, parentCID, nodeID,
	)
}

// MoveEntry moves the entry identified by nodeID from oldParentCID to newParentCID.
func (c *Cache) MoveEntry(oldParentCID, nodeID, newParentCID string) {
	_, _ = c.db.Exec(
		"UPDATE dir_entry SET parent_cid = ? WHERE parent_cid = ? AND node_id = ?",
		newParentCID, oldParentCID, nodeID,
	)
}

// InvalidateDir removes both the dir_meta row and all dir_entry rows for cid.
func (c *Cache) InvalidateDir(cid string) {
	tx, err := c.db.Begin()
	if err != nil {
		return
	}
	_, _ = tx.Exec("DELETE FROM dir_meta  WHERE cid = ?", cid)
	_, _ = tx.Exec("DELETE FROM dir_entry WHERE parent_cid = ?", cid)
	_ = tx.Commit()
}

// DeleteDirMeta marks directory cid as stale (removes meta, keeps entries).
func (c *Cache) DeleteDirMeta(cid string) {
	_, _ = c.db.Exec("DELETE FROM dir_meta WHERE cid = ?", cid)
}

// ---- rate_limit -----------------------------------------------------------

// GetRateLimit returns the rate-limit state for name (defaults all to 0).
func (c *Cache) GetRateLimit(name string) RateLimitState {
	var state RateLimitState
	err := c.db.QueryRow(
		"SELECT cooldown_until, last_request, minute_start, minute_count FROM rate_limit WHERE name = ?",
		name,
	).Scan(&state.CooldownUntil, &state.LastRequest, &state.MinuteStart, &state.MinuteCount)
	if err != nil {
		return RateLimitState{}
	}
	return state
}

// SetRateLimit upserts rate-limit fields, always overwriting all columns.
func (c *Cache) SetRateLimit(name string, state RateLimitState) {
	_, _ = c.db.Exec(
		"INSERT OR REPLACE INTO rate_limit (name, cooldown_until, last_request, minute_start, minute_count) VALUES (?, ?, ?, ?, ?)",
		name, state.CooldownUntil, state.LastRequest, state.MinuteStart, state.MinuteCount,
	)
}

// TryAcquireSlot atomically tries to acquire a rate-limit slot.
// Returns true on success, false if any limit (cooldown / QPS / QPM) is active.
func (c *Cache) TryAcquireSlot(name string, now, minInterval float64, qpm int) bool {
	if _, err := c.db.Exec(
		"INSERT OR IGNORE INTO rate_limit (name) VALUES (?)", name,
	); err != nil {
		return false
	}
	res, err := c.db.Exec(`
		UPDATE rate_limit SET
			minute_count = CASE
				WHEN ? - minute_start > 60 THEN 1
				ELSE minute_count + 1
			END,
			minute_start = CASE
				WHEN ? - minute_start > 60 THEN ?
				ELSE minute_start
			END,
			last_request = ?
		WHERE name = ?
		  AND (cooldown_until <= ? OR cooldown_until = 0)
		  AND (? - last_request >= ? OR last_request = 0)
		  AND (CASE WHEN ? - minute_start > 60 THEN 0 ELSE minute_count END) < ?
	`, now, now, now, now, name, now, now, minInterval, now, qpm)
	if err != nil {
		return false
	}
	n, _ := res.RowsAffected()
	return n > 0
}

// IncrementMinuteCount atomically bumps minute_count for name (creates row if needed).
func (c *Cache) IncrementMinuteCount(name string) {
	_, _ = c.db.Exec(
		"INSERT INTO rate_limit (name, minute_count) VALUES (?, 1) "+
			"ON CONFLICT(name) DO UPDATE SET minute_count = minute_count + 1",
		name,
	)
}

// ---- tree_entry -----------------------------------------------------------

// SetTree replaces all tree entries atomically and records snapshot metadata.
func (c *Cache) SetTree(entries []TreeEntry, rootPath string) {
	tx, err := c.db.Begin()
	if err != nil {
		return
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.Exec("DELETE FROM tree_entry"); err != nil {
		return
	}
	for _, e := range entries {
		isVideo := 0
		if e.IsVideo {
			isVideo = 1
		}
		isNFO := 0
		if e.IsNFO {
			isNFO = 1
		}
		if _, err = tx.Exec(
			"INSERT OR REPLACE INTO tree_entry (path, name, parent, is_video, is_nfo) VALUES (?, ?, ?, ?, ?)",
			e.Path, e.Name, e.Parent, isVideo, isNFO,
		); err != nil {
			return
		}
	}

	exportedAt := fmt.Sprintf("%d", time.Now().Unix())
	for _, kv := range [][2]string{
		{"root_path", rootPath},
		{"exported_at", exportedAt},
		{"entry_count", fmt.Sprintf("%d", len(entries))},
	} {
		if _, err = tx.Exec(
			"INSERT OR REPLACE INTO snapshot_meta (key, value) VALUES (?, ?)",
			kv[0], kv[1],
		); err != nil {
			return
		}
	}
	err = tx.Commit()
}

// GetTreeEntries returns all tree entries, optionally filtered by a category
// path prefix (entries whose path starts with category + "/" or equals category).
func (c *Cache) GetTreeEntries(category string) []TreeEntry {
	var rows *sql.Rows
	var err error
	if category != "" {
		rows, err = c.db.Query(
			"SELECT path, name, parent, is_video, is_nfo FROM tree_entry WHERE path LIKE ? OR path = ?",
			category+"/%", category,
		)
	} else {
		rows, err = c.db.Query(
			"SELECT path, name, parent, is_video, is_nfo FROM tree_entry",
		)
	}
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []TreeEntry
	for rows.Next() {
		var e TreeEntry
		var isVideo, isNFO int
		if err := rows.Scan(&e.Path, &e.Name, &e.Parent, &isVideo, &isNFO); err != nil {
			continue
		}
		e.IsVideo = isVideo != 0
		e.IsNFO = isNFO != 0
		out = append(out, e)
	}
	if out == nil {
		out = []TreeEntry{}
	}
	return out
}

// TreeStats returns total/video/nfo counts.
func (c *Cache) TreeStats() (total, videos, nfos int) {
	_ = c.db.QueryRow("SELECT COUNT(*) FROM tree_entry").Scan(&total)
	_ = c.db.QueryRow("SELECT COUNT(*) FROM tree_entry WHERE is_video = 1").Scan(&videos)
	_ = c.db.QueryRow("SELECT COUNT(*) FROM tree_entry WHERE is_nfo = 1").Scan(&nfos)
	return
}

// GetSnapshotMeta returns the snapshot metadata recorded by SetTree.
func (c *Cache) GetSnapshotMeta() SnapshotMeta {
	rows, err := c.db.Query("SELECT key, value FROM snapshot_meta")
	if err != nil {
		return SnapshotMeta{}
	}
	defer rows.Close()
	kv := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err == nil {
			kv[k] = v
		}
	}
	return SnapshotMeta{
		RootPath:   kv["root_path"],
		ExportedAt: kv["exported_at"],
		EntryCount: kv["entry_count"],
	}
}

// ClearTree deletes all tree_entry rows.
func (c *Cache) ClearTree() {
	tx, err := c.db.Begin()
	if err != nil {
		return
	}
	_, _ = tx.Exec("DELETE FROM tree_entry")
	_ = tx.Commit()
}

// ---- management -----------------------------------------------------------

// ClearMetadata clears all cache data except rate_limit.
func (c *Cache) ClearMetadata() {
	tx, err := c.db.Begin()
	if err != nil {
		return
	}
	for _, tbl := range []string{"path_index", "dir_meta", "dir_entry", "tree_entry", "snapshot_meta"} {
		_, _ = tx.Exec("DELETE FROM " + tbl)
	}
	_ = tx.Commit()
}

// Clear deletes everything including rate_limit.
func (c *Cache) Clear() {
	tx, err := c.db.Begin()
	if err != nil {
		return
	}
	for _, tbl := range []string{"path_index", "dir_meta", "dir_entry", "rate_limit", "tree_entry", "snapshot_meta"} {
		_, _ = tx.Exec("DELETE FROM " + tbl)
	}
	_ = tx.Commit()
}

// Stats returns a summary of cache contents and database file size.
func (c *Cache) Stats() CacheStats {
	var s CacheStats
	_ = c.db.QueryRow("SELECT COUNT(*) FROM path_index").Scan(&s.PathCount)
	_ = c.db.QueryRow("SELECT COUNT(*) FROM dir_meta").Scan(&s.DirCount)
	_ = c.db.QueryRow("SELECT COUNT(*) FROM dir_entry").Scan(&s.EntryCount)
	_ = c.db.QueryRow("SELECT COUNT(*) FROM tree_entry").Scan(&s.TreeEntries)

	if fi, err := os.Stat(c.dbPath); err == nil {
		s.DBSizeBytes = fi.Size()
	}

	rows, err := c.db.Query("SELECT name, cooldown_until, last_request, minute_start, minute_count FROM rate_limit")
	if err == nil {
		defer rows.Close()
		s.RateLimit = make(map[string]RateLimitState)
		for rows.Next() {
			var name string
			var state RateLimitState
			if err := rows.Scan(&name, &state.CooldownUntil, &state.LastRequest, &state.MinuteStart, &state.MinuteCount); err == nil {
				s.RateLimit[name] = state
			}
		}
	}
	if s.RateLimit == nil {
		s.RateLimit = make(map[string]RateLimitState)
	}
	return s
}

// StaleDirCount returns the number of directories whose listing has expired.
func (c *Cache) StaleDirCount(listingTTL int) int {
	cutoff := time.Now().Unix() - int64(listingTTL)
	var count int
	_ = c.db.QueryRow("SELECT COUNT(*) FROM dir_meta WHERE ts < ?", cutoff).Scan(&count)
	return count
}

// Close closes the database connection. Idempotent.
func (c *Cache) Close() error {
	if c.db == nil {
		return nil
	}
	err := c.db.Close()
	c.db = nil
	return err
}

// ---- helpers --------------------------------------------------------------

// scanEntry scans one row into an Entry, returning nil on ErrNoRows.
func scanEntry(row *sql.Row) (*Entry, error) {
	var e Entry
	var size sql.NullInt64
	var pickCode sql.NullString
	err := row.Scan(&e.Name, &e.Type, &e.NodeID, &size, &pickCode)
	if err == sql.ErrNoRows {
		return nil, sql.ErrNoRows
	}
	if err != nil {
		return nil, err
	}
	if size.Valid {
		e.Size = size.Int64
	}
	if pickCode.Valid {
		e.PickCode = pickCode.String
	}
	return &e, nil
}

// scanEntries reads all rows from a *sql.Rows into an []Entry slice.
func scanEntries(rows *sql.Rows) []Entry {
	var out []Entry
	for rows.Next() {
		var e Entry
		var size sql.NullInt64
		var pickCode sql.NullString
		if err := rows.Scan(&e.Name, &e.Type, &e.NodeID, &size, &pickCode); err != nil {
			continue
		}
		if size.Valid {
			e.Size = size.Int64
		}
		if pickCode.Valid {
			e.PickCode = pickCode.String
		}
		out = append(out, e)
	}
	if out == nil {
		out = []Entry{}
	}
	return out
}
