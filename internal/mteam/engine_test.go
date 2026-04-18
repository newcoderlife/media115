package mteam

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

// ── engine/Run with httptest ───────────────────────────────────────────────

func TestRun_HappyPath_DryRun(t *testing.T) {
	var searchCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/torrent/search" {
			searchCalls++
			_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"total":"1","data":[
				{"id":"1","name":"Matrix.2160p","size":"20","numfiles":"3",
				 "status":{"seeders":"10","discount":"FREE"}}
			]}}`))
			return
		}
		http.Error(w, "bad", http.StatusBadRequest)
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	report, err := Run(context.Background(), c, RunOptions{
		Rule:     Rule{Keyword: "matrix", Require4K: true, RequireFree: true},
		WatchDir: t.TempDir(),
		DryRun:   true,
		MaxPages: 1,
		PageSize: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Matched != 1 || report.Downloaded != 0 {
		t.Fatalf("report: %+v", report)
	}
}

func TestRun_Downloads_AtomicWrite(t *testing.T) {
	watch := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/torrent/search":
			_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"total":"1","data":[
				{"id":"7","name":"Show/S01 Complete","size":"20","numfiles":"3",
				 "status":{"seeders":"10","discount":"FREE"}}
			]}}`))
		case "/torrent/genDlToken":
			u := "http://" + r.Host + "/dl/7.torrent"
			_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":"` + u + `"}`))
		case "/dl/7.torrent":
			_, _ = w.Write([]byte("d8:announce4:teste"))
		default:
			http.Error(w, r.URL.Path, http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	report, err := Run(context.Background(), c, RunOptions{
		Rule:     Rule{Keyword: "", RequireFree: true},
		WatchDir: watch,
		MaxPages: 1,
		PageSize: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Downloaded != 1 || len(report.Rejections) != 0 {
		t.Fatalf("downloaded=%d rej=%v", report.Downloaded, report.Rejections)
	}
	// Check file exists and is sanitised (no slash).
	entries, _ := os.ReadDir(watch)
	if len(entries) != 1 {
		t.Fatalf("expected 1 file, got %v", entries)
	}
	name := entries[0].Name()
	if !strings.Contains(name, "[M-TEAM]7_") || !strings.HasSuffix(name, ".torrent") {
		t.Fatalf("filename: %s", name)
	}
	if strings.Contains(name, "/") {
		t.Fatalf("unsafe slash preserved: %s", name)
	}
	// Second run should skip — file already exists.
	report2, _ := Run(context.Background(), c, RunOptions{
		Rule:     Rule{Keyword: "", RequireFree: true},
		WatchDir: watch,
		MaxPages: 1,
		PageSize: 50,
	})
	if report2.Downloaded != 0 {
		t.Fatalf("second run should skip existing file")
	}
	if report2.Rejections["already downloaded"] != 1 {
		t.Fatalf("expected 'already downloaded' rejection: %v", report2.Rejections)
	}
}

func TestRun_FreshExhausted_EarlyBreak(t *testing.T) {
	var pages int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pages++
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"total":"50","data":[
			{"id":"a","name":"Foo","size":"1","createdDate":"2020-01-01 00:00:00",
			 "status":{"seeders":"1","discount":"FREE"}}
		]}}`))
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	_, _ = Run(context.Background(), c, RunOptions{
		Rule:     Rule{FreshHours: 24},
		WatchDir: t.TempDir(),
		MaxPages: 5,
		PageSize: 1,
		Now:      time.Now,
	})
	if pages != 1 {
		t.Fatalf("expected early break after 1 page, got %d", pages)
	}
}

func TestTorrentFilename_Sanitises(t *testing.T) {
	tr := &Torrent{ID: "9", Name: "A/B\\C:D*E?F\"G<H>I|J"}
	name := torrentFilename(tr)
	for _, c := range `/\:*?"<>|` {
		if strings.ContainsRune(name, c) {
			t.Fatalf("unsafe rune %q in %s", c, name)
		}
	}
	if !strings.HasPrefix(name, "[M-TEAM]9_") {
		t.Fatalf("prefix: %s", name)
	}
	// Empty name → "torrent" placeholder
	tr2 := &Torrent{ID: "0", Name: ""}
	if !strings.Contains(torrentFilename(tr2), "torrent") {
		t.Fatal("empty name should fall back")
	}
	// Over-long name is truncated
	long := strings.Repeat("x", 200)
	tr3 := &Torrent{ID: "1", Name: long}
	fn := torrentFilename(tr3)
	if len([]rune(fn)) > 100 /* 80 + prefix/suffix */ +30 {
		t.Fatalf("not truncated: %d", len(fn))
	}
	// Unsafe characters in ID are sanitised
	tr4 := &Torrent{ID: "../etc/passwd", Name: "test"}
	fn4 := torrentFilename(tr4)
	if strings.Contains(fn4, "/") {
		t.Fatalf("unsafe slash in ID not sanitised: %s", fn4)
	}
	if !strings.HasPrefix(fn4, "[M-TEAM]") {
		t.Fatalf("prefix missing: %s", fn4)
	}
	// Empty ID falls back
	tr5 := &Torrent{ID: "", Name: "test"}
	fn5 := torrentFilename(tr5)
	if !strings.Contains(fn5, "unknown") {
		t.Fatalf("empty ID should fall back: %s", fn5)
	}
}

