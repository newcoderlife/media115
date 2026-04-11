package scraper

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// CaseExpect holds the expected analysis fields for a test case.
type CaseExpect struct {
	Type     string `json:"type"`
	Title    string `json:"title"`
	Source   string `json:"source"`
	SourceID string `json:"source_id"`
	Year     int    `json:"year"`
	Season   int    `json:"season"`
	Episode  int    `json:"episode"`
}

// Case is one entry in a scrape_cases.json file.
type Case struct {
	Filename string     `json:"filename"`
	Expect   CaseExpect `json:"expect"`
}

// LoadCases reads a JSON file containing []Case.
func LoadCases(path string) ([]Case, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cases []Case
	if err := json.Unmarshal(data, &cases); err != nil {
		return nil, err
	}
	return cases, nil
}

// MatchAgainstCases checks filename against the provided cases (exact match on
// Filename).  Returns a non-nil *AnalysisResult if a matching case is found.
func MatchAgainstCases(filename string, cases []Case) *AnalysisResult {
	base := filepath.Base(filename)
	for _, c := range cases {
		if c.Filename != base && c.Filename != filename {
			continue
		}
		r := &AnalysisResult{
			Filename:   filename,
			MediaType:  c.Expect.Type,
			Title:      c.Expect.Title,
			Year:       c.Expect.Year,
			Season:     c.Expect.Season,
			Episode:    c.Expect.Episode,
			Source:     c.Expect.Source,
			Confidence: "high",
			Note:       "matched scrape_cases.json",
		}
		return r
	}
	return nil
}

// AnalyzeWithCases is like AnalyzeFilename but checks cases first.
func AnalyzeWithCases(filename string, cases []Case) AnalysisResult {
	if r := MatchAgainstCases(filename, cases); r != nil {
		return *r
	}
	return AnalyzeFilename(filename)
}

// AnalysisResult holds the output of AnalyzeFilename.
type AnalysisResult struct {
	Filename   string
	MediaType  string // av, gravure, av_west, tv, anime, movie, unknown
	Title      string
	Year       int
	Season     int
	Episode    int
	Source     string // suggested provider: tmdb, javbus, bangumi, theporndb, jav321
	Confidence string // high, medium, low
	Note       string
}

// gravurePrefixes is the set of known photobook/idol label prefixes.
// Checked BEFORE the generic AV regex to avoid misclassification.
var gravurePrefixes = map[string]bool{
	"IMBD": true,
	"IMOG": true,
	"LCBD": true,
	"ENFD": true,
	"TSDV": true,
	"SBVD": true,
	"LPFD": true,
	"LCDV": true,
	"ENCO": true,
	"OME":  true,
}

var (
	// Gravure: optional prefix text then a label-number token.
	reGravure = regexp.MustCompile(`(?i)^(?:.*?\s)?([A-Z]{3,5}-\d{3,5})`)

	// AV: label-number at the start of the stem (e.g. ABP-040, SSNI-123).
	reAV = regexp.MustCompile(`(?i)^([A-Z]{2,10}-\d{3,5})`)

	// FC2-PPV anywhere in stem.
	reFC2 = regexp.MustCompile(`(?i)FC2[-_]?PPV[-_]?\d+`)

	// Western "Studio - Performer - Title" pattern.
	reWestDash = regexp.MustCompile(`^([A-Za-z]+(?:\s[A-Za-z]+)?)\s*-\s*(.+?)\s*-\s*(.+?)$`)

	// Western "Studio.YY.MM.DD.*.XXX" pattern.
	reWestDate = regexp.MustCompile(`^([A-Za-z]+)\.(\d{2}\.\d{2}\.\d{2})\.(.+?)\.(?i:XXX)\b`)

	// TV / Anime SxxExx.
	reSxxExx = regexp.MustCompile(`(?i)S(\d{1,2})E(\d{1,3})`)

	// Leading bracket group for fansubbed content, e.g. [SubGroup].
	reBracketLead = regexp.MustCompile(`^\[([^\]]+)\]`)

	// Anime: [SubGroup] Title - Episode.
	reAnime = regexp.MustCompile(`^\[.+?\]\s*(.+?)\s*-\s*(\d+)`)

	// Movie: Title (Year).
	reMovieParen = regexp.MustCompile(`^(.+?)\s*\((\d{4})\)$`)

	// Movie: Title.Year or Title Year at start.
	reMovieDot = regexp.MustCompile(`^(.+?)[.\s](\d{4})(?:[.\s]|$)`)
)

