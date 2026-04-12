package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/newcoderlife/media115/internal/config"
	"github.com/newcoderlife/media115/internal/scraper"
)

// ── formatSize ────────────────────────────────────────────────────────────────

func TestFormatSize(t *testing.T) {
	tests := []struct {
		size int64
		want string
	}{
		{0, "0B"},
		{1, "1B"},
		{1023, "1023B"},
		{1024, "1.0K"},
		{1536, "1.5K"},
		{1024 * 1024, "1.0M"},
		{int64(1.5 * 1024 * 1024), "1.5M"},
		{1024 * 1024 * 1024, "1.0G"},
		{int64(2.5 * 1024 * 1024 * 1024), "2.5G"},
		{1024 * 1024 * 1024 * 1024, "1.0T"},
	}
	for _, tt := range tests {
		got := formatSize(tt.size)
		if got != tt.want {
			t.Errorf("formatSize(%d) = %q, want %q", tt.size, got, tt.want)
		}
	}
}

func TestFormatSizePetabyte(t *testing.T) {
	// Beyond terabytes falls through to Petabyte suffix
	size := int64(1024) * 1024 * 1024 * 1024 * 1024 * 2 // 2 PB
	got := formatSize(size)
	if !strings.HasSuffix(got, "P") {
		t.Errorf("formatSize(2PB) = %q, expected to end with 'P'", got)
	}
}

// ── trunc ─────────────────────────────────────────────────────────────────────

func TestTrunc(t *testing.T) {
	tests := []struct {
		s      string
		maxLen int
		want   string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 8, "hello w…"},
		{"hello world", 1, "…"},
		{"hello world", 0, "…"},
		{"", 5, ""},
		// Multi-byte runes
		{"日本語テスト", 4, "日本語…"},
		{"日本語テスト", 10, "日本語テスト"},
	}
	for _, tt := range tests {
		got := trunc(tt.s, tt.maxLen)
		if got != tt.want {
			t.Errorf("trunc(%q, %d) = %q, want %q", tt.s, tt.maxLen, got, tt.want)
		}
	}
}

// ── dashes ────────────────────────────────────────────────────────────────────

func TestDashes(t *testing.T) {
	if got := dashes(0); got != "" {
		t.Errorf("dashes(0) = %q, want empty string", got)
	}
	if got := dashes(5); got != "-----" {
		t.Errorf("dashes(5) = %q, want '-----'", got)
	}
	if got := dashes(1); got != "-" {
		t.Errorf("dashes(1) = %q, want '-'", got)
	}
}

// ── leafName ──────────────────────────────────────────────────────────────────

