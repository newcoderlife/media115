// Package cloud115 – raw HTTP client for the 115 cloud API.
//
// This file implements the API struct: a thin HTTP layer that handles cookies,
// rate-limiting, retry, 429/405 recovery, and M115/EC115 encryption.
// Higher-level caching lives in client.go / cached_client.go.
package cloud115

import (
	"bytes"
	"crypto/md5"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/bytedance/gg/gconv"
	"github.com/newcoderlife/media115/internal/logging"
)

// API endpoint base URLs.
const (
	WebAPI      = "https://webapi.115.com"
	ProAPI      = "https://proapi.115.com"
	QRAPI       = "https://qrcodeapi.115.com"
	PassportAPI = "https://passportapi.115.com"

	defaultUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"
)

// API is a low-level 115 cloud HTTP client (cookie mode only).
// Every public method maps to one HTTP endpoint.
type API struct {
	http      *http.Client
	cookies   string
	userID    string
	limiter   *RateLimiter
	dlLimiter *RateLimiter
	logger    *slog.Logger
}

// NewAPI constructs an API client from a cookie string.
// cache may be nil (disables rate-limit persistence).
// logger may be nil (uses slog.Default()).
func NewAPI(cookies string, cache *Cache, logger *slog.Logger) *API {
	if logger == nil {
		logger = slog.Default()
	}
	a := &API{
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
		cookies:   cookies,
		limiter:   NewRateLimiter("api", 0.5, 20, cache, logger),
		dlLimiter: NewRateLimiter("download", 0.5, 20, cache, logger),
		logger:    logger,
	}
	// Extract numeric user ID from UID=<id>_<rest> cookie fragment.
	for _, part := range strings.Split(cookies, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "UID=") {
			uid := part[4:]
			if idx := strings.Index(uid, "_"); idx >= 0 {
				uid = uid[:idx]
			}
			a.userID = uid
			break
		}
	}
	return a
}

// GetCookies returns the current cookie string.
func (a *API) GetCookies() string { return a.cookies }

// ── internal request helpers ────────────────────────────────────────────────

// cookieRequest is the central HTTP dispatch: rate-limit retry 429/405 parse.
func (a *API) cookieRequest(method, rawURL string, params url.Values, data url.Values, limiter *RateLimiter, userAgent string) (map[string]any, error) {
	if limiter == nil {
		limiter = a.limiter
	}
	if err := limiter.Acquire(); err != nil {
		return nil, err
	}

	// Build a short log line: method + path suffix + key params.
	apiPath := rawURL
	if idx := strings.Index(rawURL, ".com"); idx >= 0 {
		apiPath = rawURL[idx+4:]
		if len(apiPath) > 40 {
			apiPath = apiPath[:40]
		}
	}
	var keyParts []string
	for _, k := range []string{"cid", "path", "search_value", "pick_code", "pickcode"} {
		if v := params.Get(k); v != "" {
			keyParts = append(keyParts, k+"="+v)
		}
	}
	for _, k := range []string{"pid", "cname", "fid", "file_name", "filename", "target"} {
		if v := data.Get(k); v != "" {
			keyParts = append(keyParts, k+"="+v)
		}
	}
	logLine := method + " " + apiPath
	if len(keyParts) > 0 {
		logLine += " " + strings.Join(keyParts, " ")
	}
	logging.Log(a.logger, slog.LevelDebug, "request "+logLine, "api")

	ua := defaultUA
	if userAgent != "" {
		ua = userAgent
	}

	doRequest := func() (*http.Response, error) {
		u, err := url.Parse(rawURL)
		if err != nil {
			return nil, err
		}
		if len(params) > 0 {
			u.RawQuery = params.Encode()
		}
		var body io.Reader
		if method == "POST" && len(data) > 0 {
			body = strings.NewReader(data.Encode())
		}
		req, err := http.NewRequest(method, u.String(), body)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Cookie", a.cookies)
		req.Header.Set("User-Agent", ua)
		req.Header.Set("Origin", "https://115.com")
		req.Header.Set("Referer", "https://115.com/")
		if method == "POST" && len(data) > 0 {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		return a.http.Do(req)
	}

	const maxRetries = 3
	var resp *http.Response
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		resp, lastErr = doRequest()
		if lastErr == nil {
			break
		}
		if attempt < maxRetries-1 {
			logging.Log(a.logger, slog.LevelWarn, fmt.Sprintf("network error attempt=%d/%d err=%v", attempt+1, maxRetries, lastErr), "api")
			time.Sleep(time.Duration(3*(attempt+1)) * time.Second)
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("115 request failed after %d attempts: %w", maxRetries, lastErr)
	}
	defer resp.Body.Close()

	// Handle 429 set cooldown.
	if resp.StatusCode == 429 {
		a.limiter.SetCooldown(3600)
		a.dlLimiter.SetCooldown(3600)
		logging.Log(a.logger, slog.LevelWarn, "rate_limit 429 cooldown=3600s", "api")
		return nil, fmt.Errorf("115 API rate limit hit (429). Cooling down for 1 hour.")
	}

	// Handle 405 try renew cookies, then retry once.
	if resp.StatusCode == 405 {
		resp.Body.Close()
		if a.RenewCookies("tv") {
			resp2, err := doRequest()
			if err != nil {
				return nil, err
			}
			resp = resp2
			defer resp.Body.Close()
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("115 HTTP %d for %s", resp.StatusCode, rawURL)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("115 JSON decode: %w", err)
	}

	if err := checkBusinessErrors(result); err != nil {
		a.limiter.SetCooldown(3600)
		a.dlLimiter.SetCooldown(3600)
		return nil, err
	}

	return result, nil
}

// checkBusinessErrors detects 115 business-level rate-limit codes.
func checkBusinessErrors(result map[string]any) error {
	if result == nil {
		return nil
	}
	if errNo, ok := result["err_no"]; ok {
		var code float64
		switch v := errNo.(type) {
		case float64:
			code = v
		case int:
			code = float64(v)
		}
		errMsg := fmt.Sprintf("%v", result["error"])
		if code == 770004 || strings.Contains(errMsg, "访问上限") {
			return fmt.Errorf("115 API rate limit hit (errNo=%.0f). Cooling down.", code)
		}
	}
	return nil
}

// ── Auth ─────────────────────────────────────────────────────────────────────

// CheckLogin returns true if the current cookies are valid.
func (a *API) CheckLogin() bool {
	logging.Log(a.logger, slog.LevelDebug, "check_login sending", "api")
	req, err := http.NewRequest("GET", "https://my.115.com/?ct=guide&ac=status", nil)
	if err != nil {
		logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("check_login failed to build request err=%v", err), "api")
		return false
	}
	req.Header.Set("Cookie", a.cookies)
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("check_login network error err=%v", err), "api")
		return false
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("check_login unexpected status=%d", resp.StatusCode), "api")
		return false
	}
	defer resp.Body.Close()
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("check_login JSON decode error err=%v", err), "api")
		return false
	}
	state, _ := result["state"].(bool)
	logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("check_login state=%t", state), "api")
	return state
}

