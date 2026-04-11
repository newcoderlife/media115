package cloud115

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// ---- helpers ---------------------------------------------------------------

func newTestCache(t *testing.T) *Cache {
	t.Helper()
	c, err := NewCache(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func entry(name, typ, nodeID string, size int64, pickCode string) Entry {
	return Entry{Name: name, Type: typ, NodeID: nodeID, Size: size, PickCode: pickCode}
}

func fileEntry(name, nodeID string) Entry {
	return entry(name, "file", nodeID, 100, "pc1")
}

// ---- TestCachePathIndex ----------------------------------------------------

func TestCachePathIndex(t *testing.T) {
	t.Run("set_and_get", func(t *testing.T) {
		c := newTestCache(t)
		c.SetPath("/movies/foo", "cid_1")
		cid, ts, ok := c.GetPath("/movies/foo")
		if !ok {
			t.Fatal("expected ok=true")
		}
		if cid != "cid_1" {
			t.Errorf("cid = %q; want cid_1", cid)
		}
		if ts > time.Now().Unix() {
			t.Errorf("ts %d is in the future", ts)
		}
	})

	t.Run("overwrite", func(t *testing.T) {
		c := newTestCache(t)
		c.SetPath("/a", "old")
		c.SetPath("/a", "new")
		cid, _, ok := c.GetPath("/a")
		if !ok || cid != "new" {
			t.Errorf("expected cid=new, got %q ok=%v", cid, ok)
		}
	})

	t.Run("missing_returns_not_ok", func(t *testing.T) {
		c := newTestCache(t)
		_, _, ok := c.GetPath("/nonexistent")
		if ok {
			t.Error("expected ok=false for missing key")
		}
	})

	t.Run("delete_prefix_exact_and_children", func(t *testing.T) {
		c := newTestCache(t)
		c.SetPath("/a/b", "1")
		c.SetPath("/a/b/c", "2")
		c.SetPath("/a/b/d", "3")
		c.SetPath("/a/x", "4")
		c.DeletePathPrefix("/a/b")

		for _, p := range []string{"/a/b", "/a/b/c", "/a/b/d"} {
			if _, _, ok := c.GetPath(p); ok {
				t.Errorf("expected %q to be deleted", p)
			}
		}
		if _, _, ok := c.GetPath("/a/x"); !ok {
			t.Error("sibling /a/x should survive")
		}
	})

	t.Run("delete_prefix_does_not_hit_partial_match", func(t *testing.T) {
		c := newTestCache(t)
		c.SetPath("/a/b", "1")
		c.SetPath("/ab", "2")
		c.DeletePathPrefix("/a/b")
		if _, _, ok := c.GetPath("/ab"); !ok {
			t.Error("/ab must survive deletion of prefix /a/b")
		}
	})
}

// ---- TestCacheDirListing ---------------------------------------------------

func TestCacheDirListing(t *testing.T) {
	t.Run("set_and_get", func(t *testing.T) {
		c := newTestCache(t)
		entries := []Entry{fileEntry("a.txt", "1"), fileEntry("b.mp4", "2")}
		c.SetDirListing("dir_cid", entries)
		result := c.GetDirEntries("dir_cid")
		if len(result) != 2 {
			t.Fatalf("want 2 entries, got %d", len(result))
		}
	})

	t.Run("set_updates_dir_meta", func(t *testing.T) {
		c := newTestCache(t)
		if _, ok := c.GetDirTS("d1"); ok {
			t.Error("expected no dir_meta before set")
		}
		c.SetDirListing("d1", []Entry{fileEntry("f", "1")})
		ts, ok := c.GetDirTS("d1")
		if !ok {
			t.Fatal("expected dir_meta after set")
		}
		if ts > time.Now().Unix() {
			t.Errorf("ts %d is in the future", ts)
		}
	})

	t.Run("set_replaces_old_entries", func(t *testing.T) {
		c := newTestCache(t)
		c.SetDirListing("d1", []Entry{fileEntry("old.txt", "1")})
		c.SetDirListing("d1", []Entry{fileEntry("new.txt", "2")})
		result := c.GetDirEntries("d1")
		if len(result) != 1 || result[0].Name != "new.txt" {
			t.Errorf("expected [new.txt], got %v", result)
		}
	})

	t.Run("get_empty_returns_empty_slice", func(t *testing.T) {
		c := newTestCache(t)
		result := c.GetDirEntries("nonexistent")
		if len(result) != 0 {
			t.Errorf("expected empty, got %v", result)
		}
	})

	t.Run("find_entry_found", func(t *testing.T) {
		c := newTestCache(t)
		c.SetDirListing("d1", []Entry{fileEntry("target.txt", "n9")})
		e := c.FindEntry("d1", "target.txt")
		if e == nil || e.NodeID != "n9" {
			t.Errorf("expected node_id n9, got %v", e)
		}
	})

	t.Run("find_entry_missing", func(t *testing.T) {
		c := newTestCache(t)
		c.SetDirListing("d1", []Entry{fileEntry("f", "1")})
		if e := c.FindEntry("d1", "nope.txt"); e != nil {
			t.Errorf("expected nil, got %v", e)
		}
	})

	t.Run("find_entries", func(t *testing.T) {
		c := newTestCache(t)
		c.SetDirListing("d1", []Entry{
			fileEntry("dup.mkv", "n1"),
			fileEntry("dup.mkv", "n2"),
			fileEntry("other.txt", "n3"),
		})
		all := c.FindEntries("d1", "dup.mkv")
		if len(all) != 2 {
			t.Errorf("want 2 results, got %d", len(all))
		}
	})

	t.Run("order_dirs_first_then_name", func(t *testing.T) {
		c := newTestCache(t)
		entries := []Entry{
			entry("beta.txt", "file", "1", 100, ""),
			entry("alpha", "dir", "2", 0, ""),
			entry("gamma", "dir", "3", 0, ""),
			entry("aaa.txt", "file", "4", 100, ""),
		}
		c.SetDirListing("d1", entries)
		result := c.GetDirEntries("d1")
		if len(result) != 4 {
			t.Fatalf("want 4, got %d", len(result))
		}
		if result[0].Type != "dir" || result[1].Type != "dir" {
			t.Error("expected dirs first")
		}
		if result[0].Name != "alpha" || result[1].Name != "gamma" {
			t.Errorf("dir order wrong: %s %s", result[0].Name, result[1].Name)
		}
		if result[2].Name != "aaa.txt" || result[3].Name != "beta.txt" {
			t.Errorf("file order wrong: %s %s", result[2].Name, result[3].Name)
		}
	})
}

// ---- TestCacheWriteThrough -------------------------------------------------

func TestCacheWriteThrough(t *testing.T) {
	t.Run("add_entry", func(t *testing.T) {
		c := newTestCache(t)
		c.SetDirListing("d1", []Entry{fileEntry("a.txt", "1")})
		c.AddEntry("d1", fileEntry("b.txt", "2"))
		if got := c.GetDirEntries("d1"); len(got) != 2 {
			t.Errorf("want 2, got %d", len(got))
		}
	})

	t.Run("add_entry_upserts", func(t *testing.T) {
		c := newTestCache(t)
		c.SetDirListing("d1", []Entry{entry("a.txt", "file", "1", 10, "")})
		c.AddEntry("d1", entry("a.txt", "file", "1", 99, ""))
		got := c.GetDirEntries("d1")
		if len(got) != 1 || got[0].Size != 99 {
			t.Errorf("expected size=99, got %v", got)
		}
	})

	t.Run("remove_entry", func(t *testing.T) {
		c := newTestCache(t)
		c.SetDirListing("d1", []Entry{fileEntry("keep.txt", "1"), fileEntry("gone.txt", "2")})
		c.RemoveEntry("d1", "gone.txt")
		got := c.GetDirEntries("d1")
		if len(got) != 1 || got[0].Name != "keep.txt" {
			t.Errorf("unexpected result: %v", got)
		}
	})

	t.Run("rename_entry", func(t *testing.T) {
		c := newTestCache(t)
		c.SetDirListing("d1", []Entry{fileEntry("old.txt", "n1")})
		c.RenameEntry("d1", "n1", "new.txt")
		if c.FindEntry("d1", "old.txt") != nil {
			t.Error("old name should be gone")
		}
		if c.FindEntry("d1", "new.txt") == nil {
			t.Error("new name should exist")
		}
	})

	t.Run("move_entry", func(t *testing.T) {
		c := newTestCache(t)
		c.SetDirListing("src", []Entry{fileEntry("file.mkv", "n1")})
		c.SetDirListing("dst", []Entry{})
		c.MoveEntry("src", "n1", "dst")
		if c.FindEntry("src", "file.mkv") != nil {
			t.Error("entry should no longer be in src")
		}
		if e := c.FindEntry("dst", "file.mkv"); e == nil || e.NodeID != "n1" {
			t.Errorf("expected entry in dst, got %v", e)
		}
	})

	t.Run("remove_entry_by_id", func(t *testing.T) {
		c := newTestCache(t)
		c.SetDirListing("d1", []Entry{fileEntry("dup.mkv", "n1"), fileEntry("dup.mkv", "n2")})
		c.RemoveEntryByID("d1", "n1")
		got := c.GetDirEntries("d1")
		if len(got) != 1 || got[0].NodeID != "n2" {
			t.Errorf("expected only n2 left, got %v", got)
		}
	})

	t.Run("invalidate_dir", func(t *testing.T) {
		c := newTestCache(t)
		c.SetDirListing("d1", []Entry{fileEntry("f", "1")})
		c.InvalidateDir("d1")
		if _, ok := c.GetDirTS("d1"); ok {
			t.Error("dir_meta should be gone")
		}
		if got := c.GetDirEntries("d1"); len(got) != 0 {
			t.Errorf("dir_entry should be empty, got %v", got)
		}
	})

	t.Run("delete_dir_meta_keeps_entries", func(t *testing.T) {
		c := newTestCache(t)
		c.SetDirListing("d1", []Entry{fileEntry("f.txt", "1")})
		c.DeleteDirMeta("d1")
		if _, ok := c.GetDirTS("d1"); ok {
			t.Error("dir_meta should be gone")
		}
		got := c.GetDirEntries("d1")
		if len(got) != 1 || got[0].Name != "f.txt" {
			t.Errorf("entry should survive, got %v", got)
		}
	})

	t.Run("same_name_coexist", func(t *testing.T) {
		c := newTestCache(t)
		c.SetDirListing("d1", []Entry{
			entry("dup.mkv", "file", "n1", 100, ""),
			entry("dup.mkv", "file", "n2", 200, ""),
		})
		got := c.GetDirEntries("d1")
		if len(got) != 2 {
			t.Errorf("want 2 same-name entries, got %d", len(got))
		}
	})

	t.Run("add_entry_no_dir_meta", func(t *testing.T) {
		c := newTestCache(t)
		c.AddEntry("orphan_cid", fileEntry("file.txt", "f1"))
		e := c.FindEntry("orphan_cid", "file.txt")
		if e == nil || e.NodeID != "f1" {
			t.Errorf("expected entry, got %v", e)
		}
		if _, ok := c.GetDirTS("orphan_cid"); ok {
			t.Error("dir_meta should remain nil for orphan dir")
		}
	})
}

// ---- TestCacheRateLimit ----------------------------------------------------

func TestCacheRateLimit(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		c := newTestCache(t)
		rl := c.GetRateLimit("api_list")
		if rl.CooldownUntil != 0 || rl.LastRequest != 0 || rl.MinuteCount != 0 {
			t.Errorf("unexpected defaults: %+v", rl)
		}
	})

	t.Run("set_and_get", func(t *testing.T) {
		c := newTestCache(t)
		c.SetRateLimit("api_list", RateLimitState{CooldownUntil: 100.0, LastRequest: 99.0})
		rl := c.GetRateLimit("api_list")
		if rl.CooldownUntil != 100.0 || rl.LastRequest != 99.0 {
			t.Errorf("unexpected state: %+v", rl)
		}
	})

	t.Run("set_merges", func(t *testing.T) {
		c := newTestCache(t)
		c.SetRateLimit("x", RateLimitState{CooldownUntil: 50.0})
		c.SetRateLimit("x", RateLimitState{LastRequest: 70.0})
		rl := c.GetRateLimit("x")
		if rl.CooldownUntil != 50.0 || rl.LastRequest != 70.0 {
			t.Errorf("merge failed: %+v", rl)
		}
	})

	t.Run("cooldown", func(t *testing.T) {
		c := newTestCache(t)
		future := float64(time.Now().Unix() + 3600)
		c.SetRateLimit("api_move", RateLimitState{CooldownUntil: future})
		rl := c.GetRateLimit("api_move")
		if rl.CooldownUntil < float64(time.Now().Unix()) {
			t.Error("cooldown should be in the future")
		}
	})

	t.Run("try_acquire_slot_succeeds", func(t *testing.T) {
		c := newTestCache(t)
		now := float64(time.Now().Unix())
		if !c.TryAcquireSlot("test", now, 1.0, 20) {
			t.Fatal("expected slot acquisition to succeed")
		}
		rl := c.GetRateLimit("test")
		if rl.MinuteCount != 1 {
			t.Errorf("minute_count = %d; want 1", rl.MinuteCount)
		}
	})

	t.Run("try_acquire_slot_fails_qpm", func(t *testing.T) {
		c := newTestCache(t)
		now := float64(time.Now().Unix())
		c.SetRateLimit("test", RateLimitState{LastRequest: now - 5, MinuteCount: 20})
		// Need to also set minute_start so it's within the window.
		// We use the DB directly for minute_start via IncrementMinuteCount approach—
		// actually, set a full state via raw SQL.
		_, _ = c.db.Exec(
			"INSERT OR REPLACE INTO rate_limit (name, cooldown_until, last_request, minute_start, minute_count) VALUES (?,0,?,?,?)",
			"test", now-5, now, 20,
		)
		if c.TryAcquireSlot("test", now, 1.0, 20) {
			t.Fatal("expected slot acquisition to fail (QPM exceeded)")
		}
	})

	t.Run("try_acquire_slot_fails_cooldown", func(t *testing.T) {
		c := newTestCache(t)
		now := float64(time.Now().Unix())
		c.SetRateLimit("test", RateLimitState{CooldownUntil: now + 3600})
		if c.TryAcquireSlot("test", now, 1.0, 20) {
			t.Fatal("expected slot acquisition to fail (cooldown)")
		}
	})

	t.Run("try_acquire_slot_fails_qps", func(t *testing.T) {
		c := newTestCache(t)
		now := float64(time.Now().Unix())
		_, _ = c.db.Exec(
			"INSERT OR REPLACE INTO rate_limit (name, cooldown_until, last_request, minute_start, minute_count) VALUES (?,0,?,?,?)",
			"test", now-0.5, now-10, 5,
		)
		if c.TryAcquireSlot("test", now, 2.0, 20) {
			t.Fatal("expected slot acquisition to fail (QPS/min_interval)")
		}
	})

	t.Run("try_acquire_slot_resets_minute_window", func(t *testing.T) {
		c := newTestCache(t)
		now := float64(time.Now().Unix())
		_, _ = c.db.Exec(
			"INSERT OR REPLACE INTO rate_limit (name, cooldown_until, last_request, minute_start, minute_count) VALUES (?,0,0,?,999)",
			"test", now-120,
		)
		if !c.TryAcquireSlot("test", now, 0.0, 20) {
			t.Fatal("expected slot acquisition to succeed after minute window reset")
		}
		rl := c.GetRateLimit("test")
		if rl.MinuteCount != 1 {
			t.Errorf("minute_count = %d; want 1 (window should have reset)", rl.MinuteCount)
		}
	})
}

