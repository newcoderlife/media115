package scraper

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateMovieNFO(t *testing.T) {
	dir := t.TempDir()
	meta := &Metadata{
		Title:     "满江红",
		Year:      2023,
		Plot:      "A story of loyalty.",
		Genres:    []string{"历史", "动作"},
		Directors: []string{"张艺谋"},
		Actors:    []Actor{{Name: "沈腾", Role: "张大"}},
		UniqueIDs: map[string]string{"tmdb": "906217"},
		PosterURL: "https://example.com/poster.jpg",
	}

	outPath := filepath.Join(dir, "movie.nfo")
	if err := GenerateMovieNFO(meta, outPath); err != nil {
		t.Fatalf("GenerateMovieNFO: %v", err)
	}

	data, _ := os.ReadFile(outPath)
	content := string(data)

	for _, want := range []string{
		`<movie>`,
		`<title>满江红</title>`,
		`<year>2023</year>`,
		`<genre>历史</genre>`,
		`<actor>`,
		`<name>沈腾</name>`,
		`<uniqueid type="tmdb">906217</uniqueid>`,
		`aspect="poster"`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in output:\n%s", want, content)
		}
	}
}

func TestGenerateEpisodeNFO(t *testing.T) {
	dir := t.TempDir()
	meta := &Metadata{
		Title:     "Pilot",
		ShowTitle: "Breaking Bad",
		Season:    1,
		Episode:   1,
		Aired:     "2008-01-20",
		Plot:      "Walter White begins his descent.",
	}

	outPath := filepath.Join(dir, "episode.nfo")
	if err := GenerateEpisodeNFO(meta, outPath); err != nil {
		t.Fatalf("GenerateEpisodeNFO: %v", err)
	}

	data, _ := os.ReadFile(outPath)
	content := string(data)

	for _, want := range []string{
		`<episodedetails>`,
		`<title>Pilot</title>`,
		`<showtitle>Breaking Bad</showtitle>`,
		`<season>1</season>`,
		`<episode>1</episode>`,
		`<aired>2008-01-20</aired>`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in output:\n%s", want, content)
		}
	}
}

func TestGenerateTVShowNFO(t *testing.T) {
	dir := t.TempDir()
	meta := &Metadata{
		Title:  "The Wire",
		Year:   2002,
		Genres: []string{"Crime", "Drama"},
	}

	outPath := filepath.Join(dir, "tvshow.nfo")
	if err := GenerateTVShowNFO(meta, outPath); err != nil {
		t.Fatalf("GenerateTVShowNFO: %v", err)
	}

	data, _ := os.ReadFile(outPath)
	content := string(data)

	for _, want := range []string{
		`<tvshow>`,
		`<title>The Wire</title>`,
		`<year>2002</year>`,
		`<genre>Crime</genre>`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in output:\n%s", want, content)
		}
	}
}

func TestParseNFO(t *testing.T) {
	dir := t.TempDir()

	// Write then read back a movie NFO — round-trip test.
	original := &Metadata{
		Title:         "Oppenheimer",
		OriginalTitle: "Oppenheimer",
		Year:          2023,
		Plot:          "The story of J. Robert Oppenheimer.",
		Genres:        []string{"Drama", "History"},
		Directors:     []string{"Christopher Nolan"},
		Actors:        []Actor{{Name: "Cillian Murphy", Role: "Oppenheimer"}},
		UniqueIDs:     map[string]string{"tmdb": "872585"},
		PosterURL:     "https://example.com/poster.jpg",
		Set:           "Oppenheimer Collection",
	}

	outPath := filepath.Join(dir, "oppenheimer.nfo")
	if err := GenerateMovieNFO(original, outPath); err != nil {
		t.Fatalf("write: %v", err)
	}

	parsed, err := ParseNFO(outPath)
	if err != nil {
		t.Fatalf("ParseNFO: %v", err)
	}

	if parsed.Title != original.Title {
		t.Errorf("title: got %q, want %q", parsed.Title, original.Title)
	}
	if parsed.Year != original.Year {
		t.Errorf("year: got %d, want %d", parsed.Year, original.Year)
	}
	if len(parsed.Genres) != len(original.Genres) {
		t.Errorf("genres: got %v, want %v", parsed.Genres, original.Genres)
	}
	if len(parsed.Actors) == 0 || parsed.Actors[0].Name != "Cillian Murphy" {
		t.Errorf("actors: got %+v", parsed.Actors)
	}
	if parsed.UniqueIDs["tmdb"] != "872585" {
		t.Errorf("uniqueid tmdb: got %q", parsed.UniqueIDs["tmdb"])
	}
	if parsed.PosterURL != original.PosterURL {
		t.Errorf("posterURL: got %q, want %q", parsed.PosterURL, original.PosterURL)
	}
}

func TestParseNFOEpisode(t *testing.T) {
	dir := t.TempDir()
	original := &Metadata{
		Title:     "Pilot",
		ShowTitle: "Breaking Bad",
		Season:    1,
		Episode:   1,
	}
	outPath := filepath.Join(dir, "s01e01.nfo")
	if err := GenerateEpisodeNFO(original, outPath); err != nil {
		t.Fatalf("write: %v", err)
	}
	parsed, err := ParseNFO(outPath)
	if err != nil {
		t.Fatalf("ParseNFO: %v", err)
	}
	if parsed.Season != 1 || parsed.Episode != 1 {
		t.Errorf("season/episode: got %d/%d", parsed.Season, parsed.Episode)
	}
	if parsed.ShowTitle != "Breaking Bad" {
		t.Errorf("showtitle: got %q", parsed.ShowTitle)
	}
}
