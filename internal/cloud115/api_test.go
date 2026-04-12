package cloud115

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// newTestAPI builds an API pointed at the given server URL with nil cache and logger.
// It sets the host by replacing WebAPI/ProAPI in the target URL via a custom transport.
func newTestAPI(serverURL string) *API {
	a := NewAPI("UID=12345_test; CID=abc", nil, nil)
	// Override the http client to redirect all requests to our test server.
	a.http = &http.Client{
		Transport: &redirectTransport{target: serverURL},
	}
	return a
}

// redirectTransport rewrites the host portion of every request to the test server URL.
type redirectTransport struct {
	target string
}

func (t *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Parse the test server URL to get host + scheme.
	tu, _ := url.Parse(t.target)
	req2 := req.Clone(req.Context())
	req2.URL.Scheme = tu.Scheme
	req2.URL.Host = tu.Host
	return http.DefaultTransport.RoundTrip(req2)
}

// jsonResponse writes a JSON-encoded body with status 200.
func jsonResponse(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// ── TestAPIListFiles ─────────────────────────────────────────────────────────

func TestAPIListFiles(t *testing.T) {
	var gotQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		jsonResponse(w, map[string]any{
			"data": []map[string]any{
				{"n": "file.mkv", "fid": "123", "s": 1024, "pc": "pk1"},
			},
		})
	}))
	defer server.Close()

	api := newTestAPI(server.URL)
	files, err := api.ListFiles("0", 100, 0)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0]["fid"] != "123" {
		t.Errorf("fid = %v; want 123", files[0]["fid"])
	}
	// Verify request params.
	if got := gotQuery.Get("cid"); got != "0" {
		t.Errorf("cid param = %q; want 0", got)
	}
	if got := gotQuery.Get("limit"); got != "100" {
		t.Errorf("limit param = %q; want 100", got)
	}
}

// ── TestAPIGetDirID ──────────────────────────────────────────────────────────

func TestAPIGetDirID(t *testing.T) {
	var gotQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		jsonResponse(w, map[string]any{"state": true, "id": "9876"})
	}))
	defer server.Close()

	api := newTestAPI(server.URL)
	cid, err := api.GetDirID("/movies/action")
	if err != nil {
		t.Fatalf("GetDirID: %v", err)
	}
	if cid != "9876" {
		t.Errorf("cid = %q; want 9876", cid)
	}
	if got := gotQuery.Get("path"); got != "/movies/action" {
		t.Errorf("path param = %q; want /movies/action", got)
	}
}

// ── TestAPIMkdir ─────────────────────────────────────────────────────────────

func TestAPIMkdir(t *testing.T) {
	var gotBody url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err == nil {
			gotBody = r.PostForm
		}
		jsonResponse(w, map[string]any{"state": true, "cid": "555"})
	}))
	defer server.Close()

	api := newTestAPI(server.URL)
	result, err := api.Mkdir("0", "NewFolder")
	if err != nil {
		t.Fatalf("Mkdir: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if got := gotBody.Get("pid"); got != "0" {
		t.Errorf("pid = %q; want 0", got)
	}
	if got := gotBody.Get("cname"); got != "NewFolder" {
		t.Errorf("cname = %q; want NewFolder", got)
	}
}

// ── TestAPIRetry ──────────────────────────────────────────────────────────────

func TestAPIRetry(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			// Simulate a non-network error by closing the connection immediately.
			hj, ok := w.(http.Hijacker)
			if ok {
				conn, _, _ := hj.Hijack()
				conn.Close()
				return
			}
		}
		jsonResponse(w, map[string]any{"data": []any{}})
	}))
	defer server.Close()

	api := newTestAPI(server.URL)
	// Use a very short timeout to avoid the 3*(attempt+1) second sleeps in tests.
	// We disable the sleep by having all 3 retries succeed on attempt 3.
	// The retry loop will see connection-closed errors and retry up to 3 times.
	_, err := api.ListFiles("0", 10, 0)
	// We expect either success (attempt 3 worked) or failure after 3 attempts.
	// What matters is attempts was >= 2 (proving retry happened).
	if attempts < 2 {
		t.Errorf("expected at least 2 attempts (retry), got %d (err=%v)", attempts, err)
	}
}

// ── TestAPI429Cooldown ────────────────────────────────────────────────────────

func TestAPI429Cooldown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	api := newTestAPI(server.URL)
	_, err := api.ListFiles("0", 10, 0)
	if err == nil {
		t.Fatal("expected error on 429, got nil")
	}
	// Verify that the error message mentions rate limit.
	errMsg := err.Error()
	if !contains(errMsg, "429") && !contains(errMsg, "rate limit") {
		t.Errorf("error should mention 429 or rate limit, got: %s", errMsg)
	}
}

// ── TestAPIMove ───────────────────────────────────────────────────────────────

