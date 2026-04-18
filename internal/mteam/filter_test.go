package mteam

import (
	"testing"
	"time"
)

func nowFixed() time.Time {
	// 2026-01-30 08:00:00 Asia/Shanghai
	loc, _ := time.LoadLocation("Asia/Shanghai")
	return time.Date(2026, 1, 30, 8, 0, 0, 0, loc)
}

func baseTorrent() *Torrent {
	return &Torrent{
		ID:           "1",
		Name:         "The.Matrix.2160p.WEB-DL.x265-Group",
		Size:         "21474836480",
		NumFiles:     "3",
		CreatedDate:  "2026-01-30 07:00:00", // 1h ago
		LabelsNew:    []string{"中字", "4K"},
		IMDBRating:   "8.7",
		DoubanRating: "9.1",
		Status:       Status{Seeders: "20", Discount: DiscountFree},
	}
}

func TestRuleMatch_PassThrough(t *testing.T) {
	var r *Rule
	if got := r.Match(baseTorrent(), nowFixed()); !got.Match {
		t.Fatalf("nil rule should pass: %+v", got)
	}
	r = &Rule{}
	if got := r.Match(baseTorrent(), nowFixed()); !got.Match {
		t.Fatalf("empty rule should pass: %+v", got)
	}
}

func TestRuleMatch_AllDimensions(t *testing.T) {
	cases := []struct {
		name   string
		tweak  func(r *Rule, t *Torrent)
		reason string
	}{
		{"require_free", func(r *Rule, t *Torrent) {
			r.RequireFree = true
			t.Status.Discount = DiscountNormal
		}, "not free"},
		{"require_4k", func(r *Rule, t *Torrent) {
			r.Require4K = true
			t.Name = "The.Matrix.1080p.WEB-DL"
			t.LabelsNew = nil
		}, "not 4K"},
		{"max_files", func(r *Rule, t *Torrent) {
			r.MaxFiles = 2
			t.NumFiles = "5"
		}, "numfiles exceeds max_files"},
		{"min_size", func(r *Rule, t *Torrent) {
			r.MinSize = 100 * 1024 * 1024 * 1024
		}, "size below min_size"},
		{"max_size", func(r *Rule, t *Torrent) {
			r.MaxSize = 1024
		}, "size above max_size"},
		{"min_seeders", func(r *Rule, t *Torrent) {
			r.MinSeeders = 100
		}, "seeders below min_seeders"},
		{"exclude_keywords", func(r *Rule, t *Torrent) {
			r.ExcludeKeywords = []string{"matrix"}
		}, "matches exclude keyword: matrix"},
		{"labels_deny", func(r *Rule, t *Torrent) {
			r.LabelsDeny = []string{"4K"}
		}, "matches labels_deny: 4K"},
		{"labels_allow", func(r *Rule, t *Torrent) {
			r.LabelsAllow = []string{"HDR10"}
		}, "labels_allow not satisfied"},
		{"fresh_hours_bad_date", func(r *Rule, t *Torrent) {
			r.FreshHours = 1
			t.CreatedDate = ""
		}, "cannot parse createdDate for fresh_hours"},
		{"fresh_hours_stale", func(r *Rule, t *Torrent) {
			r.FreshHours = 1
			t.CreatedDate = "2026-01-29 06:00:00" // ~26h ago
		}, "older than fresh_hours window"},
		{"min_rating_below", func(r *Rule, t *Torrent) {
			r.MinRating = 9.5
		}, "best rating 9.1 below min_rating 9.5"},
		{"min_rating_no_data", func(r *Rule, t *Torrent) {
			r.MinRating = 5.0
			t.IMDBRating = ""
			t.DoubanRating = ""
		}, "no rating available for min_rating check"},
		{"tv_complete_mismatch", func(r *Rule, t *Torrent) {
			r.Mode = ModeTVShow
			r.TVCompleteEpisodes = true
			t.Name = "Show.S01E05.2160p"
		}, "not a TV pack"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := &Rule{}
			tr := baseTorrent()
			c.tweak(r, tr)
			got := r.Match(tr, nowFixed())
			if got.Match {
				t.Fatalf("expected reject (%s), got match", c.reason)
			}
			if got.Reason != c.reason {
				t.Fatalf("reason=%q want %q", got.Reason, c.reason)
			}
		})
	}
}

func TestRuleMatch_AllowsWhenPass(t *testing.T) {
	r := &Rule{
		RequireFree:        true,
		Require4K:          true,
		MaxFiles:           10,
		MinSize:            1024,
		MaxSize:            100 * 1024 * 1024 * 1024,
		MinSeeders:         5,
		LabelsAllow:        []string{"4K"},
		LabelsDeny:         []string{"原盘"},
		FreshHours:         24,
		MinRating:          8.0,
		Mode:               ModeTVShow,
		TVCompleteEpisodes: false,
	}
	got := r.Match(baseTorrent(), nowFixed())
	if !got.Match {
		t.Fatalf("expected match, got reject: %s", got.Reason)
	}
}

func TestIs4K_NameOrLabel(t *testing.T) {
	t1 := &Torrent{Name: "Foo.2160p.mkv"}
	if !is4K(t1) {
		t.Fatal("2160p in name should be 4K")
	}
	t2 := &Torrent{Name: "Foo.1080p", LabelsNew: []string{"UHD"}}
	if !is4K(t2) {
		t.Fatal("UHD label should be 4K")
	}
	t3 := &Torrent{Name: "Foo.1080p"}
	if is4K(t3) {
		t.Fatal("1080p without label should not be 4K")
	}
}

