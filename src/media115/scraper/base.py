"""Base scraper with shared HTTP client and rate limiting."""


from __future__ import annotations

import time

import httpx

DEFAULT_UA = (
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
    "AppleWebKit/537.36 (KHTML, like Gecko) "
    "Chrome/130.0.0.0 Safari/537.36"
)
DEFAULT_INTERVAL = 2.0  # seconds between requests
DEFAULT_TIMEOUT = 15


class ThrottledClient:
    """HTTP client with per-instance rate limiting."""

    def __init__(
        self, interval: float = DEFAULT_INTERVAL, timeout: int = DEFAULT_TIMEOUT
    ):
        self._interval = interval
        self._last_request: float = 0
        self._http = httpx.Client(
            timeout=timeout,
            headers={"User-Agent": DEFAULT_UA},
        )

    def _throttle(self):
        elapsed = time.monotonic() - self._last_request
        if elapsed < self._interval:
            time.sleep(self._interval - elapsed)
        self._last_request = time.monotonic()

    def get(self, url: str, **kwargs) -> httpx.Response:
        self._throttle()
        return self._http.get(url, **kwargs)

    def post(self, url: str, **kwargs) -> httpx.Response:
        self._throttle()
        return self._http.post(url, **kwargs)

    def close(self):
        self._http.close()
