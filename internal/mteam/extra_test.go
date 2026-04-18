package mteam

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRun_SearchErrorCountsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	report, err := Run(context.Background(), c, RunOptions{
		Rule:     Rule{Keyword: "x"},
		WatchDir: t.TempDir(),
		MaxPages: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Errors == 0 {
		t.Fatal("expected search error to bump Errors")
	}
}

func TestRun_MkdirFails(t *testing.T) {
	// WatchDir points to an existing file, so MkdirAll fails.
	f, _ := os.CreateTemp(t.TempDir(), "nope")
	_ = f.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/torrent/search":
			_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"data":[
				{"id":"1","name":"X","status":{"discount":"FREE"}}
			]}}`))
		case "/torrent/genDlToken":
			_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":"` + "http://" + r.Host + `/t"}`))
		case "/t":
			_, _ = w.Write([]byte("d4:tesi0ee"))
		}
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	report, _ := Run(context.Background(), c, RunOptions{
		Rule:     Rule{RequireFree: true},
		WatchDir: f.Name(),
		MaxPages: 1,
		PageSize: 50,
	})
	if report.Errors == 0 {
		t.Fatal("expected mkdir error to bump Errors")
	}
	if report.Downloaded != 0 {
		t.Fatalf("expected 0 downloaded on mkdir failure, got %d", report.Downloaded)
	}
}

func TestDoJSON_BadBaseURL(t *testing.T) {
	c := NewClient("K", WithBaseURL("http://127.0.0.1:1")) // nothing listening
	_, err := c.Search(context.Background(), SearchRequest{Mode: ModeNormal})
	if err == nil || !strings.Contains(err.Error(), "http") {
		t.Fatalf("expected dial error: %v", err)
	}
}

func TestDoForm_BadBaseURL(t *testing.T) {
	c := NewClient("K", WithBaseURL("http://127.0.0.1:1"))
	_, err := c.GenDlToken(context.Background(), "1")
	if err == nil {
		t.Fatal("expected dial error")
	}
}

func TestRun_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately
	c := NewClient("K")
	report, err := Run(ctx, c, RunOptions{
		Rule:     Rule{Keyword: "x"},
		WatchDir: t.TempDir(),
		MaxPages: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Errors != 1 {
		t.Fatalf("expected 1 error for cancelled ctx, got %d", report.Errors)
	}
	if report.Scanned != 0 {
		t.Fatalf("expected 0 scanned with cancelled ctx, got %d", report.Scanned)
	}
}

func TestRun_NoJunk_FilesError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/torrent/search":
			_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"data":[
				{"id":"1","name":"Good Movie","size":"20","status":{"seeders":"10","discount":"FREE"}}
			]}}`))
		case "/torrent/files":
			http.Error(w, "boom", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	report, _ := Run(context.Background(), c, RunOptions{
		Rule:     Rule{RequireFree: true, NoJunk: true},
		WatchDir: t.TempDir(),
		MaxPages: 1,
		PageSize: 50,
	})
	if report.Errors != 1 {
		t.Fatalf("expected 1 error from files call, got %d", report.Errors)
	}
}

func TestRun_NoJunk_Rejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/torrent/search":
			_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"data":[
				{"id":"1","name":"Junk Pack","size":"20","status":{"seeders":"10","discount":"FREE"}}
			]}}`))
		case "/torrent/files":
			// Return 25 video files → exceeds default threshold of 20.
			files := `{"code":"0","message":"SUCCESS","data":[`
			for i := 1; i <= 25; i++ {
				if i > 1 {
					files += ","
				}
				files += fmt.Sprintf(`{"name":"%d.mkv","size":"1"}`, i)
			}
			files += `]}`
			_, _ = w.Write([]byte(files))
		}
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	report, _ := Run(context.Background(), c, RunOptions{
		Rule:     Rule{RequireFree: true, NoJunk: true},
		WatchDir: t.TempDir(),
		MaxPages: 1,
		PageSize: 50,
	})
	if report.Skipped != 1 {
		t.Fatalf("expected 1 skipped (junk), got %d", report.Skipped)
	}
}

func TestRun_NoJunk_Passes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/torrent/search":
			_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"data":[
				{"id":"1","name":"Clean Movie","size":"20","status":{"seeders":"10","discount":"FREE"}}
			]}}`))
		case "/torrent/files":
			_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":[
				{"name":"movie.mkv","size":"5000"}
			]}`))
		case "/torrent/genDlToken":
			u := "http://" + r.Host + "/dl/1.torrent"
			_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":"` + u + `"}`))
		case "/dl/1.torrent":
			_, _ = w.Write([]byte("d8:announce4:teste"))
		}
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	report, _ := Run(context.Background(), c, RunOptions{
		Rule:     Rule{RequireFree: true, NoJunk: true},
		WatchDir: t.TempDir(),
		MaxPages: 1,
		PageSize: 50,
	})
	if report.Downloaded != 1 {
		t.Fatalf("expected 1 download, got %d", report.Downloaded)
	}
}

