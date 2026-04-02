"""115 OpenAPI client with rate limiting and token refresh."""

import time
import threading
from collections import deque

import httpx

API_BASE = "https://proapi.115.com"
PASSPORT_BASE = "https://passportapi.115.com"


class RateLimiter:
    """Three-tier rate limiter: QPS / QPM / QPH."""

    def __init__(self, qps: int = 3, qpm: int = 120, qph: int = 3600):
        self._qps = qps
        self._qpm = qpm
        self._qph = qph
        self._timestamps: deque[float] = deque()
        self._lock = threading.Lock()
        self.request_count = 0

    def acquire(self):
        sleep_time = 0.0
        with self._lock:
            now = time.monotonic()
            self._timestamps.append(now)
            self.request_count += 1

            # Clean old timestamps
            cutoff_hour = now - 3600
            while self._timestamps and self._timestamps[0] < cutoff_hour:
                self._timestamps.popleft()

            # Check QPS (most restrictive)
            cutoff_sec = now - 1
            recent_sec = sum(1 for t in self._timestamps if t >= cutoff_sec)
            if recent_sec > self._qps:
                sleep_time = 1.0 / self._qps

            # Check QPM
            cutoff_min = now - 60
            recent_min = sum(1 for t in self._timestamps if t >= cutoff_min)
            if recent_min > self._qpm:
                sleep_time = max(sleep_time, 1.0)

            # Check QPH
            if len(self._timestamps) > self._qph:
                sleep_time = max(sleep_time, self._timestamps[0] - cutoff_hour)

        # Sleep outside the lock
        if sleep_time > 0:
            time.sleep(sleep_time)


class Cloud115Client:
    def __init__(
        self,
        app_id: str,
        app_secret: str,
        access_token: str = "",
        refresh_token: str = "",
    ):
        self._app_id = app_id
        self._app_secret = app_secret
        self._access_token = access_token
        self._refresh_token = refresh_token
        self._limiter = RateLimiter()
        self._http = httpx.Client(timeout=15)
        self._token_expires_at: float = 0

    def close(self):
        self._http.close()

    def __enter__(self):
        return self

    def __exit__(self, *args):
        self.close()

    def _ensure_token(self):
        if (
            self._refresh_token
            and self._token_expires_at
            and time.time() >= self._token_expires_at
        ):
            self.refresh_access_token()

    def _request(
        self,
        method: str,
        path: str,
        params: dict | None = None,
        data: dict | None = None,
        json: dict | None = None,
    ) -> dict:
        self._ensure_token()
        self._limiter.acquire()
        headers = {"Authorization": f"Bearer {self._access_token}"}

        resp = self._http.request(
            method,
            f"{API_BASE}{path}",
            params=params,
            data=data,
            json=json,
            headers=headers,
        )
        resp.raise_for_status()
        return resp.json()

    def refresh_access_token(self):
        resp = self._http.post(
            f"{PASSPORT_BASE}/open/refreshToken",
            data={
                "app_id": self._app_id,
                "app_secret": self._app_secret,
                "refresh_token": self._refresh_token,
            },
        )
        resp.raise_for_status()
        result = resp.json()
        data = result.get("data")
        if not data:
            raise ValueError(f"Token refresh failed: {result}")
        self._access_token = data["access_token"]
        self._refresh_token = data["refresh_token"]
        self._token_expires_at = time.time() + data.get("expires_in", 7200)

    def list_files(
        self, dir_id: str = "0", limit: int = 100, offset: int = 0
    ) -> list[dict]:
        result = self._request(
            "GET",
            "/open/ufile/files",
            params={"cid": dir_id, "limit": limit, "offset": offset},
        )
        return result.get("data", [])

    def search(self, keyword: str, dir_id: str = "0") -> list[dict]:
        result = self._request(
            "GET",
            "/open/ufile/search",
            params={"search_value": keyword, "cid": dir_id},
        )
        return result.get("data", [])

    def download_url(self, pick_code: str) -> str:
        result = self._request(
            "POST", "/open/ufile/downurl", data={"pick_code": pick_code}
        )
        data = result.get("data", {})
        for val in data.values():
            url_info = val.get("url", {})
            if isinstance(url_info, dict) and "url" in url_info:
                return url_info["url"]
            if isinstance(url_info, str):
                return url_info
        raise ValueError(f"No download URL found for pick_code={pick_code}")

    def mkdir(self, parent_id: str, name: str) -> dict:
        result = self._request(
            "POST", "/open/folder/create", data={"pid": parent_id, "cname": name}
        )
        return result.get("data", {})

    def rapid_upload(
        self, dir_id: str, filename: str, file_size: int, sha1: str, pre_sha1: str
    ) -> dict:
        return self._request(
            "POST",
            "/open/upload/init",
            data={
                "pid": dir_id,
                "filename": filename,
                "filesize": str(file_size),
                "sha1": sha1,
                "pre_sha1": pre_sha1,
            },
        )

    def move(self, file_ids: list[str], target_dir_id: str) -> dict:
        return self._request(
            "POST",
            "/open/ufile/move",
            data={"fid": ",".join(file_ids), "pid": target_dir_id},
        )

    def rename(self, file_id: str, new_name: str) -> dict:
        return self._request(
            "POST", "/open/ufile/update", data={"fid": file_id, "file_name": new_name}
        )

    def delete(self, file_ids: list[str]) -> dict:
        return self._request(
            "POST", "/open/ufile/delete", data={"fid": ",".join(file_ids)}
        )
