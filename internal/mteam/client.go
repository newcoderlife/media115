package mteam

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/bytedance/gg/gconv"

	"github.com/newcoderlife/media115/internal/logging"
)

// DefaultBaseURL is M-Team's public OpenAPI root.
const DefaultBaseURL = "https://api.m-team.cc/api"

// Default per-request timeout.
const defaultTimeout = 30 * time.Second

// Client talks to the M-Team HTTP API.
type Client struct {
	baseURL     string
	apiKey      string
	httpClient  *http.Client
	minInterval time.Duration

	mu      sync.Mutex
	lastReq time.Time
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient overrides the default http.Client (used in tests).
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.httpClient = h }
}

// WithBaseURL overrides the API base URL.
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = strings.TrimRight(u, "/") }
}

// WithMinInterval sets the minimum wall-clock gap between outbound requests.
// Zero disables local pacing.
func WithMinInterval(d time.Duration) Option {
	return func(c *Client) { c.minInterval = d }
}

// NewClient builds a Client with the given API key and options.
func NewClient(apiKey string, opts ...Option) *Client {
	c := &Client{
		baseURL:     DefaultBaseURL,
		apiKey:      apiKey,
		httpClient:  &http.Client{Timeout: defaultTimeout},
		minInterval: 0,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// pace waits until at least minInterval has elapsed since the previous call.
// Called under the client mutex.
func (c *Client) pace(ctx context.Context) error {
	if c.minInterval <= 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.lastReq.IsZero() {
		wait := c.minInterval - time.Since(c.lastReq)
		if wait > 0 {
			timer := time.NewTimer(wait)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	c.lastReq = time.Now()
	return nil
}

// doJSON posts jsonBody to path and returns the response body.
func (c *Client) doJSON(ctx context.Context, path string, jsonBody any) ([]byte, error) {
	if err := c.pace(ctx); err != nil {
		return nil, err
	}
	buf, err := json.Marshal(jsonBody)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(buf))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	return c.send(req)
}

// doForm posts form values to path and returns the response body.
// Pass nil for parameterless POST endpoints.
func (c *Client) doForm(ctx context.Context, path string, form url.Values) ([]byte, error) {
	if err := c.pace(ctx); err != nil {
		return nil, err
	}
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	}
	req.Header.Set("x-api-key", c.apiKey)
	return c.send(req)
}

func (c *Client) send(req *http.Request) ([]byte, error) {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		snippet := string(body)
		if len(snippet) > 256 {
			snippet = snippet[:256]
		}
		return nil, fmt.Errorf("mteam: http %d: %s", resp.StatusCode, snippet)
	}
	return body, nil
}

// Search calls /torrent/search with the given request body.
func (c *Client) Search(ctx context.Context, req SearchRequest) (*SearchData, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("mteam: api_key is not configured")
	}
	if req.PageNumber <= 0 {
		req.PageNumber = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 20
	}
	if req.PageSize > 200 {
		req.PageSize = 200
	}
	if req.Mode == "" {
		req.Mode = ModeNormal
	}
	body, err := c.doJSON(ctx, "/torrent/search", req)
	if err != nil {
		return nil, err
	}
	var out SearchResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode search: %w", err)
	}
	if !isSuccess(out.Code, out.Message) {
		return nil, fmt.Errorf("mteam: search failed: code=%s message=%s", string(out.Code), out.Message)
	}
	logging.Log(slog.LevelDebug, fmt.Sprintf("mteam search keyword=%q mode=%s page=%d results=%d total=%d",
		req.Keyword, req.Mode, req.PageNumber, len(out.Data.List), gconv.To[int64, string](out.Data.Total)), "mteam")
	return &out.Data, nil
}

// GenDlToken calls /torrent/genDlToken and returns the direct download URL.
func (c *Client) GenDlToken(ctx context.Context, torrentID string) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("mteam: api_key is not configured")
	}
	if torrentID == "" {
		return "", fmt.Errorf("mteam: empty torrent id")
	}
	body, err := c.doForm(ctx, "/torrent/genDlToken", url.Values{"id": {torrentID}})
	if err != nil {
		return "", err
	}
	var out GenericResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("decode token: %w", err)
	}
	if !isSuccess(out.Code, out.Message) {
		return "", fmt.Errorf("mteam: genDlToken failed: code=%s message=%s", string(out.Code), out.Message)
	}
	var link string
	if err := json.Unmarshal(out.Data, &link); err != nil {
		return "", fmt.Errorf("decode token link: %w", err)
	}
	if link == "" {
		return "", fmt.Errorf("mteam: empty download url")
	}
	return link, nil
}

// DownloadTorrent fetches the .torrent bytes from the pre-signed URL.
func (c *Client) DownloadTorrent(ctx context.Context, dlURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("mteam: download http %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read torrent: %w", err)
	}
	if len(data) < 8 || data[0] != 'd' {
		return nil, fmt.Errorf("mteam: response is not a bencoded torrent (got %d bytes)", len(data))
	}
	return data, nil
}

// isSuccess reports whether the generic envelope signals success.
// M-Team returns code as 0 (int) or "0" (string) and message "SUCCESS".
func isSuccess(code json.RawMessage, message string) bool {
	if strings.EqualFold(message, "SUCCESS") {
		return true
	}
	s := strings.Trim(string(code), `"`)
	return s == "0"
}

// Files calls /torrent/files to get the file list of a torrent.
func (c *Client) Files(ctx context.Context, torrentID string) ([]TorrentFile, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("mteam: api_key is not configured")
	}
	if torrentID == "" {
		return nil, fmt.Errorf("mteam: empty torrent id")
	}
	body, err := c.doForm(ctx, "/torrent/files", url.Values{"id": {torrentID}})
	if err != nil {
		return nil, err
	}
	var out GenericResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode files: %w", err)
	}
	if !isSuccess(out.Code, out.Message) {
		return nil, fmt.Errorf("mteam: files failed: code=%s message=%s", string(out.Code), out.Message)
	}
	var files []TorrentFile
	if err := json.Unmarshal(out.Data, &files); err != nil {
		return nil, fmt.Errorf("decode file list: %w", err)
	}
	logging.Log(slog.LevelDebug, fmt.Sprintf("mteam files id=%s count=%d", torrentID, len(files)), "mteam")
	return files, nil
}

// fetchCatalog calls a parameterless POST endpoint and returns []CatalogItem.
func (c *Client) fetchCatalog(ctx context.Context, path string) ([]CatalogItem, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("mteam: api_key is not configured")
	}
	body, err := c.doForm(ctx, path, nil)
	if err != nil {
		return nil, err
	}
	var out GenericResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode catalog: %w", err)
	}
	if !isSuccess(out.Code, out.Message) {
		return nil, fmt.Errorf("mteam: %s failed: code=%s message=%s", path, string(out.Code), out.Message)
	}
	var items []CatalogItem
	if err := json.Unmarshal(out.Data, &items); err != nil {
		return nil, fmt.Errorf("decode catalog list: %w", err)
	}
	return items, nil
}

// Catalog calls /torrent/<kind>List and returns the catalog items.
// Valid kinds: category, source, standard, videoCodec, audioCodec, team, processing, medium.
func (c *Client) Catalog(ctx context.Context, kind string) ([]CatalogItem, error) {
	return c.fetchCatalog(ctx, "/torrent/"+kind+"List")
}
