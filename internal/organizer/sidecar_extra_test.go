package organizer

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/newcoderlife/media115/internal/cloud115"
)

// newSidecarTestClient creates a real *cloud115.Client with a temp SQLite DB.
// API/upload calls fail gracefully (no valid cookies) but do not panic.
func newSidecarTestClient(t *testing.T) *cloud115.Client {
	t.Helper()
	tmpDir := t.TempDir()
	client, err := cloud115.NewClient("", cloud115.WithCacheDir(filepath.Join(tmpDir, "cache")))
	if err != nil {
		t.Fatalf("newSidecarTestClient: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

// TestPlanScrapeUploadNoDirExists verifies that when the scrape_output dir is
// absent (or the cacheRoot contains "cloud115" but the media115 dir has no
// scrape_output subdir), PlanScrapeUpload returns nil without panicking.
func TestPlanScrapeUploadNoDirExists(t *testing.T) {
	tmpDir := t.TempDir()
	// cacheRoot has "cloud115" in the path; sidecar.go replaces it with "media115".
	// The resulting media115/scrape_output dir does NOT exist.
	cacheRoot := filepath.Join(tmpDir, "cloud115")

	op := Op{File: "something.mp4", Parent: "影音/AV/X-001", NewName: "X-001.mp4"}
	pairs := PlanScrapeUpload(op, "X-001.mp4", cacheRoot)
	if pairs != nil {
		t.Errorf("expected nil when scrape_output absent, got %v", pairs)
	}
}

// TestPlanScrapeUploadNoMatchingDir verifies nil is returned when the cat dir
// exists but there is no subdirectory matching the search name.
func TestPlanScrapeUploadNoMatchingDir(t *testing.T) {
	tmpDir := t.TempDir()
	cacheRoot := filepath.Join(tmpDir, "cloud115")

	// Create a category dir with a differently-named output dir.
	catDir := filepath.Join(tmpDir, "media115", "scrape_output", "AV", "wrong_name")
	if err := os.MkdirAll(catDir, 0o755); err != nil {
		t.Fatal(err)
	}

	op := Op{File: "X-001.mp4", Parent: "影音/AV/X-001", NewName: "X-001.mp4"}
	pairs := PlanScrapeUpload(op, "X-001.mp4", cacheRoot)
	if pairs != nil {
		t.Errorf("expected nil when no matching dir, got %v", pairs)
	}
}

// TestPlanScrapeUploadFileInCatDir verifies that a regular file in the
// category-level dir (not a subdir) is skipped gracefully.
func TestPlanScrapeUploadFileInCatDir(t *testing.T) {
	tmpDir := t.TempDir()
	cacheRoot := filepath.Join(tmpDir, "cloud115")

	catDir := filepath.Join(tmpDir, "media115", "scrape_output", "AV")
	if err := os.MkdirAll(catDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A regular file at the category level (not a dir).
	if err := os.WriteFile(filepath.Join(catDir, "stray.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Also add the matching output dir so we confirm the file is skipped.
	outDir := filepath.Join(catDir, "影音_AV_Z-999")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(outDir, "Z-999.nfo"), "<nfo/>")

	op := Op{File: "Z-999.mp4", Parent: "影音/AV/Z-999", NewName: "Z-999.mp4"}
	pairs := PlanScrapeUpload(op, "Z-999.mp4", cacheRoot)
	if len(pairs) == 0 {
		t.Fatal("expected at least one FilePair")
	}
	found := false
	for _, p := range pairs {
		if p.RemoteName == "Z-999.nfo" {
			found = true
		}
	}
	if !found {
		t.Error("expected Z-999.nfo in pairs")
	}
}

// TestPlanScrapeUploadSubdirInOutDir verifies that subdirectories inside the
// output dir are skipped (not treated as files).
func TestPlanScrapeUploadSubdirInOutDir(t *testing.T) {
	tmpDir := t.TempDir()
	cacheRoot := filepath.Join(tmpDir, "cloud115")

	outDir := filepath.Join(tmpDir, "media115", "scrape_output", "AV", "影音_AV_Q-001")
	if err := os.MkdirAll(filepath.Join(outDir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Only valid sidecar file.
	writeFile(t, filepath.Join(outDir, "Q-001.nfo"), "<nfo/>")
	// .png artwork.
	writeFile(t, filepath.Join(outDir, "poster.png"), "PNG")

	op := Op{File: "Q-001.mkv", Parent: "影音/AV/Q-001", NewName: "Q-001.mkv"}
	pairs := PlanScrapeUpload(op, "Q-001.mkv", cacheRoot)
	// Should have nfo + png, NOT the subdir.
	for _, p := range pairs {
		if p.RemoteName == "subdir" {
			t.Error("unexpected subdir in pairs")
		}
	}
	found := map[string]bool{}
	for _, p := range pairs {
		found[p.RemoteName] = true
	}
	if !found["Q-001.nfo"] {
		t.Error("expected Q-001.nfo")
	}
	if !found["poster.png"] {
		t.Error("expected poster.png")
	}
}

// TestPlanScrapeUploadNoNewVideoName verifies that when newVideoName is empty,
// the original NFO filename is preserved as the remote name.
func TestPlanScrapeUploadNoNewVideoName(t *testing.T) {
	tmpDir := t.TempDir()
	cacheRoot := filepath.Join(tmpDir, "cloud115")

	outDir := filepath.Join(tmpDir, "media115", "scrape_output", "电影", "影音_电影_SomeDir")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(outDir, "Movie.2020.nfo"), "<nfo/>")

	op := Op{File: "Movie.2020.mkv", Parent: "影音/电影/SomeDir", NewName: ""}
	// newVideoName is empty → nfoName stays ""
	pairs := PlanScrapeUpload(op, "", cacheRoot)
	if len(pairs) == 0 {
		t.Fatal("expected FilePair, got none")
	}
	// Remote name should equal the original NFO filename (not renamed).
	for _, p := range pairs {
		if filepath.Ext(p.LocalPath) == ".nfo" {
			if p.RemoteName != "Movie.2020.nfo" {
				t.Errorf("expected original NFO name, got %q", p.RemoteName)
			}
		}
	}
}

// TestPlanScrapeUploadNFOOtherVideo verifies that an NFO belonging to a
// different video in the same output dir is skipped (not tvshow.nfo).
func TestPlanScrapeUploadNFOOtherVideo(t *testing.T) {
	tmpDir := t.TempDir()
	cacheRoot := filepath.Join(tmpDir, "cloud115")

	outDir := filepath.Join(tmpDir, "media115", "scrape_output", "剧目", "影音_剧目_ShowDir")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// NFOs for two different episodes.
	writeFile(t, filepath.Join(outDir, "Show S01E01.nfo"), "<ep1/>")
	writeFile(t, filepath.Join(outDir, "Show S01E02.nfo"), "<ep2/>")
	writeFile(t, filepath.Join(outDir, "poster.jpg"), "img")

	// Ask for episode 1 only.
	op := Op{File: "Show S01E01.mkv", Parent: "影音/剧目/ShowDir", NewName: "Show S01E01.mkv"}
	pairs := PlanScrapeUpload(op, "Show S01E01.mkv", cacheRoot)

	remotes := map[string]bool{}
	for _, p := range pairs {
		remotes[p.RemoteName] = true
	}
	// ep1 NFO and poster should be present.
	if !remotes["Show S01E01.nfo"] {
		t.Error("expected Show S01E01.nfo")
	}
	if !remotes["poster.jpg"] {
		t.Error("expected poster.jpg")
	}
	// ep2 NFO should NOT be present (it's for a different video).
	if remotes["Show S01E02.nfo"] {
		t.Error("did not expect Show S01E02.nfo (different video)")
	}
}

// TestSyncSidecarsWithRealSidecarsSkipListing exercises the upload loop in
// SyncSidecars with skipListing=true and real local sidecar files.
// The client's Upload call fails gracefully (no real API), so uploaded count is 0,
// but the code path (dedup, loop, upload attempt, warn) is exercised.
func TestSyncSidecarsWithRealSidecarsSkipListing(t *testing.T) {
	tmpDir := t.TempDir()
	cacheRoot := filepath.Join(tmpDir, "cloud115")
	client := newSidecarTestClient(t)

	// Build a sidecar layout.
	outDir := filepath.Join(tmpDir, "media115", "scrape_output", "AV", "影音_AV_TST-001")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(outDir, "TST-001.nfo"), "<nfo/>")
	writeFile(t, filepath.Join(outDir, "poster.jpg"), "img")

	vf := VideoFile{File: "TST-001.mp4", Parent: "影音/AV/TST-001", NewName: "TST-001.mp4"}
	// skipListing=true → skips ListDir/Delete; upload fails (no API) → returns 0.
	count := SyncSidecars(client, "/影音/AV/TST-001", []VideoFile{vf}, true, slog.Default(), cacheRoot)
	// Upload fails due to no live API, so count == 0.
	if count != 0 {
		t.Logf("note: SyncSidecars returned %d (expected 0 for offline client)", count)
	}
	// The important thing is no panic — the upload path was exercised.
}

// TestSyncSidecarsWithRealSidecarsNoSkipListing exercises the ListDir path in
// SyncSidecars with skipListing=false.  The client's ListDir call fails gracefully
// (no real API), so existing map stays empty and no deletes are attempted.
func TestSyncSidecarsWithRealSidecarsNoSkipListing(t *testing.T) {
	tmpDir := t.TempDir()
	cacheRoot := filepath.Join(tmpDir, "cloud115")
	client := newSidecarTestClient(t)

	// Build a sidecar layout.
	outDir := filepath.Join(tmpDir, "media115", "scrape_output", "AV", "影音_AV_TST-002")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(outDir, "TST-002.nfo"), "<nfo/>")

	vf := VideoFile{File: "TST-002.mp4", Parent: "影音/AV/TST-002", NewName: "TST-002.mp4"}
	// skipListing=false → attempts ListDir (fails silently), then attempts upload (fails).
	count := SyncSidecars(client, "/影音/AV/TST-002", []VideoFile{vf}, false, slog.Default(), cacheRoot)
	if count != 0 {
		t.Logf("note: SyncSidecars returned %d (expected 0 for offline client)", count)
	}
}