func TestLeafName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/foo/bar/baz", "baz"},
		{"/foo/bar", "bar"},
		{"/foo", "foo"},
		{"noSlash", "noSlash"},
		{"", ""},
		{"/", ""},
	}
	for _, tt := range tests {
		got := leafName(tt.input)
		if got != tt.want {
			t.Errorf("leafName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ── normalizeMediaType ────────────────────────────────────────────────────────

func TestNormalizeMediaType(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"anime", "tv"},
		{"gravure", "av"},
		{"av_west", "av"},
		{"movie", "movie"},
		{"tv", "tv"},
		{"av", "av"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		got := normalizeMediaType(tt.input)
		if got != tt.want {
			t.Errorf("normalizeMediaType(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ── mediaCacheDir ─────────────────────────────────────────────────────────────

func TestMediaCacheDir(t *testing.T) {
	got := mediaCacheDir()
	// Should contain "media115" (not "cloud115")
	if strings.Contains(got, "cloud115") {
		t.Errorf("mediaCacheDir() = %q should not contain 'cloud115'", got)
	}
	if !strings.Contains(got, "media115") {
		t.Errorf("mediaCacheDir() = %q should contain 'media115'", got)
	}
}

// ── scrapeCachePath ───────────────────────────────────────────────────────────

func TestScrapeCachePath(t *testing.T) {
	path := scrapeCachePath("movie", "Inception.2010")
	if !strings.Contains(path, "movie") {
		t.Errorf("scrapeCachePath: %q should contain 'movie'", path)
	}
	if !strings.HasSuffix(path, "Inception.2010.json") {
		t.Errorf("scrapeCachePath: %q should end with 'Inception.2010.json'", path)
	}
}

// ── countJSONFiles ────────────────────────────────────────────────────────────

func TestCountJSONFiles(t *testing.T) {
	dir := t.TempDir()

	// Empty directory
	if n := countJSONFiles(dir); n != 0 {
		t.Errorf("empty dir: countJSONFiles = %d, want 0", n)
	}

	// Add some files
	_ = os.WriteFile(filepath.Join(dir, "a.json"), []byte("{}"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "b.json"), []byte("{}"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "c.txt"), []byte("hi"), 0o644)

	if n := countJSONFiles(dir); n != 2 {
		t.Errorf("dir with 2 json + 1 txt: countJSONFiles = %d, want 2", n)
	}

	// Add a sub-directory
	sub := filepath.Join(dir, "sub")
	_ = os.Mkdir(sub, 0o755)
	_ = os.WriteFile(filepath.Join(sub, "d.json"), []byte("{}"), 0o644)

	if n := countJSONFiles(dir); n != 3 {
		t.Errorf("with subdir: countJSONFiles = %d, want 3", n)
	}
}

func TestCountJSONFilesNonExistent(t *testing.T) {
	if n := countJSONFiles("/nonexistent/path/xyz"); n != 0 {
		t.Errorf("nonexistent dir: countJSONFiles = %d, want 0", n)
	}
}

// ── loadCases ─────────────────────────────────────────────────────────────────

func TestLoadCasesReturnsNilWhenFileAbsent(t *testing.T) {
	// Override XDG_CACHE_HOME so mediaCacheDir points to a temp location
	// that has no scrape_cases.json.
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	result := loadCases()
	if result != nil {
		t.Errorf("loadCases() = %v, want nil when file absent", result)
	}
}

func TestLoadCasesReturnsDataWhenFileExists(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	// Create the directory structure that mediaCacheDir() / loadCases() expects
	cacheBase := config.CacheDir() // respects XDG_CACHE_HOME
	m115Dir := strings.Replace(cacheBase, "cloud115", "media115", 1)
	_ = os.MkdirAll(m115Dir, 0o755)

	cases := []scraper.Case{
		{Filename: "Test.Movie.2023.mkv", Expect: scraper.CaseExpect{Type: "movie", Title: "Test Movie", Year: 2023}},
	}
	data, _ := json.Marshal(cases)
	_ = os.WriteFile(filepath.Join(m115Dir, "scrape_cases.json"), data, 0o644)

	result := loadCases()
	if len(result) != 1 {
		t.Fatalf("loadCases() = %d cases, want 1", len(result))
	}
	if result[0].Filename != "Test.Movie.2023.mkv" {
		t.Errorf("loadCases()[0].Filename = %q", result[0].Filename)
	}
}

// ── registerProviders ─────────────────────────────────────────────────────────

func TestRegisterProviders(t *testing.T) {
	// Reset the global registry so this test is isolated.
	scraper.Reset()

	cfg := config.Default()
	// jav321 and javfree register without tokens.
	registerProviders(cfg)

	// At minimum, jav321 and javfree must be registered for "av".
	avProviders := scraper.GetProviders("av")
	if len(avProviders) < 2 {
		t.Errorf("expected at least 2 av providers (jav321, javfree), got %d", len(avProviders))
	}

	// Verify provider names include jav321 and javfree.
	names := make(map[string]bool)
	for _, p := range avProviders {
		names[p.Name()] = true
	}
	for _, want := range []string{"jav321", "javfree"} {
		if !names[want] {
			t.Errorf("expected provider %q to be registered", want)
		}
	}

	scraper.Reset()
}

func TestRegisterProvidersWithTokens(t *testing.T) {
	scraper.Reset()

	cfg := config.Default()
	cfg.Auth.TMDB.Token = "fake-tmdb"
	cfg.Auth.Bangumi.Token = "fake-bangumi"
	cfg.Auth.ThePornDB.Token = "fake-tpdb"
	cfg.Auth.StashDB.APIKey = "fake-stashdb"
	registerProviders(cfg)

	// movie and tv should have tmdb registered
	movieProviders := scraper.GetProviders("movie")
	foundTMDB := false
	for _, p := range movieProviders {
		if p.Name() == "tmdb" {
			foundTMDB = true
		}
	}
	if !foundTMDB {
		t.Error("expected 'tmdb' provider for 'movie' type")
	}

	// av_west should have theporndb
	avWestProviders := scraper.GetProviders("av_west")
	foundTPDB := false
	for _, p := range avWestProviders {
		if p.Name() == "theporndb" {
			foundTPDB = true
		}
	}
	if !foundTPDB {
		t.Error("expected 'theporndb' provider for 'av_west' type")
	}

	scraper.Reset()
}

// ── Cobra command registration ────────────────────────────────────────────────

func TestRootCommandHasAllSubcommands(t *testing.T) {
	wantCmds := []string{"scan", "scrape", "scrape-fix", "organize", "doctor"}
	for _, name := range wantCmds {
		found := false
		for _, cmd := range rootCmd.Commands() {
			if cmd.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("rootCmd is missing subcommand %q", name)
		}
	}
}

func TestRootCommandVerboseFlag(t *testing.T) {
	if rootCmd.PersistentFlags().Lookup("verbose") == nil {
		t.Error("rootCmd missing --verbose flag")
	}
}

func TestScanCmdAllFlag(t *testing.T) {
	if scanCmd.Flags().Lookup("all") == nil {
		t.Error("scanCmd missing --all flag")
	}
}

func TestScrapeCmdFlags(t *testing.T) {
	for _, name := range []string{"force", "limit"} {
		if scrapeCmd.Flags().Lookup(name) == nil {
			t.Errorf("scrapeCmd missing --%s flag", name)
		}
	}
}

func TestOrganizeCmdExecuteFlag(t *testing.T) {
	if organizeCmd.Flags().Lookup("execute") == nil {
		t.Error("organizeCmd missing --execute flag")
	}
}

// ── scrapeGet / scrapePut / scrapeIsNotFound ───────────────────────────────────

func TestScrapePutAndGet(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	entry := map[string]any{"title": "Test Movie", "_cached_at": float64(1000000)}
	scrapePut("movie", "test-movie", entry)

	got := scrapeGet("movie", "test-movie")
	if got == nil {
		t.Fatal("scrapeGet returned nil after scrapePut")
	}
	if got["title"] != "Test Movie" {
		t.Errorf("title = %v, want 'Test Movie'", got["title"])
	}
}

func TestScrapeGetReturnsNilForMissingFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	if got := scrapeGet("movie", "nonexistent"); got != nil {
		t.Errorf("expected nil for missing cache, got %v", got)
	}
}

func TestScrapeGetReturnsNilForInvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	// Write invalid JSON to the expected path.
	p := scrapeCachePath("movie", "bad-file")
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	_ = os.WriteFile(p, []byte("not valid json"), 0o644)

	if got := scrapeGet("movie", "bad-file"); got != nil {
		t.Errorf("expected nil for invalid JSON, got %v", got)
	}
}

func TestScrapeIsNotFoundTrue(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	// Fresh not_found entry with current timestamp.
	scrapePut("movie", "gone-movie", map[string]any{
		"_not_found": true,
		"_cached_at": float64(timeNowUnix()),
	})

	if !scrapeIsNotFound("movie", "gone-movie") {
		t.Error("expected scrapeIsNotFound = true for fresh not_found entry")
	}
}

func TestScrapeIsNotFoundFalseWhenExpired(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	// Old entry (100 days ago).
	old := float64(timeNowUnix() - 100*24*3600)
	scrapePut("movie", "old-movie", map[string]any{
		"_not_found": true,
		"_cached_at": old,
	})

	if scrapeIsNotFound("movie", "old-movie") {
		t.Error("expected scrapeIsNotFound = false for expired entry")
	}
}

func TestScrapeIsNotFoundFalseWhenAbsent(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	if scrapeIsNotFound("movie", "absent-movie") {
		t.Error("expected scrapeIsNotFound = false when no cache entry")
	}
}

func TestScrapeIsNotFoundFalseWhenNotMarked(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	// Entry exists but _not_found is not set.
	scrapePut("movie", "found-movie", map[string]any{
		"title":      "Good Movie",
		"_cached_at": float64(timeNowUnix()),
	})

	if scrapeIsNotFound("movie", "found-movie") {
		t.Error("expected scrapeIsNotFound = false when _not_found not set")
	}
}

func TestScrapeIsNotFoundNoTimestamp(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	// Entry with _not_found but no _cached_at — should conservatively return true.
	scrapePut("movie", "notimestamp-movie", map[string]any{
		"_not_found": true,
	})

	if !scrapeIsNotFound("movie", "notimestamp-movie") {
		t.Error("expected scrapeIsNotFound = true when no timestamp (conservative)")
	}
}

// timeNowUnix returns the current Unix timestamp.
func timeNowUnix() int64 {
	return time.Now().Unix()
}

// saveFileMap tests ────────────────────────────────────────────────────────────

func TestSaveFileMapMovie(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	sr := &scraper.ScrapeResult{
		Status: "ok",
		Match:  "Inception",
		IDs:    map[string]string{"tmdb": "27205"},
		Meta: &scraper.Metadata{
			Title:         "Inception",
			OriginalTitle: "Inception",
			Year:          2010,
		},
	}
	saveFileMap("Inception.2010.mkv", "movie", "Inception", sr)

	// Verify the JSON file was written.
	cacheDir := mediaCacheDir()
	path := filepath.Join(cacheDir, "scrape", "file_map", "Inception.2010.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file_map not written: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if m["type"] != "movie" {
		t.Errorf("type = %v, want 'movie'", m["type"])
	}
	if m["tmdb_id"] != "27205" {
		t.Errorf("tmdb_id = %v, want '27205'", m["tmdb_id"])
	}
}

func TestSaveFileMapTV(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	sr := &scraper.ScrapeResult{
		Status: "ok",
		Match:  "Breaking Bad S01E01",
		IDs:    map[string]string{"tmdb": "1396"},
		Meta: &scraper.Metadata{
			Title:     "Pilot",
			ShowTitle: "Breaking Bad",
			Year:      2008,
			Season:    1,
			Episode:   1,
		},
	}
	saveFileMap("Breaking.Bad.S01E01.mkv", "tv", "Breaking Bad", sr)

	cacheDir := mediaCacheDir()
	path := filepath.Join(cacheDir, "scrape", "file_map", "Breaking.Bad.S01E01.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file_map not written: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if m["type"] != "tv" {
		t.Errorf("type = %v, want 'tv'", m["type"])
	}
	if m["showtitle"] != "Breaking Bad" {
		t.Errorf("showtitle = %v, want 'Breaking Bad'", m["showtitle"])
	}
}

func TestSaveFileMapTVNoMeta(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	sr := &scraper.ScrapeResult{
		Status: "ok",
		Match:  "Some Show",
		IDs:    map[string]string{"tmdb": "999"},
	}
	saveFileMap("Some.Show.S02E03.mkv", "tv", "Some Show", sr)

	cacheDir := mediaCacheDir()
	path := filepath.Join(cacheDir, "scrape", "file_map", "Some.Show.S02E03.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file_map not written: %v", err)
	}
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	// When meta is nil, title and showtitle fall back to sr.Match
	if m["title"] != "Some Show" {
		t.Errorf("title = %v, want 'Some Show'", m["title"])
	}
	if m["showtitle"] != "Some Show" {
		t.Errorf("showtitle = %v, want 'Some Show'", m["showtitle"])
	}
}

func TestSaveFileMapAV(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	sr := &scraper.ScrapeResult{
		Status: "ok",
		Match:  "ABC-123",
		IDs:    map[string]string{"jav321": "ABC-123"},
		Meta: &scraper.Metadata{
			Title: "Test AV Title",
		},
	}
	saveFileMap("ABC-123.mkv", "av", "ABC-123", sr)

	cacheDir := mediaCacheDir()
	path := filepath.Join(cacheDir, "scrape", "file_map", "ABC-123.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file_map not written: %v", err)
	}
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	if m["type"] != "av" {
		t.Errorf("type = %v, want 'av'", m["type"])
	}
	if m["number"] != "ABC-123" {
		t.Errorf("number = %v, want 'ABC-123'", m["number"])
	}
}

func TestSaveFileMapAVWest(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	sr := &scraper.ScrapeResult{
		Status: "ok",
		Match:  "WestScene Title",
		IDs:    map[string]string{"theporndb": "scene-99"},
	}
	saveFileMap("WestScene.Title.mkv", "av_west", "WestScene-Title", sr)

	cacheDir := mediaCacheDir()
	path := filepath.Join(cacheDir, "scrape", "file_map", "WestScene.Title.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file_map not written: %v", err)
	}
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	// av_west normalizes to "av"
	if m["type"] != "av" {
		t.Errorf("type = %v, want 'av'", m["type"])
	}
	// For av_west, number is the original query
	if m["number"] != "WestScene-Title" {
		t.Errorf("number = %v, want 'WestScene-Title'", m["number"])
	}
}

func TestSaveFileMapAVNoIDsFallsBackToMatch(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	sr := &scraper.ScrapeResult{
		Status: "ok",
		Match:  "XYZ-456",
		IDs:    map[string]string{},
	}
	saveFileMap("XYZ-456.mp4", "av", "XYZ-456", sr)

	cacheDir := mediaCacheDir()
	path := filepath.Join(cacheDir, "scrape", "file_map", "XYZ-456.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file_map not written: %v", err)
	}
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	// Falls back to sr.Match
	if m["number"] != "XYZ-456" {
		t.Errorf("number = %v, want 'XYZ-456'", m["number"])
	}
}

