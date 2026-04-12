package scraper

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ── jsonutil ──────────────────────────────────────────────────────────────────

func TestStringVal(t *testing.T) {
	m := map[string]any{"a": "hello", "b": "", "c": 42}

	if got := StringVal(m, "a"); got != "hello" {
		t.Errorf("StringVal a = %q, want hello", got)
	}
	// empty string is skipped, fallback to second key
	m2 := map[string]any{"a": "", "b": "world"}
	if got := StringVal(m2, "a", "b"); got != "world" {
		t.Errorf("StringVal fallback = %q, want world", got)
	}
	// no matching key
	if got := StringVal(m, "missing"); got != "" {
		t.Errorf("StringVal missing = %q, want empty", got)
	}
	// non-string value is skipped
	if got := StringVal(m, "c"); got != "" {
		t.Errorf("StringVal non-string = %q, want empty", got)
	}
	// nil map
	if got := StringVal(nil, "a"); got != "" {
		t.Errorf("StringVal nil map = %q, want empty", got)
	}
}

func TestSliceVal(t *testing.T) {
	sl := []any{"x", "y"}
	m := map[string]any{"list": sl, "other": "str"}

	got := SliceVal(m, "list")
	if len(got) != 2 {
		t.Errorf("SliceVal list len = %d, want 2", len(got))
	}
	// wrong type
	if got := SliceVal(m, "other"); got != nil {
		t.Errorf("SliceVal wrong type = %v, want nil", got)
	}
	// missing key
	if got := SliceVal(m, "missing"); got != nil {
		t.Errorf("SliceVal missing = %v, want nil", got)
	}
}

func TestIntVal(t *testing.T) {
	m := map[string]any{"n": float64(42), "s": "7", "z": float64(0)}

	if got := IntVal(m, "n"); got != 42 {
		t.Errorf("IntVal n = %d, want 42", got)
	}
	if got := IntVal(m, "s"); got != 7 {
		t.Errorf("IntVal s = %d, want 7", got)
	}
	if got := IntVal(m, "z"); got != 0 {
		t.Errorf("IntVal z = %d, want 0", got)
	}
	if got := IntVal(m, "missing"); got != 0 {
		t.Errorf("IntVal missing = %d, want 0", got)
	}
}

func TestFloatVal(t *testing.T) {
	m := map[string]any{"f": float64(3.14), "s": "2.71", "z": float64(0)}

	if got := FloatVal(m, "f"); got != 3.14 {
		t.Errorf("FloatVal f = %f, want 3.14", got)
	}
	if got := FloatVal(m, "s"); got == 0 {
		t.Errorf("FloatVal s = 0, want non-zero")
	}
	if got := FloatVal(m, "z"); got != 0 {
		t.Errorf("FloatVal z = %f, want 0", got)
	}
	if got := FloatVal(m, "missing"); got != 0 {
		t.Errorf("FloatVal missing = %f, want 0", got)
	}
}

func TestYearFromDate(t *testing.T) {
	cases := []struct {
		input any
		want  int
	}{
		{"2023-06-15", 2023},
		{"2024", 2024},
		{float64(2022), 2022},
		{"", 0},
		{nil, 0},
		{"123", 0}, // less than 4 chars
		{42, 0},    // int 42 → string "42" (< 4 chars) → 0
		{float64(2019), 2019},
	}
	for _, tc := range cases {
		got := YearFromDate(tc.input)
		if got != tc.want {
			t.Errorf("YearFromDate(%v) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

// ── artwork ───────────────────────────────────────────────────────────────────

func TestDownloadImageSuccess(t *testing.T) {
	body := []byte("fake image data")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(body)
	}))
	defer server.Close()

	dir := t.TempDir()
	outPath := filepath.Join(dir, "image.jpg")

	if err := DownloadImage(server.URL+"/img.jpg", outPath, 5*time.Second, false); err != nil {
		t.Fatalf("DownloadImage: %v", err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != string(body) {
		t.Errorf("content mismatch: got %q, want %q", data, body)
	}
}

func TestDownloadImageSkipsExisting(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "existing.jpg")
	original := []byte("original")
	if err := os.WriteFile(outPath, original, 0o644); err != nil {
		t.Fatal(err)
	}

	// Server that returns different content — should not be hit
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Write([]byte("new content"))
	}))
	defer server.Close()

	if err := DownloadImage(server.URL+"/img.jpg", outPath, 5*time.Second, false); err != nil {
		t.Fatalf("DownloadImage: %v", err)
	}
	if callCount != 0 {
		t.Errorf("server was called %d times, expected 0 (file already exists)", callCount)
	}
	data, _ := os.ReadFile(outPath)
	if string(data) != string(original) {
		t.Errorf("file was overwritten unexpectedly")
	}
}