// RenewCookies attempts to silently refresh the session cookies via QR token flow.
func (a *API) RenewCookies(app string) bool {
	if a.cookies == "" {
		logging.Log(a.logger, slog.LevelDebug, "renew_cookies no cookies, skipping", "api")
		return false
	}
	if app == "" {
		app = "tv"
	}
	client := &http.Client{Timeout: 15 * time.Second}

	doGet := func(rawURL string, params url.Values, cookieHdr string) (*http.Response, error) {
		u, err := url.Parse(rawURL)
		if err != nil {
			return nil, err
		}
		if len(params) > 0 {
			u.RawQuery = params.Encode()
		}
		req, err := http.NewRequest("GET", u.String(), nil)
		if err != nil {
			return nil, err
		}
		if cookieHdr != "" {
			req.Header.Set("Cookie", cookieHdr)
		}
		return client.Do(req)
	}

	// Step 1: get token.
	logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("renew_cookies step1 fetching token app=%s", app), "api")
	resp, err := doGet(QRAPI+"/api/1.0/"+app+"/1.0/token/", nil, "")
	if err != nil {
		logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("renew_cookies step1 failed err=%v", err), "api")
		return false
	}
	defer resp.Body.Close()
	var tokenResult map[string]any
	if json.NewDecoder(resp.Body).Decode(&tokenResult) != nil {
		logging.Log(a.logger, slog.LevelDebug, "renew_cookies step1 JSON decode failed", "api")
		return false
	}
	tokenData, _ := tokenResult["data"].(map[string]any)
	if tokenData == nil {
		logging.Log(a.logger, slog.LevelDebug, "renew_cookies step1 no token data", "api")
		return false
	}
	uid, _ := tokenData["uid"].(string)
	if uid == "" {
		logging.Log(a.logger, slog.LevelDebug, "renew_cookies step1 empty uid", "api")
		return false
	}
	logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("renew_cookies step1 ok uid=%s", uid), "api")

	// Step 2: prompt.
	logging.Log(a.logger, slog.LevelDebug, "renew_cookies step2 prompt", "api")
	resp2, err := doGet(QRAPI+"/api/2.0/prompt.php", url.Values{"uid": {uid}}, a.cookies)
	if err != nil {
		logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("renew_cookies step2 failed err=%v", err), "api")
		return false
	}
	resp2.Body.Close()
	logging.Log(a.logger, slog.LevelDebug, "renew_cookies step2 ok", "api")

	// Step 3: slogin.
	logging.Log(a.logger, slog.LevelDebug, "renew_cookies step3 slogin", "api")
	resp3, err := doGet(QRAPI+"/api/2.0/slogin.php",
		url.Values{"key": {uid}, "uid": {uid}, "client": {"0"}}, a.cookies)
	if err != nil {
		logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("renew_cookies step3 failed err=%v", err), "api")
		return false
	}
	resp3.Body.Close()
	logging.Log(a.logger, slog.LevelDebug, "renew_cookies step3 ok", "api")

	// Step 4: exchange for new cookies.
	logging.Log(a.logger, slog.LevelDebug, "renew_cookies step4 exchange for new cookies", "api")
	formData := url.Values{"account": {uid}, "app": {app}}
	req, err := http.NewRequest("POST", PassportAPI+"/app/1.0/"+app+"/1.0/login/qrcode",
		strings.NewReader(formData.Encode()))
	if err != nil {
		logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("renew_cookies step4 build request failed err=%v", err), "api")
		return false
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp4, err := client.Do(req)
	if err != nil {
		logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("renew_cookies step4 request failed err=%v", err), "api")
		return false
	}
	defer resp4.Body.Close()
	var loginResult map[string]any
	if json.NewDecoder(resp4.Body).Decode(&loginResult) != nil {
		logging.Log(a.logger, slog.LevelDebug, "renew_cookies step4 JSON decode failed", "api")
		return false
	}
	data, _ := loginResult["data"].(map[string]any)
	if data == nil {
		logging.Log(a.logger, slog.LevelDebug, "renew_cookies step4 no data in response", "api")
		return false
	}
	cookies, _ := data["cookie"].(map[string]any)
	if len(cookies) == 0 {
		logging.Log(a.logger, slog.LevelDebug, "renew_cookies step4 empty cookies", "api")
		return false
	}
	var parts []string
	for k, v := range cookies {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	cookieStr := strings.Join(parts, "; ")
	if len(cookieStr) < 10 {
		logging.Log(a.logger, slog.LevelDebug, "renew_cookies step4 cookie string too short", "api")
		return false
	}
	a.cookies = cookieStr
	for _, part := range strings.Split(cookieStr, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "UID=") {
			uid := part[4:]
			if idx := strings.Index(uid, "_"); idx >= 0 {
				uid = uid[:idx]
			}
			a.userID = uid
			break
		}
	}
	logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("renew_cookies success cookieLen=%d", len(cookieStr)), "api")
	return true
}

