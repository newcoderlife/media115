package scraper

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const tmdbImageBase = "https://image.tmdb.org/t/p"

// DownloadImage fetches url and saves it to outputPath.
// If force is false and outputPath already exists, the download is skipped.
// timeout controls the HTTP request deadline; pass 0 to use the default (30s).
func DownloadImage(url, outputPath string, timeout time.Duration, force bool) error {
	if !force {
		if _, err := os.Stat(outputPath); err == nil {
			return nil // already exists, skip
		}
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("DownloadImage %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("DownloadImage %s: HTTP %d", url, resp.StatusCode)
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// SavePoster builds a TMDB poster URL from imagePath (e.g. "/abc.jpg"),
// downloads it, and saves it as filename inside outDir.
// size defaults to "w500" when empty.
func SavePoster(imagePath, outDir, size, filename string) error {
	if size == "" {
		size = "w500"
	}
	if filename == "" {
		filename = "poster.jpg"
	}
	url := fmt.Sprintf("%s/%s%s", tmdbImageBase, size, imagePath)
	return DownloadImage(url, filepath.Join(outDir, filename), 0, false)
}

// TMDBImageURL returns the full TMDB image URL for a file path.
func TMDBImageURL(filePath, size string) string {
	if filePath == "" {
		return ""
	}
	if size == "" {
		size = "original"
	}
	return fmt.Sprintf("%s/%s%s", tmdbImageBase, size, filePath)
}
