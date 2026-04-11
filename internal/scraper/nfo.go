package scraper

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// xmlActor is used for NFO actor elements.
type xmlActor struct {
	Name  string `xml:"name,omitempty"`
	Role  string `xml:"role,omitempty"`
	Thumb string `xml:"thumb,omitempty"`
}

// xmlUniqueID is used for NFO uniqueid elements.
type xmlUniqueID struct {
	Type  string `xml:"type,attr"`
	Value string `xml:",chardata"`
}

// xmlSet is the set/collection element.
type xmlSet struct {
	Name string `xml:"name,omitempty"`
}

// xmlThumb carries the poster URL and aspect attribute.
type xmlThumb struct {
	Aspect string `xml:"aspect,attr,omitempty"`
	Value  string `xml:",chardata"`
}

// xmlFanart carries the fanart element with nested thumbs.
type xmlFanart struct {
	Thumbs []string `xml:"thumb"`
}

// movieNFO is the Kodi movie NFO structure.
type movieNFO struct {
	XMLName       xml.Name      `xml:"movie"`
	Title         string        `xml:"title,omitempty"`
	OriginalTitle string        `xml:"originaltitle,omitempty"`
	Year          string        `xml:"year,omitempty"`
	Premiered     string        `xml:"premiered,omitempty"`
	Plot          string        `xml:"plot,omitempty"`
	Tagline       string        `xml:"tagline,omitempty"`
	Runtime       string        `xml:"runtime,omitempty"`
	Rating        string        `xml:"rating,omitempty"`
	Votes         string        `xml:"votes,omitempty"`
	Set           *xmlSet       `xml:"set,omitempty"`
	Studios       []string      `xml:"studio"`
	Genres        []string      `xml:"genre"`
	Tags          []string      `xml:"tag"`
	Countries     []string      `xml:"country"`
	Credits       []string      `xml:"credits"`
	Directors     []string      `xml:"director"`
	Actors        []xmlActor    `xml:"actor"`
	UniqueIDs     []xmlUniqueID `xml:"uniqueid"`
	Thumb         *xmlThumb     `xml:"thumb,omitempty"`
	Fanart        *xmlFanart    `xml:"fanart,omitempty"`
}

// episodeNFO is the Kodi episodedetails NFO structure.
type episodeNFO struct {
	XMLName   xml.Name      `xml:"episodedetails"`
	Title     string        `xml:"title,omitempty"`
	ShowTitle string        `xml:"showtitle,omitempty"`
	Season    string        `xml:"season,omitempty"`
	Episode   string        `xml:"episode,omitempty"`
	Plot      string        `xml:"plot,omitempty"`
	Aired     string        `xml:"aired,omitempty"`
	Rating    string        `xml:"rating,omitempty"`
	Votes     string        `xml:"votes,omitempty"`
	Runtime   string        `xml:"runtime,omitempty"`
	Directors []string      `xml:"director"`
	Credits   []string      `xml:"credits"`
	Actors    []xmlActor    `xml:"actor"`
	UniqueIDs []xmlUniqueID `xml:"uniqueid"`
}

// tvshowNFO is the Kodi tvshow NFO structure.
type tvshowNFO struct {
	XMLName       xml.Name      `xml:"tvshow"`
	Title         string        `xml:"title,omitempty"`
	OriginalTitle string        `xml:"originaltitle,omitempty"`
	ShowTitle     string        `xml:"showtitle,omitempty"`
	Year          string        `xml:"year,omitempty"`
	Plot          string        `xml:"plot,omitempty"`
	Rating        string        `xml:"rating,omitempty"`
	Votes         string        `xml:"votes,omitempty"`
	Premiered     string        `xml:"premiered,omitempty"`
	Studios       []string      `xml:"studio"`
	Genres        []string      `xml:"genre"`
	Tags          []string      `xml:"tag"`
	Actors        []xmlActor    `xml:"actor"`
	UniqueIDs     []xmlUniqueID `xml:"uniqueid"`
	Thumb         *xmlThumb     `xml:"thumb,omitempty"`
	Fanart        *xmlFanart    `xml:"fanart,omitempty"`
}

// ── Public API ────────────────────────────────────────────────────────────────