// ---- TestCacheTreeEntry ----------------------------------------------------

func TestCacheTreeEntry(t *testing.T) {
	makeTree := func(path, name, parent string, isVideo, isNFO bool) TreeEntry {
		return TreeEntry{Path: path, Name: name, Parent: parent, IsVideo: isVideo, IsNFO: isNFO}
	}

	t.Run("set_and_get_all", func(t *testing.T) {
		c := newTestCache(t)
		entries := []TreeEntry{
			makeTree("AV/DANDY-001/DANDY-001.mkv", "DANDY-001.mkv", "AV/DANDY-001", true, false),
			makeTree("AV/DANDY-001/DANDY-001.nfo", "DANDY-001.nfo", "AV/DANDY-001", false, true),
			makeTree("电影/Dune (2021)/Dune (2021).mkv", "Dune (2021).mkv", "电影/Dune (2021)", true, false),
		}
		c.SetTree(entries, "")
		result := c.GetTreeEntries("")
		if len(result) != 3 {
			t.Fatalf("want 3, got %d", len(result))
		}
	})

	t.Run("get_entries_fields", func(t *testing.T) {
		c := newTestCache(t)
		c.SetTree([]TreeEntry{makeTree("AV/DANDY-001/DANDY-001.mkv", "DANDY-001.mkv", "AV/DANDY-001", true, false)}, "")
		result := c.GetTreeEntries("")
		if len(result) != 1 {
			t.Fatalf("want 1")
		}
		e := result[0]
		if e.Path != "AV/DANDY-001/DANDY-001.mkv" {
			t.Errorf("path = %q", e.Path)
		}
		if e.Name != "DANDY-001.mkv" {
			t.Errorf("name = %q", e.Name)
		}
		if !e.IsVideo || e.IsNFO {
			t.Errorf("flags wrong: is_video=%v is_nfo=%v", e.IsVideo, e.IsNFO)
		}
	})

	t.Run("filter", func(t *testing.T) {
		c := newTestCache(t)
		c.SetTree([]TreeEntry{
			makeTree("AV/DANDY-001/DANDY-001.mkv", "DANDY-001.mkv", "AV/DANDY-001", true, false),
			makeTree("电影/Dune (2021)/Dune (2021).mkv", "Dune (2021).mkv", "电影/Dune (2021)", true, false),
		}, "")
		result := c.GetTreeEntries("AV")
		if len(result) != 1 || result[0].Path != "AV/DANDY-001/DANDY-001.mkv" {
			t.Errorf("filter wrong: %v", result)
		}
	})

	t.Run("stats", func(t *testing.T) {
		c := newTestCache(t)
		c.SetTree([]TreeEntry{
			makeTree("AV/X/X.mkv", "X.mkv", "AV/X", true, false),
			makeTree("AV/X/X.nfo", "X.nfo", "AV/X", false, true),
			makeTree("电影/Y/Y.mkv", "Y.mkv", "电影/Y", true, false),
		}, "")
		total, videos, nfos := c.TreeStats()
		if total != 3 || videos != 2 || nfos != 1 {
			t.Errorf("stats: total=%d videos=%d nfos=%d", total, videos, nfos)
		}
	})

	t.Run("snapshot_meta", func(t *testing.T) {
		c := newTestCache(t)
		c.SetTree([]TreeEntry{makeTree("x/y.mkv", "y.mkv", "x", true, false)}, "/root")
		meta := c.GetSnapshotMeta()
		if meta.RootPath != "/root" {
			t.Errorf("root_path = %q", meta.RootPath)
		}
		if meta.EntryCount != "1" {
			t.Errorf("entry_count = %q", meta.EntryCount)
		}
		if meta.ExportedAt == "" {
			t.Error("exported_at should be set")
		}
	})

	t.Run("clear_tree", func(t *testing.T) {
		c := newTestCache(t)
		c.SetTree([]TreeEntry{makeTree("a/b.mkv", "b.mkv", "a", true, false)}, "")
		c.ClearTree()
		if got := c.GetTreeEntries(""); len(got) != 0 {
			t.Errorf("expected empty, got %v", got)
		}
	})

	t.Run("set_tree_replaces_existing", func(t *testing.T) {
		c := newTestCache(t)
		c.SetTree([]TreeEntry{makeTree("old/file.mkv", "file.mkv", "old", true, false)}, "")
		c.SetTree([]TreeEntry{makeTree("new/film.mkv", "film.mkv", "new", true, false)}, "")
		result := c.GetTreeEntries("")
		if len(result) != 1 || result[0].Path != "new/film.mkv" {
			t.Errorf("unexpected: %v", result)
		}
	})

	t.Run("set_tree_empty", func(t *testing.T) {
		c := newTestCache(t)
		c.SetTree([]TreeEntry{makeTree("a/b.mkv", "b.mkv", "a", true, false)}, "")
		c.SetTree([]TreeEntry{}, "")
		if got := c.GetTreeEntries(""); len(got) != 0 {
			t.Errorf("expected empty, got %v", got)
		}
	})
}