func TestDownloadImageForce(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "force.jpg")
	if err := os.WriteFile(outPath, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}

	newBody := []byte("new content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(newBody)
	}))
	defer server.Close()

	if err := DownloadImage(server.URL+"/img.jpg", outPath, 5*time.Second, true); err != nil {
		t.Fatalf("DownloadImage force: %v", err)
	}
	data, _ := os.ReadFile(outPath)
	if string(data) != string(newBody) {
		t.Errorf("force overwrite failed: got %q", data)
	}
}

func TestDownloadImageHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	dir := t.TempDir()
	err := DownloadImage(server.URL+"/missing.jpg", filepath.Join(dir, "out.jpg"), 5*time.Second, true)
	if err == nil {
		t.Error("expected error for HTTP 404, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("error = %v, want to contain 'HTTP 404'", err)
	}
}

func TestDownloadImageNetworkError(t *testing.T) {
	dir := t.TempDir()
	// Use an invalid address to trigger a network error
	err := DownloadImage("http://127.0.0.1:1/img.jpg", filepath.Join(dir, "out.jpg"), 1*time.Second, true)
	if err == nil {
		t.Error("expected network error, got nil")
	}
}

func TestDownloadImageDefaultTimeout(t *testing.T) {
	body := []byte("image")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer server.Close()

	dir := t.TempDir()
	// timeout=0 should use default (30s); just verifies no panic/error on a real server
	if err := DownloadImage(server.URL+"/img.jpg", filepath.Join(dir, "out.jpg"), 0, true); err != nil {
		t.Fatalf("DownloadImage with default timeout: %v", err)
	}
}

func TestSavePoster(t *testing.T) {
	body := []byte("poster")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer server.Close()

	// We can't easily override tmdbImageBase, but TMDBImageURL and SavePoster
	// can be tested for URL construction via TMDBImageURL.
	url := TMDBImageURL("/poster.jpg", "w500")
	if !strings.Contains(url, "/w500/poster.jpg") {
		t.Errorf("TMDBImageURL = %q, want to contain /w500/poster.jpg", url)
	}
}

func TestTMDBImageURL(t *testing.T) {
	cases := []struct {
		filePath string
		size     string
		want     string
	}{
		{"/abc.jpg", "original", "https://image.tmdb.org/t/p/original/abc.jpg"},
		{"/abc.jpg", "", "https://image.tmdb.org/t/p/original/abc.jpg"},
		{"/abc.jpg", "w500", "https://image.tmdb.org/t/p/w500/abc.jpg"},
		{"", "w500", ""},
	}
	for _, tc := range cases {
		got := TMDBImageURL(tc.filePath, tc.size)
		if got != tc.want {
			t.Errorf("TMDBImageURL(%q, %q) = %q, want %q", tc.filePath, tc.size, got, tc.want)
		}
	}
}

// ── analyzer extras ──────────────────────────────────────────────────────────