func TestIsTVPack(t *testing.T) {
	cases := map[string]bool{
		"Show.S01E01-E12.Complete": true,
		"Show.S01 全 12 集":          true,
		"Show.完结.2024":             true,
		"Show.S01E01":              false,
		"Random.Movie.2024":        false,
	}
	for name, want := range cases {
		if got := isTVPack(name); got != want {
			t.Errorf("isTVPack(%q)=%v want %v", name, got, want)
		}
	}
}

func TestParseRating(t *testing.T) {
	cases := []struct {
		in string
		v  float64
		ok bool
	}{
		{"", 0, false},
		{"-", 0, false},
		{"7.5", 7.5, true},
		{"abc", 0, false},
	}
	for _, c := range cases {
		v, ok := parseRating(c.in)
		if v != c.v || ok != c.ok {
			t.Errorf("parseRating(%q)=(%v,%v) want (%v,%v)", c.in, v, ok, c.v, c.ok)
		}
	}
}

func TestParseCreatedDate(t *testing.T) {
	if _, ok := parseCreatedDate(""); ok {
		t.Fatal("empty should fail")
	}
	if _, ok := parseCreatedDate("garbage"); ok {
		t.Fatal("garbage should fail")
	}
	v, ok := parseCreatedDate("2026-01-30 07:00:00")
	if !ok {
		t.Fatal("well-formed parse failed")
	}
	if v.Year() != 2026 || v.Month() != 1 || v.Day() != 30 {
		t.Fatalf("unexpected time: %v", v)
	}
	// RFC3339 fallback path.
	v2, ok := parseCreatedDate("2026-01-30T07:00:00Z")
	if !ok || v2.Year() != 2026 {
		t.Fatalf("rfc3339 fallback: %v / %v", v2, ok)
	}
}

func TestLabelsContain_Empty(t *testing.T) {
	if _, ok := labelsContain(nil, []string{"a"}); ok {
		t.Fatal("empty labels should not match")
	}
	if _, ok := labelsContain([]string{"a"}, nil); ok {
		t.Fatal("empty targets should not match")
	}
}

func TestContainsFold_Empty(t *testing.T) {
	if _, ok := containsFold("abc", nil); ok {
		t.Fatal("nil subs should not match")
	}
	if _, ok := containsFold("abc", []string{"", "  "}); ok {
		t.Fatal("blank-only subs should not match")
	}
}

func TestIsFree(t *testing.T) {
	for _, d := range []string{DiscountFree, Discount2XFree, "free", "_2x_free"} {
		if !isFree(d) {
			t.Errorf("isFree(%q) should be true", d)
		}
	}
	for _, d := range []string{DiscountNormal, DiscountPercent50, ""} {
		if isFree(d) {
			t.Errorf("isFree(%q) should be false", d)
		}
	}
}

func TestIsHQ(t *testing.T) {
	tests := []struct {
		labels []string
		want   bool
	}{
		{[]string{"中字", "HDR"}, true},
		{[]string{"DV"}, true},
		{[]string{"Dolby Vision", "4K"}, true},
		{[]string{"HDR10+"}, true},
		{[]string{"4K", "中字"}, false},
		{nil, false},
	}
	for _, tc := range tests {
		t2 := &Torrent{LabelsNew: tc.labels}
		if got := isHQ(t2); got != tc.want {
			t.Errorf("isHQ(labels=%v)=%v want %v", tc.labels, got, tc.want)
		}
	}
}

func TestCheckFiles(t *testing.T) {
	files := []TorrentFile{
		{Name: "movie.mkv", Size: "1000"},
		{Name: "sub.srt", Size: "50"},
		{Name: "sample.mp4", Size: "100"},
	}
	if m := CheckFiles(files, 5); !m.Match {
		t.Fatalf("expected match: %s", m.Reason)
	}
	if m := CheckFiles(files, 1); m.Match {
		t.Fatal("expected reject for >1 video file")
	}
	if m := CheckFiles(files, 0); !m.Match {
		t.Fatalf("expected match with default limit: %s", m.Reason)
	}
}

func TestJunkVideoThreshold(t *testing.T) {
	cases := map[string]int{
		ModeMovie:  20,
		ModeTVShow: 500,
		ModeMusic:  100,
		ModeAdult:  0,
		ModeNormal: 20,
		"":         20,
	}
	for mode, want := range cases {
		if got := JunkVideoThreshold(mode); got != want {
			t.Errorf("JunkVideoThreshold(%q)=%d want %d", mode, got, want)
		}
	}
}

func TestRuleMatch_RequireHQ(t *testing.T) {
	r := &Rule{RequireHQ: true}
	tr := baseTorrent()
	tr.LabelsNew = append(tr.LabelsNew, "HDR")
	if got := r.Match(tr, nowFixed()); !got.Match {
		t.Fatalf("expected HQ pass: %s", got.Reason)
	}
	tr2 := baseTorrent()
	if got := r.Match(tr2, nowFixed()); got.Match {
		t.Fatal("expected HQ reject without HDR label")
	}
}

func TestRuleMatch_MinRating_OR_Logic(t *testing.T) {
	r := &Rule{MinRating: 8.0}
	// Only IMDB present → pass
	tr := baseTorrent()
	tr.DoubanRating = ""
	if got := r.Match(tr, nowFixed()); !got.Match {
		t.Fatalf("imdb-only should pass: %s", got.Reason)
	}
	// Only Douban present → pass
	tr2 := baseTorrent()
	tr2.IMDBRating = ""
	if got := r.Match(tr2, nowFixed()); !got.Match {
		t.Fatalf("douban-only should pass: %s", got.Reason)
	}
	// Both present, best is above → pass
	tr3 := baseTorrent()
	tr3.IMDBRating = "7.0"
	tr3.DoubanRating = "8.5"
	if got := r.Match(tr3, nowFixed()); !got.Match {
		t.Fatalf("best-of-two should pass: %s", got.Reason)
	}
}