func TestSaveFileMapMovieNoMeta(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	sr := &scraper.ScrapeResult{
		Status: "ok",
		Match:  "Some Movie",
		IDs:    map[string]string{},
	}
	saveFileMap("Some.Movie.mkv", "movie", "Some Movie", sr)

	cacheDir := mediaCacheDir()
	path := filepath.Join(cacheDir, "scrape", "file_map", "Some.Movie.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file_map not written: %v", err)
	}
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	if m["title"] != "Some Movie" {
		t.Errorf("title = %v, want 'Some Movie'", m["title"])
	}
}

func TestSaveFileMapAnimeNormalizesToTV(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)

	sr := &scraper.ScrapeResult{
		Status: "ok",
		Match:  "Naruto S01E01",
		IDs:    map[string]string{"tmdb": "46260"},
		Meta: &scraper.Metadata{
			Title:     "Enter: Naruto Uzumaki!",
			ShowTitle: "Naruto",
			Season:    1,
			Episode:   1,
		},
	}
	saveFileMap("Naruto.S01E01.mkv", "anime", "Naruto", sr)

	cacheDir := mediaCacheDir()
	path := filepath.Join(cacheDir, "scrape", "file_map", "Naruto.S01E01.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file_map not written: %v", err)
	}
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	// anime normalizes to "tv"
	if m["type"] != "tv" {
		t.Errorf("type = %v, want 'tv'", m["type"])
	}
}
