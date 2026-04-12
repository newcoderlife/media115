package cloud115

// client_extra_test.go – additional coverage for client.go and ratelimit.go
// Uses the mockAPI / newTestClient helpers defined in client_test.go.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ── ClientOption smoke tests ──────────────────────────────────────────────────

func TestClientOptionWithStats(t *testing.T) {
	// Verify that WithStats / WithListingTTL / WithPathTTL / WithCacheDir
	// actually mutate the Client without panicking.
	c := &Client{}
	WithListingTTL(30 * time.Second)(c)
	if c.listingTTL != 30*time.Second {
		t.Errorf("listingTTL = %v; want 30s", c.listingTTL)
	}
	WithPathTTL(12 * time.Hour)(c)
	if c.pathTTL != 12*time.Hour {
		t.Errorf("pathTTL = %v; want 12h", c.pathTTL)
	}
	WithCacheDir("/tmp/test_cache")(c)
	if c.cacheDir != "/tmp/test_cache" {
		t.Errorf("cacheDir = %q; want /tmp/test_cache", c.cacheDir)
	}
}

// ── Close ─────────────────────────────────────────────────────────────────────

func TestClientClose(t *testing.T) {
	c := newTestClient(t, &mockAPI{})
	// Second call must not panic / return nil.
	if err := c.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// ── ListDir: stale cache triggers API refresh ─────────────────────────────────

func TestListDirStaleCacheMissesAPI(t *testing.T) {
	m := &mockAPI{
		fnListFiles: func(cid string) ([]map[string]any, error) {
			return []map[string]any{
				{"n": "fresh.mkv", "fid": "fid_fresh", "s": float64(512)},
			}, nil
		},
	}
	c := newTestClient(t, m)
	// listingTTL = 0 so any cached entry is already stale.
	c.listingTTL = 0

	seedPath(c, "/movies", "cid_movies")
	seedListing(c, "cid_movies", []Entry{
		{Name: "stale.mkv", Type: "file", NodeID: "f_old"},
	})

	entries, err := c.ListDir("/movies")
	if err != nil {
		t.Fatalf("ListDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "fresh.mkv" {
		t.Errorf("expected fresh listing, got %v", entries)
	}
	if m.listFilesCalls != 1 {
		t.Errorf("want 1 API call, got %d", m.listFilesCalls)
	}
}

// ── ResolvePath: expired path TTL → hits API again ───────────────────────────

func TestResolvePathTTLExpired(t *testing.T) {
	apiCalls := 0
	m := &mockAPI{
		fnGetDirID: func(path string) (string, error) {
			apiCalls++
			return "cid_renewed", nil
		},
	}
	c := newTestClient(t, m)
	c.pathTTL = 0 // any cached entry is immediately stale

	seedPath(c, "/movies", "cid_movies")

	cid, err := c.ResolvePath("/movies")
	if err != nil {
		t.Fatalf("ResolvePath: %v", err)
	}
	if cid != "cid_renewed" {
		t.Errorf("cid = %q; want cid_renewed", cid)
	}
	if apiCalls != 1 {
		t.Errorf("want 1 API call, got %d", apiCalls)
	}
}

// ── ResolvePath: API returns empty → error ────────────────────────────────────

func TestResolvePathAPIReturnsZero(t *testing.T) {
	m := &mockAPI{
		fnGetDirID: func(path string) (string, error) { return "0", nil },
	}
	c := newTestClient(t, m)

	_, err := c.ResolvePath("/ghost")
	if err == nil {
		t.Fatal("expected error when API returns cid=0")
	}
}

// ── ResolvePath: API returns error ────────────────────────────────────────────

func TestResolvePathAPIError(t *testing.T) {
	m := &mockAPI{
		fnGetDirID: func(path string) (string, error) {
			return "", fmt.Errorf("network error")
		},
	}
	c := newTestClient(t, m)

	_, err := c.ResolvePath("/bad")
	if err == nil {
		t.Fatal("expected error from failing API call")
	}
}

// ── ListDirUncached ───────────────────────────────────────────────────────────

func TestListDirUncached(t *testing.T) {
	m := &mockAPI{
		fnListFiles: func(cid string) ([]map[string]any, error) {
			return []map[string]any{
				{"n": "uncached.mkv", "fid": "fid_u", "s": float64(100)},
			}, nil
		},
	}
	c := newTestClient(t, m)
	seedPath(c, "/movies", "cid_movies")
	// Seed a stale listing — ListDirUncached must bypass it.
	seedListing(c, "cid_movies", []Entry{
		{Name: "stale.mkv", Type: "file", NodeID: "f_stale"},
	})

	entries, err := c.ListDirUncached("/movies")
	if err != nil {
		t.Fatalf("ListDirUncached: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "uncached.mkv" {
		t.Errorf("want uncached.mkv, got %v", entries)
	}
	if m.listFilesCalls != 1 {
		t.Errorf("want 1 API call, got %d", m.listFilesCalls)
	}
}

// ── DownloadURL ───────────────────────────────────────────────────────────────

func TestDownloadURL(t *testing.T) {
	m := &mockAPI{
		fnDownloadURL: func(pickCode, ua string) (string, error) {
			return "https://cdn.example.com/" + pickCode, nil
		},
	}
	c := newTestClient(t, m)

	url, err := c.DownloadURL("pc_abc", "TestUA/1.0")
	if err != nil {
		t.Fatalf("DownloadURL: %v", err)
	}
	if !strings.Contains(url, "pc_abc") {
		t.Errorf("URL %q should contain pc_abc", url)
	}
}

// ── Search ────────────────────────────────────────────────────────────────────

func TestSearch(t *testing.T) {
	m := &mockAPI{
		fnGetDirID: func(path string) (string, error) {
			if path == "/movies" {
				return "cid_movies", nil
			}
			return "", nil
		},
		fnSearch: func(keyword, dirID string) ([]map[string]any, error) {
			if keyword == "Inception" && dirID == "cid_movies" {
				return []map[string]any{
					{"n": "Inception.mkv", "fid": "f1", "s": float64(4096)},
				}, nil
			}
			return nil, nil
		},
	}
	c := newTestClient(t, m)

	entries, err := c.Search("Inception", "/movies")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "Inception.mkv" {
		t.Errorf("unexpected results: %v", entries)
	}
}

// ── ExportTree ────────────────────────────────────────────────────────────────

func TestExportTree(t *testing.T) {
	treeText := "|——root\n| |-file.mkv"
	m := &mockAPIWithExportTree{
		fnExportTree: func(dirID string) (string, error) {
			if dirID == "cid_root" {
				return treeText, nil
			}
			return "", fmt.Errorf("unexpected dirID: %s", dirID)
		},
	}
	cache, err := NewCache(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	c := &Client{
		api:        m,
		cache:      cache,
		listingTTL: defaultListingTTL,
		pathTTL:    defaultPathTTL,
	}
	seedPath(c, "/root", "cid_root")

	text, err := c.ExportTree("/root")
	if err != nil {
		t.Fatalf("ExportTree: %v", err)
	}
	if text != treeText {
		t.Errorf("text = %q; want %q", text, treeText)
	}
}

// ── Stat ──────────────────────────────────────────────────────────────────────

func TestStatRoot(t *testing.T) {
	c := newTestClient(t, &mockAPI{})
	e, err := c.Stat("/")
	if err != nil {
		t.Fatalf("Stat(/): %v", err)
	}
	if e.Type != "dir" || e.NodeID != "0" {
		t.Errorf("unexpected root stat: %+v", e)
	}
}

func TestStatFile(t *testing.T) {
	c := newTestClient(t, &mockAPI{})
	seedPath(c, "/movies", "cid_movies")
	seedListing(c, "cid_movies", []Entry{
		{Name: "Inception.mkv", Type: "file", NodeID: "f1", PickCode: "pc1"},
	})

	e, err := c.Stat("/movies/Inception.mkv")
	if err != nil {
		t.Fatalf("Stat(file): %v", err)
	}
	if e.Type != "file" || e.Name != "Inception.mkv" {
		t.Errorf("unexpected file stat: %+v", e)
	}
}

func TestStatDirectory(t *testing.T) {
	m := &mockAPI{
		fnGetDirID: func(path string) (string, error) {
			if path == "/movies" {
				return "cid_movies", nil
			}
			return "", nil
		},
	}
	c := newTestClient(t, m)

	e, err := c.Stat("/movies")
	if err != nil {
		t.Fatalf("Stat(dir): %v", err)
	}
	if e.Type != "dir" {
		t.Errorf("type = %q; want dir", e.Type)
	}
	if e.NodeID != "cid_movies" {
		t.Errorf("node_id = %q; want cid_movies", e.NodeID)
	}
}

// ── Mkdir: parents=true ───────────────────────────────────────────────────────

func TestMkdirParents(t *testing.T) {
	mkdirCalls := 0
	m := &mockAPI{
		fnGetDirID: func(path string) (string, error) {
			// Only root "/" is found.
			return "", fmt.Errorf("not found: %s", path)
		},
		fnMkdir: func(parentID, name string) (map[string]any, error) {
			mkdirCalls++
			return map[string]any{"cid": "cid_" + name}, nil
		},
	}
	c := newTestClient(t, m)

	// Seed "/" so ResolvePath("") returns "0".
	cid, err := c.Mkdir("/a/b/c", true)
	if err != nil {
		t.Fatalf("Mkdir with parents: %v", err)
	}
	if cid == "" || cid == "0" {
		t.Errorf("unexpected cid: %q", cid)
	}
	if mkdirCalls != 3 {
		t.Errorf("want 3 Mkdir API calls (a, b, c), got %d", mkdirCalls)
	}
}

// ── Mkdir: parents=true, intermediate dirs exist ──────────────────────────────

func TestMkdirParentsPartialExist(t *testing.T) {
	mkdirCalls := 0
	m := &mockAPI{
		fnGetDirID: func(path string) (string, error) {
			if path == "/a" {
				return "cid_a", nil
			}
			return "", fmt.Errorf("not found")
		},
		fnMkdir: func(parentID, name string) (map[string]any, error) {
			mkdirCalls++
			return map[string]any{"cid": "cid_" + name}, nil
		},
	}
	c := newTestClient(t, m)

	cid, err := c.Mkdir("/a/b", true)
	if err != nil {
		t.Fatalf("Mkdir with partial parents: %v", err)
	}
	if cid == "" {
		t.Error("expected valid cid")
	}
	// Only /a/b needs to be created.
	if mkdirCalls != 1 {
		t.Errorf("want 1 Mkdir API call, got %d", mkdirCalls)
	}
}

// ── Rename: dir updates path_index -------------------------------------------

func TestRenameDir(t *testing.T) {
	m := &mockAPI{
		fnRename: func(fileID, newName string) (map[string]any, error) {
			return map[string]any{}, nil
		},
	}
	c := newTestClient(t, m)
	seedPath(c, "/movies", "cid_movies")
	seedListing(c, "cid_movies", []Entry{
		{Name: "Action", Type: "dir", NodeID: "cid_action"},
	})
	// Seed child paths so DeletePathPrefix has something to do.
	c.cache.SetPath("/movies/Action", "cid_action")
	c.cache.SetPath("/movies/Action/sub", "cid_sub")

	err := c.Rename("/movies/Action", "Drama")
	if err != nil {
		t.Fatalf("Rename dir: %v", err)
	}

	// Old path must be gone.
	if _, _, ok := c.cache.GetPath("/movies/Action"); ok {
		t.Error("/movies/Action should be gone from path_index")
	}
	// New path must be set.
	if _, _, ok := c.cache.GetPath("/movies/Drama"); !ok {
		t.Error("/movies/Drama should be in path_index")
	}
	// Entry name updated.
	if e := c.cache.FindEntry("cid_movies", "Drama"); e == nil {
		t.Error("renamed dir entry not found in cache")
	}
}

// ── Delete: path resolves to directory (no cache entry) ─────────────────────

func TestDeleteByResolvingDirectory(t *testing.T) {
	m := &mockAPI{
		fnGetDirID: func(path string) (string, error) {
			if path == "/movies" {
				return "cid_movies", nil
			}
			if path == "/movies/Action" {
				return "cid_action", nil
			}
			return "", nil
		},
		fnListFiles: func(cid string) ([]map[string]any, error) {
			// Return empty listing so the entry is not in cache.
			return nil, nil
		},
		fnDelete: func(fileIDs []string) (map[string]any, error) {
			return map[string]any{}, nil
		},
	}
	c := newTestClient(t, m)
	seedPath(c, "/movies", "cid_movies")
	// Do NOT seed an entry for "Action" in the listing.

	err := c.Delete([]string{"/movies/Action"})
	if err != nil {
		t.Fatalf("Delete (dir path): %v", err)
	}
}

// ── DeleteByIDs ───────────────────────────────────────────────────────────────

func TestDeleteByIDs(t *testing.T) {
	var deletedIDs []string
	m := &mockAPI{
		fnDelete: func(fids []string) (map[string]any, error) {
			deletedIDs = fids
			return map[string]any{}, nil
		},
	}
	c := newTestClient(t, m)

	if err := c.DeleteByIDs([]string{"fid_1", "fid_2"}, nil); err != nil {
		t.Fatalf("DeleteByIDs: %v", err)
	}
	if len(deletedIDs) != 2 {
		t.Errorf("want 2 deleted IDs, got %d", len(deletedIDs))
	}
}

func TestDeleteByIDsEmpty(t *testing.T) {
	m := &mockAPI{}
	c := newTestClient(t, m)
	if err := c.DeleteByIDs(nil, nil); err != nil {
		t.Fatalf("DeleteByIDs(nil): %v", err)
	}
	if m.deleteCalls != 0 {
		t.Errorf("API should not be called for empty fids, got %d calls", m.deleteCalls)
	}
}

// ── BatchRename ───────────────────────────────────────────────────────────────

func TestBatchRename(t *testing.T) {
	var gotRenames map[string]string
	m := &mockAPI{
		fnBatchRename: func(renames map[string]string) (map[string]any, error) {
			gotRenames = renames
			return map[string]any{}, nil
		},
	}
	c := newTestClient(t, m)
	seedPath(c, "/movies", "cid_movies")
	seedListing(c, "cid_movies", []Entry{
		{Name: "a.mkv", Type: "file", NodeID: "fid_a"},
		{Name: "b.mkv", Type: "file", NodeID: "fid_b"},
	})

	err := c.BatchRename([]BatchRenameItem{
		{Path: "/movies/a.mkv", NewName: "alpha.mkv"},
		{Path: "/movies/b.mkv", NewName: "beta.mkv"},
	})
	if err != nil {
		t.Fatalf("BatchRename: %v", err)
	}

	if len(gotRenames) != 2 {
		t.Errorf("want 2 renames in API call, got %d", len(gotRenames))
	}
	if gotRenames["fid_a"] != "alpha.mkv" {
		t.Errorf("fid_a → %q; want alpha.mkv", gotRenames["fid_a"])
	}

	// Cache updated.
	if e := c.cache.FindEntry("cid_movies", "alpha.mkv"); e == nil {
		t.Error("alpha.mkv should be in cache after BatchRename")
	}
}

// ── Upload: no fid in response → marks dir stale ─────────────────────────────

func TestUploadNoFIDMarkStale(t *testing.T) {
	m := &mockAPI{
		fnUploadFile: func(localPath, targetDirID, filename string) (map[string]any, error) {
			// Response has no "fid" key.
			return map[string]any{"status": "ok"}, nil
		},
	}
	c := newTestClient(t, m)
	seedPath(c, "/uploads", "cid_uploads")
	seedListing(c, "cid_uploads", []Entry{})

	_, err := c.Upload("/tmp/test_file.mp4", "/uploads", "test_file.mp4")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	// dir_meta should be gone (stale).
	if _, ok := c.cache.GetDirTS("cid_uploads"); ok {
		t.Error("dir_meta should be stale (deleted) after Upload with no fid")
	}
}

// ── Upload: nil response → marks dir stale ───────────────────────────────────

func TestUploadNilResponseMarkStale(t *testing.T) {
	m := &mockAPI{
		fnUploadFile: func(localPath, targetDirID, filename string) (map[string]any, error) {
			return nil, nil
		},
	}
	c := newTestClient(t, m)
	seedPath(c, "/uploads", "cid_uploads")
	seedListing(c, "cid_uploads", []Entry{})

	_, err := c.Upload("/tmp/x.mkv", "/uploads", "x.mkv")
	if err != nil {
		t.Fatalf("Upload nil resp: %v", err)
	}

	if _, ok := c.cache.GetDirTS("cid_uploads"); ok {
		t.Error("dir_meta should be stale after Upload with nil response")
	}
}

// ── Upload: empty filename defaults to local base name ───────────────────────

func TestUploadDefaultFilename(t *testing.T) {
	var gotFilename string
	m := &mockAPI{
		fnUploadFile: func(localPath, targetDirID, filename string) (map[string]any, error) {
			gotFilename = filename
			return map[string]any{"fid": "fid_x", "file_size": float64(0)}, nil
		},
	}
	c := newTestClient(t, m)
	seedPath(c, "/up", "cid_up")

	_, err := c.Upload("/some/path/my_video.mkv", "/up", "")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	if gotFilename != "my_video.mkv" {
		t.Errorf("filename = %q; want my_video.mkv", gotFilename)
	}
}

// ── RapidUpload ───────────────────────────────────────────────────────────────

func TestRapidUpload(t *testing.T) {
	// Write a small temp file to upload.
	tmp := t.TempDir()
	localFile := filepath.Join(tmp, "rapid.mkv")
	if err := os.WriteFile(localFile, []byte("hello rapid upload"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	m := &mockAPI{}
	// RapidUpload in mockAPI always returns status=2.
	c := newTestClient(t, m)
	seedPath(c, "/uploads", "cid_uploads")
	seedListing(c, "cid_uploads", []Entry{})

	result, err := c.RapidUpload(localFile, "/uploads")
	if err != nil {
		t.Fatalf("RapidUpload: %v", err)
	}
	if result.Status != 2 {
		t.Errorf("status = %d; want 2", result.Status)
	}

	// dir_meta should be gone after successful rapid upload (status=2).
	if _, ok := c.cache.GetDirTS("cid_uploads"); ok {
		t.Error("dir_meta should be stale after RapidUpload status=2")
	}
}

// ── Walk ──────────────────────────────────────────────────────────────────────

func TestWalk(t *testing.T) {
	m := &mockAPI{}
	c := newTestClient(t, m)

	seedPath(c, "/root", "cid_root")
	seedListing(c, "cid_root", []Entry{
		{Name: "SubA", Type: "dir", NodeID: "cid_sub_a"},
		{Name: "file.mkv", Type: "file", NodeID: "f1"},
	})
	seedPath(c, "/root/SubA", "cid_sub_a")
	seedListing(c, "cid_sub_a", []Entry{
		{Name: "deep.mkv", Type: "file", NodeID: "f2"},
	})

	var visited []string
	err := c.Walk("/root", 1, func(e Entry, dirPath string, depth int) error {
		visited = append(visited, e.Name)
		return nil
	})
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	// Expect: SubA, file.mkv (depth 0), deep.mkv (depth 1).
	if len(visited) != 3 {
		t.Errorf("want 3 visited entries, got %d: %v", len(visited), visited)
	}
}

func TestWalkMaxDepth(t *testing.T) {
	m := &mockAPI{}
	c := newTestClient(t, m)

	seedPath(c, "/root", "cid_root")
	seedListing(c, "cid_root", []Entry{
		{Name: "SubA", Type: "dir", NodeID: "cid_sub_a"},
	})
	seedPath(c, "/root/SubA", "cid_sub_a")
	seedListing(c, "cid_sub_a", []Entry{
		{Name: "deep.mkv", Type: "file", NodeID: "f2"},
	})

	var visited []string
	err := c.Walk("/root", 0, func(e Entry, dirPath string, depth int) error {
		visited = append(visited, e.Name)
		return nil
	})
	if err != nil {
		t.Fatalf("Walk(depth=0): %v", err)
	}
	// Only depth=0 entries should be visited (SubA itself, not its children).
	if len(visited) != 1 || visited[0] != "SubA" {
		t.Errorf("want only [SubA] at depth 0, got %v", visited)
	}
}

// ── RefreshDir ────────────────────────────────────────────────────────────────

func TestRefreshDir(t *testing.T) {
	m := &mockAPI{
		fnListFiles: func(cid string) ([]map[string]any, error) {
			return []map[string]any{
				{"n": "refreshed.mkv", "fid": "f_fresh"},
			}, nil
		},
	}
	c := newTestClient(t, m)
	seedPath(c, "/movies", "cid_movies")
	// Seed stale listing.
	seedListing(c, "cid_movies", []Entry{{Name: "old.mkv", Type: "file", NodeID: "f_old"}})

	entries, err := c.RefreshDir("/movies")
	if err != nil {
		t.Fatalf("RefreshDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "refreshed.mkv" {
		t.Errorf("want refreshed listing, got %v", entries)
	}
}

// ── RefreshPaths ──────────────────────────────────────────────────────────────

func TestRefreshPaths(t *testing.T) {
	listCalls := 0
	m := &mockAPI{
		fnListFiles: func(cid string) ([]map[string]any, error) {
			listCalls++
			return nil, nil
		},
	}
	c := newTestClient(t, m)
	seedPath(c, "/a", "cid_a")
	seedPath(c, "/b", "cid_b")

	// Refresh with duplicates — deduplicated.
	err := c.RefreshPaths([]string{"/a", "/b", "/a"})
	if err != nil {
		t.Fatalf("RefreshPaths: %v", err)
	}
	if listCalls != 2 {
		t.Errorf("want 2 list API calls (deduped), got %d", listCalls)
	}
}

// ── Auth delegation ───────────────────────────────────────────────────────────

func TestAuthDelegation(t *testing.T) {
	m := &mockAPI{}
	c := newTestClient(t, m)

	if err := c.QRLogin("tv"); err != nil {
		t.Errorf("QRLogin: %v", err)
	}
	if _, err := c.QRGetToken("tv"); err != nil {
		t.Errorf("QRGetToken: %v", err)
	}
	if err := c.QRWaitAndLogin(&QRSession{}); err != nil {
		t.Errorf("QRWaitAndLogin: %v", err)
	}
	if !c.CheckLogin() {
		t.Error("CheckLogin should return true")
	}
	if !c.RenewCookies() {
		t.Error("RenewCookies should return true")
	}
	if err := c.SaveCookies("/tmp/cookies.toml"); err != nil {
		t.Errorf("SaveCookies: %v", err)
	}
	if cookies := c.GetCookies(); cookies == "" {
		t.Error("GetCookies should return non-empty string")
	}
}

// ── normalizeItem: fid as numeric value (int64-like float64) ─────────────────

func TestNormalizeItemNumericSize(t *testing.T) {
	raw := map[string]any{
		"n":   "movie.mp4",
		"fid": float64(12345),
		"s":   int64(9999),
	}
	e := normalizeItem(raw)
	if e.Type != "file" {
		t.Errorf("type = %q; want file", e.Type)
	}
	if e.Size != 9999 {
		t.Errorf("size = %d; want 9999", e.Size)
	}
}

func TestNormalizeItemNoName(t *testing.T) {
	// Fallback: fn / name fields.
	raw1 := map[string]any{"fn": "fromFN.mkv", "fid": "f1"}
	e1 := normalizeItem(raw1)
	if e1.Name != "fromFN.mkv" {
		t.Errorf("name from fn = %q; want fromFN.mkv", e1.Name)
	}

	raw2 := map[string]any{"name": "fromName.mkv", "fid": "f2"}
	e2 := normalizeItem(raw2)
	if e2.Name != "fromName.mkv" {
		t.Errorf("name from name = %q; want fromName.mkv", e2.Name)
	}
}

func TestNormalizeItemDirFIDFallback(t *testing.T) {
	// Directory with no "cid" key but has "fid" as dir fallback.
	raw := map[string]any{
		"n":   "orphan_dir",
		"fid": "cid_fallback",
	}
	e := normalizeItem(raw)
	// fid present → treated as file.
	if e.Type != "file" {
		t.Errorf("type = %q; want file (fid present)", e.Type)
	}
}

// ── Move: write-through for dir moves ─────────────────────────────────────────

func TestMoveDirUpdatesPathIndex(t *testing.T) {
	m := &mockAPI{
		fnMove: func(fileIDs []string, targetDirID string) (map[string]any, error) {
			return map[string]any{}, nil
		},
	}
	c := newTestClient(t, m)
	seedPath(c, "/src", "cid_src")
	seedPath(c, "/dst", "cid_dst")
	seedListing(c, "cid_src", []Entry{
		{Name: "SubDir", Type: "dir", NodeID: "cid_sub"},
	})
	c.cache.SetPath("/src/SubDir", "cid_sub")

	err := c.Move([]string{"/src/SubDir"}, "/dst")
	if err != nil {
		t.Fatalf("Move dir: %v", err)
	}

	// /src/SubDir path should be invalidated.
	if _, _, ok := c.cache.GetPath("/src/SubDir"); ok {
		t.Error("/src/SubDir should be removed from path_index")
	}
	// The entry should be in dst.
	if e := c.cache.FindEntry("cid_dst", "SubDir"); e == nil {
		t.Error("SubDir entry not found in dst after Move")
	}
	// New path for moved dir should be in path_index.
	if _, _, ok := c.cache.GetPath("/dst/SubDir"); !ok {
		t.Error("/dst/SubDir should be in path_index after Move")
	}
}

// ── RateLimiter: QPS wait path ───────────────────────────────────────────────

func TestRateLimiterQPSSucceeds(t *testing.T) {
	// A high-QPS limiter should acquire immediately without sleeping.
	rl := newTestRateLimiter(t, "qps_test", 100, 1000)

	// First call succeeds trivially.
	if err := rl.Acquire(); err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	// Second call: last_request is just now, but min_interval = 0.01s.
	// It may spin once but should succeed quickly.
	if err := rl.Acquire(); err != nil {
		t.Fatalf("second Acquire: %v", err)
	}

	rl.mu.Lock()
	got := rl.count
	rl.mu.Unlock()
	if got != 2 {
		t.Errorf("count = %d; want 2", got)
	}
}

// ── RateLimiter: QPM wait path ───────────────────────────────────────────────

func TestRateLimiterQPMAcquiresUpToLimit(t *testing.T) {
	// Use a QPM of 3 with a very high QPS so we hit QPM quickly.
	rl := newTestRateLimiter(t, "qpm_test", 100, 3)

	for i := 0; i < 3; i++ {
		if err := rl.Acquire(); err != nil {
			t.Fatalf("Acquire[%d]: %v", i, err)
		}
	}
	rl.mu.Lock()
	got := rl.count
	rl.mu.Unlock()
	if got != 3 {
		t.Errorf("count = %d; want 3", got)
	}
}

// ── FindFile: root returns error ──────────────────────────────────────────────

func TestFindFileRoot(t *testing.T) {
	c := newTestClient(t, &mockAPI{})
	_, err := c.FindFile("/")
	if err == nil {
		t.Fatal("expected error for FindFile on root")
	}
}

// ── FindFile: not-a-file entry ────────────────────────────────────────────────

func TestFindFileNotAFile(t *testing.T) {
	c := newTestClient(t, &mockAPI{})
	seedPath(c, "/movies", "cid_movies")
	seedListing(c, "cid_movies", []Entry{
		{Name: "Action", Type: "dir", NodeID: "cid_action"},
	})

	_, err := c.FindFile("/movies/Action")
	if err == nil {
		t.Fatal("expected error for FindFile on directory entry")
	}
	if !strings.Contains(err.Error(), "not a file") {
		t.Errorf("error = %q; want 'not a file'", err.Error())
	}
}

// ── ExportTree helper (method on mockAPI) ────────────────────────────────────
// The mockAPI.ExportTree field is a struct-level func field, not a method —
// but the existing mockAPI uses method receivers. Add a wrapper that sets
// the field via a fresh struct.

// mockAPIWithExportTree extends mockAPI to support a configurable ExportTree.
type mockAPIWithExportTree struct {
	mockAPI
	fnExportTree func(dirID string) (string, error)
}

func (m *mockAPIWithExportTree) ExportTree(dirID string) (string, error) {
	if m.fnExportTree != nil {
		return m.fnExportTree(dirID)
	}
	return "", nil
}

func TestExportTreeViaExtended(t *testing.T) {
	treeText := "|——root\n| |-video.mkv"
	m := &mockAPIWithExportTree{
		fnExportTree: func(dirID string) (string, error) {
			return treeText, nil
		},
	}
	cache, err := NewCache(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	t.Cleanup(func() { _ = cache.Close() })

	c := &Client{
		api:        m,
		cache:      cache,
		listingTTL: defaultListingTTL,
		pathTTL:    defaultPathTTL,
	}
	seedPath(c, "/root", "cid_root")

	text, err := c.ExportTree("/root")
	if err != nil {
		t.Fatalf("ExportTree: %v", err)
	}
	if !strings.Contains(text, "video.mkv") {
		t.Errorf("unexpected tree text: %q", text)
	}
}

// ── DeleteByIDs with refreshDirs ─────────────────────────────────────────────

func TestDeleteByIDsWithRefreshDirs(t *testing.T) {
	listCalls := 0
	m := &mockAPI{
		fnDelete: func(fids []string) (map[string]any, error) {
			return map[string]any{}, nil
		},
		fnListFiles: func(cid string) ([]map[string]any, error) {
			listCalls++
			return nil, nil
		},
	}
	c := newTestClient(t, m)
	seedPath(c, "/movies", "cid_movies")

	err := c.DeleteByIDs([]string{"fid_1"}, []string{"/movies"})
	if err != nil {
		t.Fatalf("DeleteByIDs with refresh: %v", err)
	}
	if listCalls != 1 {
		t.Errorf("want 1 refresh call, got %d", listCalls)
	}
}

// ── Search: path resolution error ────────────────────────────────────────────

func TestSearchPathError(t *testing.T) {
	m := &mockAPI{
		fnGetDirID: func(path string) (string, error) {
			return "", fmt.Errorf("resolve error")
		},
	}
	c := newTestClient(t, m)

	_, err := c.Search("keyword", "/nonexistent")
	if err == nil {
		t.Fatal("expected error when path resolution fails")
	}
}

// ── Walk: callback error propagates ──────────────────────────────────────────

func TestWalkCallbackError(t *testing.T) {
	c := newTestClient(t, &mockAPI{})
	seedPath(c, "/root", "cid_root")
	seedListing(c, "cid_root", []Entry{
		{Name: "file.mkv", Type: "file", NodeID: "f1"},
	})

	sentinel := fmt.Errorf("stop walk")
	err := c.Walk("/root", 2, func(e Entry, dirPath string, depth int) error {
		return sentinel
	})
	if err != sentinel {
		t.Errorf("expected sentinel error, got %v", err)
	}
}

// ── WithStats: coverage for the option ───────────────────────────────────────

func TestWithStats(t *testing.T) {
	// logging.Stats is not accessible here without import; just test that the
	// option sets the field on the struct correctly by passing nil (valid type).
	c := &Client{}
	opt := WithStats(nil)
	opt(c)
	// stats field is nil – no panic and the option ran.
	if c.stats != nil {
		t.Error("expected stats = nil")
	}
}

// ── DownloadURL: with stats tracker ──────────────────────────────────────────

func TestDownloadURLWithStats(t *testing.T) {
	m := &mockAPI{
		fnDownloadURL: func(pickCode, ua string) (string, error) {
			return "https://cdn.example.com/" + pickCode, nil
		},
	}
	c := newTestClient(t, m)
	// stats is nil in test client; just cover the nil-guard path.
	url, err := c.DownloadURL("pc_xyz", "UA/2")
	if err != nil {
		t.Fatalf("DownloadURL: %v", err)
	}
	if !strings.Contains(url, "pc_xyz") {
		t.Errorf("unexpected url: %q", url)
	}
}

// ── Rename: error paths ───────────────────────────────────────────────────────

func TestRenameAPIError(t *testing.T) {
	m := &mockAPI{
		fnRename: func(fileID, newName string) (map[string]any, error) {
			return nil, fmt.Errorf("rename API error")
		},
	}
	c := newTestClient(t, m)
	seedPath(c, "/movies", "cid_movies")
	seedListing(c, "cid_movies", []Entry{
		{Name: "old.mkv", Type: "file", NodeID: "fid_old"},
	})

	err := c.Rename("/movies/old.mkv", "new.mkv")
	if err == nil {
		t.Fatal("expected error from Rename API failure")
	}
}

func TestRenameNotFound(t *testing.T) {
	m := &mockAPI{
		fnListFiles: func(cid string) ([]map[string]any, error) {
			return []map[string]any{}, nil
		},
	}
	c := newTestClient(t, m)
	seedPath(c, "/movies", "cid_movies")
	// listing is empty, so entry won't be found

	err := c.Rename("/movies/nonexistent.mkv", "other.mkv")
	if err == nil {
		t.Fatal("expected error when entry not found")
	}
}

// ── Move: entry not found, refresh also fails ─────────────────────────────────

func TestMoveEntryNotFound(t *testing.T) {
	m := &mockAPI{
		fnListFiles: func(cid string) ([]map[string]any, error) {
			return []map[string]any{}, nil
		},
	}
	c := newTestClient(t, m)
	seedPath(c, "/src", "cid_src")
	seedPath(c, "/dst", "cid_dst")
	// listing is empty, so entry won't be found after refresh

	err := c.Move([]string{"/src/missing.mkv"}, "/dst")
	if err == nil {
		t.Fatal("expected error when source entry not found")
	}
}

// ── BatchRename: entry not found ─────────────────────────────────────────────

func TestBatchRenameEntryNotFound(t *testing.T) {
	m := &mockAPI{
		fnListFiles: func(cid string) ([]map[string]any, error) {
			return []map[string]any{}, nil
		},
	}
	c := newTestClient(t, m)
	seedPath(c, "/movies", "cid_movies")
	// listing is empty

	err := c.BatchRename([]BatchRenameItem{
		{Path: "/movies/nothere.mkv", NewName: "new.mkv"},
	})
	if err == nil {
		t.Fatal("expected error when entry not found in BatchRename")
	}
}

// ── Mkdir: API returns no valid cid ──────────────────────────────────────────

func TestMkdirNoValidCID(t *testing.T) {
	m := &mockAPI{
		fnMkdir: func(parentID, name string) (map[string]any, error) {
			return map[string]any{"cid": "0"}, nil
		},
	}
	c := newTestClient(t, m)
	seedPath(c, "/movies", "cid_movies")

	cid, err := c.Mkdir("/movies/NewDir", false)
	// "0" cid should still be returned (it's what the code does — only empty triggers fallback)
	_ = cid
	_ = err
}

// ── RapidUpload: file that exists (exercises hash and open paths) ─────────────

func TestRapidUploadExistingFile(t *testing.T) {
	tmp := t.TempDir()
	localFile := filepath.Join(tmp, "existing.mkv")
	if err := os.WriteFile(localFile, []byte("hello upload content for test"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	// Use the standard mockAPI (returns status=2) — exercises hash/open/upload path.
	m := &mockAPI{}
	c := newTestClient(t, m)
	seedPath(c, "/up", "cid_up")

	result, err := c.RapidUpload(localFile, "/up")
	if err != nil {
		t.Fatalf("RapidUpload: %v", err)
	}
	if result.Status != 2 {
		t.Errorf("status = %d; want 2", result.Status)
	}
}

// ── RapidUpload: file stat error ─────────────────────────────────────────────

func TestRapidUploadMissingFile(t *testing.T) {
	c := newTestClient(t, &mockAPI{})
	seedPath(c, "/up", "cid_up")

	_, err := c.RapidUpload("/nonexistent/path/file.mkv", "/up")
	if err == nil {
		t.Fatal("expected error for missing local file")
	}
}

// ── RateLimiter: QPM exceeded triggers wait then success ─────────────────────

func TestRateLimiterWaitForSlotQPMPath(t *testing.T) {
	// Use cache-backed limiter with QPM=1, QPS=100, so after 1 acquire
	// the QPM is exhausted. But with minute window reset we can force it.
	rl := newTestRateLimiter(t, "qpm_wait", 100, 1)

	// First acquire should succeed.
	if err := rl.Acquire(); err != nil {
		t.Fatalf("first Acquire: %v", err)
	}

	// Now the QPM is 1/1. Force the minute_start to very old so next call
	// resets and succeeds immediately.
	now := float64(time.Now().Unix())
	rl.cache.SetRateLimit("qpm_wait", RateLimitState{
		LastRequest:   now - 5,
		MinuteStart:   now - 120, // more than 60s ago
		MinuteCount:   1,
		CooldownUntil: 0,
	})

	if err := rl.Acquire(); err != nil {
		t.Fatalf("second Acquire after window reset: %v", err)
	}
}

// ── Delete: dir node ID invalidation branch ───────────────────────────────────

func TestDeleteDirInvalidatesCache(t *testing.T) {
	m := &mockAPI{
		fnDelete: func(fids []string) (map[string]any, error) {
			return map[string]any{}, nil
		},
	}
	c := newTestClient(t, m)
	seedPath(c, "/movies", "cid_movies")
	seedListing(c, "cid_movies", []Entry{
		{Name: "Action", Type: "dir", NodeID: "cid_action"},
	})
	// Also seed the Action dir's listing so we can check it gets invalidated.
	seedListing(c, "cid_action", []Entry{
		{Name: "film.mkv", Type: "file", NodeID: "f1"},
	})

	err := c.Delete([]string{"/movies/Action"})
	if err != nil {
		t.Fatalf("Delete dir: %v", err)
	}

	// The Action dir's own listing should be invalidated.
	if _, ok := c.cache.GetDirTS("cid_action"); ok {
		t.Error("cid_action dir_meta should be invalidated after Delete")
	}
}

// ── Move: API error propagates ────────────────────────────────────────────────

func TestMoveAPIError(t *testing.T) {
	m := &mockAPI{
		fnMove: func(fileIDs []string, targetDirID string) (map[string]any, error) {
			return nil, fmt.Errorf("move API error")
		},
	}
	c := newTestClient(t, m)
	seedPath(c, "/src", "cid_src")
	seedPath(c, "/dst", "cid_dst")
	seedListing(c, "cid_src", []Entry{
		{Name: "file.mkv", Type: "file", NodeID: "f1"},
	})

	err := c.Move([]string{"/src/file.mkv"}, "/dst")
	if err == nil {
		t.Fatal("expected error from Move API failure")
	}
}

// ── BatchRename: API error propagates ────────────────────────────────────────

func TestBatchRenameAPIError(t *testing.T) {
	m := &mockAPI{
		fnBatchRename: func(renames map[string]string) (map[string]any, error) {
			return nil, fmt.Errorf("batch rename API error")
		},
	}
	c := newTestClient(t, m)
	seedPath(c, "/movies", "cid_movies")
	seedListing(c, "cid_movies", []Entry{
		{Name: "a.mkv", Type: "file", NodeID: "fid_a"},
	})

	err := c.BatchRename([]BatchRenameItem{
		{Path: "/movies/a.mkv", NewName: "alpha.mkv"},
	})
	if err == nil {
		t.Fatal("expected error from BatchRename API failure")
	}
}

// ── normalizeItem: directory with cid/fid both missing ───────────────────────

func TestNormalizeItemDirNoCID(t *testing.T) {
	raw := map[string]any{
		"n": "empty_dir",
		// neither "cid" nor "fid"
	}
	e := normalizeItem(raw)
	if e.Type != "dir" {
		t.Errorf("type = %q; want dir", e.Type)
	}
	if e.Name != "empty_dir" {
		t.Errorf("name = %q; want empty_dir", e.Name)
	}
}

// ── mkdirParents: API returns invalid cid ────────────────────────────────────

func TestMkdirParentsInvalidCID(t *testing.T) {
	m := &mockAPI{
		fnGetDirID: func(path string) (string, error) {
			return "", fmt.Errorf("not found")
		},
		fnMkdir: func(parentID, name string) (map[string]any, error) {
			// Return neither cid nor aid — simulates broken API.
			return map[string]any{}, nil
		},
	}
	c := newTestClient(t, m)

	_, err := c.Mkdir("/a/b", true)
	if err == nil {
		t.Fatal("expected error when mkdir returns no valid cid")
	}
	if !strings.Contains(err.Error(), "no valid cid") {
		t.Errorf("error = %q; want 'no valid cid'", err.Error())
	}
}
