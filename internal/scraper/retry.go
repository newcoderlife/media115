package scraper

import (
	"net/http"
	"time"
)

// RetryGet performs an HTTP GET with retry on network errors.
func RetryGet(client *http.Client, url string, headers map[string]string, maxRetries int) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
			continue
		}
		return resp, nil
	}
	return nil, lastErr
}