// formatNum converts a JSON number (float64) to a clean integer string.
// When JSON decodes into map[string]any, numbers become float64, so
// fmt.Sprintf("%v", float64(1775973641)) produces "1.775973641e+09" (scientific
// notation). This helper avoids that by casting through int64 first.
func formatNum(v any) string {
	if n := gconv.To[int64, any](v); n != 0 {
		return strconv.FormatInt(n, 10)
	}
	return gconv.To[string, any](v)
}

// QRLogin performs an interactive QR-code login and updates the client cookies.
func (a *API) QRLogin(app string) error {
	if app == "" {
		app = "tv"
	}
	logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("qr_login fetching token app=%s", app), "api")
	client := &http.Client{Timeout: 35 * time.Second}

	resp, err := client.Get(QRAPI + "/api/1.0/" + app + "/1.0/token/")
	if err != nil {
		logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("qr_login token fetch failed err=%v", err), "api")
		return err
	}
	var tokenResult map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&tokenResult)
	resp.Body.Close()

	tokenData, _ := tokenResult["data"].(map[string]any)
	if tokenData == nil {
		return fmt.Errorf("QRLogin: no token data")
	}
	uid, _ := tokenData["uid"].(string)
	qrTime := formatNum(tokenData["time"])
	sign, _ := tokenData["sign"].(string)
	logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("qr_login token obtained uid=%s", uid), "api")

	qrImageURL := fmt.Sprintf("%s/api/1.0/web/1.0/qrcode?qrfrom=1&client=0d&uid=%s", QRAPI, uid)
	fmt.Printf("Scan QR: %s\n", qrImageURL)
	fmt.Println("Waiting for scan...")

	maxPolls := 180 // 3 minutes
	loginConfirmed := false
	for i := 0; i < maxPolls; i++ {
		params := url.Values{
			"uid":  {uid},
			"time": {qrTime},
			"sign": {sign},
			"_":    {fmt.Sprintf("%d", time.Now().Unix())},
		}
		u, _ := url.Parse(QRAPI + "/get/status/")
		u.RawQuery = params.Encode()
		statusResp, err := client.Get(u.String())
		if err != nil {
			logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("qr_login poll error attempt=%d err=%v", i, err), "api")
			time.Sleep(time.Second)
			continue
		}
		body, _ := io.ReadAll(statusResp.Body)
		statusResp.Body.Close()
		if len(body) == 0 {
			time.Sleep(time.Second)
			continue
		}
		var statusResult map[string]any
		if json.Unmarshal(body, &statusResult) != nil {
			time.Sleep(time.Second)
			continue
		}
		statusData, _ := statusResult["data"].(map[string]any)
		status := 0
		if s, ok := statusData["status"].(float64); ok {
			status = int(s)
		}
		logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("qr_login poll attempt=%d status=%d", i, status), "api")
		switch status {
		case 0:
			time.Sleep(2 * time.Second)
		case 1:
			fmt.Println("QR scanned, waiting for confirmation...")
			time.Sleep(time.Second)
		case 2:
			fmt.Println("Login confirmed!")
			loginConfirmed = true
		case -1:
			return fmt.Errorf("QR code expired")
		case -2:
			return fmt.Errorf("login cancelled")
		default:
			time.Sleep(time.Second)
		}
		if loginConfirmed {
			break
		}
	}
	if !loginConfirmed {
		return fmt.Errorf("QR login timed out after %d seconds", maxPolls)
	}

	formData := url.Values{"account": {uid}, "app": {app}}
	req, err := http.NewRequest("POST",
		PassportAPI+"/app/1.0/"+app+"/1.0/login/qrcode",
		strings.NewReader(formData.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginResp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer loginResp.Body.Close()
	var loginResult map[string]any
	_ = json.NewDecoder(loginResp.Body).Decode(&loginResult)
	loginData, _ := loginResult["data"].(map[string]any)
	if loginData == nil {
		return fmt.Errorf("QR login: no data in response")
	}
	cookies, _ := loginData["cookie"].(map[string]any)
	var parts []string
	for k, v := range cookies {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	cookieStr := strings.Join(parts, "; ")
	a.cookies = cookieStr
	fmt.Printf("Login success! Cookie length: %d\n", len(cookieStr))
	return nil
}

// QRSession holds the state needed for a two-phase QR login.
type QRSession struct {
	UID    string `json:"uid"`
	Time   string `json:"time"`
	Sign   string `json:"sign"`
	App    string `json:"app"`
	QRURL  string `json:"qr_url"`
}

// QRGetToken fetches a QR login token and returns the session (phase 1).
// Call QRWaitAndLogin with the returned session to complete the login (phase 2).
func (a *API) QRGetToken(app string) (*QRSession, error) {
	if app == "" {
		app = "tv"
	}
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(QRAPI + "/api/1.0/" + app + "/1.0/token/")
	if err != nil {
		return nil, err
	}
	var tokenResult map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&tokenResult)
	resp.Body.Close()

	tokenData, _ := tokenResult["data"].(map[string]any)
	if tokenData == nil {
		return nil, fmt.Errorf("QRGetToken: no token data")
	}
	uid, _ := tokenData["uid"].(string)
	qrTime := formatNum(tokenData["time"])
	sign, _ := tokenData["sign"].(string)
	qrURL := fmt.Sprintf("%s/api/1.0/web/1.0/qrcode?qrfrom=1&client=0d&uid=%s", QRAPI, uid)

	return &QRSession{
		UID:   uid,
		Time:  qrTime,
		Sign:  sign,
		App:   app,
		QRURL: qrURL,
	}, nil
}

// QRWaitAndLogin polls for QR scan completion and finalizes login (phase 2).
// Updates the API cookie string on success.
func (a *API) QRWaitAndLogin(sess *QRSession) error {
	logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("qr_wait_and_login starting poll uid=%s app=%s", sess.UID, sess.App), "api")
	client := &http.Client{Timeout: 35 * time.Second}

	maxPolls := 180 // 3 minutes
	loginConfirmed := false
	for i := 0; i < maxPolls; i++ {
		params := url.Values{
			"uid":  {sess.UID},
			"time": {sess.Time},
			"sign": {sess.Sign},
			"_":    {fmt.Sprintf("%d", time.Now().Unix())},
		}
		u, _ := url.Parse(QRAPI + "/get/status/")
		u.RawQuery = params.Encode()
		statusResp, err := client.Get(u.String())
		if err != nil {
			logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("qr_wait_and_login poll error attempt=%d err=%v", i, err), "api")
			time.Sleep(time.Second)
			continue
		}
		body, _ := io.ReadAll(statusResp.Body)
		statusResp.Body.Close()
		if len(body) == 0 {
			time.Sleep(time.Second)
			continue
		}
		var statusResult map[string]any
		if json.Unmarshal(body, &statusResult) != nil {
			time.Sleep(time.Second)
			continue
		}
		statusData, _ := statusResult["data"].(map[string]any)
		status := 0
		if s, ok := statusData["status"].(float64); ok {
			status = int(s)
		}
		logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("qr_wait_and_login poll attempt=%d status=%d", i, status), "api")
		switch status {
		case 0:
			time.Sleep(2 * time.Second)
		case 1:
			fmt.Println("QR scanned, waiting for confirmation...")
			time.Sleep(time.Second)
		case 2:
			fmt.Println("Login confirmed!")
			loginConfirmed = true
		case -1:
			return fmt.Errorf("QR code expired")
		case -2:
			return fmt.Errorf("login cancelled")
		default:
			time.Sleep(time.Second)
		}
		if loginConfirmed {
			break
		}
	}
	if !loginConfirmed {
		return fmt.Errorf("QR login timed out after %d seconds", maxPolls)
	}

	formData := url.Values{"account": {sess.UID}, "app": {sess.App}}
	req, err := http.NewRequest("POST",
		PassportAPI+"/app/1.0/"+sess.App+"/1.0/login/qrcode",
		strings.NewReader(formData.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginResp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer loginResp.Body.Close()
	var loginResult map[string]any
	_ = json.NewDecoder(loginResp.Body).Decode(&loginResult)
	loginData, _ := loginResult["data"].(map[string]any)
	if loginData == nil {
		return fmt.Errorf("QR login: no data in response")
	}
	cookies, _ := loginData["cookie"].(map[string]any)
	var parts []string
	for k, v := range cookies {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	cookieStr := strings.Join(parts, "; ")
	a.cookies = cookieStr
	// Extract userID from cookies (same as constructor).
	for _, part := range strings.Split(cookieStr, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "UID=") {
			a.userID = strings.SplitN(part[4:], "_", 2)[0]
			break
		}
	}
	fmt.Printf("Login success! Cookie length: %d\n", len(a.cookies))
	return nil
}

// SaveCookies updates the TOML config file at path, replacing the cookies value.
func (a *API) SaveCookies(path string) error {
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	lines := strings.Split(string(data), "\n")
	replaced := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "cookies") && strings.Contains(line, "=") {
			lines[i] = fmt.Sprintf(`cookies = %q`, a.cookies)
			replaced = true
			break
		}
	}
	if !replaced {
		lines = append(lines, fmt.Sprintf(`cookies = %q`, a.cookies))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o600)
}

