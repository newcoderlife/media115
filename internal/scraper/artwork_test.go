package scraper

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSavePosterDefaultSizeAndFilename(t *testing.T) {
	tmp := t.TempDir()
	// Pre-create the output file so download is skipped (force=false in SavePoster)
	posterPath := filepath.Join(tmp, "poster.jpg")
	if err := os.WriteFile(posterPath, []byte("existing poster"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Both size and filename are empty → defaults to "w500" and "poster.jpg"
	err := SavePoster("/abc.jpg", tmp, "", "")
	if err != nil {
		t.Errorf("SavePoster with existing poster.jpg: %v", err)
	}
}

func TestSavePosterCustomSizeAndFilename(t *testing.T) {
	tmp := t.TempDir()
	customPath := filepath.Join(tmp, "custom.jpg")
	if err := os.WriteFile(customPath, []byte("custom poster"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := SavePoster("/abc.jpg", tmp, "w200", "custom.jpg")
	if err != nil {
		t.Errorf("SavePoster with custom size/filename: %v", err)
	}
}