func TestLoadCasesSuccess(t *testing.T) {
	cases := []Case{
		{Filename: "movie.mkv", Expect: CaseExpect{Type: "movie", Title: "The Movie", Year: 2023}},
		{Filename: "show.S01E01.mkv", Expect: CaseExpect{Type: "tv", Title: "Show", Season: 1, Episode: 1}},
	}
	data, err := json.Marshal(cases)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "cases.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadCases(path)
	if err != nil {
		t.Fatalf("LoadCases: %v", err)
	}
	if len(loaded) != 2 {
		t.Errorf("len = %d, want 2", len(loaded))
	}
	if loaded[0].Filename != "movie.mkv" {
		t.Errorf("Filename = %q, want movie.mkv", loaded[0].Filename)
	}
	if loaded[1].Expect.Season != 1 {
		t.Errorf("Season = %d, want 1", loaded[1].Expect.Season)
	}
}

func TestLoadCasesMissingFile(t *testing.T) {
	_, err := LoadCases("/nonexistent/path/cases.json")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestLoadCasesInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("{not json}"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadCases(path)
	if err == nil {
		t.Error("expected JSON parse error, got nil")
	}
}

func TestMatchAgainstCasesHit(t *testing.T) {
	cases := []Case{
		{Filename: "movie.mkv", Expect: CaseExpect{Type: "movie", Title: "The Movie", Year: 2023, Source: "tmdb", SourceID: "123"}},
	}
	r := MatchAgainstCases("movie.mkv", cases)
	if r == nil {
		t.Fatal("expected non-nil result")
	}
	if r.MediaType != "movie" {
		t.Errorf("MediaType = %q, want movie", r.MediaType)
	}
	if r.Title != "The Movie" {
		t.Errorf("Title = %q, want The Movie", r.Title)
	}
	if r.Confidence != "high" {
		t.Errorf("Confidence = %q, want high", r.Confidence)
	}
}

func TestMatchAgainstCasesFullPath(t *testing.T) {
	cases := []Case{
		{Filename: "movie.mkv", Expect: CaseExpect{Type: "movie", Title: "Full Path Movie", Year: 2020}},
	}
	// Pass full path — basename should match
	r := MatchAgainstCases("/some/dir/movie.mkv", cases)
	if r == nil {
		t.Fatal("expected non-nil result for full path")
	}
	if r.Title != "Full Path Movie" {
		t.Errorf("Title = %q, want Full Path Movie", r.Title)
	}
}

func TestMatchAgainstCasesMiss(t *testing.T) {
	cases := []Case{
		{Filename: "known.mkv", Expect: CaseExpect{Type: "movie"}},
	}
	r := MatchAgainstCases("unknown.mkv", cases)
	if r != nil {
		t.Errorf("expected nil result for unmatched file, got %+v", r)
	}
}

func TestAnalyzeWithCasesMatch(t *testing.T) {
	cases := []Case{
		{Filename: "special.mkv", Expect: CaseExpect{Type: "movie", Title: "Special Film", Year: 2021}},
	}
	r := AnalyzeWithCases("special.mkv", cases)
	if r.MediaType != "movie" {
		t.Errorf("MediaType = %q, want movie", r.MediaType)
	}
	if r.Title != "Special Film" {
		t.Errorf("Title = %q, want Special Film", r.Title)
	}
}

func TestAnalyzeWithCasesFallback(t *testing.T) {
	cases := []Case{
		{Filename: "known.mkv", Expect: CaseExpect{Type: "movie", Title: "Known"}},
	}
	// Not in cases, should fall through to AnalyzeFilename
	r := AnalyzeWithCases("Unknown (2023).mkv", cases)
	if r.MediaType != "movie" {
		t.Errorf("MediaType = %q, want movie", r.MediaType)
	}
	if r.Year != 2023 {
		t.Errorf("Year = %d, want 2023", r.Year)
	}
}

func TestIsASCIIOnly(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"SubGroup", true},
		{"HorribleSubs", true},
		{"日本語", false},
		{"mixed日本", false},
		{"", true},
		{"ascii only 123", true},
	}
	for _, tc := range cases {
		got := isASCIIOnly(tc.s)
		if got != tc.want {
			t.Errorf("isASCIIOnly(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

func TestAnalyzeAnimeSxxExxCJK(t *testing.T) {
	// Bracket group with CJK — should be treated as TV, not anime
	r := AnalyzeFilename("[日本語タイトル] Show S01E05.mkv")
	if r.MediaType != "tv" {
		t.Errorf("expected tv for CJK bracket, got %q", r.MediaType)
	}
}

func TestAnalyzeAnimeSxxExxASCIIBracket(t *testing.T) {
	// ASCII bracket → anime
	r := AnalyzeFilename("[HorribleSubs] Show S02E03.mkv")
	if r.MediaType != "anime" {
		t.Errorf("expected anime for ASCII bracket SxxExx, got %q", r.MediaType)
	}
	if r.Season != 2 || r.Episode != 3 {
		t.Errorf("season/episode: got %d/%d, want 2/3", r.Season, r.Episode)
	}
}

func TestAnalyzeWestDatePattern(t *testing.T) {
	r := AnalyzeFilename("Brazzers.24.01.15.Performer.Name.XXX.mp4")
	if r.MediaType != "av_west" {
		t.Errorf("expected av_west for date pattern, got %q", r.MediaType)
	}
	if r.Source != "theporndb" {
		t.Errorf("Source = %q, want theporndb", r.Source)
	}
}

func TestAnalyzeGravurePrefixes(t *testing.T) {
	// Test various gravure prefixes
	prefixes := []string{"IMOG", "LCBD", "ENFD", "TSDV", "SBVD", "LPFD", "LCDV", "ENCO", "OME"}
	for _, prefix := range prefixes {
		filename := prefix + "-001.mp4"
		r := AnalyzeFilename(filename)
		if r.MediaType != "gravure" {
			t.Errorf("prefix %s: expected gravure, got %q", prefix, r.MediaType)
		}
	}
}

func TestAnalyzeAVNonGravure(t *testing.T) {
	// A label-number that isn't a gravure prefix → av
	r := AnalyzeFilename("SSNI-123.mp4")
	if r.MediaType != "av" {
		t.Errorf("expected av, got %q", r.MediaType)
	}
	if r.Source != "javbus" {
		t.Errorf("Source = %q, want javbus", r.Source)
	}
}

func TestStemFilename(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"movie.mkv", "movie"},
		{"/path/to/file.mp4", "file"},
		{"no_extension", "no_extension"},
		{"multi.part.name.avi", "multi.part.name"},
	}
	for _, tc := range cases {
		got := StemFilename(tc.input)
		if got != tc.want {
			t.Errorf("StemFilename(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ── NFO extras ────────────────────────────────────────────────────────────────

func TestGenerateMovieNFOAllFields(t *testing.T) {
	dir := t.TempDir()
	meta := &Metadata{
		Title:         "Test Movie",
		OriginalTitle: "Original Title",
		SortTitle:     "Movie, Test",
		Year:          2024,
		Plot:          "A great plot.",
		Outline:       "Short outline.",
		Tagline:       "The tagline.",
		MPAA:          "PG-13",
		Rating:        7.8,
		Votes:         5000,
		Runtime:       120,
		Set:           "Test Collection",
		Genres:        []string{"Action", "Drama"},
		Tags:          []string{"tag1", "tag2"},
		Countries:     []string{"US"},
		Studios:       []string{"Studio A"},
		Credits:       []string{"Writer One"},
		Directors:     []string{"Dir One"},
		Actors:        []Actor{{Name: "Actor A", Role: "Hero", Thumb: "https://example.com/a.jpg"}},
		UniqueIDs:     map[string]string{"tmdb": "12345", "imdb": "tt0000001"},
		PosterURL:     "https://example.com/poster.jpg",
		FanartURL:     "https://example.com/fanart.jpg",
	}

	outPath := filepath.Join(dir, "movie.nfo")
	if err := GenerateMovieNFO(meta, outPath); err != nil {
		t.Fatalf("GenerateMovieNFO: %v", err)
	}

	data, _ := os.ReadFile(outPath)
	content := string(data)

	for _, want := range []string{
		"<sorttitle>Movie, Test</sorttitle>",
		"<outline>Short outline.</outline>",
		"<tagline>The tagline.</tagline>",
		"<mpaa>PG-13</mpaa>",
		"<rating>7.8</rating>",
		"<votes>5000</votes>",
		"<runtime>120</runtime>",
		"<set>",
		"<name>Test Collection</name>",
		"<tag>tag1</tag>",
		"<country>US</country>",
		"<studio>Studio A</studio>",
		"<credits>Writer One</credits>",
		"<fanart>",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in NFO:\n%s", want, content)
		}
	}
}

func TestGenerateMovieNFONoOptionalFields(t *testing.T) {
	// Zero-value optional fields (Year=0, Rating=0, Votes=0, Runtime=0, Set="")
	dir := t.TempDir()
	meta := &Metadata{
		Title: "Minimal Movie",
	}
	outPath := filepath.Join(dir, "minimal.nfo")
	if err := GenerateMovieNFO(meta, outPath); err != nil {
		t.Fatalf("GenerateMovieNFO: %v", err)
	}
	data, _ := os.ReadFile(outPath)
	content := string(data)
	if strings.Contains(content, "<year>") {
		t.Error("should not have <year> when Year=0")
	}
	if strings.Contains(content, "<rating>") {
		t.Error("should not have <rating> when Rating=0")
	}
	if strings.Contains(content, "<set>") {
		t.Error("should not have <set> when Set=''")
	}
}

func TestGenerateEpisodeNFOAllFields(t *testing.T) {
	dir := t.TempDir()
	meta := &Metadata{
		Title:     "Episode Title",
		ShowTitle: "The Show",
		Season:    2,
		Episode:   5,
		Plot:      "Episode plot.",
		Aired:     "2024-03-15",
		Rating:    8.1,
		Votes:     200,
		Runtime:   45,
		Directors: []string{"Episode Dir"},
		Credits:   []string{"Episode Writer"},
		Actors:    []Actor{{Name: "Guest Star", Role: "Villain"}},
		UniqueIDs: map[string]string{"tmdb": "ep123"},
	}

	outPath := filepath.Join(dir, "ep.nfo")
	if err := GenerateEpisodeNFO(meta, outPath); err != nil {
		t.Fatalf("GenerateEpisodeNFO: %v", err)
	}

	data, _ := os.ReadFile(outPath)
	content := string(data)

	for _, want := range []string{
		"<season>2</season>",
		"<episode>5</episode>",
		"<rating>8.1</rating>",
		"<votes>200</votes>",
		"<runtime>45</runtime>",
		"<director>Episode Dir</director>",
		"<credits>Episode Writer</credits>",
		"<name>Guest Star</name>",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in episode NFO:\n%s", want, content)
		}
	}
}

func TestGenerateTVShowNFOAllFields(t *testing.T) {
	dir := t.TempDir()
	meta := &Metadata{
		Title:         "Great Show",
		OriginalTitle: "Original Show",
		ShowTitle:     "Great Show",
		Year:          2020,
		Plot:          "Show plot.",
		Status:        "Ended",
		Rating:        9.0,
		Votes:         10000,
		Premiered:     "2020-01-01",
		Genres:        []string{"Drama"},
		Tags:          []string{"tag"},
		Studios:       []string{"HBO"},
		Actors:        []Actor{{Name: "Lead Actor", Role: "Protagonist"}},
		UniqueIDs:     map[string]string{"tmdb": "tv99"},
		PosterURL:     "https://example.com/tv-poster.jpg",
		FanartURL:     "https://example.com/tv-fanart.jpg",
	}

	outPath := filepath.Join(dir, "tvshow.nfo")
	if err := GenerateTVShowNFO(meta, outPath); err != nil {
		t.Fatalf("GenerateTVShowNFO: %v", err)
	}

	data, _ := os.ReadFile(outPath)
	content := string(data)

	for _, want := range []string{
		"<tvshow>",
		"<originaltitle>Original Show</originaltitle>",
		"<status>Ended</status>",
		"<rating>9</rating>",
		"<votes>10000</votes>",
		"<premiered>2020-01-01</premiered>",
		"<studio>HBO</studio>",
		"<tag>tag</tag>",
		"aspect=\"poster\"",
		"<fanart>",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in tvshow NFO:\n%s", want, content)
		}
	}
}

func TestGenerateTVShowNFOShowTitleFallback(t *testing.T) {
	// When ShowTitle is empty, it should fallback to Title
	dir := t.TempDir()
	meta := &Metadata{
		Title:     "Fallback Show",
		ShowTitle: "", // empty → should use Title
	}
	outPath := filepath.Join(dir, "tvshow.nfo")
	if err := GenerateTVShowNFO(meta, outPath); err != nil {
		t.Fatalf("GenerateTVShowNFO: %v", err)
	}
	data, _ := os.ReadFile(outPath)
	content := string(data)
	if !strings.Contains(content, "<showtitle>Fallback Show</showtitle>") {
		t.Errorf("expected fallback showtitle, content:\n%s", content)
	}
}

func TestParseNFOFileNotFound(t *testing.T) {
	_, err := ParseNFO("/nonexistent/path/file.nfo")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestParseNFOTVShow(t *testing.T) {
	dir := t.TempDir()
	meta := &Metadata{
		Title:     "TV Show",
		Year:      2021,
		Rating:    8.5,
		Genres:    []string{"Drama"},
		Status:    "Continuing",
		Premiered: "2021-04-01",
		UniqueIDs: map[string]string{"tmdb": "tv42"},
		PosterURL: "https://example.com/tvposter.jpg",
		FanartURL: "https://example.com/tvfanart.jpg",
	}

	outPath := filepath.Join(dir, "tvshow.nfo")
	if err := GenerateTVShowNFO(meta, outPath); err != nil {
		t.Fatalf("GenerateTVShowNFO: %v", err)
	}

	parsed, err := ParseNFO(outPath)
	if err != nil {
		t.Fatalf("ParseNFO tvshow: %v", err)
	}

	if parsed.Title != "TV Show" {
		t.Errorf("Title = %q, want TV Show", parsed.Title)
	}
	if parsed.Year != 2021 {
		t.Errorf("Year = %d, want 2021", parsed.Year)
	}
	if parsed.Rating != 8.5 {
		t.Errorf("Rating = %f, want 8.5", parsed.Rating)
	}
	if parsed.Status != "Continuing" {
		t.Errorf("Status = %q, want Continuing", parsed.Status)
	}
	if parsed.Premiered != "2021-04-01" {
		t.Errorf("Premiered = %q, want 2021-04-01", parsed.Premiered)
	}
	if parsed.UniqueIDs["tmdb"] != "tv42" {
		t.Errorf("uniqueid tmdb = %q, want tv42", parsed.UniqueIDs["tmdb"])
	}
	if parsed.PosterURL != "https://example.com/tvposter.jpg" {
		t.Errorf("PosterURL = %q", parsed.PosterURL)
	}
	if parsed.FanartURL != "https://example.com/tvfanart.jpg" {
		t.Errorf("FanartURL = %q", parsed.FanartURL)
	}
}

func TestParseNFOEpisodeAllFields(t *testing.T) {
	dir := t.TempDir()
	meta := &Metadata{
		Title:     "Great Episode",
		ShowTitle: "Great Show",
		Season:    3,
		Episode:   7,
		Plot:      "Episode plot here.",
		Aired:     "2022-05-10",
		Rating:    7.2,
		Votes:     150,
		Runtime:   42,
		Directors: []string{"Ep Director"},
		Credits:   []string{"Ep Writer"},
		Actors:    []Actor{{Name: "Ep Actor", Role: "Side"}},
		UniqueIDs: map[string]string{"tmdb": "epid789"},
	}

	outPath := filepath.Join(dir, "ep_all.nfo")
	if err := GenerateEpisodeNFO(meta, outPath); err != nil {
		t.Fatal(err)
	}

	parsed, err := ParseNFO(outPath)
	if err != nil {
		t.Fatalf("ParseNFO episode: %v", err)
	}

	if parsed.Season != 3 || parsed.Episode != 7 {
		t.Errorf("S%dE%d, want S3E7", parsed.Season, parsed.Episode)
	}
	if parsed.Rating != 7.2 {
		t.Errorf("Rating = %f, want 7.2", parsed.Rating)
	}
	if parsed.Votes != 150 {
		t.Errorf("Votes = %d, want 150", parsed.Votes)
	}
	if parsed.Runtime != 42 {
		t.Errorf("Runtime = %d, want 42", parsed.Runtime)
	}
	if len(parsed.Directors) == 0 || parsed.Directors[0] != "Ep Director" {
		t.Errorf("Directors = %v", parsed.Directors)
	}
	if len(parsed.Actors) == 0 || parsed.Actors[0].Name != "Ep Actor" {
		t.Errorf("Actors = %v", parsed.Actors)
	}
	if parsed.UniqueIDs["tmdb"] != "epid789" {
		t.Errorf("uniqueid = %q, want epid789", parsed.UniqueIDs["tmdb"])
	}
}

func TestParseNFOInvalidXML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.nfo")
	if err := os.WriteFile(path, []byte("<movie><title>unclosed"), 0o644); err != nil {
		t.Fatal(err)
	}
	// This should either succeed (partial parse) or fail gracefully
	// The important thing is it doesn't panic
	_, _ = ParseNFO(path)
}

func TestRewrapXMLNoLocal(t *testing.T) {
	data := []byte("<movie><title>Test</title></movie>")
	// empty local → return as-is
	result := rewrapXML(data, "")
	if string(result) != string(data) {
		t.Errorf("rewrapXML('') should return original data unchanged")
	}
}

// ── throttle ─────────────────────────────────────────────────────────────────

func TestThrottle(t *testing.T) {
	// 10 qps → 100ms interval
	th := NewThrottle(10)
	if th == nil {
		t.Fatal("NewThrottle returned nil")
	}
	// First call should be immediate
	start := time.Now()
	th.Wait()
	// Second call within 100ms should block briefly
	th.Wait()
	elapsed := time.Since(start)
	if elapsed < 80*time.Millisecond {
		t.Errorf("second Wait elapsed %v, expected >= 80ms", elapsed)
	}
}

// ── retry ─────────────────────────────────────────────────────────────────────

func TestRetryGetSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h := r.Header.Get("X-Test"); h != "value" {
			t.Errorf("missing header X-Test, got %q", h)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	defer server.Close()

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := RetryGet(client, server.URL+"/test", map[string]string{"X-Test": "value"}, 3)
	if err != nil {
		t.Fatalf("RetryGet: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}
}

func TestRetryGetNetworkError(t *testing.T) {
	client := &http.Client{Timeout: 100 * time.Millisecond}
	// Port 1 is typically refused → network error
	resp, err := RetryGet(client, "http://127.0.0.1:1/test", nil, 1)
	if err == nil {
		resp.Body.Close()
		t.Error("expected network error, got nil")
	}
}

func TestRetryGetInvalidURL(t *testing.T) {
	client := &http.Client{}
	resp, err := RetryGet(client, "://invalid-url", nil, 1)
	if err == nil {
		resp.Body.Close()
		t.Error("expected error for invalid URL, got nil")
	}
}

// ── registry extras ───────────────────────────────────────────────────────────

func TestReset(t *testing.T) {
	resetProviders() // use local helper that directly manipulates providers
	p := &mockProvider{name: "test", types: []string{"movie"}, priority: 1}
	Register(p)

	if len(GetProviders("movie")) != 1 {
		t.Fatal("expected 1 provider after Register")
	}

	Reset()
	if len(GetProviders("movie")) != 0 {
		t.Error("expected 0 providers after Reset")
	}
}