// ── Read API ─────────────────────────────────────────────────────────────────

// ListFiles returns up to limit entries in dirID starting at offset.
func (a *API) ListFiles(dirID string, limit, offset int) ([]map[string]any, error) {
	result, err := a.cookieRequest("GET", WebAPI+"/files", url.Values{
		"aid":      {"1"},
		"cid":      {dirID},
		"limit":    {fmt.Sprintf("%d", limit)},
		"offset":   {fmt.Sprintf("%d", offset)},
		"show_dir": {"1"},
		"o":        {"user_ptime"},
		"asc":      {"1"},
		"natsort":  {"1"},
		"format":   {"json"},
	}, nil, nil, "")
	if err != nil {
		return nil, err
	}
	return toSliceOfMaps(result["data"]), nil
}

// ListFilesAll returns all entries in dirID, handling pagination automatically.
func (a *API) ListFilesAll(dirID string) ([]map[string]any, error) {
	var all []map[string]any
	offset := 0
	pages := 0
	for {
		batch, err := a.ListFiles(dirID, 1000, offset)
		if err != nil {
			return nil, err
		}
		pages++
		if len(batch) == 0 {
			break
		}
		all = append(all, batch...)
		if len(batch) < 1000 {
			break
		}
		offset += len(batch)
	}
	logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("list_files_all cid=%s %d items (%d pages)", dirID, len(all), pages), "api")
	return all, nil
}

