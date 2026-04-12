package organizer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/newcoderlife/media115/internal/cloud115"
)

// fakeCache returns a mock cacheGet function that returns scrapeInfo for given stems.
func fakeCache(data map[string]map[string]any) func(source, key string) map[string]any {
	return func(source, key string) map[string]any {
		if source == "file_map" {
			return data[key]
		}
		return nil
	}
}

func makeEntry(path, name, parent string) cloud115.TreeEntry {
	return cloud115.TreeEntry{
		Path:    path,
		Name:    name,
		Parent:  parent,
		IsVideo: true,
		IsNFO:   false,
	}
}

func TestBuildPlanMovie(t *testing.T) {
	entries := []cloud115.TreeEntry{
		makeEntry("影音/电影/old folder/Inception.2010.mkv", "Inception.2010.mkv", "影音/电影/old folder"),
	}
	cache := fakeCache(map[string]map[string]any{
		"Inception.2010": {
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
	op := ops[0]
	if op.Action != "rename" {
		t.Errorf("expected action=rename, got %q", op.Action)
	}
	if op.NewFolder != "Inception (2010)" {
		t.Errorf("unexpected NewFolder %q", op.NewFolder)
	}
	if op.NewName != "Inception (2010).mkv" {
		t.Errorf("unexpected NewName %q", op.NewName)
	}
	if op.SourceID != "27205" {
		t.Errorf("unexpected SourceID %q", op.SourceID)
	}
}

func TestBuildPlanAV(t *testing.T) {
	// File with cut suffix: ABP-123-C.FHD.mp4
	// Should preserve -C suffix.
	entries := []cloud115.TreeEntry{
		makeEntry("影音/AV/ABP-123/ABP-123-C.FHD.mp4", "ABP-123-C.FHD.mp4", "影音/AV/ABP-123"),
	}
	cache := fakeCache(map[string]map[string]any{
		"ABP-123-C.FHD": {
			"type":   "av",
			"number": "ABP-123",
		},
	})

	ops := BuildPlan("AV", entries, cache)

	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	op := ops[0]
	if op.Action != "rename" {
		t.Errorf("expected action=rename for suffix change, got %q", op.Action)
	}
	// NewName should be ABP-123-C.mp4
	if op.NewName != "ABP-123-C.mp4" {
		t.Errorf("unexpected NewName %q (want ABP-123-C.mp4)", op.NewName)
	}
}

func TestBuildPlanAVAlreadyCorrect(t *testing.T) {
	// File already correctly named: ABP-040/ABP-040.mp4
	entries := []cloud115.TreeEntry{
		makeEntry("影音/AV/ABP-040/ABP-040.mp4", "ABP-040.mp4", "影音/AV/ABP-040"),
	}
	cache := fakeCache(map[string]map[string]any{
		"ABP-040": {
			"type":   "av",
			"number": "ABP-040",
		},
	})

	ops := BuildPlan("AV", entries, cache)

	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	if ops[0].Action != "skip" {
		t.Errorf("expected skip for already-correct AV, got %q", ops[0].Action)
	}
}

func TestBuildPlanTV(t *testing.T) {
	// The file has a different name than the target — so both folder and file should be renamed.
	entries := []cloud115.TreeEntry{
		makeEntry("影音/剧目/random folder/breaking.bad.s01e01.mkv", "breaking.bad.s01e01.mkv", "影音/剧目/random folder"),
	}
	cache := fakeCache(map[string]map[string]any{
		"breaking.bad.s01e01": {
			"type":      "tv",
			"showtitle": "Breaking Bad",
			"year":      float64(2008),
			"season":    float64(1),
			"episode":   float64(1),
			"tmdb_id":   float64(1396),
		},
	})

	ops := BuildPlan("剧目", entries, cache)

	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	op := ops[0]
	if op.Action != "rename" {
		t.Errorf("expected action=rename, got %q", op.Action)
	}
	if op.NewFolder != "Breaking Bad (2008)" {
		t.Errorf("unexpected NewFolder %q", op.NewFolder)
	}
	if op.NewName != "Breaking Bad S01E01.mkv" {
		t.Errorf("unexpected NewName %q (got %q)", op.NewName, op.NewName)
	}
}

func TestBuildPlanTVFolderOnlyRename(t *testing.T) {
	// The file has the correct episode name already; only folder needs renaming.
	entries := []cloud115.TreeEntry{
		makeEntry("影音/剧目/random folder/Breaking Bad S01E01.mkv", "Breaking Bad S01E01.mkv", "影音/剧目/random folder"),
	}
	cache := fakeCache(map[string]map[string]any{
		"Breaking Bad S01E01": {
			"type":      "tv",
			"showtitle": "Breaking Bad",
			"year":      float64(2008),
			"season":    float64(1),
			"episode":   float64(1),
			"tmdb_id":   float64(1396),
		},
	})

	ops := BuildPlan("剧目", entries, cache)

	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	op := ops[0]
	if op.Action != "rename" {
		t.Errorf("expected action=rename (folder move), got %q", op.Action)
	}
	if op.NewFolder != "Breaking Bad (2008)" {
		t.Errorf("unexpected NewFolder %q", op.NewFolder)
	}
	// NewName should be empty — file already has the correct name.
	if op.NewName != "" {
		t.Errorf("expected empty NewName (file already correct), got %q", op.NewName)
	}
}

func TestBuildPlanSkip(t *testing.T) {
	// File already in standard format with no scrape info.
	entries := []cloud115.TreeEntry{
		makeEntry("影音/电影/Inception (2010)/Inception (2010).mkv", "Inception (2010).mkv", "影音/电影/Inception (2010)"),
	}
	cache := fakeCache(map[string]map[string]any{}) // no cache entries

	ops := BuildPlan("电影", entries, cache)

	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	op := ops[0]
	if op.Action != "skip" {
		t.Errorf("expected skip, got %q", op.Action)
	}
	if op.Reason != "already in standard format" {
		t.Errorf("unexpected reason %q", op.Reason)
	}
}

// TestBuildPlanWithFullFileMap verifies that BuildPlan correctly reads a
// file_map cache entry with all required fields and produces the right rename op.
func TestBuildPlanWithFullFileMap(t *testing.T) {
	// Create a temp dir with file_map JSON files.
	tmpDir := t.TempDir()
	fmDir := filepath.Join(tmpDir, "file_map")
	if err := os.MkdirAll(fmDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write a movie file_map entry for stem "Test.Movie.2024".
	movieFM := map[string]any{
		"type":       "movie",
		"title":      "测试电影",
		"year":       float64(2024),
		"tmdb_id":    float64(12345),
		"_cached_at": float64(time.Now().Unix()),
	}
	data, err := json.Marshal(movieFM)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fmDir, "Test.Movie.2024.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Tree entries: one video file under the 电影 category.
	entries := []cloud115.TreeEntry{
		{
			Path:    "电影/TestDir/Test.Movie.2024.mkv",
			Name:    "Test.Movie.2024.mkv",
			Parent:  "电影/TestDir",
			IsVideo: true,
		},
	}

	// cacheGet reads JSON from our temp dir.
	cacheGet := func(source, key string) map[string]any {
		path := filepath.Join(fmDir, key+".json")
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		var result map[string]any
		if jsonErr := json.Unmarshal(b, &result); jsonErr != nil {
			return nil
		}
		return result
	}

	ops := BuildPlan("电影", entries, cacheGet)

	// Should have exactly one rename op.
	if len(ops) != 1 {
		t.Fatalf("expected 1 op, got %d", len(ops))
	}
	op := ops[0]

	if op.File != "Test.Movie.2024.mkv" {
		t.Errorf("File = %q, want Test.Movie.2024.mkv", op.File)
	}
	if op.Action != "rename" {
		t.Errorf("Action = %q, want rename", op.Action)
	}
	if op.NewFolder != "测试电影 (2024)" {
		t.Errorf("NewFolder = %q, want 测试电影 (2024)", op.NewFolder)
	}
	if op.NewName != "测试电影 (2024).mkv" {
		t.Errorf("NewName = %q, want 测试电影 (2024).mkv", op.NewName)
	}
}

func TestBuildPlanTVWithStringYear(t *testing.T) {
	// Regression: Python file_map stores year as string "2021"
	entries := []cloud115.TreeEntry{
		{Path: "剧目/Show (2021)/Show S01E01.mkv", Name: "Show S01E01.mkv",
			Parent: "剧目/Show (2021)", IsVideo: true},
	}
	cacheGet := func(source, key string) map[string]any {
		if key == "Show S01E01" {
			return map[string]any{
				"type": "tv", "showtitle": "Show", "title": "Ep1",
				"year": "2021", "season": float64(1), "episode": float64(1),
			}
		}
		return nil
	}
	ops := BuildPlan("剧目", entries, cacheGet)
	for _, op := range ops {
		if op.File == "Show S01E01.mkv" {
			if op.Action != "skip" {
				// Already in correct folder "Show (2021)" with correct name
				if op.NewFolder != "" && op.NewFolder != "Show (2021)" {
					t.Errorf("unexpected NewFolder: %q", op.NewFolder)
				}
			}
			return
		}
	}
	t.Fatal("expected op for Show S01E01.mkv")
}

func TestExtractAVSuffix(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"-C", "-C"},
		{"-C.FHD", "-C"},
		{"A.FHD", ".A"},
		{".A", ".A"},
		{"B_4K^WM", ".B"},
		{".Part1", ".Part1"},
		{"Part2.HD", ".Part2"},
		{"_CD1", ".CD1"},
		{".FHD", ""},
		{".HD", ""},
		{"", ""},
		{".2160p.DMM.WEB-DL", ""},
	}
	for _, c := range cases {
		got := ExtractAVSuffix(c.in)
		if got != c.want {
			t.Errorf("ExtractAVSuffix(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
