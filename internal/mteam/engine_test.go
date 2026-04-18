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