// GetDirID resolves an absolute path string to a directory ID.
// Returns "" when the path does not exist (including when the API returns id=0).
func (a *API) GetDirID(path string) (string, error) {
	result, err := a.cookieRequest("GET", WebAPI+"/files/getid", url.Values{"path": {path}}, nil, nil, "")
	if err != nil {
		return "", err
	}
	state, _ := result["state"].(bool)
	if !state {
		logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("get_dir_id not found (state=false) path=%s", path), "api")
		return "", nil
	}
	cid := formatNum(result["id"])
	// 115 returns id="0" for non-existent paths even when state=true.
	if cid == "" || cid == "0" {
		logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("get_dir_id not found (id=0) path=%s", path), "api")
		return "", nil
	}
	logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("get_dir_id path=%s cid=%s", path, cid), "api")
	return cid, nil
}

// Search returns entries matching keyword in dirID.
func (a *API) Search(keyword, dirID string) ([]map[string]any, error) {
	result, err := a.cookieRequest("GET", WebAPI+"/files/search", url.Values{
		"search_value": {keyword},
		"cid":          {dirID},
		"format":       {"json"},
	}, nil, nil, "")
	if err != nil {
		return nil, err
	}
	data := toSliceOfMaps(result["data"])
	logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("search keyword=%s cid=%s %d results", keyword, dirID, len(data)), "api")
	return data, nil
}

// DownloadURL returns the download URL for the given pick code.
// userAgent is sent in the request (affects whether CDN gives a direct URL).
func (a *API) DownloadURL(pickCode, userAgent string) (string, error) {
	key := GenerateM115Key()
	payload, _ := json.Marshal(map[string]string{"pickcode": pickCode})
	encrypted := M115Encode(key, string(payload))

	if err := a.dlLimiter.Acquire(); err != nil {
		return "", err
	}

	ua := defaultUA
	if userAgent != "" {
		ua = userAgent
	}

	rawURL := ProAPI + "/app/chrome/downurl"
	params := url.Values{"t": {fmt.Sprintf("%d", time.Now().Unix())}}
	postData := url.Values{"data": {encrypted}}

	doPost := func() (*http.Response, error) {
		u, _ := url.Parse(rawURL)
		u.RawQuery = params.Encode()
		req, err := http.NewRequest("POST", u.String(), strings.NewReader(postData.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Cookie", a.cookies)
		req.Header.Set("User-Agent", ua)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "https://115.com")
		req.Header.Set("Referer", "https://115.com/")
		return a.http.Do(req)
	}

	const maxRetries = 3
	var resp *http.Response
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		resp, lastErr = doPost()
		if lastErr == nil {
			break
		}
		if attempt < maxRetries-1 {
			logging.Log(a.logger, slog.LevelWarn, fmt.Sprintf("download_url network error attempt=%d/%d", attempt+1, maxRetries), "api")
			time.Sleep(time.Duration(3*(attempt+1)) * time.Second)
		}
	}
	if lastErr != nil {
		return "", fmt.Errorf("115 download failed after %d attempts: %w", maxRetries, lastErr)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		a.limiter.SetCooldown(3600)
		a.dlLimiter.SetCooldown(3600)
		return "", fmt.Errorf("115 API rate limit hit (429). Cooling down.")
	}
	if resp.StatusCode == 405 {
		resp.Body.Close()
		if a.RenewCookies("tv") {
			resp2, err := doPost()
			if err != nil {
				return "", err
			}
			resp = resp2
			defer resp.Body.Close()
		}
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("115 download HTTP %d", resp.StatusCode)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("115 download JSON: %w", err)
	}
	if err := checkBusinessErrors(result); err != nil {
		a.limiter.SetCooldown(3600)
		a.dlLimiter.SetCooldown(3600)
		return "", err
	}

	encData, _ := result["data"].(string)
	decrypted := M115Decode(key, encData)
	var decoded map[string]any
	if err := json.Unmarshal([]byte(decrypted), &decoded); err != nil {
		return "", fmt.Errorf("115 download decode: %w", err)
	}
	for _, val := range decoded {
		valMap, _ := val.(map[string]any)
		if valMap == nil {
			continue
		}
		switch urlVal := valMap["url"].(type) {
		case map[string]any:
			if u, ok := urlVal["url"].(string); ok && u != "" {
				return u, nil
			}
		case string:
			if urlVal != "" {
				return urlVal, nil
			}
		}
	}
	return "", fmt.Errorf("no download URL for pick_code=%s", pickCode)
}