// GenerateMovieNFO writes a Kodi-standard movie NFO to outputPath.
func GenerateMovieNFO(meta *Metadata, outputPath string) error {
	nfo := movieNFO{
		Title:         meta.Title,
		OriginalTitle: meta.OriginalTitle,
		Plot:          meta.Plot,
		Tagline:       meta.Tagline,
		Premiered:     meta.Premiered,
		Genres:        meta.Genres,
		Tags:          meta.Tags,
		Countries:     meta.Countries,
		Studios:       meta.Studios,
		Credits:       meta.Credits,
		Directors:     meta.Directors,
	}
	if meta.Year != 0 {
		nfo.Year = strconv.Itoa(meta.Year)
	}
	if meta.Runtime != 0 {
		nfo.Runtime = strconv.Itoa(meta.Runtime)
	}
	if meta.Rating != 0 {
		nfo.Rating = strconv.FormatFloat(meta.Rating, 'f', -1, 64)
	}
	if meta.Votes != 0 {
		nfo.Votes = strconv.Itoa(meta.Votes)
	}
	if meta.Set != "" {
		nfo.Set = &xmlSet{Name: meta.Set}
	}
	for _, a := range meta.Actors {
		nfo.Actors = append(nfo.Actors, xmlActor{Name: a.Name, Role: a.Role, Thumb: a.Thumb})
	}
	for k, v := range meta.UniqueIDs {
		nfo.UniqueIDs = append(nfo.UniqueIDs, xmlUniqueID{Type: k, Value: v})
	}
	if meta.PosterURL != "" {
		nfo.Thumb = &xmlThumb{Aspect: "poster", Value: meta.PosterURL}
	}
	if meta.FanartURL != "" {
		nfo.Fanart = &xmlFanart{Thumbs: []string{meta.FanartURL}}
	}
	return writeNFO(nfo, outputPath)
}

// GenerateEpisodeNFO writes a Kodi-standard episodedetails NFO to outputPath.
func GenerateEpisodeNFO(meta *Metadata, outputPath string) error {
	nfo := episodeNFO{
		Title:     meta.Title,
		ShowTitle: meta.ShowTitle,
		Plot:      meta.Plot,
		Aired:     meta.Aired,
		Directors: meta.Directors,
		Credits:   meta.Credits,
	}
	if meta.Season != 0 {
		nfo.Season = strconv.Itoa(meta.Season)
	}
	if meta.Episode != 0 {
		nfo.Episode = strconv.Itoa(meta.Episode)
	}
	if meta.Rating != 0 {
		nfo.Rating = strconv.FormatFloat(meta.Rating, 'f', -1, 64)
	}
	if meta.Votes != 0 {
		nfo.Votes = strconv.Itoa(meta.Votes)
	}
	if meta.Runtime != 0 {
		nfo.Runtime = strconv.Itoa(meta.Runtime)
	}
	for _, a := range meta.Actors {
		nfo.Actors = append(nfo.Actors, xmlActor{Name: a.Name, Role: a.Role, Thumb: a.Thumb})
	}
	for k, v := range meta.UniqueIDs {
		nfo.UniqueIDs = append(nfo.UniqueIDs, xmlUniqueID{Type: k, Value: v})
	}
	return writeNFO(nfo, outputPath)
}

// GenerateTVShowNFO writes a Kodi-standard tvshow NFO to outputPath.
func GenerateTVShowNFO(meta *Metadata, outputPath string) error {
	showTitle := meta.ShowTitle
	if showTitle == "" {
		showTitle = meta.Title
	}
	nfo := tvshowNFO{
		Title:         meta.Title,
		OriginalTitle: meta.OriginalTitle,
		ShowTitle:     showTitle,
		Premiered:     meta.Premiered,
		Plot:          meta.Plot,
		Studios:       meta.Studios,
		Genres:        meta.Genres,
		Tags:          meta.Tags,
	}
	if meta.Year != 0 {
		nfo.Year = strconv.Itoa(meta.Year)
	}
	if meta.Rating != 0 {
		nfo.Rating = strconv.FormatFloat(meta.Rating, 'f', -1, 64)
	}
	if meta.Votes != 0 {
		nfo.Votes = strconv.Itoa(meta.Votes)
	}
	for _, a := range meta.Actors {
		nfo.Actors = append(nfo.Actors, xmlActor{Name: a.Name, Role: a.Role, Thumb: a.Thumb})
	}
	for k, v := range meta.UniqueIDs {
		nfo.UniqueIDs = append(nfo.UniqueIDs, xmlUniqueID{Type: k, Value: v})
	}
	if meta.PosterURL != "" {
		nfo.Thumb = &xmlThumb{Aspect: "poster", Value: meta.PosterURL}
	}
	if meta.FanartURL != "" {
		nfo.Fanart = &xmlFanart{Thumbs: []string{meta.FanartURL}}
	}
	return writeNFO(nfo, outputPath)
}

// ParseNFO reads a Kodi NFO file (movie, episodedetails, or tvshow) and
// returns the embedded metadata.
func ParseNFO(nfoPath string) (*Metadata, error) {
	data, err := os.ReadFile(nfoPath)
	if err != nil {
		return nil, fmt.Errorf("ParseNFO: read %s: %w", nfoPath, err)
	}
	return parseNFOBytes(data)
}

