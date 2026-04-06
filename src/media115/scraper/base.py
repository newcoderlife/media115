"""Base scraper with shared HTTP client and rate limiting."""

from __future__ import annotations

import httpx

from media115.cache import _cache_dir
from media115.rate_limit import RateLimiter

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
            qps=qps, qpm=qpm,
            state_path=_cache_dir() / f"rate_limit_{name}.json",
        )
        self._http = httpx.Client(timeout=timeout, headers={"User-Agent": DEFAULT_UA})

    def _throttle(self):
        self._limiter.acquire()

    def get(self, url: str, **kwargs) -> httpx.Response:
        self._throttle()
        return self._http.get(url, **kwargs)

    def post(self, url: str, **kwargs) -> httpx.Response:
        self._throttle()
        return self._http.post(url, **kwargs)

    def close(self):
        self._http.close()