// ── pickBest ───────────────────────────────────────────────────────────────

func TestPickBest_PrefersHighCompleted(t *testing.T) {
	ts := []*Torrent{
		{ID: "1", Name: "A", CreatedDate: "2025-12-01 00:00:00", Status: Status{TimesCompleted: "5"}},
		{ID: "2", Name: "B", CreatedDate: "2025-01-01 00:00:00", Status: Status{TimesCompleted: "100"}},
		{ID: "3", Name: "C", CreatedDate: "2025-06-01 00:00:00", Status: Status{TimesCompleted: "30"}},
	}
	got := pickBest(ts)
	if got.ID != "2" {
		t.Fatalf("expected id=2 (100 completed), got id=%s", got.ID)
	}
}

func TestPickBest_BelowThreshold_NewestFirst(t *testing.T) {
	ts := []*Torrent{
		{ID: "1", Name: "Old", CreatedDate: "2025-01-01 00:00:00", Status: Status{TimesCompleted: "5"}},
		{ID: "2", Name: "New", CreatedDate: "2025-12-01 00:00:00", Status: Status{TimesCompleted: "3"}},
		{ID: "3", Name: "Mid", CreatedDate: "2025-06-01 00:00:00", Status: Status{TimesCompleted: "10"}},
	}
	got := pickBest(ts)
	if got.ID != "2" {
		t.Fatalf("expected id=2 (newest below threshold), got id=%s", got.ID)
	}
}

func TestPickBest_MixedThreshold(t *testing.T) {
	ts := []*Torrent{
		{ID: "1", Name: "BelowButNew", CreatedDate: "2026-01-01 00:00:00", Status: Status{TimesCompleted: "15"}},
		{ID: "2", Name: "AboveButOld", CreatedDate: "2020-01-01 00:00:00", Status: Status{TimesCompleted: "25"}},
	}
	got := pickBest(ts)
	if got.ID != "2" {
		t.Fatalf("expected id=2 (above threshold wins over below), got id=%s", got.ID)
	}
}

func TestPickBest_SingleCandidate(t *testing.T) {
	ts := []*Torrent{
		{ID: "1", Name: "Only", Status: Status{TimesCompleted: "0"}},
	}
	got := pickBest(ts)
	if got.ID != "1" {
		t.Fatalf("expected id=1, got id=%s", got.ID)
	}
}

// ── Run with PickBest ──────────────────────────────────────────────────────