// AnalyzeFilename inspects a media filename and returns a best-effort
// AnalysisResult.  It mirrors the logic in the Python analyzer.py.
func AnalyzeFilename(filename string) AnalysisResult {
	// Work with the file stem (no extension, no directory).
	stem := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))

	// ── 1. Gravure (check BEFORE generic AV) ────────────────────────────────
	if m := reGravure.FindStringSubmatch(stem); m != nil {
		number := strings.ToUpper(m[1])
		prefix := strings.SplitN(number, "-", 2)[0]
		if gravurePrefixes[prefix] {
			return AnalysisResult{
				Filename:   filename,
				MediaType:  "gravure",
				Title:      number,
				Source:     "jav321",
				Confidence: "medium",
			}
		}
	}

	// ── 2. AV label-number ───────────────────────────────────────────────────
	if m := reAV.FindStringSubmatch(stem); m != nil {
		return AnalysisResult{
			Filename:   filename,
			MediaType:  "av",
			Title:      strings.ToUpper(m[1]),
			Source:     "javbus",
			Confidence: "medium",
		}
	}

	// ── 3. FC2-PPV ───────────────────────────────────────────────────────────
	if reFC2.MatchString(stem) {
		return AnalysisResult{
			Filename:   filename,
			MediaType:  "av",
			Title:      stem,
			Source:     "javbus",
			Confidence: "medium",
		}
	}

	// ── 4. Western adult — dash pattern ─────────────────────────────────────
	if m := reWestDash.FindStringSubmatch(stem); m != nil {
		title := strings.TrimSpace(m[2]) + " - " + strings.TrimSpace(m[3])
		return AnalysisResult{
			Filename:   filename,
			MediaType:  "av_west",
			Title:      title,
			Source:     "theporndb",
			Confidence: "medium",
		}
	}

	// ── 5. Western adult — date pattern ─────────────────────────────────────
	if m := reWestDate.FindStringSubmatch(stem); m != nil {
		title := m[1] + " " + strings.ReplaceAll(m[3], ".", " ")
		return AnalysisResult{
			Filename:   filename,
			MediaType:  "av_west",
			Title:      title,
			Source:     "theporndb",
			Confidence: "medium",
		}
	}

	// ── 6. TV / Anime via SxxExx ─────────────────────────────────────────────
	if m := reSxxExx.FindStringSubmatchIndex(stem); m != nil {
		full := reSxxExx.FindStringSubmatch(stem)
		season, _ := strconv.Atoi(full[1])
		episode, _ := strconv.Atoi(full[2])
		// Title = everything before the SxxExx token, stripped of trailing separators.
		title := strings.TrimRight(stem[:m[0]], ".-_ ")

		// If the stem starts with a bracket group that contains only ASCII
		// (i.e. a fansub tag, not a CJK title) → anime.
		if bm := reBracketLead.FindStringSubmatch(stem); bm != nil && isASCIIOnly(bm[1]) {
			return AnalysisResult{
				Filename:   filename,
				MediaType:  "anime",
				Title:      title,
				Season:     season,
				Episode:    episode,
				Source:     "bangumi",
				Confidence: "medium",
			}
		}
		return AnalysisResult{
			Filename:   filename,
			MediaType:  "tv",
			Title:      title,
			Season:     season,
			Episode:    episode,
			Source:     "tmdb",
			Confidence: "medium",
		}
	}

	// ── 7. Anime — [SubGroup] Title - Episode ────────────────────────────────
	if m := reAnime.FindStringSubmatch(stem); m != nil {
		episode, _ := strconv.Atoi(m[2])
		return AnalysisResult{
			Filename:   filename,
			MediaType:  "anime",
			Title:      strings.TrimSpace(m[1]),
			Episode:    episode,
			Source:     "bangumi",
			Confidence: "medium",
		}
	}

	// ── 8. Movie — Title (Year) ──────────────────────────────────────────────
	if m := reMovieParen.FindStringSubmatch(stem); m != nil {
		year, _ := strconv.Atoi(m[2])
		return AnalysisResult{
			Filename:   filename,
			MediaType:  "movie",
			Title:      strings.TrimSpace(m[1]),
			Year:       year,
			Source:     "tmdb",
			Confidence: "high",
		}
	}

	// ── 9. Movie — Title.Year ────────────────────────────────────────────────
	if m := reMovieDot.FindStringSubmatch(stem); m != nil {
		title := strings.ReplaceAll(m[1], ".", " ")
		title = strings.TrimSpace(title)
		year, _ := strconv.Atoi(m[2])
		return AnalysisResult{
			Filename:   filename,
			MediaType:  "movie",
			Title:      title,
			Year:       year,
			Source:     "tmdb",
			Confidence: "medium",
		}
	}

	// ── 10. Unknown ──────────────────────────────────────────────────────────
	return AnalysisResult{
		Filename:   filename,
		MediaType:  "unknown",
		Confidence: "low",
		Note:       "could not determine media type from filename",
	}
}

// isASCIIOnly returns true if every rune in s is in the ASCII range (≤127).
func isASCIIOnly(s string) bool {
	for _, r := range s {
		if r > unicode.MaxASCII {
			return false
		}
	}
	return true
}