// ---- TestCacheManagement ---------------------------------------------------

func TestCacheManagement(t *testing.T) {
	t.Run("double_close", func(t *testing.T) {
		c, err := NewCache(filepath.Join(t.TempDir(), "test.db"))
		if err != nil {
			t.Fatal(err)
		}
		if err := c.Close(); err != nil {
			t.Errorf("first Close: %v", err)
		}
		if err := c.Close(); err != nil {
			t.Errorf("second Close: %v", err)
		}
	})

	t.Run("clear", func(t *testing.T) {
		c := newTestCache(t)
		c.SetPath("/a", "1")
		c.SetDirListing("d1", []Entry{fileEntry("f", "1")})
		c.SetRateLimit("x", RateLimitState{CooldownUntil: 1.0})
		c.Clear()

		if _, _, ok := c.GetPath("/a"); ok {
			t.Error("path should be cleared")
		}
		if got := c.GetDirEntries("d1"); len(got) != 0 {
			t.Error("dir entries should be cleared")
		}
		if _, ok := c.GetDirTS("d1"); ok {
			t.Error("dir meta should be cleared")
		}
		if rl := c.GetRateLimit("x"); rl.CooldownUntil != 0 {
			t.Errorf("rate_limit should be cleared, got %+v", rl)
		}
	})

	t.Run("clear_metadata", func(t *testing.T) {
		c := newTestCache(t)
		c.SetPath("/a", "1")
		c.SetDirListing("cid1", []Entry{entry("f", "file", "f1", 1, "pc")})
		c.SetRateLimit("api", RateLimitState{MinuteCount: 5})
		c.ClearMetadata()

		if _, _, ok := c.GetPath("/a"); ok {
			t.Error("path should be cleared")
		}
		if _, ok := c.GetDirTS("cid1"); ok {
			t.Error("dir meta should be cleared")
		}
		if rl := c.GetRateLimit("api"); rl.MinuteCount != 5 {
			t.Errorf("rate_limit should survive clear_metadata, got %+v", rl)
		}
	})

	t.Run("stats", func(t *testing.T) {
		c := newTestCache(t)
		s := c.Stats()
		if s.PathCount != 0 || s.DirCount != 0 || s.EntryCount != 0 {
			t.Errorf("unexpected stats on empty cache: %+v", s)
		}
		if s.DBSizeBytes < 0 {
			t.Error("db_size_bytes should be >= 0")
		}
	})

	t.Run("stats_after_inserts", func(t *testing.T) {
		c := newTestCache(t)
		c.SetPath("/a", "1")
		c.SetPath("/b", "2")
		c.SetDirListing("d1", []Entry{fileEntry("x", "1"), fileEntry("y", "2")})
		s := c.Stats()
		if s.PathCount != 2 || s.DirCount != 1 || s.EntryCount != 2 {
			t.Errorf("stats wrong: %+v", s)
		}
		if s.DBSizeBytes <= 0 {
			t.Error("db_size_bytes should be > 0")
		}
	})

	t.Run("stats_includes_rate_limit", func(t *testing.T) {
		c := newTestCache(t)
		c.SetRateLimit("api", RateLimitState{CooldownUntil: 42.0, LastRequest: 10.0, MinuteCount: 3})
		s := c.Stats()
		rl, ok := s.RateLimit["api"]
		if !ok {
			t.Fatal("expected rate_limit entry for 'api'")
		}
		if rl.CooldownUntil != 42.0 || rl.MinuteCount != 3 {
			t.Errorf("rate_limit state wrong: %+v", rl)
		}
	})

	t.Run("stale_dir_count", func(t *testing.T) {
		c := newTestCache(t)
		c.SetDirListing("fresh", []Entry{entry("f", "file", "f1", 1, "pc")})
		if n := c.StaleDirCount(3600); n != 0 {
			t.Errorf("stale_dir_count = %d; want 0 for fresh dir", n)
		}
		// Force the ts to 0 (very old) directly.
		_, _ = c.db.Exec("UPDATE dir_meta SET ts = 0 WHERE cid = 'fresh'")
		if n := c.StaleDirCount(3600); n != 1 {
			t.Errorf("stale_dir_count = %d; want 1 for forced-old dir", n)
		}
	})

	t.Run("stats_includes_tree_entries", func(t *testing.T) {
		c := newTestCache(t)
		c.SetTree([]TreeEntry{{Path: "AV/X/X.mkv", Name: "X.mkv", Parent: "AV/X", IsVideo: true}}, "")
		s := c.Stats()
		if s.TreeEntries != 1 {
			t.Errorf("tree_entries = %d; want 1", s.TreeEntries)
		}
	})
}