func TestRun_PickBest_DryRun(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"total":"3","data":[
			{"id":"1","name":"Small.1080p","size":"5000000000","status":{"seeders":"50","discount":"FREE","timesCompleted":"100"},"createdDate":"2025-01-01 00:00:00"},
			{"id":"2","name":"Big.2160p","size":"50000000000","status":{"seeders":"10","discount":"FREE","timesCompleted":"200"},"createdDate":"2025-06-01 00:00:00"},
			{"id":"3","name":"New.720p","size":"2000000000","status":{"seeders":"5","discount":"FREE","timesCompleted":"8"},"createdDate":"2026-04-01 00:00:00"}
		]}}`))
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	report, err := Run(context.Background(), c, RunOptions{
		Rule:     Rule{RequireFree: true},
		WatchDir: t.TempDir(),
		DryRun:   true,
		PickBest: true,
		MaxPages: 1,
		PageSize: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Matched != 3 {
		t.Fatalf("expected 3 matched, got %d", report.Matched)
	}
	if report.Downloaded != 0 {
		t.Fatalf("expected 0 downloaded in dry-run, got %d", report.Downloaded)
	}
	if report.Skipped != 2 {
		t.Fatalf("expected 2 skipped (non-selected), got %d", report.Skipped)
	}
	if report.Rejections["pick-best: not selected"] != 2 {
		t.Fatalf("expected 2 pick-best rejections, got %v", report.Rejections)
	}
}

func TestRun_PickBest_DownloadsOnlyOne(t *testing.T) {
	watch := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/torrent/search":
			_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"total":"2","data":[
				{"id":"1","name":"LowCompleted","size":"20","status":{"seeders":"10","discount":"FREE","timesCompleted":"5"},"createdDate":"2025-01-01 00:00:00"},
				{"id":"2","name":"HighCompleted","size":"20","status":{"seeders":"10","discount":"FREE","timesCompleted":"50"},"createdDate":"2024-01-01 00:00:00"}
			]}}`))
		case "/torrent/genDlToken":
			u := "http://" + r.Host + "/dl/best.torrent"
			_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":"` + u + `"}`))
		case "/dl/best.torrent":
			_, _ = w.Write([]byte("d8:announce4:teste"))
		default:
			http.Error(w, r.URL.Path, http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	report, err := Run(context.Background(), c, RunOptions{
		Rule:     Rule{RequireFree: true},
		WatchDir: watch,
		PickBest: true,
		MaxPages: 1,
		PageSize: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Matched != 2 {
		t.Fatalf("expected 2 matched, got %d", report.Matched)
	}
	if report.Downloaded != 1 {
		t.Fatalf("expected 1 downloaded, got %d", report.Downloaded)
	}
	if report.Skipped != 1 {
		t.Fatalf("expected 1 skipped (non-selected), got %d", report.Skipped)
	}
	if report.Rejections["pick-best: not selected"] != 1 {
		t.Fatalf("expected 1 pick-best rejection, got %v", report.Rejections)
	}
	entries, _ := os.ReadDir(watch)
	if len(entries) != 1 {
		t.Fatalf("expected 1 file in watch dir, got %d", len(entries))
	}
}

func TestPickBest_MalformedCompleted(t *testing.T) {
	ts := []*Torrent{
		{ID: "1", Name: "BadNum", CreatedDate: "2025-12-01 00:00:00", Status: Status{TimesCompleted: "abc"}},
		{ID: "2", Name: "GoodNum", CreatedDate: "2025-01-01 00:00:00", Status: Status{TimesCompleted: "50"}},
	}
	got := pickBest(ts)
	// "abc" parses as 0 (< 20), id=2 has 50 (≥ 20) → id=2 wins.
	if got.ID != "2" {
		t.Fatalf("expected id=2 (valid completed beats malformed), got id=%s", got.ID)
	}
}

func TestPickBest_EqualCompleted_NewerWins(t *testing.T) {
	ts := []*Torrent{
		{ID: "1", Name: "Older", CreatedDate: "2025-01-01 00:00:00", Status: Status{TimesCompleted: "30"}},
		{ID: "2", Name: "Newer", CreatedDate: "2025-06-01 00:00:00", Status: Status{TimesCompleted: "30"}},
	}
	got := pickBest(ts)
	// Both ≥ 20 with equal completions → newer CreatedDate wins.
	if got.ID != "2" {
		t.Fatalf("expected id=2 (newer date breaks tie), got id=%s", got.ID)
	}
}

func TestPickBest_EqualCompletedAndDate_IDBreaks(t *testing.T) {
	ts := []*Torrent{
		{ID: "10", Name: "A", CreatedDate: "2025-06-01 00:00:00", Status: Status{TimesCompleted: "30"}},
		{ID: "20", Name: "B", CreatedDate: "2025-06-01 00:00:00", Status: Status{TimesCompleted: "30"}},
	}
	got := pickBest(ts)
	// Same completions, same date → higher ID wins.
	if got.ID != "20" {
		t.Fatalf("expected id=20 (higher ID breaks tie), got id=%s", got.ID)
	}
}

func TestRun_PickBest_DownloadError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/torrent/search":
			_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"total":"1","data":[
				{"id":"1","name":"Movie","size":"20","status":{"seeders":"10","discount":"FREE","timesCompleted":"50"},"createdDate":"2025-01-01 00:00:00"}
			]}}`))
		case "/torrent/genDlToken":
			// Return error to trigger download failure.
			_, _ = w.Write([]byte(`{"code":"1","message":"FAIL"}`))
		default:
			http.Error(w, r.URL.Path, http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	report, err := Run(context.Background(), c, RunOptions{
		Rule:     Rule{RequireFree: true},
		WatchDir: t.TempDir(),
		PickBest: true,
		MaxPages: 1,
		PageSize: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Matched != 1 {
		t.Fatalf("expected 1 matched, got %d", report.Matched)
	}
	if report.Errors != 1 {
		t.Fatalf("expected 1 error, got %d", report.Errors)
	}
	if report.Downloaded != 0 {
		t.Fatalf("expected 0 downloaded, got %d", report.Downloaded)
	}
}

func TestHumanSize(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0B"},
		{512, "512B"},
		{1 << 20, "1.0M"},
		{int64(1.5 * float64(1<<30)), "1.5G"},
		{2 * (1 << 40), "2.0T"},
	}
	for _, c := range cases {
		got := humanSize(c.in)
		if got != c.want {
			t.Errorf("humanSize(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}
