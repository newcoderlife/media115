package organizer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/newcoderlife/media115/internal/cloud115"
)

// ── sanitize ──────────────────────────────────────────────────────────────────

func TestSanitize(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// Characters that must be stripped.
		{`Hello: World`, `Hello World`},
		{`foo<bar>baz`, `foobarbaz`},
		{`a"b`, `ab`},
		{`a/b\c`, `abc`},
		{`a|b?c*d`, `abcd`},
		// Leading/trailing spaces trimmed.
		{"  title  ", "title"},
		// Normal name unchanged.
		{"Breaking Bad (2008)", "Breaking Bad (2008)"},
		// Empty string.
		{"", ""},
	}
	for _, c := range cases {
		got := sanitize(c.in)
		if got != c.want {
			t.Errorf("sanitize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ── stem ──────────────────────────────────────────────────────────────────────

func TestStem(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"file.mkv", "file"},
		{"file.tar.gz", "file.tar"},
		{"noext", "noext"},
		{"", ""},
		{".hidden", ""}, // filepath.Ext(".hidden") == ".hidden" → stem is empty
		{"file.nfo", "file"},
		{"Breaking Bad S01E01.mkv", "Breaking Bad S01E01"},
	}
	for _, c := range cases {
		got := stem(c.in)
		if got != c.want {
			t.Errorf("stem(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ── toInt ─────────────────────────────────────────────────────────────────────

func TestToInt(t *testing.T) {
	cases := []struct {
		in   any
		want int
	}{
		// Date strings: truncate to year.
		{"2023-01-15", 2023},
		{"2021/06/30", 2021},
		// Plain year string.
		{"2008", 2008},
		// Float64 (JSON numbers).
		{float64(2010), 2010},
		{float64(0), 0},
		// Nil / unknown type.
		{nil, 0},
		// Integer directly.
		{int(1999), 1999},
		// String that cannot be parsed as int.
		{"abc", 0},
		// Four-char string that is not a date.
		{"abcd", 0},
	}
	for _, c := range cases {
		got := toInt(c.in)
		if got != c.want {
			t.Errorf("toInt(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

// ── ExtractAVSuffix additional patterns ───────────────────────────────────────

func TestExtractAVSuffixAdditional(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		// -C followed by a letter is NOT a cut marker.
		{"-CA.FHD", ""},
		// Lowercase disc letters.
		{"a.FHD", ".A"},
		{"b_4K", ".B"},
		{".c", ".C"},
		{"_d", ".D"},
		// CD patterns with separators.
		{".CD2", ".CD2"},
		{"_CD1.FHD", ".CD1"},
		// Part patterns with separators.
		{"_Part1.FHD", ".Part1"},
		{"-Part2", ".Part2"},
		// E is not a disc letter (only A-D).
		{"E.FHD", ""},
		// Plain digit is not a disc letter.
		{"1.FHD", ""},
	}
	for _, c := range cases {
		got := ExtractAVSuffix(c.in)
		if got != c.want {
			t.Errorf("ExtractAVSuffix(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// ── fileMapCachePut / fileMapCacheGet round-trip ───────────────────────────────

func TestFileMapCacheRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	// Override XDG_CACHE_HOME so config.CacheDir() points into our temp dir.
	// config.CacheDir() appends "cloud115" to the XDG_CACHE_HOME base.
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	data := map[string]any{
		"type":    "movie",
		"title":   "Test Film",
		"year":    float64(2024),
		"tmdb_id": float64(99999),
	}

	fileMapCachePut("file_map", "Test.Film.2024", data)

	got := fileMapCacheGet("file_map", "Test.Film.2024")
	if got == nil {
		t.Fatal("fileMapCacheGet returned nil after Put")
	}
	if got["title"] != "Test Film" {
		t.Errorf("title = %v, want 'Test Film'", got["title"])
	}
	if got["year"] != float64(2024) {
		t.Errorf("year = %v, want 2024", got["year"])
	}
}

func TestFileMapCacheGetMissing(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	// Key that was never written.
	got := fileMapCacheGet("file_map", "nonexistent_key_xyz")
	if got != nil {
		t.Errorf("expected nil for missing key, got %v", got)
	}
}

func TestFileMapCacheGetNotFound(t *testing.T) {
	// A cached entry with _not_found=true should be treated as absent.
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	// Write a not_found entry manually.
	notFoundData := map[string]any{"_not_found": true}
	fileMapCachePut("file_map", "missing_title", notFoundData)

	got := fileMapCacheGet("file_map", "missing_title")
	if got != nil {
		t.Errorf("expected nil for _not_found entry, got %v", got)
	}
}

func TestFileMapCacheGetInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	// Manually write a corrupted JSON file.
	cacheDir := filepath.Join(tmpDir, "media115", "scrape", "file_map")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cacheDir, "bad_json.json")
	if err := os.WriteFile(path, []byte("NOT JSON {{{"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := fileMapCacheGet("file_map", "bad_json")
	if got != nil {
		t.Errorf("expected nil for bad JSON, got %v", got)
	}
}

// ── BuildPlan additional paths ─────────────────────────────────────────────────

func TestBuildPlanNonVideoFiltered(t *testing.T) {
	// Non-video entries must be ignored.
	entries := []cloud115.TreeEntry{
		{Path: "影音/电影/foo/foo.nfo", Name: "foo.nfo", Parent: "影音/电影/foo", IsVideo: false, IsNFO: true},
		{Path: "影音/电影/foo/foo.mkv", Name: "foo.mkv", Parent: "影音/电影/foo", IsVideo: true},
	}
	cache := fakeCache(map[string]map[string]any{
		"foo": {
			"type":  "movie",
			"title": "Foo",
			"year":  float64(2020),
		},
	})
	ops := BuildPlan("电影", entries, cache)
	// Only the video entry should produce an op.
	if len(ops) != 1 {
		t.Fatalf("expected 1 op (non-video skipped), got %d", len(ops))
	}
}

func TestBuildPlanCategoryFilter(t *testing.T) {
	// Entries NOT in the requested category path must not produce ops.
	entries := []cloud115.TreeEntry{
		makeEntry("影音/AV/ABP-040/ABP-040.mp4", "ABP-040.mp4", "影音/AV/ABP-040"),
	}
	cache := fakeCache(map[string]map[string]any{
		"ABP-040": {"type": "av", "number": "ABP-040"},
	})
	// Ask for 电影 — AV entry should be filtered out.
	ops := BuildPlan("电影", entries, cache)
	if len(ops) != 0 {
		t.Errorf("expected 0 ops for wrong category, got %d", len(ops))
	}
}

func TestBuildPlanNoScrapeUnknown(t *testing.T) {
	// When scrape info is absent but file is NOT in standard format.
	entries := []cloud115.TreeEntry{
		makeEntry("影音/电影/junk/random.garbage.file.mkv", "random.garbage.file.mkv", "影音/电影/junk"),
	}
	cache := fakeCache(map[string]map[string]any{}) // nothing cached
	ops := BuildPlan("电影", entries, cache)
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	op := ops[0]
	if op.Action != "skip" {
		t.Errorf("expected skip, got %q", op.Action)
	}
	if op.Reason != "no scrape result cached" {
		t.Errorf("unexpected reason %q", op.Reason)
	}
}

func TestBuildPlanMovieAlreadyCorrect(t *testing.T) {
	// Movie already in the correct folder with the correct filename → skip.
	entries := []cloud115.TreeEntry{
		makeEntry("影音/电影/Inception (2010)/Inception (2010).mkv", "Inception (2010).mkv", "影音/电影/Inception (2010)"),
	}
	cache := fakeCache(map[string]map[string]any{
		"Inception (2010)": {
			"type":    "movie",
			"title":   "Inception",
			"year":    float64(2010),
			"tmdb_id": float64(27205),
		},
	})
	ops := BuildPlan("电影", entries, cache)
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	if ops[0].Action != "skip" {
		t.Errorf("expected skip for already-correct movie, got %q", ops[0].Action)
	}
	if ops[0].Reason != "already correct" {
		t.Errorf("unexpected reason %q", ops[0].Reason)
	}
}

func TestBuildPlanMovieMissingTitle(t *testing.T) {
	// Movie scrape info with empty title → skip (no rename possible).
	entries := []cloud115.TreeEntry{
		makeEntry("影音/电影/unknown/unknown.mkv", "unknown.mkv", "影音/电影/unknown"),
	}
	cache := fakeCache(map[string]map[string]any{
		"unknown": {
			"type":  "movie",
			"title": "", // empty
			"year":  float64(2020),
		},
	})
	ops := BuildPlan("电影", entries, cache)
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	if ops[0].Action != "skip" {
		t.Errorf("expected skip for missing title, got %q", ops[0].Action)
	}
}

func TestBuildPlanMovieWithNumberFallback(t *testing.T) {
	// When tmdb_id is absent, source ID should fall back to "number".
	entries := []cloud115.TreeEntry{
		makeEntry("影音/AV/TEST-001/TEST-001.mkv", "TEST-001.mkv", "影音/AV/TEST-001"),
	}
	cache := fakeCache(map[string]map[string]any{
		"TEST-001": {
			"type":   "av",
			"number": "TEST-001",
			// no tmdb_id
		},
	})
	ops := BuildPlan("AV", entries, cache)
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	if ops[0].SourceID != "TEST-001" {
		t.Errorf("expected SourceID=TEST-001, got %q", ops[0].SourceID)
	}
}

func TestBuildPlanTVNoYear(t *testing.T) {
	// TV with show title but no year → folder without year.
	entries := []cloud115.TreeEntry{
		makeEntry("影音/剧目/folder/Show S01E01.mkv", "Show S01E01.mkv", "影音/剧目/folder"),
	}
	cache := fakeCache(map[string]map[string]any{
		"Show S01E01": {
			"type":      "tv",
			"showtitle": "My Show",
			// no year
			"season":  float64(1),
			"episode": float64(1),
		},
	})
	ops := BuildPlan("剧目", entries, cache)
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	op := ops[0]
	// NewFolder should be just the title (no year).
	if op.NewFolder != "My Show" {
		t.Errorf("expected NewFolder='My Show', got %q", op.NewFolder)
	}
}

func TestBuildPlanTVTitleFallback(t *testing.T) {
	// TV: when showtitle is absent, fall back to title field.
	entries := []cloud115.TreeEntry{
		makeEntry("影音/剧目/folder/ep.mkv", "ep.mkv", "影音/剧目/folder"),
	}
	cache := fakeCache(map[string]map[string]any{
		"ep": {
			"type":    "tv",
			"title":   "Fallback Title",
			"year":    float64(2020),
			"season":  float64(2),
			"episode": float64(3),
		},
	})
	ops := BuildPlan("剧目", entries, cache)
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	op := ops[0]
	if op.Action != "rename" {
		t.Errorf("expected rename, got %q", op.Action)
	}
	if op.NewFolder != "Fallback Title (2020)" {
		t.Errorf("unexpected NewFolder %q", op.NewFolder)
	}
	if op.NewName != "Fallback Title S02E03.mkv" {
		t.Errorf("unexpected NewName %q", op.NewName)
	}
}

func TestBuildPlanTVAlreadyCorrect(t *testing.T) {
	// TV: both folder and filename already match → skip with "already correct".
	entries := []cloud115.TreeEntry{
		makeEntry("影音/剧目/Breaking Bad (2008)/Breaking Bad S01E01.mkv",
			"Breaking Bad S01E01.mkv", "影音/剧目/Breaking Bad (2008)"),
	}
	cache := fakeCache(map[string]map[string]any{
		"Breaking Bad S01E01": {
			"type":      "tv",
			"showtitle": "Breaking Bad",
			"year":      float64(2008),
			"season":    float64(1),
			"episode":   float64(1),
		},
	})
	ops := BuildPlan("剧目", entries, cache)
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	op := ops[0]
	if op.Action != "skip" {
		t.Errorf("expected skip, got %q", op.Action)
	}
	if op.Reason != "already correct" {
		t.Errorf("unexpected reason %q", op.Reason)
	}
}

func TestBuildPlanTVMissingSeasonEpisode(t *testing.T) {
	// TV: show title + year present but season/episode missing → only folder rename.
	entries := []cloud115.TreeEntry{
		makeEntry("影音/剧目/random/ep.mkv", "ep.mkv", "影音/剧目/random"),
	}
	cache := fakeCache(map[string]map[string]any{
		"ep": {
			"type":      "tv",
			"showtitle": "Good Show",
			"year":      float64(2022),
			// season and episode absent → toInt returns 0
		},
	})
	ops := BuildPlan("剧目", entries, cache)
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	op := ops[0]
	// Folder rename only (no episode rename since season==0 or episode==0).
	if op.NewFolder != "Good Show (2022)" {
		t.Errorf("unexpected NewFolder %q", op.NewFolder)
	}
	if op.NewName != "" {
		t.Errorf("expected empty NewName, got %q", op.NewName)
	}
}

func TestBuildPlanAVPartSuffix(t *testing.T) {
	// AV file with .Part1 suffix should be preserved in the new name.
	entries := []cloud115.TreeEntry{
		makeEntry("影音/AV/IPX-100/IPX-100.Part1.FHD.mp4", "IPX-100.Part1.FHD.mp4", "影音/AV/IPX-100"),
	}
	cache := fakeCache(map[string]map[string]any{
		"IPX-100.Part1.FHD": {
			"type":   "av",
			"number": "IPX-100",
		},
	})
	ops := BuildPlan("AV", entries, cache)
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	op := ops[0]
	if op.NewName != "IPX-100.Part1.mp4" {
		t.Errorf("expected IPX-100.Part1.mp4, got %q", op.NewName)
	}
}

func TestBuildPlanDefaultCacheGet(t *testing.T) {
	// When cacheGet is nil, BuildPlan must use the real fileMapCacheGet.
	// Use XDG_CACHE_HOME override to point at a temp dir with a known entry.
	tmpDir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmpDir)

	// Write a cache entry directly.
	cacheDir := filepath.Join(tmpDir, "media115", "scrape", "file_map")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}
	entry := map[string]any{
		"type":    "movie",
		"title":   "Cache Test",
		"year":    float64(2022),
		"tmdb_id": float64(11111),
	}
	data, _ := json.Marshal(entry)
	if err := os.WriteFile(filepath.Join(cacheDir, "cache.test.mkv.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	entries := []cloud115.TreeEntry{
		makeEntry("影音/电影/folder/cache.test.mkv.mkv", "cache.test.mkv.mkv", "影音/电影/folder"),
	}

	// nil cacheGet → falls back to fileMapCacheGet.
	ops := BuildPlan("电影", entries, nil)
	// The stem of "cache.test.mkv.mkv" is "cache.test.mkv" — matches our file.
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	if ops[0].Action != "rename" {
		t.Errorf("expected rename when cache entry exists, got %q", ops[0].Action)
	}
}

// ── execute.go helpers ────────────────────────────────────────────────────────

func TestIsVideoFile(t *testing.T) {
	hits := []string{
		"movie.mkv", "video.MKV", "film.mp4", "show.avi",
		"stream.ts", "old.rmvb", "flash.flv", "windows.wmv",
		"quicktime.mov", "apple.m4v", "disk.iso",
	}
	for _, name := range hits {
		if !isVideoFile(name) {
			t.Errorf("isVideoFile(%q) = false, want true", name)
		}
	}

	misses := []string{
		"photo.jpg", "doc.pdf", "subtitle.srt", "info.nfo",
		"archive.zip", "noext", "",
	}
	for _, name := range misses {
		if isVideoFile(name) {
			t.Errorf("isVideoFile(%q) = true, want false", name)
		}
	}
}

func TestStemFilename(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"file.mkv", "file"},
		{"file.tar.gz", "file.tar"},
		{"noext", "noext"},
		{"/path/to/file.mp4", "file"},
		{"影音/AV/ABP-040/ABP-040.mp4", "ABP-040"},
		// filepath.Ext(".hidden") == ".hidden", so stem is empty; stemFilename uses LastIndex
		// which finds index 0, and idx > 0 is false → returns base as-is.
		{".hidden", ".hidden"},
	}
	for _, c := range cases {
		got := stemFilename(c.in)
		if got != c.want {
			t.Errorf("stemFilename(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestLeafName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"/影音/AV/ABP-040", "ABP-040"},
		{"a/b/c", "c"},
		{"noSlash", "noSlash"},
		{"/single", "single"},
		{"", ""},
	}
	for _, c := range cases {
		got := leafName(c.in)
		if got != c.want {
			t.Errorf("leafName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