func TestRun_NoJunk_SkipsAPIWhenNumFilesWithinThreshold(t *testing.T) {
	var filesCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/torrent/search":
			// numfiles=5, within movie threshold (20) → should skip /torrent/files call
			_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"data":[
				{"id":"1","name":"Small Movie","size":"20","numfiles":"5",
				 "status":{"seeders":"10","discount":"FREE"}}
			]}}`))
		case "/torrent/files":
			filesCalled = true
			_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":[]}`))
		case "/torrent/genDlToken":
			u := "http://" + r.Host + "/dl/1.torrent"
			_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":"` + u + `"}`))
		case "/dl/1.torrent":
			_, _ = w.Write([]byte("d8:announce4:teste"))
		}
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	report, _ := Run(context.Background(), c, RunOptions{
		Rule:     Rule{RequireFree: true, NoJunk: true, Mode: ModeMovie},
		WatchDir: t.TempDir(),
		MaxPages: 1,
		PageSize: 50,
	})
	if filesCalled {
		t.Fatal("expected /torrent/files to be skipped when numfiles within threshold")
	}
	if report.Downloaded != 1 {
		t.Fatalf("expected 1 download, got %d", report.Downloaded)
	}
}

func TestRun_NoJunk_SkippedForAdultMode(t *testing.T) {
	var filesCalled bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/torrent/search":
			_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"data":[
				{"id":"1","name":"Adult","size":"20","status":{"seeders":"10","discount":"FREE"}}
			]}}`))
		case "/torrent/files":
			filesCalled = true
			_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":[]}`))
		case "/torrent/genDlToken":
			u := "http://" + r.Host + "/dl/1.torrent"
			_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":"` + u + `"}`))
		case "/dl/1.torrent":
			_, _ = w.Write([]byte("d8:announce4:teste"))
		}
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	report, _ := Run(context.Background(), c, RunOptions{
		Rule:     Rule{RequireFree: true, NoJunk: true, Mode: ModeAdult},
		WatchDir: t.TempDir(),
		MaxPages: 1,
		PageSize: 50,
	})
	if filesCalled {
		t.Fatal("expected /torrent/files to be skipped for adult mode")
	}
	if report.Downloaded != 1 {
		t.Fatalf("expected 1 download, got %d", report.Downloaded)
	}
}

func TestRun_DownloadGenTokenError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/torrent/search":
			_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"data":[
				{"id":"1","name":"Foo","size":"20","status":{"seeders":"10","discount":"FREE"}}
			]}}`))
		case "/torrent/genDlToken":
			_, _ = w.Write([]byte(`{"code":"1","message":"FAIL","data":null}`))
		}
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	report, _ := Run(context.Background(), c, RunOptions{
		Rule:     Rule{RequireFree: true},
		WatchDir: t.TempDir(),
		MaxPages: 1,
		PageSize: 50,
	})
	if report.Errors != 1 {
		t.Fatalf("expected 1 download error, got %d", report.Errors)
	}
}

func TestRun_DownloadBadTorrentData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/torrent/search":
			_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"data":[
				{"id":"1","name":"Foo","size":"20","status":{"seeders":"10","discount":"FREE"}}
			]}}`))
		case "/torrent/genDlToken":
			u := "http://" + r.Host + "/dl"
			_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":"` + u + `"}`))
		case "/dl":
			_, _ = w.Write([]byte("not-bencoded-data"))
		}
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	report, _ := Run(context.Background(), c, RunOptions{
		Rule:     Rule{RequireFree: true},
		WatchDir: t.TempDir(),
		MaxPages: 1,
		PageSize: 50,
	})
	if report.Errors != 1 {
		t.Fatalf("expected 1 download error (bad torrent), got %d", report.Errors)
	}
}

func TestRun_DefaultsApplied(t *testing.T) {
	// Ensure Run doesn't crash with zero-value RunOptions fields (MaxPages, PageSize, Now).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"data":[]}}`))
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	report, err := Run(context.Background(), c, RunOptions{
		Rule:     Rule{Keyword: "x"},
		WatchDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Scanned != 0 {
		t.Fatalf("expected 0 scanned, got %d", report.Scanned)
	}
	if report.Errors != 0 {
		t.Fatalf("expected 0 errors with defaults, got %d", report.Errors)
	}
}

func TestRun_MultiPage(t *testing.T) {
	var pages int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pages++
		// Return exactly pageSize results to trigger next page.
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"total":"100","data":[
			{"id":"` + fmt.Sprintf("%d", pages) + `","name":"T","size":"1",
			 "status":{"seeders":"1","discount":"FREE"}}
		]}}`))
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	report, _ := Run(context.Background(), c, RunOptions{
		Rule:     Rule{RequireFree: true},
		WatchDir: t.TempDir(),
		DryRun:   true,
		MaxPages: 3,
		PageSize: 1, // each page returns 1 = pageSize, so it paginates
	})
	if pages != 3 {
		t.Fatalf("expected 3 pages, got %d", pages)
	}
	if report.Scanned != 3 {
		t.Fatalf("expected 3 scanned across pages, got %d", report.Scanned)
	}
}

func TestSend_LongErrorBody(t *testing.T) {
	// Return a >256 byte error body to cover the truncation branch in send().
	longBody := strings.Repeat("x", 300)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(longBody))
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	_, err := c.Search(context.Background(), SearchRequest{Mode: ModeNormal})
	if err == nil {
		t.Fatal("expected error")
	}
	// The snippet should be truncated to 256 chars.
	if !strings.Contains(err.Error(), "http 400") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSearch_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	_, err := c.Search(context.Background(), SearchRequest{})
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("expected decode error: %v", err)
	}
}

func TestGenDlToken_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	_, err := c.GenDlToken(context.Background(), "1")
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("expected decode error: %v", err)
	}
}

func TestGenDlToken_BadDataJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":123}`))
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	_, err := c.GenDlToken(context.Background(), "1")
	if err == nil || !strings.Contains(err.Error(), "decode token link") {
		t.Fatalf("expected decode token link error: %v", err)
	}
}

