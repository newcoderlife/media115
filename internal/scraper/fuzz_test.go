package scraper

import "testing"

func FuzzParseNFOBytes(f *testing.F) {
	f.Add([]byte(`<?xml version="1.0" encoding="UTF-8"?><movie><title>Test</title><year>2023</year></movie>`))
	f.Add([]byte(`<episodedetails><title>Pilot</title><season>1</season><episode>1</episode></episodedetails>`))
	f.Add([]byte(`<tvshow><title>Show</title></tvshow>`))
	f.Add([]byte(`<movie></movie>`))
	f.Add([]byte(``))

	f.Fuzz(func(t *testing.T, data []byte) {
		// parseNFOBytes must not panic on any input.
		_, _ = parseNFOBytes(data)
	})
}

func FuzzAnalyzeFilename(f *testing.F) {
	f.Add("ABP-040.HD.wmv")
	f.Add("满江红 (2023).mkv")
	f.Add("Show S01E01.mkv")
	f.Add("Vixen - Lana - Title.mp4")
	f.Add("尾野寺みさ IMBD-334.mp4")
	f.Add("FC2-PPV-1234567.mp4")
	f.Add("")
	f.Add("Studio.24.01.15.Performer.XXX.1080p.mp4")

	f.Fuzz(func(t *testing.T, filename string) {
		// AnalyzeFilename must not panic on any input.
		AnalyzeFilename(filename)
	})
}
