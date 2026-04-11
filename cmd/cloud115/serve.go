package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strings"

	"github.com/newcoderlife/media115/internal/cloud115"
	"github.com/newcoderlife/media115/internal/config"
	"github.com/spf13/cobra"
)

var (
	serveHost string
	servePort int
)

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "启动 strm-proxy：Jellyfin 反向代理 + 115 直链 302 跳转",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := getConfig()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}

		// Flag overrides config
		host := cfg.Proxy.Host
		port := cfg.Proxy.Port
		if cmd.Flags().Changed("host") {
			host = serveHost
		}
		if cmd.Flags().Changed("port") {
			port = servePort
		}
		jellyfinURL := cfg.Proxy.JellyfinURL

		client, err := getClient()
		if err != nil {
			return err
		}

		mux := buildProxyMux(client, jellyfinURL)
		addr := fmt.Sprintf("%s:%d", host, port)
		fmt.Printf("strm-proxy 启动: http://%s\n", addr)
		fmt.Printf("Jellyfin URL: %s\n", jellyfinURL)
		return http.ListenAndServe(addr, mux) //nolint:gosec
	},
}

var pickCodeRe = regexp.MustCompile(`/play/(\w+)`)

func extractPickCode(strmURL string) string {
	m := pickCodeRe.FindStringSubmatch(strmURL)
	if len(m) > 1 {
		return m[1]
	}
	return ""
}

// excludedHeaders are hop-by-hop headers that should not be forwarded.
var excludedHeaders = map[string]bool{
	"host":              true,
	"transfer-encoding": true,
	"content-encoding":  true,
	"content-length":    true,
}

func buildProxyMux(client *cloud115.Client, jellyfinURL string) http.Handler {
	jellyfinTarget, _ := url.Parse(jellyfinURL)
	proxy := httputil.NewSingleHostReverseProxy(jellyfinTarget)
	// Preserve original director but fix Host header
	origDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		origDirector(req)
		req.Host = jellyfinTarget.Host
	}

	mux := http.NewServeMux()

	// /play/{pick_code} → DownloadURL → 302
	mux.HandleFunc("/play/", func(w http.ResponseWriter, r *http.Request) {
		pickCode := strings.TrimPrefix(r.URL.Path, "/play/")
		if pickCode == "" {
			http.Error(w, "missing pick_code", http.StatusBadRequest)
			return
		}
		ua := r.Header.Get("User-Agent")
		dlURL, err := client.DownloadURL(pickCode, ua)
		if err != nil {
			http.Error(w, fmt.Sprintf("获取下载链接失败: %v", err), http.StatusInternalServerError)
			return
		}
		http.Redirect(w, r, dlURL, http.StatusFound)
	})

	// /redirect/{file_path} → StreamURL → 302
	mux.HandleFunc("/redirect/", func(w http.ResponseWriter, r *http.Request) {
		filePath := strings.TrimPrefix(r.URL.Path, "/redirect/")
		decoded, err := url.PathUnescape(filePath)
		if err != nil {
			decoded = filePath
		}
		if !strings.HasPrefix(decoded, "/") {
			decoded = "/" + decoded
		}
		ua := r.Header.Get("User-Agent")
		dlURL, err := client.StreamURL(decoded, ua)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				http.Error(w, err.Error(), http.StatusNotFound)
			} else {
				http.Error(w, fmt.Sprintf("Error: %v", err), http.StatusInternalServerError)
			}
			return
		}
		http.Redirect(w, r, dlURL, http.StatusFound)
	})

	// /Videos/{item_id}/{action} — intercept STRM files, proxy everything else
	mux.HandleFunc("/Videos/", func(w http.ResponseWriter, r *http.Request) {
		// Parse /Videos/{item_id}/{action}
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/Videos/"), "/", 2)
		if len(parts) < 2 {
			proxy.ServeHTTP(w, r)
			return
		}
		itemID := parts[0]
		action := parts[1]
		actionBase := strings.ToLower(strings.SplitN(action, ".", 2)[0])
		if actionBase != "stream" && actionBase != "original" {
			proxy.ServeHTTP(w, r)
			return
		}

		mediaSourceID := r.URL.Query().Get("mediasourceid")
		apiKey := r.URL.Query().Get("api_key")
		queryID := mediaSourceID
		if queryID == "" {
			queryID = itemID
		}

		itemsURL := fmt.Sprintf("%s/Items?Ids=%s&Fields=Path,MediaSources&api_key=%s",
			jellyfinURL, url.QueryEscape(queryID), url.QueryEscape(apiKey))
		resp, err := http.Get(itemsURL) //nolint:gosec
		if err != nil {
			proxy.ServeHTTP(w, r)
			return
		}
		defer resp.Body.Close()

		var result struct {
			Items []struct {
				Path         string `json:"Path"`
				MediaSources []struct {
					Path string `json:"Path"`
				} `json:"MediaSources"`
			} `json:"Items"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || len(result.Items) == 0 {
			proxy.ServeHTTP(w, r)
			return
		}

		item := result.Items[0]
		if !strings.HasSuffix(strings.ToLower(item.Path), ".strm") {
			proxy.ServeHTTP(w, r)
			return
		}

		ua := r.Header.Get("User-Agent")
		for _, ms := range item.MediaSources {
			pickCode := extractPickCode(ms.Path)
			if pickCode != "" {
				dlURL, err := client.DownloadURL(pickCode, ua)
				if err == nil {
					http.Redirect(w, r, dlURL, http.StatusFound)
					return
				}
			}
		}
		proxy.ServeHTTP(w, r)
	})

	// Catch-all → proxy to Jellyfin
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		proxyToJellyfin(w, r, jellyfinURL)
	})

	return mux
}

func proxyToJellyfin(w http.ResponseWriter, r *http.Request, jellyfinURL string) {
	target := jellyfinURL + r.URL.RequestURI()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body failed", http.StatusInternalServerError)
		return
	}

	req, err := http.NewRequest(r.Method, target, strings.NewReader(string(body)))
	if err != nil {
		http.Error(w, "build request failed", http.StatusInternalServerError)
		return
	}

	for k, vs := range r.Header {
		if !excludedHeaders[strings.ToLower(k)] {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, fmt.Sprintf("proxy error: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	for k, vs := range resp.Header {
		if !excludedHeaders[strings.ToLower(k)] {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body) //nolint:errcheck
}

func init() {
	cfg := config.Default()
	serveCmd.Flags().StringVar(&serveHost, "host", cfg.Proxy.Host, "监听主机")
	serveCmd.Flags().IntVar(&servePort, "port", cfg.Proxy.Port, "监听端口")
	rootCmd.AddCommand(serveCmd)
}