func TestAPIMove(t *testing.T) {
	var gotBody url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotBody = r.PostForm
		jsonResponse(w, map[string]any{"state": true})
	}))
	defer server.Close()

	api := newTestAPI(server.URL)
	_, err := api.Move([]string{"fid1", "fid2"}, "dir99")
	if err != nil {
		t.Fatalf("Move: %v", err)
	}
	if got := gotBody.Get("pid"); got != "dir99" {
		t.Errorf("pid = %q; want dir99", got)
	}
	if got := gotBody.Get("fid[0]"); got != "fid1" {
		t.Errorf("fid[0] = %q; want fid1", got)
	}
	if got := gotBody.Get("fid[1]"); got != "fid2" {
		t.Errorf("fid[1] = %q; want fid2", got)
	}
}

// ── TestAPIDelete ─────────────────────────────────────────────────────────────

func TestAPIDelete(t *testing.T) {
	var gotBody url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotBody = r.PostForm
		jsonResponse(w, map[string]any{"state": true})
	}))
	defer server.Close()

	api := newTestAPI(server.URL)
	_, err := api.Delete([]string{"a1", "b2"})
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got := gotBody.Get("fid[0]"); got != "a1" {
		t.Errorf("fid[0] = %q; want a1", got)
	}
}

// ── TestAPISearch ─────────────────────────────────────────────────────────────

func TestAPISearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, map[string]any{
			"data": []map[string]any{
				{"n": "found.mkv", "fid": "77"},
			},
		})
	}))
	defer server.Close()

	api := newTestAPI(server.URL)
	results, err := api.Search("found", "0")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

// ── TestAPIRename ─────────────────────────────────────────────────────────────

func TestAPIRename(t *testing.T) {
	var gotBody url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotBody = r.PostForm
		jsonResponse(w, map[string]any{"state": true})
	}))
	defer server.Close()

	api := newTestAPI(server.URL)
	_, err := api.Rename("fid99", "new_name.mkv")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if got := gotBody.Get("fid"); got != "fid99" {
		t.Errorf("fid = %q; want fid99", got)
	}
	if got := gotBody.Get("file_name"); got != "new_name.mkv" {
		t.Errorf("file_name = %q; want new_name.mkv", got)
	}
}

// ── TestAPIBatchRename ────────────────────────────────────────────────────────

func TestAPIBatchRename(t *testing.T) {
	var gotBody url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotBody = r.PostForm
		jsonResponse(w, map[string]any{"state": true})
	}))
	defer server.Close()

	api := newTestAPI(server.URL)
	_, err := api.BatchRename(map[string]string{"fid1": "a.mkv", "fid2": "b.mkv"})
	if err != nil {
		t.Fatalf("BatchRename: %v", err)
	}
	if got := gotBody.Get("files_new_name[fid1]"); got != "a.mkv" {
		t.Errorf("files_new_name[fid1] = %q; want a.mkv", got)
	}
}

// ── TestAPIUserIDExtraction ───────────────────────────────────────────────────

func TestAPIUserIDExtraction(t *testing.T) {
	tests := []struct {
		cookies string
		wantUID string
	}{
		{"UID=99887_abc; CID=xyz", "99887"},
		{"CID=xyz; UID=12345_extra_parts; SEID=foo", "12345"},
		{"no-uid-cookie", ""},
	}
	for _, tc := range tests {
		a := NewAPI(tc.cookies, nil, nil)
		if a.userID != tc.wantUID {
			t.Errorf("cookies=%q: userID=%q; want %q", tc.cookies, a.userID, tc.wantUID)
		}
	}
}

// ── TestAPIListFilesAll ───────────────────────────────────────────────────────

func TestAPIListFilesAll(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		offset := r.URL.Query().Get("offset")
		if offset == "0" || offset == "" {
			// Return exactly 2 items (< 1000 → single page).
			jsonResponse(w, map[string]any{
				"data": []map[string]any{
					{"fid": "1"}, {"fid": "2"},
				},
			})
		} else {
			jsonResponse(w, map[string]any{"data": []any{}})
		}
	}))
	defer server.Close()

	api := newTestAPI(server.URL)
	all, err := api.ListFilesAll("0")
	if err != nil {
		t.Fatalf("ListFilesAll: %v", err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 items, got %d", len(all))
	}
	if calls != 1 {
		t.Errorf("expected 1 HTTP call, got %d", calls)
	}
}

// ── TestFormatNumFloat64 ──────────────────────────────────────────────────────

func TestFormatNumFloat64(t *testing.T) {
	// Simulate JSON-decoded timestamp: json.Unmarshal into map[string]any gives float64.
	result := formatNum(float64(1775973641))
	if result != "1775973641" {
		t.Fatalf("got %q, want 1775973641", result)
	}
	// Must NOT be scientific notation.
	if strings.Contains(result, "e") || strings.Contains(result, "E") {
		t.Fatalf("got scientific notation: %q", result)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexStr(s, sub) >= 0)
}

func indexStr(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
