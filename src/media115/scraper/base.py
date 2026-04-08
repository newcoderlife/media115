"""Base scraper with shared HTTP client and rate limiting."""

from __future__ import annotations

import httpx

from cloud115.rate_limit import RateLimiter

DEFAULT_UA = (
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
    "AppleWebKit/537.36 (KHTML, like Gecko) "
    "Chrome/130.0.0.0 Safari/537.36"
)
DEFAULT_TIMEOUT = 15


class ThrottledClient:
    """HTTP client with per-instance rate limiting."""

    def __init__(self, name: str = "scraper", qps: float = 0.8, qpm: int = 40, timeout: int = DEFAULT_TIMEOUT):
        self._limiter = RateLimiter(
            name, qps=qps, qpm=qpm
        )
        self._http = httpx.Client(timeout=timeout, headers={"User-Agent": DEFAULT_UA})

    def _throttle(self):
        self._limiter.acquire()

    def get(self, url: str, **kwargs) -> httpx.Response:
        from media115.log import get_logger
        get_logger().debug("HTTP GET %s", url[:80])
        self._throttle()
        return self._http.get(url, **kwargs)

    def post(self, url: str, **kwargs) -> httpx.Response:
        from media115.log import get_logger
        get_logger().debug("HTTP POST %s", url[:80])
        self._throttle()
        return self._http.post(url, **kwargs)

    def close(self):
        self._http.close()
