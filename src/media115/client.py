"""115 cloud client supporting both cookie mode and OpenAPI mode.

Cookie mode: works immediately, no approval needed. Based on py115/p115client.
OpenAPI mode: requires approved app_id/app_secret from open.115.com.
"""

import json
import time
import threading
from collections import deque
from pathlib import Path

import httpx

from media115._crypto import generate_m115_key, m115_encode, m115_decode

# API endpoints
WEB_API = "https://webapi.115.com"
PRO_API = "https://proapi.115.com"
QR_API = "https://qrcodeapi.115.com"
PASSPORT_API = "https://passportapi.115.com"
OPEN_API = "https://proapi.115.com"


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

            cutoff_hour = now - 3600
            while self._timestamps and self._timestamps[0] < cutoff_hour:
                self._timestamps.popleft()

            cutoff_sec = now - 1
            recent_sec = sum(1 for t in self._timestamps if t >= cutoff_sec)
            if recent_sec > self._qps:
                sleep_time = 1.0 / self._qps

            cutoff_min = now - 60
            recent_min = sum(1 for t in self._timestamps if t >= cutoff_min)
            if recent_min > self._qpm:
                sleep_time = max(sleep_time, 1.0)

            if len(self._timestamps) > self._qph:
                sleep_time = max(sleep_time, self._timestamps[0] - cutoff_hour)

        if sleep_time > 0:
            time.sleep(sleep_time)