// ExportTree exports a directory tree and returns the UTF-16LE decoded text.
func (a *API) ExportTree(dirID string) (string, error) {
	logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("export_tree starting cid=%s", dirID), "api")

	// Start export.
	startResult, err := a.cookieRequest("POST", WebAPI+"/files/export_dir",
		nil, url.Values{"file_ids": {dirID}, "target": {"U_1_0"}}, nil, "")
	if err != nil {
		return "", err
	}
	exportID := "0"
	if d, ok := startResult["data"].(map[string]any); ok {
		if id, ok := d["export_id"]; ok {
			exportID = formatNum(id)
		}
	}

	// Poll for completion.
	var pickCode, fileID string
	for i := 0; i < 60; i++ {
		time.Sleep(3 * time.Second)
		statusResult, err := a.cookieRequest("GET", WebAPI+"/files/export_dir",
			url.Values{"export_id": {exportID}}, nil, nil, "")
		if err != nil {
			continue
		}
		var statusData map[string]any
		switch d := statusResult["data"].(type) {
		case map[string]any:
			statusData = d
		case []any:
			if len(d) > 0 {
				statusData, _ = d[0].(map[string]any)
			}
		}
		if statusData != nil {
			if pc, ok := statusData["pick_code"].(string); ok && pc != "" {
				pickCode = pc
				if fid, ok := statusData["file_id"]; ok {
					fileID = formatNum(fid)
				}
				break
			}
		}
	}
	if pickCode == "" {
		return "", fmt.Errorf("export_tree: timed out waiting for pick_code")
	}

	dlURL := "https://115.com/"
	dlParams := url.Values{
		"ct":       {"download"},
		"ac":       {"video"},
		"pickcode": {pickCode},
	}
	u, _ := url.Parse(dlURL)
	u.RawQuery = dlParams.Encode()

	cookieHdr := a.cookies
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", defaultUA)

	// Parse cookies into a jar so they are sent on every request in the
	// redirect chain, matching Python httpx's cookies=dict behaviour.
	jar, _ := cookiejar.New(nil)
	var parsedCookies []*http.Cookie
	for _, part := range strings.Split(cookieHdr, ";") {
		part = strings.TrimSpace(part)
		if i := strings.Index(part, "="); i > 0 {
			parsedCookies = append(parsedCookies, &http.Cookie{
				Name:  part[:i],
				Value: part[i+1:],
			})
		}
	}
	jarURL, _ := url.Parse("https://115.com/")
	jar.SetCookies(jarURL, parsedCookies)
	dlClient := &http.Client{
		Timeout: 30 * time.Second,
		Jar:     jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			req.Header.Set("User-Agent", defaultUA)
			if len(via) > 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
	dlResp, err := dlClient.Do(req)
	if err != nil {
		return "", err
	}
	defer dlResp.Body.Close()
	content, err := io.ReadAll(dlResp.Body)
	if err != nil {
		return "", err
	}
	if dlResp.StatusCode != http.StatusOK || len(content) < 100 {
		return "", fmt.Errorf("export_tree: download failed (status=%d len=%d)", dlResp.StatusCode, len(content))
	}

	// Clean up the temporary tree file from 115.
	if fileID != "" && fileID != "0" {
		_, _ = a.Delete([]string{fileID})
	}

	text := decodeUTF16LE(content)
	logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("export_tree done cid=%s lines=%d", dirID, strings.Count(text, "\n")), "api")
	return text, nil
}

// UploadInfo returns the userID and userKey required for rapid-upload requests.
func (a *API) UploadInfo() (userID, userKey string, err error) {
	if err := a.limiter.Acquire(); err != nil {
		return "", "", err
	}
	req, err := http.NewRequest("GET", ProAPI+"/app/uploadinfo", nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Cookie", a.cookies)
	req.Header.Set("User-Agent", defaultUA)
	resp, err := a.http.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", err
	}
	userID = formatNum(result["user_id"])
	userKey, _ = result["userkey"].(string)
	return userID, userKey, nil
}

// ── Write API ─────────────────────────────────────────────────────────────────

// Mkdir creates a directory named name under parentID.
func (a *API) Mkdir(parentID, name string) (map[string]any, error) {
	result, err := a.cookieRequest("POST", WebAPI+"/files/add",
		nil, url.Values{"pid": {parentID}, "cname": {name}}, nil, "")
	if err != nil {
		return nil, err
	}
	logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("mkdir pid=%s name=%s", parentID, name), "api")
	return result, nil
}

// Move moves fileIDs into targetDirID.
func (a *API) Move(fileIDs []string, targetDirID string) (map[string]any, error) {
	data := url.Values{"pid": {targetDirID}}
	for i, fid := range fileIDs {
		data.Set(fmt.Sprintf("fid[%d]", i), fid)
	}
	result, err := a.cookieRequest("POST", WebAPI+"/files/move", nil, data, nil, "")
	if err != nil {
		return nil, err
	}
	logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("move count=%d pid=%s", len(fileIDs), targetDirID), "api")
	return result, nil
}

