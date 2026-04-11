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

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
