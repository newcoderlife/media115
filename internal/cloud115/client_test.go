package cloud115

import (
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
)

// ─── mock API ────────────────────────────────────────────────────────────────

// mockAPI implements apiCaller for tests. Each exported fn field is an
// optional callback; the zero value is a safe default (returns empty results).
type mockAPI struct {
	fnGetDirID    func(path string) (string, error)
	fnListFiles   func(cid string) ([]map[string]any, error)
	fnMkdir       func(parentID, name string) (map[string]any, error)
	fnRename      func(fileID, newName string) (map[string]any, error)
	fnDelete      func(fileIDs []string) (map[string]any, error)
	fnMove        func(fileIDs []string, targetDirID string) (map[string]any, error)
	fnBatchRename func(renames map[string]string) (map[string]any, error)
	fnUploadFile  func(localPath, targetDirID, filename string) (map[string]any, error)
	fnDownloadURL func(pickCode, userAgent string) (string, error)
	fnSearch      func(keyword, dirID string) ([]map[string]any, error)

	getDirIDCalls  int
	listFilesCalls int
	mkdirCalls     int
	renameCalls    int
	deleteCalls    int
	moveCalls      int
	downloadCalls  int
}

func (m *mockAPI) GetDirID(path string) (string, error) {
	m.getDirIDCalls++
	if m.fnGetDirID != nil {
		return m.fnGetDirID(path)
	}
	return "", nil
}

func (m *mockAPI) ListFilesAll(cid string) ([]map[string]any, error) {
	m.listFilesCalls++
	if m.fnListFiles != nil {
		return m.fnListFiles(cid)
	}
	return nil, nil
}

func (m *mockAPI) Mkdir(parentID, name string) (map[string]any, error) {
	m.mkdirCalls++
	if m.fnMkdir != nil {
		return m.fnMkdir(parentID, name)
	}
	return map[string]any{"cid": "new_cid"}, nil
}

func (m *mockAPI) Rename(fileID, newName string) (map[string]any, error) {
	m.renameCalls++
	if m.fnRename != nil {
		return m.fnRename(fileID, newName)
	}
	return map[string]any{}, nil
}

func (m *mockAPI) Delete(fileIDs []string) (map[string]any, error) {
	m.deleteCalls++
	if m.fnDelete != nil {
		return m.fnDelete(fileIDs)
	}
	return map[string]any{}, nil
}

func (m *mockAPI) Move(fileIDs []string, targetDirID string) (map[string]any, error) {
	m.moveCalls++
	if m.fnMove != nil {
		return m.fnMove(fileIDs, targetDirID)
	}
	return map[string]any{}, nil
}

func (m *mockAPI) BatchRename(renames map[string]string) (map[string]any, error) {
	if m.fnBatchRename != nil {
		return m.fnBatchRename(renames)
	}
	return map[string]any{}, nil
}

func (m *mockAPI) UploadFile(localPath, targetDirID, filename string) (map[string]any, error) {
	if m.fnUploadFile != nil {
		return m.fnUploadFile(localPath, targetDirID, filename)
	}
	return nil, nil
}

func (m *mockAPI) DownloadURL(pickCode, userAgent string) (string, error) {
	m.downloadCalls++
	if m.fnDownloadURL != nil {
		return m.fnDownloadURL(pickCode, userAgent)
	}
	return "https://cdn.example.com/file", nil
}

func (m *mockAPI) Search(keyword, dirID string) ([]map[string]any, error) {
	if m.fnSearch != nil {
		return m.fnSearch(keyword, dirID)
	}
	return nil, nil
}

func (m *mockAPI) ExportTree(dirID string) (string, error) { return "", nil }

func (m *mockAPI) RapidUpload(dirID, filename string, fileSize int64, fileSHA1 string, fileStream io.ReadSeeker) (map[string]any, error) {
	return map[string]any{"status": float64(2)}, nil
}

func (m *mockAPI) QRLogin(app string) error        { return nil }
func (m *mockAPI) CheckLogin() bool                { return true }
func (m *mockAPI) RenewCookies(app string) bool    { return true }
func (m *mockAPI) SaveCookies(path string) error   { return nil }
func (m *mockAPI) GetCookies() string              { return "mock=cookie" }

