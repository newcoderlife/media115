package organizer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPlanScrapeUpload(t *testing.T) {
	// Build a temporary scrape_output layout:
	// cacheRoot/scrape_output/AV/影音_AV_ABP-040/{ABP-040.nfo,poster.jpg}
	tmpDir := t.TempDir()
	// cacheRoot is the cloud115 dir; sidecar.go replaces "cloud115" → "media115"
	// so we set tmpDir as the "media115" root.
	cacheRoot := filepath.Join(tmpDir, "cloud115")

	outputDir := filepath.Join(tmpDir, "media115", "scrape_output", "AV", "影音_AV_ABP-040")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(outputDir, "ABP-040.nfo"), "<nfo/>")
	writeFile(t, filepath.Join(outputDir, "poster.jpg"), "JPEG")

	op := Op{
		File:    "ABP-040.mp4",
		Parent:  "影音/AV/ABP-040",
		NewName: "ABP-040.mp4",
	}
	pairs := PlanScrapeUpload(op, "ABP-040.mp4", cacheRoot)

	if len(pairs) == 0 {
		t.Fatal("expected at least one FilePair, got none")
	}

	found := map[string]bool{}
	for _, p := range pairs {
		found[p.RemoteName] = true
	}
	if !found["ABP-040.nfo"] {
		t.Error("expected ABP-040.nfo in pairs")
	}
	if !found["poster.jpg"] {
		t.Error("expected poster.jpg in pairs")
	}
}

func TestPlanScrapeUploadRenamesNFO(t *testing.T) {
	// When newVideoName differs from original, NFO should be renamed.
	tmpDir := t.TempDir()
	cacheRoot := filepath.Join(tmpDir, "cloud115")

	outputDir := filepath.Join(tmpDir, "media115", "scrape_output", "电影", "影音_电影_olddir")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(outputDir, "Inception.2010.nfo"), "<nfo/>")

	op := Op{
		File:    "Inception.2010.mkv",
		Parent:  "影音/电影/olddir",
		NewName: "Inception (2010).mkv",
	}
	pairs := PlanScrapeUpload(op, "Inception (2010).mkv", cacheRoot)

	if len(pairs) == 0 {
		t.Fatal("expected FilePair, got none")
	}
	// The NFO remote name should be derived from the new video name.
	for _, p := range pairs {
		if filepath.Ext(p.LocalPath) == ".nfo" {
			if p.RemoteName != "Inception (2010).nfo" {
				t.Errorf("expected remote NFO name 'Inception (2010).nfo', got %q", p.RemoteName)
			}
		}
	}
}

func TestSyncSidecarsSkipListing(t *testing.T) {
	// When skipListing=true, SyncSidecars should not call ListDir (use a nil
	// client and confirm no panic if there are no local sidecar files).
	// With no local sidecar files, SyncSidecars returns 0 without touching the client.
	tmpDir := t.TempDir()
	cacheRoot := filepath.Join(tmpDir, "cloud115")

	vf := VideoFile{File: "nonexistent.mp4", Parent: "影音/AV/X-999", NewName: ""}
	// Client is nil — if SyncSidecars tried to call ListDir it would panic.
	count := SyncSidecars(nil, "/影音/AV/X-999", []VideoFile{vf}, true, nil, cacheRoot)
	if count != 0 {
		t.Errorf("expected 0 uploads, got %d", count)
	}
}

// TestSyncSidecarsMultipleVideos verifies that PlanScrapeUpload correctly
// handles multiple video files sharing the same sidecar directory, and that
// when results are deduplicated the shared files appear only once.
func TestSyncSidecarsMultipleVideos(t *testing.T) {
	tmpDir := t.TempDir()
	// cacheRoot is the "cloud115" path; sidecar.go replaces "cloud115" → "media115".
	cacheRoot := filepath.Join(tmpDir, "cloud115")

	// Build the scrape_output dir for the show directory.
	// Parent for both episodes is "剧目/ShowDir", so searchName = "剧目_ShowDir".
	outputDir := filepath.Join(tmpDir, "media115", "scrape_output", "剧目", "剧目_ShowDir")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(outputDir, "Show S01E01.nfo"), "<ep1/>")
	writeFile(t, filepath.Join(outputDir, "Show S01E02.nfo"), "<ep2/>")
	writeFile(t, filepath.Join(outputDir, "tvshow.nfo"), "<tvshow/>")
	writeFile(t, filepath.Join(outputDir, "poster.jpg"), "img")

	// Plan sidecar uploads for each episode.
	pairs1 := PlanScrapeUpload(
		Op{File: "Show S01E01.mkv", Parent: "剧目/ShowDir", NewName: "Show S01E01.mkv"},
		"Show S01E01.mkv",
		cacheRoot,
	)
	pairs2 := PlanScrapeUpload(
		Op{File: "Show S01E02.mkv", Parent: "剧目/ShowDir", NewName: "Show S01E02.mkv"},
		"Show S01E02.mkv",
		cacheRoot,
	)

	// Deduplicate by remote name (simulating SyncSidecars dedup logic).
	allPairs := map[string]bool{}
	for _, p := range pairs1 {
		allPairs[p.RemoteName] = true
	}
	for _, p := range pairs2 {
		allPairs[p.RemoteName] = true
	}

	// Expected unique remote files: Show S01E01.nfo, Show S01E02.nfo, tvshow.nfo, poster.jpg
	const wantUnique = 4
	if len(allPairs) != wantUnique {
		t.Fatalf("expected %d unique files after dedup, got %d: %v", wantUnique, len(allPairs), allPairs)
	}

	for _, name := range []string{"Show S01E01.nfo", "Show S01E02.nfo", "tvshow.nfo", "poster.jpg"} {
		if !allPairs[name] {
			t.Errorf("expected %q in combined pairs, but it was absent", name)
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