// ---- TestCacheMigration ----------------------------------------------------

func TestCacheMigration(t *testing.T) {
	t.Run("old_schema_migrated", func(t *testing.T) {
		dbFile := filepath.Join(t.TempDir(), "old.db")

		// Create a v1 database with the old PK (parent_cid, name).
		db, err := sql.Open("sqlite", dbFile)
		if err != nil {
			t.Fatal(err)
		}
		_, err = db.Exec(`
			CREATE TABLE dir_entry (
				parent_cid TEXT NOT NULL,
				name       TEXT NOT NULL,
				type       TEXT NOT NULL,
				node_id    TEXT NOT NULL,
				size       INTEGER,
				pick_code  TEXT,
				PRIMARY KEY (parent_cid, name)
			)
		`)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = db.Exec("INSERT INTO dir_entry VALUES ('p1','f.txt','file','old_node',100,'pc')")
		_, _ = db.Exec("PRAGMA user_version = 1")
		_ = db.Close()

		// Opening with NewCache must not error and must wipe old data.
		c, err := NewCache(dbFile)
		if err != nil {
			t.Fatalf("NewCache after old schema: %v", err)
		}
		defer func() { _ = c.Close() }()

		// Old data should be gone.
		if e := c.FindEntry("p1", "f.txt"); e != nil {
			t.Error("expected old entry to be wiped after migration")
		}
		// New schema should accept id-first operations.
		c.AddEntry("p1", fileEntry("new.txt", "nA"))
		c.AddEntry("p1", fileEntry("new.txt", "nB"))
		if got := c.GetDirEntries("p1"); len(got) != 2 {
			t.Errorf("expected 2 same-name entries after migration, got %d", len(got))
		}
	})

	t.Run("schema_version_is_set", func(t *testing.T) {
		dbFile := filepath.Join(t.TempDir(), "v.db")
		c, err := NewCache(dbFile)
		if err != nil {
			t.Fatal(err)
		}
		var version int
		_ = c.db.QueryRow("PRAGMA user_version").Scan(&version)
		_ = c.Close()
		if version != schemaVersion {
			t.Errorf("user_version = %d; want %d", version, schemaVersion)
		}
	})

	t.Run("fresh_db_no_migration", func(t *testing.T) {
		dbFile := filepath.Join(t.TempDir(), "fresh.db")
		c1, err := NewCache(dbFile)
		if err != nil {
			t.Fatal(err)
		}
		c1.AddEntry("d1", fileEntry("keep.txt", "n1"))
		_ = c1.Close()

		c2, err := NewCache(dbFile)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = c2.Close() }()
		if e := c2.FindEntry("d1", "keep.txt"); e == nil {
			t.Error("data should survive second open (no migration needed)")
		}
	})
}
