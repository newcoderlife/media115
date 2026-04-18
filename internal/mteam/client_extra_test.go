package mteam

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_Files_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/torrent/files" {
			t.Errorf("path=%q", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("id") != "99" {
			t.Errorf("id=%q", r.Form.Get("id"))
		}
		resp := GenericResponse{
			Code:    json.RawMessage(`"0"`),
			Message: "SUCCESS",
		}
		data, _ := json.Marshal([]TorrentFile{
			{Name: "movie.mkv", Size: "5000000000"},
			{Name: "sub.srt", Size: "12345"},
		})
		resp.Data = data
		out, _ := json.Marshal(resp)
		_, _ = w.Write(out)
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))
	files, err := c.Files(context.Background(), "99")
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(files) != 2 {
		t.Fatalf("len=%d", len(files))
	}
	if files[0].Name != "movie.mkv" {
		t.Fatalf("name=%s", files[0].Name)
	}
}

func TestClient_Files_Errors(t *testing.T) {
	c := NewClient("")
	if _, err := c.Files(context.Background(), "1"); err == nil {
		t.Fatal("expected no-key error")
	}
	c2 := NewClient("K")
	if _, err := c2.Files(context.Background(), ""); err == nil {
		t.Fatal("expected empty-id error")
	}
}

func TestClient_Catalog_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := GenericResponse{
			Code:    json.RawMessage(`"0"`),
			Message: "SUCCESS",
		}
		data, _ := json.Marshal([]CatalogItem{
			{ID: 1, Name: "Movie", NameCht: "電影", NameChs: "电影"},
			{ID: 2, Name: "TV", NameCht: "劇集", NameChs: "剧集"},
		})
		resp.Data = data
		out, _ := json.Marshal(resp)
		_, _ = w.Write(out)
	}))
	defer srv.Close()
	c := NewClient("K", WithBaseURL(srv.URL), WithHTTPClient(srv.Client()))

	items, err := c.Catalog(context.Background(), "category")
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len=%d", len(items))
	}
	if items[0].Name != "Movie" || items[0].NameChs != "电影" {
		t.Fatalf("item=%+v", items[0])
	}
}

func TestClient_CatalogList_NoKey(t *testing.T) {
	c := NewClient("")
	if _, err := c.Catalog(context.Background(), "category"); err == nil {
		t.Fatal("expected no-key error")
	}
}
