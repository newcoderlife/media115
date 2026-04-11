package scraper

import (
	"testing"
)

func TestAnalyzeAV(t *testing.T) {
	r := AnalyzeFilename("ABP-040.HD.wmv")
	if r.MediaType != "av" {
		t.Errorf("expected av, got %q (title=%q)", r.MediaType, r.Title)
	}
	if r.Title != "ABP-040" {
		t.Errorf("expected title ABP-040, got %q", r.Title)
	}
}

func TestAnalyzeGravure(t *testing.T) {
	r := AnalyzeFilename("尾野寺みさ IMBD-334.mp4")
	if r.MediaType != "gravure" {
		t.Errorf("expected gravure, got %q (title=%q)", r.MediaType, r.Title)
	}
	if r.Title != "IMBD-334" {
		t.Errorf("expected title IMBD-334, got %q", r.Title)
	}
}

func TestAnalyzeWestern(t *testing.T) {
	r := AnalyzeFilename("Vixen - Lana - Title.mp4")
	if r.MediaType != "av_west" {
		t.Errorf("expected av_west, got %q", r.MediaType)
	}
}

func TestAnalyzeTV(t *testing.T) {
	r := AnalyzeFilename("Show S01E01.mkv")
	if r.MediaType != "tv" {
		t.Errorf("expected tv, got %q", r.MediaType)
	}
	if r.Season != 1 {
		t.Errorf("expected season 1, got %d", r.Season)
	}
	if r.Episode != 1 {
		t.Errorf("expected episode 1, got %d", r.Episode)
	}
}

func TestAnalyzeMovie(t *testing.T) {
	r := AnalyzeFilename("Title (2024).mkv")
	if r.MediaType != "movie" {
		t.Errorf("expected movie, got %q", r.MediaType)
	}
	if r.Year != 2024 {
		t.Errorf("expected year 2024, got %d", r.Year)
	}
	if r.Confidence != "high" {
		t.Errorf("expected confidence high, got %q", r.Confidence)
	}
}

func TestAnalyzeUnknown(t *testing.T) {
	r := AnalyzeFilename("random.mkv")
	if r.MediaType != "unknown" {
		t.Errorf("expected unknown, got %q", r.MediaType)
	}
}

// Additional coverage tests.

func TestAnalyzeFC2(t *testing.T) {
	r := AnalyzeFilename("FC2-PPV-1234567.mp4")
	if r.MediaType != "av" {
		t.Errorf("expected av for FC2-PPV, got %q", r.MediaType)
	}
}

func TestAnalyzeAnime(t *testing.T) {
	r := AnalyzeFilename("[SubGroup] Show Name - 05 [1080p].mkv")
	if r.MediaType != "anime" {
		t.Errorf("expected anime, got %q", r.MediaType)
	}
	if r.Episode != 5 {
		t.Errorf("expected episode 5, got %d", r.Episode)
	}
}

func TestAnalyzeMovieDot(t *testing.T) {
	r := AnalyzeFilename("Oppenheimer.2023.1080p.mkv")
	if r.MediaType != "movie" {
		t.Errorf("expected movie, got %q", r.MediaType)
	}
	if r.Year != 2023 {
		t.Errorf("expected year 2023, got %d", r.Year)
	}
}