// Rename renames fileID to newName.
func (a *API) Rename(fileID, newName string) (map[string]any, error) {
	result, err := a.cookieRequest("POST", WebAPI+"/files/edit",
		nil, url.Values{"fid": {fileID}, "file_name": {newName}}, nil, "")
	if err != nil {
		return nil, err
	}
	logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("rename fid=%s new_name=%s", fileID, newName), "api")
	return result, nil
}

// Delete deletes fileIDs.
func (a *API) Delete(fileIDs []string) (map[string]any, error) {
	data := url.Values{}
	for i, fid := range fileIDs {
		data.Set(fmt.Sprintf("fid[%d]", i), fid)
	}
	result, err := a.cookieRequest("POST", WebAPI+"/rb/delete", nil, data, nil, "")
	if err != nil {
		return nil, err
	}
	logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("delete count=%d", len(fileIDs)), "api")
	return result, nil
}

// BatchRename renames multiple files in one API call.
// renames maps fileID newName.
func (a *API) BatchRename(renames map[string]string) (map[string]any, error) {
	data := url.Values{}
	for fid, name := range renames {
		data.Set(fmt.Sprintf("files_new_name[%s]", fid), name)
	}
	result, err := a.cookieRequest("POST", WebAPI+"/files/batch_rename", nil, data, nil, "")
	if err != nil {
		return nil, err
	}
	logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("batch_rename count=%d", len(renames)), "api")
	return result, nil
}

// UploadFile uploads a local file to targetDirID via OSS.
func (a *API) UploadFile(localPath, targetDirID, filename string) (map[string]any, error) {
	if filename == "" {
		filename = filepath.Base(localPath)
	}

	if err := a.limiter.Acquire(); err != nil {
		return nil, err
	}

	// Step 1: init upload.
	var initResult map[string]any
	for attempt := 0; attempt < 3; attempt++ {
		initData := url.Values{
			"filename": {filename},
			"target":   {"U_1_" + targetDirID},
		}
		req, err := http.NewRequest("POST", "https://uplb.115.com/3.0/sampleinitupload.php",
			strings.NewReader(initData.Encode()))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Cookie", a.cookies)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Origin", "https://115.com")
		req.Header.Set("Referer", "https://115.com/")
		resp, err := a.http.Do(req)
		if err != nil {
			if attempt < 2 {
				logging.Log(a.logger, slog.LevelWarn, fmt.Sprintf("upload_file init failed attempt=%d", attempt+1), "api")
				time.Sleep(time.Duration(3*(attempt+1)) * time.Second)
				continue
			}
			return nil, err
		}
		_ = json.NewDecoder(resp.Body).Decode(&initResult)
		resp.Body.Close()
		break
	}
	if initResult == nil {
		return nil, fmt.Errorf("upload: init returned nil")
	}

	// Step 2: POST to OSS using a streaming multipart writer with io.Pipe.
	// Re-open the file for each attempt so retries work correctly.
	for attempt := 0; attempt < 3; attempt++ {
		f, err := os.Open(localPath)
		if err != nil {
			return nil, fmt.Errorf("upload: open file: %w", err)
		}

		pr, pw := io.Pipe()
		mw := multipart.NewWriter(pw)
		go func() {
			defer f.Close()
			for _, kv := range []struct{ k, v string }{
				{"key", fmt.Sprintf("%v", initResult["object"])},
				{"OSSAccessKeyId", fmt.Sprintf("%v", initResult["accessid"])},
				{"policy", fmt.Sprintf("%v", initResult["policy"])},
				{"signature", fmt.Sprintf("%v", initResult["signature"])},
				{"callback", fmt.Sprintf("%v", initResult["callback"])},
			} {
				_ = mw.WriteField(kv.k, kv.v)
			}
			fw, err := mw.CreateFormFile("file", filename)
			if err != nil {
				pw.CloseWithError(err)
				return
			}
			if _, err := io.Copy(fw, f); err != nil {
				pw.CloseWithError(err)
				return
			}
			mw.Close()
			pw.Close()
		}()

		host := fmt.Sprintf("%v", initResult["host"])
		req, err := http.NewRequest("POST", host, pr)
		if err != nil {
			pr.CloseWithError(err)
			return nil, err
		}
		req.Header.Set("Content-Type", mw.FormDataContentType())
		ossClient := &http.Client{Timeout: 60 * time.Second}
		resp, err := ossClient.Do(req)
		if err != nil {
			pr.CloseWithError(err)
			if attempt < 2 {
				logging.Log(a.logger, slog.LevelWarn, fmt.Sprintf("upload_file oss failed attempt=%d", attempt+1), "api")
				time.Sleep(time.Duration(3*(attempt+1)) * time.Second)
				continue
			}
			return nil, err
		}
		if resp.StatusCode == http.StatusOK {
			var ossResult map[string]any
			_ = json.NewDecoder(resp.Body).Decode(&ossResult)
			resp.Body.Close()
			logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("upload_file filename=%s path=%s dir=%s", filename, localPath, targetDirID), "api")
			if data, ok := ossResult["data"].(map[string]any); ok {
				return data, nil
			}
			return ossResult, nil
		}
		logging.Log(a.logger, slog.LevelWarn, fmt.Sprintf("upload_file failed filename=%s status=%d", filename, resp.StatusCode), "api")
		resp.Body.Close()
		return nil, fmt.Errorf("115 OSS upload failed: HTTP %d", resp.StatusCode)
	}
	return nil, fmt.Errorf("upload: all OSS attempts failed")
}