// StemFilename returns the base name without extension.
func StemFilename(filename string) string {
	base := filepath.Base(filename)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// ── Internal helpers ──────────────────────────────────────────────────────────

func writeNFO(v any, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}

	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// nfoRaw is a catch-all struct that decodes any NFO root element.
type nfoRaw struct {
	Title         string   `xml:"title"`
	OriginalTitle string   `xml:"originaltitle"`
	ShowTitle     string   `xml:"showtitle"`
	Year          string   `xml:"year"`
	Plot          string   `xml:"plot"`
	Tagline       string   `xml:"tagline"`
	Runtime       string   `xml:"runtime"`
	Rating        string   `xml:"rating"`
	Votes         string   `xml:"votes"`
	Premiered     string   `xml:"premiered"`
	Aired         string   `xml:"aired"`
	Season        string   `xml:"season"`
	Episode       string   `xml:"episode"`
	Genres        []string `xml:"genre"`
	Tags          []string `xml:"tag"`
	Countries     []string `xml:"country"`
	Directors     []string `xml:"director"`
	Credits       []string `xml:"credits"`
	Studios       []string `xml:"studio"`

	Set struct {
		Name string `xml:"name"`
		Text string `xml:",chardata"`
	} `xml:"set"`

	Actors []struct {
		Name  string `xml:"name"`
		Role  string `xml:"role"`
		Thumb string `xml:"thumb"`
	} `xml:"actor"`

	UniqueIDs []struct {
		Type  string `xml:"type,attr"`
		Value string `xml:",chardata"`
	} `xml:"uniqueid"`

	Thumb struct {
		Aspect string `xml:"aspect,attr"`
		Value  string `xml:",chardata"`
	} `xml:"thumb"`

	Fanart struct {
		Thumbs []string `xml:"thumb"`
	} `xml:"fanart"`
}

func parseNFOBytes(data []byte) (*Metadata, error) {
	// Detect root element name.
	type rootDetect struct {
		XMLName xml.Name
	}
	var rd rootDetect
	_ = xml.Unmarshal(data, &rd)

	// Re-wrap with a neutral root so nfoRaw can decode regardless of tag name.
	wrapped := rewrapXML(data, rd.XMLName.Local)

	var raw nfoRaw
	if err := xml.Unmarshal(wrapped, &raw); err != nil {
		return nil, fmt.Errorf("parseNFO: unmarshal: %w", err)
	}

	meta := &Metadata{
		Title:         raw.Title,
		OriginalTitle: raw.OriginalTitle,
		ShowTitle:     raw.ShowTitle,
		Plot:          raw.Plot,
		Tagline:       raw.Tagline,
		Premiered:     raw.Premiered,
		Aired:         raw.Aired,
		Genres:        raw.Genres,
		Tags:          raw.Tags,
		Countries:     raw.Countries,
		Directors:     raw.Directors,
		Credits:       raw.Credits,
		Studios:       raw.Studios,
	}

	parseInt := func(s string) int {
		v, _ := strconv.Atoi(strings.TrimSpace(s))
		return v
	}
	meta.Year = parseInt(raw.Year)
	meta.Runtime = parseInt(raw.Runtime)
	meta.Votes = parseInt(raw.Votes)
	meta.Season = parseInt(raw.Season)
	meta.Episode = parseInt(raw.Episode)
	if raw.Rating != "" {
		meta.Rating, _ = strconv.ParseFloat(strings.TrimSpace(raw.Rating), 64)
	}

	setName := strings.TrimSpace(raw.Set.Name)
	if setName == "" {
		setName = strings.TrimSpace(raw.Set.Text)
	}
	meta.Set = setName

	for _, a := range raw.Actors {
		meta.Actors = append(meta.Actors, Actor{Name: a.Name, Role: a.Role, Thumb: a.Thumb})
	}

	if len(raw.UniqueIDs) > 0 {
		meta.UniqueIDs = make(map[string]string, len(raw.UniqueIDs))
		for _, uid := range raw.UniqueIDs {
			if uid.Type != "" {
				meta.UniqueIDs[uid.Type] = uid.Value
			}
		}
	}

	if raw.Thumb.Value != "" {
		meta.PosterURL = strings.TrimSpace(raw.Thumb.Value)
	}
	if len(raw.Fanart.Thumbs) > 0 {
		meta.FanartURL = raw.Fanart.Thumbs[0]
	}

	return meta, nil
}

// rewrapXML replaces the outermost element tag in data with <nforoot>…</nforoot>
// so that nfoRaw can decode any NFO flavour with a single struct.
func rewrapXML(data []byte, local string) []byte {
	if local == "" {
		return data
	}
	s := string(data)

	// Strip XML declaration.
	if idx := strings.Index(s, "?>"); idx >= 0 {
		s = strings.TrimSpace(s[idx+2:])
	}

	// Replace opening tag (may have attributes/namespace).
	openTag := "<" + local
	if strings.HasPrefix(s, openTag) {
		s = "<nforoot" + s[len(openTag):]
	}

	// Replace closing tag.
	closeTag := "</" + local + ">"
	if strings.HasSuffix(s, closeTag) {
		s = s[:len(s)-len(closeTag)] + "</nforoot>"
	}

	return []byte(`<?xml version="1.0"?>` + "\n" + s)
}