class Cloud115Client:
    """Unified 115 client. Use `from_cookies` or `from_openapi` to create."""

    _USER_AGENT = (
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
        "AppleWebKit/537.36 (KHTML, like Gecko) "
        "Chrome/130.0.0.0 Safari/537.36"
    )

    def __init__(self):
        self._http = httpx.Client(
            timeout=15,
            headers={"User-Agent": self._USER_AGENT},
        )
        self._limiter = RateLimiter()
        self._mode: str = ""  # "cookie" or "openapi"
        # Cookie mode
        self._cookies: str = ""
        self._user_id: str = ""
        # OpenAPI mode
        self._app_id: str = ""
        self._app_secret: str = ""
        self._access_token: str = ""
        self._refresh_token: str = ""
        self._token_expires_at: float = 0

    def close(self):
        self._http.close()

    def __enter__(self):
        return self

    def __exit__(self, *args):
        self.close()

    # ── Factory methods ──────────────────────────────────────────────

    @classmethod
    def from_cookies(cls, cookies: str) -> "Cloud115Client":
        """Create client from cookie string (UID=...; CID=...; SEID=...)."""
        client = cls()
        client._mode = "cookie"
        client._cookies = cookies
        # Extract user_id from UID cookie
        for part in cookies.split(";"):
            part = part.strip()
            if part.startswith("UID="):
                client._user_id = part[4:].split("_")[0]
                break
        return client

    @classmethod
    def from_cookie_file(cls, path: Path) -> "Cloud115Client":
        """Load cookies from a text file."""
        return cls.from_cookies(path.read_text().strip())

    @classmethod
    def from_openapi(
        cls,
        app_id: str,
        app_secret: str,
        access_token: str = "",
        refresh_token: str = "",
    ) -> "Cloud115Client":
        """Create client using OpenAPI credentials."""
        client = cls()
        client._mode = "openapi"
        client._app_id = app_id
        client._app_secret = app_secret
        client._access_token = access_token
        client._refresh_token = refresh_token
        return client

    # ── QR code login ────────────────────────────────────────────────

    @classmethod
    def qr_login(cls, app: str = "tv") -> "Cloud115Client":
        """Interactive QR code login. Returns a client with fresh cookies.

        app: device type. Use 'tv' or 'qandroid' to avoid IP ban issues.
              'web' may trigger "IP login exception" if used too frequently.
        """
        http = httpx.Client(timeout=35)

        # Step 1: Get QR token
        resp = http.get(f"{QR_API}/api/1.0/{app}/1.0/token/")
        token_data = resp.json()["data"]
        uid = token_data["uid"]
        qr_time = token_data["time"]
        sign = token_data["sign"]

        # Step 2: Show QR code
        qr_content = f"https://115.com/scan/dg-{uid}"
        qr_image_url = f"{QR_API}/api/1.0/web/1.0/qrcode?qrfrom=1&client=0d&uid={uid}"
        _print_qr(qr_content, qr_image_url)
        print("Waiting for scan...")

        # Step 3: Poll for scan status
        while True:
            try:
                resp = http.get(
                    f"{QR_API}/get/status/",
                    params={
                        "uid": uid,
                        "time": qr_time,
                        "sign": sign,
                        "_": int(time.time()),
                    },
                )
                if not resp.content:
                    # Long-poll timeout, empty response — retry
                    time.sleep(1)
                    continue
                result = resp.json()
            except (httpx.ReadTimeout, ValueError):
                # Timeout or invalid JSON — retry
                time.sleep(1)
                continue

            status = result.get("data", {}).get("status", 0)

            if status == 0:
                time.sleep(2)
                continue
            elif status == 1:
                print("QR scanned, waiting for confirmation...")
                time.sleep(1)
                continue
            elif status == 2:
                print("Login confirmed!")
                break
            elif status == -1:
                http.close()
                raise TimeoutError("QR code expired")
            elif status == -2:
                http.close()
                raise RuntimeError("Login cancelled")
            time.sleep(1)

        # Step 4: Get cookies
        resp = http.post(
            f"{PASSPORT_API}/app/1.0/{app}/1.0/login/qrcode",
            data={"account": uid, "app": app},
        )
        login_data = resp.json().get("data", {})
        cookies = login_data.get("cookie", {})

        # Build cookie string
        if isinstance(cookies, dict):
            cookie_str = "; ".join(f"{k}={v}" for k, v in cookies.items())
        else:
            cookie_str = str(cookies)

        http.close()
        print(f"Login success! Cookie length: {len(cookie_str)}")
        return cls.from_cookies(cookie_str)

    def check_login(self) -> bool:
        """Check if current credentials are valid. Returns True if logged in."""
        try:
            if self._mode == "cookie":
                resp = self._http.get(
                    "https://my.115.com/?ct=guide&ac=status",
                    headers={"Cookie": self._cookies},
                    timeout=10,
                )
                if resp.status_code == 200:
                    data = resp.json()
                    return data.get("state", False)
                return False
            else:
                # OpenAPI: try listing root
                self.list_files(dir_id="0", limit=1)
                return True
        except Exception:
            return False

    def renew_cookies(self, app: str = "tv") -> bool:
        """Auto-renew cookies without user interaction.

        Uses the current cookie to programmatically scan a new QR code,
        confirm it, and get fresh cookies. No manual scan needed.
        Returns True if renewal succeeded, False otherwise.
        """
        if self._mode != "cookie" or not self._cookies:
            return False

        try:
            http_headers = {"Cookie": self._cookies}

            # Step 1: Get a new QR token
            resp = self._http.get(
                f"{QR_API}/api/1.0/{app}/1.0/token/",
            )
            resp.raise_for_status()
            token_data = resp.json().get("data", {})
            uid = token_data.get("uid")
            if not uid:
                return False

            # Step 2: Auto-scan the QR code (using current cookie)
            resp = self._http.get(
                f"{QR_API}/api/2.0/prompt.php",
                params={"uid": uid},
                headers=http_headers,
            )
            resp.raise_for_status()

            # Step 3: Auto-confirm
            resp = self._http.get(
                f"{QR_API}/api/2.0/slogin.php",
                params={"key": uid, "uid": uid, "client": 0},
                headers=http_headers,
            )
            resp.raise_for_status()

            # Step 4: Get new cookies
            resp = self._http.post(
                f"{PASSPORT_API}/app/1.0/{app}/1.0/login/qrcode",
                data={"account": uid, "app": app},
            )
            resp.raise_for_status()
            login_data = resp.json().get("data", {})
            cookies = login_data.get("cookie", {})

            if isinstance(cookies, dict):
                cookie_str = "; ".join(f"{k}={v}" for k, v in cookies.items())
            else:
                cookie_str = str(cookies)

            if not cookie_str or len(cookie_str) < 10:
                return False

            self._cookies = cookie_str
            # Re-extract user_id
            for part in cookie_str.split(";"):
                part = part.strip()
                if part.startswith("UID="):
                    self._user_id = part[4:].split("_")[0]
                    break

            return True
        except Exception:
            return False

    def save_cookies_to_env(self, env_path: Path):
        """Save cookies to .env file as CLOUD_115_COOKIES=..."""
        if self._mode != "cookie":
            raise ValueError("Not in cookie mode")

        lines = []
        replaced = False
        if env_path.exists():
            for line in env_path.read_text().splitlines():
                if line.startswith("CLOUD_115_COOKIES="):
                    lines.append(f"CLOUD_115_COOKIES={self._cookies}")
                    replaced = True
                else:
                    lines.append(line)
        if not replaced:
            lines.append(f"CLOUD_115_COOKIES={self._cookies}")
        env_path.write_text("\n".join(lines) + "\n")

    # ── Internal request methods ─────────────────────────────────────

    def _cookie_request(
        self,
        method: str,
        url: str,
        params: dict | None = None,
        data: dict | None = None,
    ) -> dict:
        self._limiter.acquire()
        resp = self._http.request(
            method,
            url,
            params=params,
            data=data,
            headers={"Cookie": self._cookies},
        )
        if resp.status_code == 405:
            # Cookie expired — try auto-renewal
            if self.renew_cookies():
                resp = self._http.request(
                    method,
                    url,
                    params=params,
                    data=data,
                    headers={"Cookie": self._cookies},
                )
        resp.raise_for_status()
        return resp.json()

    def _openapi_request(
        self,
        method: str,
        path: str,
        params: dict | None = None,
        data: dict | None = None,
    ) -> dict:
        # Auto-refresh token
        if (
            self._refresh_token
            and self._token_expires_at
            and time.time() >= self._token_expires_at
        ):
            self.refresh_access_token()

        self._limiter.acquire()
        resp = self._http.request(
            method,
            f"{OPEN_API}{path}",
            params=params,
            data=data,
            headers={"Authorization": f"Bearer {self._access_token}"},
        )
        resp.raise_for_status()
        return resp.json()

    def _request(self, method: str, path: str, **kwargs) -> dict:
        if self._mode == "cookie":
            url = f"{WEB_API}{path}" if not path.startswith("http") else path
            return self._cookie_request(method, url, **kwargs)
        else:
            return self._openapi_request(method, path, **kwargs)

    # ── OpenAPI token refresh ────────────────────────────────────────

    def refresh_access_token(self):
        resp = self._http.post(
            f"{PASSPORT_API}/open/refreshToken",
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

    # ── Public API ───────────────────────────────────────────────────

    def list_files(
        self, dir_id: str = "0", limit: int = 100, offset: int = 0
    ) -> list[dict]:
        if self._mode == "cookie":
            result = self._cookie_request(
                "GET",
                f"{WEB_API}/files",
                params={
                    "cid": dir_id,
                    "limit": limit,
                    "offset": offset,
                    "show_dir": 1,
                    "format": "json",
                },
            )
            return result.get("data", [])
        else:
            result = self._openapi_request(
                "GET",
                "/open/ufile/files",
                params={"cid": dir_id, "limit": limit, "offset": offset},
            )
            return result.get("data", [])

    def list_files_all(self, dir_id: str = "0") -> list[dict]:
        """List all files in a directory (handles pagination)."""
        all_files = []
        offset = 0
        while True:
            batch = self.list_files(dir_id=dir_id, limit=1000, offset=offset)
            if not batch:
                break
            all_files.extend(batch)
            if len(batch) < 1000:
                break
            offset += len(batch)
        return all_files

    def list_files_recursive(self, dir_id: str = "0") -> list[dict]:
        """Recursively list all files and directories."""
        result = []
        items = self.list_files_all(dir_id=dir_id)
        for item in items:
            is_dir = "cid" in item and not item.get("sha")
            item["_is_dir"] = is_dir
            item["_parent_id"] = dir_id
            result.append(item)
            if is_dir:
                child_id = item.get("cid", "")
                if child_id and child_id != dir_id:
                    time.sleep(0.5)
                    result.extend(self.list_files_recursive(dir_id=str(child_id)))
        return result

    def resolve_path(self, path: str) -> str:
        """Resolve a path like '/影音/电影/' to a dir_id.

        Walks the directory tree from root, matching each path component.
        Returns the dir_id of the final directory.
        """
        parts = [p for p in path.strip("/").split("/") if p]
        if not parts:
            return "0"

        current_id = "0"
        for part in parts:
            items = self.list_files_all(dir_id=current_id)
            found = False
            for item in items:
                name = item.get("fn", item.get("n", ""))
                is_dir = "cid" in item or (not item.get("sha") and not item.get("pc"))
                if is_dir and name == part:
                    current_id = str(item.get("cid", item.get("fid", "")))
                    found = True
                    break
            if not found:
                raise FileNotFoundError(
                    f"Directory not found: '{part}' in path '{path}'"
                )
        return current_id

    def search(self, keyword: str, dir_id: str = "0") -> list[dict]:
        if self._mode == "cookie":
            result = self._cookie_request(
                "GET",
                f"{WEB_API}/files/search",
                params={"search_value": keyword, "cid": dir_id, "format": "json"},
            )
            return result.get("data", [])
        else:
            result = self._openapi_request(
                "GET",
                "/open/ufile/search",
                params={"search_value": keyword, "cid": dir_id},
            )
            return result.get("data", [])

    def download_url(self, pick_code: str) -> str:
        if self._mode == "cookie":
            return self._download_url_cookie(pick_code)
        else:
            return self._download_url_openapi(pick_code)

    def _download_url_cookie(self, pick_code: str) -> str:
        """Get download URL using M115 encryption (cookie mode)."""
        key = generate_m115_key()
        payload = json.dumps({"pickcode": pick_code})
        encrypted = m115_encode(key, payload)

        self._limiter.acquire()
        resp = self._http.post(
            f"{PRO_API}/app/chrome/downurl",
            params={"t": int(time.time())},
            data={"data": encrypted},
            headers={"Cookie": self._cookies},
        )
        resp.raise_for_status()
        result = resp.json()

        decrypted = m115_decode(key, result["data"])
        data = json.loads(decrypted)

        for val in data.values():
            url_info = val.get("url", {})
            if isinstance(url_info, dict) and "url" in url_info:
                return url_info["url"]
            if isinstance(url_info, str) and url_info:
                return url_info
        raise ValueError(f"No download URL for pick_code={pick_code}")

    def _download_url_openapi(self, pick_code: str) -> str:
        """Get download URL using OpenAPI (no encryption needed)."""
        result = self._openapi_request(
            "POST", "/open/ufile/downurl", data={"pick_code": pick_code}
        )
        data = result.get("data", {})
        for val in data.values():
            url_info = val.get("url", {})
            if isinstance(url_info, dict) and "url" in url_info:
                return url_info["url"]
            if isinstance(url_info, str) and url_info:
                return url_info
        raise ValueError(f"No download URL for pick_code={pick_code}")

    def mkdir(self, parent_id: str, name: str) -> dict:
        if self._mode == "cookie":
            return self._cookie_request(
                "POST",
                f"{WEB_API}/files/add",
                data={"pid": parent_id, "cname": name},
            )
        else:
            return self._openapi_request(
                "POST",
                "/open/folder/create",
                data={"pid": parent_id, "cname": name},
            )

    def rapid_upload(
        self, dir_id: str, filename: str, file_size: int, sha1: str, pre_sha1: str
    ) -> dict:
        if self._mode == "cookie":
            # Cookie mode rapid upload requires EC115 encryption (ECDH+AES+LZ4),
            # which is not yet implemented. Fall back to error.
            raise NotImplementedError(
                "Rapid upload via cookie mode requires EC115 encryption (not yet implemented). "
                "Use OpenAPI mode or upload manually."
            )
        return self._openapi_request(
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

    def copy(self, file_ids: list[str], target_dir_id: str) -> dict:
        if self._mode == "cookie":
            data = {"pid": target_dir_id}
            for i, fid in enumerate(file_ids):
                data[f"fid[{i}]"] = fid
            return self._cookie_request("POST", f"{WEB_API}/files/copy", data=data)
        else:
            return self._openapi_request(
                "POST",
                "/open/ufile/copy",
                data={"fid": ",".join(file_ids), "pid": target_dir_id},
            )

    def move(self, file_ids: list[str], target_dir_id: str) -> dict:
        if self._mode == "cookie":
            data = {"pid": target_dir_id}
            for i, fid in enumerate(file_ids):
                data[f"fid[{i}]"] = fid
            return self._cookie_request("POST", f"{WEB_API}/files/move", data=data)
        else:
            return self._openapi_request(
                "POST",
                "/open/ufile/move",
                data={"fid": ",".join(file_ids), "pid": target_dir_id},
            )

    def rename(self, file_id: str, new_name: str) -> dict:
        if self._mode == "cookie":
            return self._cookie_request(
                "POST",
                f"{WEB_API}/files/edit",
                data={"fid": file_id, "file_name": new_name},
            )
        else:
            return self._openapi_request(
                "POST",
                "/open/ufile/update",
                data={"fid": file_id, "file_name": new_name},
            )

    def delete(self, file_ids: list[str]) -> dict:
        if self._mode == "cookie":
            data = {}
            for i, fid in enumerate(file_ids):
                data[f"fid[{i}]"] = fid
            return self._cookie_request("POST", f"{WEB_API}/rb/delete", data=data)
        else:
            return self._openapi_request(
                "POST",
                "/open/ufile/delete",
                data={"fid": ",".join(file_ids)},
            )


def _print_qr(content: str, image_url: str):
    """Print QR login URL for the user to open in browser."""
    print(f"Scan QR: {image_url}")