func TestFiles_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"1","message":"FAIL","data":null}`))
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	_, err := c.Files(context.Background(), "1")
	if err == nil || !strings.Contains(err.Error(), "files failed") {
		t.Fatalf("expected files API error: %v", err)
	}
}

func TestFiles_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	_, err := c.Files(context.Background(), "1")
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("expected decode error: %v", err)
	}
}

func TestFiles_BadDataJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":"not-array"}`))
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	_, err := c.Files(context.Background(), "1")
	if err == nil || !strings.Contains(err.Error(), "decode file list") {
		t.Fatalf("expected decode file list error: %v", err)
	}
}

func TestCatalog_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"1","message":"FAIL","data":null}`))
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	_, err := c.Catalog(context.Background(), "category")
	if err == nil || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("expected catalog API error: %v", err)
	}
}

func TestCatalog_BadJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	_, err := c.Catalog(context.Background(), "category")
	if err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("expected decode error: %v", err)
	}
}

func TestCatalog_BadDataJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":"not-array"}`))
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	_, err := c.Catalog(context.Background(), "category")
	if err == nil || !strings.Contains(err.Error(), "decode catalog list") {
		t.Fatalf("expected decode catalog list error: %v", err)
	}
}

func TestDownloadTorrent_BadURL(t *testing.T) {
	c := NewClient("K")
	_, err := c.DownloadTorrent(context.Background(), "http://127.0.0.1:1/x")
	if err == nil {
		t.Fatal("expected dial error")
	}
}

func TestRun_WritePartFails(t *testing.T) {
	// WatchDir is a directory, but we make it read-only so WriteFile fails.
	watch := filepath.Join(t.TempDir(), "readonly")
	if err := os.MkdirAll(watch, 0o555); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/torrent/search":
			_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"data":[
				{"id":"1","name":"X","status":{"discount":"FREE"}}
			]}}`))
		case "/torrent/genDlToken":
			u := "http://" + r.Host + "/dl"
			_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":"` + u + `"}`))
		case "/dl":
			_, _ = w.Write([]byte("d8:announce4:teste"))
		}
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	report, _ := Run(context.Background(), c, RunOptions{
		Rule:     Rule{RequireFree: true},
		WatchDir: watch,
		MaxPages: 1,
		PageSize: 50,
	})
	if report.Errors != 1 {
		t.Fatalf("expected 1 write error, got %d errors", report.Errors)
	}
}

func TestRun_FreshHours_UnparsableDate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"data":[
			{"id":"1","name":"Good","size":"20","createdDate":"garbage",
			 "status":{"seeders":"10","discount":"FREE"}}
		]}}`))
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	report, _ := Run(context.Background(), c, RunOptions{
		Rule:     Rule{RequireFree: true, FreshHours: 24},
		WatchDir: t.TempDir(),
		MaxPages: 1,
		PageSize: 50,
		Now:      time.Now,
	})
	if report.Skipped != 1 {
		t.Fatalf("expected 1 skipped (unparsable date), got %d", report.Skipped)
	}
}

func TestRun_FreshHours_WithinWindow(t *testing.T) {
	// Use Asia/Shanghai to match parseCreatedDate's timezone.
	loc, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Now().In(loc)
	fresh := now.Add(-30 * time.Minute).Format("2006-01-02 15:04:05")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/torrent/search":
			_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"data":[
				{"id":"1","name":"Fresh","size":"20","createdDate":"` + fresh + `",
				 "status":{"seeders":"10","discount":"FREE"}}
			]}}`))
		case "/torrent/genDlToken":
			u := "http://" + r.Host + "/dl"
			_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":"` + u + `"}`))
		case "/dl":
			_, _ = w.Write([]byte("d8:announce4:teste"))
		}
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	report, _ := Run(context.Background(), c, RunOptions{
		Rule:     Rule{RequireFree: true, FreshHours: 24},
		WatchDir: t.TempDir(),
		MaxPages: 1,
		PageSize: 50,
		Now:      func() time.Time { return now },
	})
	if report.Downloaded != 1 {
		t.Fatalf("expected 1 download (fresh), got %d", report.Downloaded)
	}
}