// RapidUpload attempts an instant (hash-match) upload. Returns status 2 on success.
func (a *API) RapidUpload(dirID, filename string, fileSize int64, fileSHA1 string, fileStream io.ReadSeeker) (map[string]any, error) {
	userID, userKey, err := a.UploadInfo()
	if err != nil {
		return nil, err
	}

	ec, err := NewEC115Cipher()
	if err != nil {
		return nil, err
	}

	target := "U_1_" + dirID
	appVer := "2.0.3.6"
	signKey := ""
	signVal := ""

	for attempt := 0; attempt < 3; attempt++ {
		logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("rapid_upload filename=%s attempt=%d", filename, attempt+1), "api")
		timestamp := fmt.Sprintf("%d", time.Now().Unix())

		userHashBytes := md5.Sum([]byte(userID))
		userHash := fmt.Sprintf("%x", userHashBytes)

		h1Bytes := sha1.Sum([]byte(userID + fileSHA1 + target + "0"))
		h1 := fmt.Sprintf("%x", h1Bytes)

		sigBytes := sha1.Sum([]byte(userKey + h1 + "000000"))
		sig := strings.ToUpper(fmt.Sprintf("%x", sigBytes))

		tokenData := TokenSalt + fileSHA1 + fmt.Sprintf("%d", fileSize) +
			signKey + signVal + userID + timestamp + userHash + appVer
		tokenBytes := md5.Sum([]byte(tokenData))
		token := fmt.Sprintf("%x", tokenBytes)

		form := url.Values{
			"appid":      {"0"},
			"appversion": {appVer},
			"userid":     {userID},
			"filename":   {filename},
			"filesize":   {fmt.Sprintf("%d", fileSize)},
			"fileid":     {fileSHA1},
			"target":     {target},
			"sig":        {sig},
			"t":          {timestamp},
			"token":      {token},
		}
		if signKey != "" {
			form.Set("sign_key", signKey)
			form.Set("sign_val", signVal)
		}

		formBytes := []byte(form.Encode())
		encrypted := ec.Encode(formBytes)

		encToken := ec.EncodeToken()
		rawURL := "https://uplb.115.com/4.0/initupload.php?k_ec=" + url.QueryEscape(encToken)
		req, err := http.NewRequest("POST", rawURL, bytes.NewReader(encrypted))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Cookie", a.cookies)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if err := a.limiter.Acquire(); err != nil {
			return nil, err
		}
		resp, err := a.http.Do(req)
		if err != nil {
			return nil, err
		}
		rawBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("115 rapid_upload HTTP %d", resp.StatusCode)
		}
		decoded, err := ec.Decode(rawBody)
		if err != nil {
			return nil, fmt.Errorf("115 rapid_upload decode: %w", err)
		}
		var result map[string]any
		if err := json.Unmarshal(decoded, &result); err != nil {
			return nil, fmt.Errorf("115 rapid_upload JSON: %w", err)
		}

		status, _ := result["status"].(float64)
		switch int(status) {
		case 2:
			logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("rapid_upload success filename=%s pickcode=%v", filename, result["pickcode"]), "api")
			return map[string]any{"status": 2, "pickcode": result["pickcode"]}, nil
		case 7:
			if sc, _ := result["statuscode"].(float64); int(sc) == 701 && fileStream != nil {
				signKey, _ = result["sign_key"].(string)
				signRange, _ := result["sign_check"].(string)
				parts := strings.SplitN(signRange, "-", 2)
				if len(parts) == 2 {
					var start, end int
					fmt.Sscanf(parts[0], "%d", &start)
					fmt.Sscanf(parts[1], "%d", &end)
					_, _ = fileStream.Seek(int64(start), io.SeekStart)
					chunk, _ := io.ReadAll(io.LimitReader(fileStream, int64(end-start+1)))
					signValBytes := sha1.Sum(chunk)
					signVal = strings.ToUpper(fmt.Sprintf("%x", signValBytes))
				}
				continue
			}
			fallthrough
		default:
			logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("rapid_upload needs actual upload filename=%s status=%d", filename, int(status)), "api")
			return map[string]any{"status": int(status)}, nil
		}
	}
	logging.Log(a.logger, slog.LevelDebug, fmt.Sprintf("rapid_upload exceeded retries filename=%s", filename), "api")
	return map[string]any{"status": 0}, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// toSliceOfMaps converts an interface{} to []map[string]any safely.
func toSliceOfMaps(v any) []map[string]any {
	switch arr := v.(type) {
	case []any:
		out := make([]map[string]any, 0, len(arr))
		for _, item := range arr {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	case []map[string]any:
		return arr
	}
	return nil
}

// decodeUTF16LE converts a UTF-16 LE byte slice to a Go string,
// correctly handling surrogate pairs via unicode/utf16.
func decodeUTF16LE(b []byte) string {
	if len(b) < 2 {
		return string(b)
	}
	// Strip BOM if present.
	if b[0] == 0xFF && b[1] == 0xFE {
		b = b[2:]
	}
	u16 := make([]uint16, len(b)/2)
	for i := range u16 {
		u16[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	return string(utf16.Decode(u16))
}
