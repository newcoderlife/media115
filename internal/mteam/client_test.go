package mteam

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIsSuccess(t *testing.T) {
	cases := []struct {
		code    string
		message string
		ok      bool
	}{
		{`"0"`, "SUCCESS", true},
		{`0`, "SUCCESS", true},
		{`"0"`, "", true},
		{`0`, "", true},
		{`"1"`, "FAIL", false},
		{`1`, "error", false},
	}
	for _, c := range cases {
		if got := isSuccess([]byte(c.code), c.message); got != c.ok {
			t.Errorf("isSuccess(%s,%q)=%v want %v", c.code, c.message, got, c.ok)
		}
	}
}

func TestClient_Search_OK(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.URL.Path != "/torrent/search" {
			t.Errorf("path=%q", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "KEY" {
			t.Errorf("missing api key header")
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"keyword":"matrix"`) {
			t.Errorf("missing keyword in body: %s", body)
		}
		if !strings.Contains(string(body), `"mode":"movie"`) {
			t.Errorf("missing mode in body: %s", body)
		}
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"total":"1","data":[{"id":"1","name":"matrix"}]}}`))
	}))
	defer srv.Close()

	c := NewClient("KEY", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	data, err := c.Search(context.Background(), SearchRequest{Keyword: "matrix", Mode: ModeMovie})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if !called || len(data.List) != 1 || data.List[0].ID != "1" {
		t.Fatalf("unexpected: called=%v data=%+v", called, data)
	}
}

func TestClient_Search_Defaults(t *testing.T) {
	var bodyStr string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodyStr = string(b)
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":{"data":[]}}`))
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	if _, err := c.Search(context.Background(), SearchRequest{PageSize: 9999}); err != nil {
		t.Fatalf("search: %v", err)
	}
	// Defaults: pageNumber=1, pageSize capped at 200, mode=normal
	for _, want := range []string{`"pageNumber":1`, `"pageSize":200`, `"mode":"normal"`} {
		if !strings.Contains(bodyStr, want) {
			t.Errorf("body missing %q: %s", want, bodyStr)
		}
	}
}

func TestClient_Search_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"1","message":"bad","data":null}`))
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	_, err := c.Search(context.Background(), SearchRequest{})
	if err == nil || !strings.Contains(err.Error(), "code=") {
		t.Fatalf("expected api error, got: %v", err)
	}
}

func TestClient_Search_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	_, err := c.Search(context.Background(), SearchRequest{})
	if err == nil || !strings.Contains(err.Error(), "http 500") {
		t.Fatalf("expected http 500, got: %v", err)
	}
}

func TestClient_Search_NoKey(t *testing.T) {
	c := NewClient("")
	_, err := c.Search(context.Background(), SearchRequest{})
	if err == nil {
		t.Fatal("expected error for empty api key")
	}
}

func TestClient_GenDlToken_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/torrent/genDlToken" {
			t.Errorf("path=%q", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("id") != "42" {
			t.Errorf("id=%q", r.Form.Get("id"))
		}
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":"https://dl.example/42.torrent"}`))
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	url, err := c.GenDlToken(context.Background(), "42")
	if err != nil {
		t.Fatalf("%v", err)
	}
	if url != "https://dl.example/42.torrent" {
		t.Fatalf("url=%s", url)
	}
}

func TestClient_GenDlToken_Errors(t *testing.T) {
	c := NewClient("")
	if _, err := c.GenDlToken(context.Background(), "1"); err == nil {
		t.Fatal("expected no-key error")
	}
	c2 := NewClient("K")
	if _, err := c2.GenDlToken(context.Background(), ""); err == nil {
		t.Fatal("expected empty-id error")
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"1","message":"nope","data":null}`))
	}))
	defer srv.Close()
	c3 := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	if _, err := c3.GenDlToken(context.Background(), "1"); err == nil {
		t.Fatal("expected api error")
	}

	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"0","message":"SUCCESS","data":""}`))
	}))
	defer srv2.Close()
	c4 := NewClient("K", WithBaseURL(srv2.URL), WithHTTPClient(srv2.Client()))
	if _, err := c4.GenDlToken(context.Background(), "1"); err == nil {
		t.Fatal("expected empty-url error")
	}
}

func TestClient_DownloadTorrent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("d8:announce4:teste"))
	}))
	defer srv.Close()
	c := NewClient("K", WithHTTPClient(srv.Client()))
	data, err := c.DownloadTorrent(context.Background(), srv.URL+"/x.torrent")
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(data) < 8 || data[0] != 'd' {
		t.Fatalf("bad bytes: %q", data)
	}
}

func TestClient_DownloadTorrent_NotBencoded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>403 forbidden</html>"))
	}))
	defer srv.Close()
	c := NewClient("K", WithHTTPClient(srv.Client()))
	_, err := c.DownloadTorrent(context.Background(), srv.URL+"/x.torrent")
	if err == nil || !strings.Contains(err.Error(), "bencoded") {
		t.Fatalf("expected bencoded error, got: %v", err)
	}
}

func TestClient_DownloadTorrent_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()
	c := NewClient("K", WithHTTPClient(srv.Client()))
	_, err := c.DownloadTorrent(context.Background(), srv.URL+"/x")
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected 404, got: %v", err)
	}
}

func TestClient_Pace(t *testing.T) {
	c := NewClient("K", WithMinInterval(50*time.Millisecond))
	ctx := context.Background()
	start := time.Now()
	_ = c.pace(ctx) // first call, no wait
	_ = c.pace(ctx) // second, should wait ≥50ms
	elapsed := time.Since(start)
	if elapsed < 50*time.Millisecond {
		t.Fatalf("pace didn't throttle: %v", elapsed)
	}
}