// ─── helpers ─────────────────────────────────────────────────────────────────

// newTestClient builds a Client with a real (temp) cache and the given mock.
func newTestClient(t *testing.T, m *mockAPI) *Client {
	t.Helper()
	cache, err := NewCache(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	t.Cleanup(func() { _ = cache.Close() })

	return &Client{
		api:        m,
		cache:      cache,
		listingTTL: defaultListingTTL,
		pathTTL:    defaultPathTTL,
		logger:     slog.Default(),
	}
}

const (
	defaultListingTTL = 3600e9 // 1h in nanoseconds (time.Duration)
	defaultPathTTL    = 86400e9
)

func seedPath(c *Client, path, cid string) {
	c.cache.SetPath(path, cid)
}

func seedListing(c *Client, parentCID string, entries []Entry) {
	c.cache.SetDirListing(parentCID, entries)
}

// ─── READ TESTS ──────────────────────────────────────────────────────────────

func TestResolvePathRoot(t *testing.T) {
	c := newTestClient(t, &mockAPI{})
	cid, err := c.ResolvePath("/")
	if err != nil {
		t.Fatalf("ResolvePath(/): %v", err)
	}
	if cid != "0" {
		t.Errorf("cid = %q; want 0", cid)
	}
}

func TestResolvePathCacheHit(t *testing.T) {
	m := &mockAPI{}
	c := newTestClient(t, m)
	seedPath(c, "/movies", "cid_movies")

	cid, err := c.ResolvePath("/movies")
	if err != nil {
		t.Fatalf("ResolvePath: %v", err)
	}
	if cid != "cid_movies" {
		t.Errorf("cid = %q; want cid_movies", cid)
	}
	if m.getDirIDCalls != 0 {
		t.Errorf("API should not have been called, got %d calls", m.getDirIDCalls)
	}
}

func TestResolvePathCacheMiss(t *testing.T) {
	m := &mockAPI{
		fnGetDirID: func(path string) (string, error) {
			if path == "/movies" {
				return "cid_from_api", nil
			}
			return "", nil
		},
	}
	c := newTestClient(t, m)

	cid, err := c.ResolvePath("/movies")
	if err != nil {
		t.Fatalf("ResolvePath: %v", err)
	}
	if cid != "cid_from_api" {
		t.Errorf("cid = %q; want cid_from_api", cid)
	}
	if m.getDirIDCalls != 1 {
		t.Errorf("want 1 API call, got %d", m.getDirIDCalls)
	}

	// Second call must hit cache, no extra API call.
	_, _ = c.ResolvePath("/movies")
	if m.getDirIDCalls != 1 {
		t.Errorf("want 1 total API calls after cache hit, got %d", m.getDirIDCalls)
	}
}

func TestResolvePathNotFound(t *testing.T) {
	m := &mockAPI{
		fnGetDirID: func(path string) (string, error) { return "", nil },
	}
	c := newTestClient(t, m)

	_, err := c.ResolvePath("/does/not/exist")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestListDirCacheMiss(t *testing.T) {
	rawItems := []map[string]any{
		{"n": "file.mkv", "fid": "fid_1", "s": float64(100), "pc": "pc1"},
		{"n": "subdir", "cid": "cid_sub"},
	}
	m := &mockAPI{
		fnListFiles: func(cid string) ([]map[string]any, error) { return rawItems, nil },
	}
	c := newTestClient(t, m)
	// Root → "0", listing is fetched from API.
	entries, err := c.ListDir("/")
	if err != nil {
		t.Fatalf("ListDir: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("want 2 entries, got %d", len(entries))
	}
	if m.listFilesCalls != 1 {
		t.Errorf("want 1 API call, got %d", m.listFilesCalls)
	}
}

func TestListDirCacheHit(t *testing.T) {
	m := &mockAPI{}
	c := newTestClient(t, m)

	seedPath(c, "/movies", "cid_movies")
	seedListing(c, "cid_movies", []Entry{
		{Name: "Inception.mkv", Type: "file", NodeID: "f1", PickCode: "pc_i"},
	})

	entries, err := c.ListDir("/movies")
	if err != nil {
		t.Fatalf("ListDir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("want 1 entry, got %d", len(entries))
	}
	if entries[0].Name != "Inception.mkv" {
		t.Errorf("name = %q; want Inception.mkv", entries[0].Name)
	}
	if m.listFilesCalls != 0 {
		t.Errorf("API should not have been called, got %d calls", m.listFilesCalls)
	}
}

func TestFindFile(t *testing.T) {
	m := &mockAPI{}
	c := newTestClient(t, m)

	seedPath(c, "/movies", "cid_movies")
	seedListing(c, "cid_movies", []Entry{
		{Name: "Inception.mkv", Type: "file", NodeID: "fid_1", PickCode: "pc_inc", Size: 4_000_000_000},
	})

	e, err := c.FindFile("/movies/Inception.mkv")
	if err != nil {
		t.Fatalf("FindFile: %v", err)
	}
	if e.Name != "Inception.mkv" {
		t.Errorf("Name = %q; want Inception.mkv", e.Name)
	}
	if e.PickCode != "pc_inc" {
		t.Errorf("PickCode = %q; want pc_inc", e.PickCode)
	}
	if e.Size != 4_000_000_000 {
		t.Errorf("Size = %d; want 4000000000", e.Size)
	}
	if e.Type != "file" {
		t.Errorf("Type = %q; want file", e.Type)
	}
}

func TestFindFileNotFound(t *testing.T) {
	m := &mockAPI{
		fnListFiles: func(cid string) ([]map[string]any, error) {
			return []map[string]any{}, nil
		},
	}
	c := newTestClient(t, m)
	seedPath(c, "/movies", "cid_movies")

	_, err := c.FindFile("/movies/nonexistent.mkv")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestStreamURL(t *testing.T) {
	m := &mockAPI{
		fnDownloadURL: func(pickCode, ua string) (string, error) {
			return "https://cdn.example.com/" + pickCode, nil
		},
	}
	c := newTestClient(t, m)
	seedPath(c, "/movies", "cid_movies")
	seedListing(c, "cid_movies", []Entry{
		{Name: "Inception.mkv", Type: "file", NodeID: "fid_1", PickCode: "pc_inc"},
	})

	url, err := c.StreamURL("/movies/Inception.mkv", "")
	if err != nil {
		t.Fatalf("StreamURL: %v", err)
	}
	if !strings.Contains(url, "pc_inc") {
		t.Errorf("URL %q should contain pick_code pc_inc", url)
	}
	if m.downloadCalls != 1 {
		t.Errorf("want 1 download API call, got %d", m.downloadCalls)
	}
}

// ─── WRITE TESTS ─────────────────────────────────────────────────────────────

func TestMkdir(t *testing.T) {
	newCID := "cid_new_dir"
	m := &mockAPI{
		fnMkdir: func(parentID, name string) (map[string]any, error) {
			return map[string]any{"cid": newCID}, nil
		},
	}
	c := newTestClient(t, m)
	seedPath(c, "/movies", "cid_movies")

	cid, err := c.Mkdir("/movies/Action", false)
	if err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if cid != newCID {
		t.Errorf("cid = %q; want %s", cid, newCID)
	}

	// Write-through: path_index.
	cachedCID, _, ok := c.cache.GetPath("/movies/Action")
	if !ok {
		t.Fatal("path not cached after Mkdir")
	}
	if cachedCID != newCID {
		t.Errorf("cached cid = %q; want %s", cachedCID, newCID)
	}

	// Write-through: entry in parent listing.
	e := c.cache.FindEntry("cid_movies", "Action")
	if e == nil {
		t.Fatal("dir entry not added to parent listing")
	}
	if e.Type != "dir" {
		t.Errorf("entry type = %q; want dir", e.Type)
	}
}

func TestRename(t *testing.T) {
	var renamedFID, renamedTo string
	m := &mockAPI{
		fnRename: func(fileID, newName string) (map[string]any, error) {
			renamedFID = fileID
			renamedTo = newName
			return map[string]any{}, nil
		},
	}
	c := newTestClient(t, m)
	seedPath(c, "/movies", "cid_movies")
	seedListing(c, "cid_movies", []Entry{
		{Name: "old_name.mkv", Type: "file", NodeID: "fid_old", PickCode: "pc_old"},
	})

	err := c.Rename("/movies/old_name.mkv", "new_name.mkv")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if renamedFID != "fid_old" {
		t.Errorf("API called with fid %q; want fid_old", renamedFID)
	}
	if renamedTo != "new_name.mkv" {
		t.Errorf("API called with name %q; want new_name.mkv", renamedTo)
	}

	// Write-through: entry reflects new name.
	e := c.cache.FindEntry("cid_movies", "new_name.mkv")
	if e == nil {
		t.Fatal("renamed entry not found in cache")
	}
	if e.NodeID != "fid_old" {
		t.Errorf("node_id = %q; want fid_old", e.NodeID)
	}
}

func TestDelete(t *testing.T) {
	var deletedIDs []string
	m := &mockAPI{
		fnDelete: func(fileIDs []string) (map[string]any, error) {
			deletedIDs = fileIDs
			return map[string]any{}, nil
		},
	}
	c := newTestClient(t, m)
	seedPath(c, "/movies", "cid_movies")
	seedListing(c, "cid_movies", []Entry{
		{Name: "Inception.mkv", Type: "file", NodeID: "fid_1"},
	})

	err := c.Delete([]string{"/movies/Inception.mkv"})
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(deletedIDs) != 1 || deletedIDs[0] != "fid_1" {
		t.Errorf("deleted IDs = %v; want [fid_1]", deletedIDs)
	}

	// Write-through: entry gone from cache.
	e := c.cache.FindEntry("cid_movies", "Inception.mkv")
	if e != nil {
		t.Error("entry still in cache after Delete")
	}
}

func TestMove(t *testing.T) {
	var movedFIDs []string
	var movedDest string
	m := &mockAPI{
		fnMove: func(fileIDs []string, targetDirID string) (map[string]any, error) {
			movedFIDs = fileIDs
			movedDest = targetDirID
			return map[string]any{}, nil
		},
	}
	c := newTestClient(t, m)
	seedPath(c, "/movies", "cid_movies")
	seedPath(c, "/archive", "cid_archive")
	seedListing(c, "cid_movies", []Entry{
		{Name: "Inception.mkv", Type: "file", NodeID: "fid_1"},
	})

	err := c.Move([]string{"/movies/Inception.mkv"}, "/archive")
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if len(movedFIDs) != 1 || movedFIDs[0] != "fid_1" {
		t.Errorf("moved FIDs = %v; want [fid_1]", movedFIDs)
	}
	if movedDest != "cid_archive" {
		t.Errorf("dest = %q; want cid_archive", movedDest)
	}

	// Write-through: entry in dest parent.
	e := c.cache.FindEntry("cid_archive", "Inception.mkv")
	if e == nil {
		t.Fatal("entry not found in dest after Move")
	}
}

func TestUploadWithFID(t *testing.T) {
	m := &mockAPI{
		fnUploadFile: func(localPath, targetDirID, filename string) (map[string]any, error) {
			return map[string]any{
				"fid":       "fid_uploaded",
				"pick_code": "pc_uploaded",
				"file_size": float64(1024),
			}, nil
		},
	}
	c := newTestClient(t, m)
	seedPath(c, "/uploads", "cid_uploads")

	_, err := c.Upload("/tmp/test.mkv", "/uploads", "test.mkv")
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}

	// Write-through: file entry cached.
	e := c.cache.FindEntry("cid_uploads", "test.mkv")
	if e == nil {
		t.Fatal("entry not cached after Upload with fid")
	}
	if e.Type != "file" {
		t.Errorf("type = %q; want file", e.Type)
	}
	if e.NodeID != "fid_uploaded" {
		t.Errorf("node_id = %q; want fid_uploaded", e.NodeID)
	}
}

// ─── CACHE MANAGEMENT TESTS ──────────────────────────────────────────────────

func TestWarm(t *testing.T) {
	callCount := 0
	m := &mockAPI{
		fnGetDirID: func(path string) (string, error) {
			if path == "/root" {
				return "cid_root", nil
			}
			return "", fmt.Errorf("unexpected path: %s", path)
		},
		fnListFiles: func(cid string) ([]map[string]any, error) {
			callCount++
			switch cid {
			case "cid_root":
				return []map[string]any{
					{"n": "subA", "cid": "cid_subA"},
					{"n": "file.mkv", "fid": "fid_1"},
				}, nil
			case "cid_subA":
				return []map[string]any{
					{"n": "deep.mkv", "fid": "fid_2"},
				}, nil
			default:
				return nil, nil
			}
		},
	}
	c := newTestClient(t, m)

	var visited []string
	err := c.Warm("/root", 2, func(path string, count int) {
		visited = append(visited, path)
	})
	if err != nil {
		t.Fatalf("Warm: %v", err)
	}

	if callCount != 2 {
		t.Errorf("want 2 API list calls, got %d", callCount)
	}
	if len(visited) != 2 {
		t.Errorf("want 2 progress callbacks, got %d", len(visited))
	}

	// child dir must be in path_index.
	cid, _, ok := c.cache.GetPath("/root/subA")
	if !ok {
		t.Fatal("child dir path not cached during Warm")
	}
	if cid != "cid_subA" {
		t.Errorf("cached cid = %q; want cid_subA", cid)
	}
}

func TestInvalidate(t *testing.T) {
	m := &mockAPI{}
	c := newTestClient(t, m)

	seedPath(c, "/movies", "cid_movies")
	seedPath(c, "/movies/Action", "cid_action")
	seedListing(c, "cid_movies", []Entry{
		{Name: "Action", Type: "dir", NodeID: "cid_action"},
	})

	err := c.Invalidate("/movies")
	if err != nil {
		t.Fatalf("Invalidate: %v", err)
	}

	if _, _, ok := c.cache.GetPath("/movies"); ok {
		t.Error("/movies still in path_index after Invalidate")
	}
	if _, _, ok := c.cache.GetPath("/movies/Action"); ok {
		t.Error("/movies/Action still in path_index after Invalidate")
	}
	if _, ok := c.cache.GetDirTS("cid_movies"); ok {
		t.Error("dir_meta still present for cid_movies after Invalidate")
	}
	entries := c.cache.GetDirEntries("cid_movies")
	if len(entries) != 0 {
		t.Errorf("dir_entry still present after Invalidate: %v", entries)
	}
}

func TestCacheClear(t *testing.T) {
	m := &mockAPI{}
	c := newTestClient(t, m)

	seedPath(c, "/movies", "cid_movies")
	seedListing(c, "cid_movies", []Entry{
		{Name: "Inception.mkv", Type: "file", NodeID: "fid_1"},
	})

	err := c.CacheClear()
	if err != nil {
		t.Fatalf("CacheClear: %v", err)
	}

	s, _ := c.CacheStatus()
	if s.PathCount != 0 {
		t.Errorf("PathCount = %d; want 0 after CacheClear", s.PathCount)
	}
	if s.EntryCount != 0 {
		t.Errorf("EntryCount = %d; want 0 after CacheClear", s.EntryCount)
	}
}

// ─── TREE TESTS ──────────────────────────────────────────────────────────────

var sampleTree = `|——根目录
| |-影音
| | |-AV
| | | |-ABP-040
| | | | |-ABP-040.mkv
| | | | |-ABP-040.nfo
| | | |-ABP-041
| | | | |-ABP-041.mp4
| | |-ignore_this_dir
`

func TestParseTreeText(t *testing.T) {
	videoExts := map[string]struct{}{".mkv": {}, ".mp4": {}}
	entries := parseTreeText(sampleTree, videoExts, ".nfo")

	// Expect: ABP-040.mkv (video), ABP-040.nfo (nfo), ABP-041.mp4 (video)
	if len(entries) != 3 {
		t.Fatalf("want 3 entries, got %d: %v", len(entries), entries)
	}

	byName := make(map[string]TreeEntry, len(entries))
	for _, e := range entries {
		byName[e.Name] = e
	}

	if e, ok := byName["ABP-040.mkv"]; !ok {
		t.Error("ABP-040.mkv missing")
	} else if !e.IsVideo {
		t.Error("ABP-040.mkv should be IsVideo=true")
	}

	if e, ok := byName["ABP-040.nfo"]; !ok {
		t.Error("ABP-040.nfo missing")
	} else if !e.IsNFO {
		t.Error("ABP-040.nfo should be IsNFO=true")
	}

	if e, ok := byName["ABP-041.mp4"]; !ok {
		t.Error("ABP-041.mp4 missing")
	} else if !e.IsVideo {
		t.Error("ABP-041.mp4 should be IsVideo=true")
	}
}

func TestSaveTree(t *testing.T) {
	c := newTestClient(t, &mockAPI{})

	n, err := c.SaveTree(sampleTree, []string{".mkv", ".mp4"}, "/影音")
	if err != nil {
		t.Fatalf("SaveTree: %v", err)
	}
	if n != 3 {
		t.Errorf("saved %d entries; want 3", n)
	}

	stats, err := c.TreeStats()
	if err != nil {
		t.Fatalf("TreeStats: %v", err)
	}
	if stats.Total != 3 {
		t.Errorf("Total = %d; want 3", stats.Total)
	}
	if stats.Videos != 2 {
		t.Errorf("Videos = %d; want 2", stats.Videos)
	}
	if stats.NFOs != 1 {
		t.Errorf("NFOs = %d; want 1", stats.NFOs)
	}
}

func TestTreeEntries(t *testing.T) {
	c := newTestClient(t, &mockAPI{})
	_, _ = c.SaveTree(sampleTree, []string{".mkv", ".mp4"}, "")

	all, err := c.TreeEntries("")
	if err != nil {
		t.Fatalf("TreeEntries: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("want 3 total entries, got %d", len(all))
	}

	// Filter by category prefix — "影音/AV/ABP-040" should match exactly 2 entries.
	filtered, err := c.TreeEntries("影音/AV/ABP-040")
	if err != nil {
		t.Fatalf("TreeEntries filtered: %v", err)
	}
	if len(filtered) != 2 {
		t.Errorf("want 2 filtered entries, got %d", len(filtered))
	}
}

// ─── NORMALIZE TESTS ─────────────────────────────────────────────────────────

func TestNormalize(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "/"},
		{"/", "/"},
		{"movies", "/movies"},
		{"/movies/", "/movies"},
		{"/a/b/c/", "/a/b/c"},
		{"  /foo  ", "/foo"},
	}
	for _, tc := range cases {
		got := normalize(tc.in)
		if got != tc.want {
			t.Errorf("normalize(%q) = %q; want %q", tc.in, got, tc.want)
		}
	}
}

// ─── NORMALIZE ITEM TESTS ────────────────────────────────────────────────────

func TestNormalizeItemFile(t *testing.T) {
	raw := map[string]any{
		"n":   "video.mkv",
		"fid": "fid_42",
		"s":   float64(1234567),
		"pc":  "pc_abc",
	}
	e := normalizeItem(raw)
	if e.Type != "file" {
		t.Errorf("type = %q; want file", e.Type)
	}
	if e.Name != "video.mkv" {
		t.Errorf("name = %q; want video.mkv", e.Name)
	}
	if e.NodeID != "fid_42" {
		t.Errorf("node_id = %q; want fid_42", e.NodeID)
	}
	if e.Size != 1234567 {
		t.Errorf("size = %d; want 1234567", e.Size)
	}
	if e.PickCode != "pc_abc" {
		t.Errorf("pick_code = %q; want pc_abc", e.PickCode)
	}
}

func TestNormalizeItemDir(t *testing.T) {
	raw := map[string]any{
		"n":   "subdir",
		"cid": "cid_99",
	}
	e := normalizeItem(raw)
	if e.Type != "dir" {
		t.Errorf("type = %q; want dir", e.Type)
	}
	if e.NodeID != "cid_99" {
		t.Errorf("node_id = %q; want cid_99", e.NodeID)
	}
}
