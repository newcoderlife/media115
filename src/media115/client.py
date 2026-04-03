"""115 cloud client supporting both cookie mode and OpenAPI mode.

Cookie mode: works immediately, no approval needed. Based on py115/p115client.
OpenAPI mode: requires approved app_id/app_secret from open.115.com.
"""

import json
import time
import threading
from pathlib import Path

import httpx

from media115._crypto import generate_m115_key, m115_encode, m115_decode

# API endpoints
WEB_API = "https://webapi.115.com"
PRO_API = "https://proapi.115.com"
QR_API = "https://qrcodeapi.115.com"
PASSPORT_API = "https://passportapi.115.com"
OPEN_API = "https://proapi.115.com"

# Rate limit state file — lives next to .env in project root
_RATE_LIMIT_STATE_KEY = "CLOUD_115_COOLDOWN_UNTIL"


def _read_cooldown(env_path: Path) -> float:
    """Read cooldown timestamp from .env."""
    if not env_path.exists():
        return 0
    for line in env_path.read_text().splitlines():
        if line.startswith(f"{_RATE_LIMIT_STATE_KEY}="):
            try:
                return float(line.split("=", 1)[1].strip())
            except ValueError:
                return 0
    return 0


def _write_cooldown(env_path: Path, until: float):
    """Write cooldown timestamp to .env."""
    lines = []
    replaced = False
    if env_path.exists():
        for line in env_path.read_text().splitlines():
            if line.startswith(f"{_RATE_LIMIT_STATE_KEY}="):
                lines.append(f"{_RATE_LIMIT_STATE_KEY}={until}")
                replaced = True
            else:
                lines.append(line)
    if not replaced:
        lines.append(f"{_RATE_LIMIT_STATE_KEY}={until}")
    env_path.write_text("\n".join(lines) + "\n")


class RateLimiter:
    """Rate limiter with QPS control and persistent cooldown across processes."""

    def __init__(self, qps: int = 3, env_path: Path | None = None):
        self._qps = qps
        self._env_path = env_path
        self._lock = threading.Lock()
        self._last_request: float = 0
        self.request_count = 0

    def acquire(self):
        with self._lock:
            now = time.time()
            # Check persistent cooldown
            if self._env_path:
                cooldown_until = _read_cooldown(self._env_path)
                if now < cooldown_until:
                    wait = cooldown_until - now
                    raise RuntimeError(
                        f"115 API is in cooldown for {int(wait)}s more "
                        f"(until {time.strftime('%H:%M:%S', time.localtime(cooldown_until))}). "
                        f"Try again later."
                    )
            # QPS throttle (in-process only, good enough for single CLI)
            elapsed = now - self._last_request
            min_interval = 1.0 / self._qps
            if elapsed < min_interval:
                time.sleep(min_interval - elapsed)
            self._last_request = time.time()
            self.request_count += 1

    def set_cooldown(self, seconds: float):
        """Set a global cooldown, persisted to .env for cross-process enforcement."""
        until = time.time() + seconds
        if self._env_path:
            _write_cooldown(self._env_path, until)


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
        self._env_path = Path.cwd() / ".env"
        self._limiter = RateLimiter(qps=3, env_path=self._env_path)
        self._download_limiter = RateLimiter(qps=1, env_path=self._env_path)
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
        """Interactive QR code login. Returns a client with fresh cookies."""
        http = httpx.Client(timeout=35)

        resp = http.get(f"{QR_API}/api/1.0/{app}/1.0/token/")
        token_data = resp.json()["data"]
        uid = token_data["uid"]
        qr_time = token_data["time"]
        sign = token_data["sign"]

        qr_content = f"https://115.com/scan/dg-{uid}"
        qr_image_url = f"{QR_API}/api/1.0/web/1.0/qrcode?qrfrom=1&client=0d&uid={uid}"
        _print_qr(qr_content, qr_image_url)
        print("Waiting for scan...")

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
                    time.sleep(1)
                    continue
                result = resp.json()
            except (httpx.ReadTimeout, ValueError):
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

        resp = http.post(
            f"{PASSPORT_API}/app/1.0/{app}/1.0/login/qrcode",
            data={"account": uid, "app": app},
        )
        login_data = resp.json().get("data", {})
        cookies = login_data.get("cookie", {})

        if isinstance(cookies, dict):
            cookie_str = "; ".join(f"{k}={v}" for k, v in cookies.items())
        else:
            cookie_str = str(cookies)

        http.close()
        print(f"Login success! Cookie length: {len(cookie_str)}")
        return cls.from_cookies(cookie_str)

    def check_login(self) -> bool:
        """Check if current credentials are valid."""
        try:
            if self._mode == "cookie":
                resp = self._http.get(
                    "https://my.115.com/?ct=guide&ac=status",
                    headers={"Cookie": self._cookies},
                    timeout=10,
                )
                if resp.status_code == 200:
                    return resp.json().get("state", False)
                return False
            else:
                self.list_files(dir_id="0", limit=1)
                return True
        except Exception:
            return False

    def renew_cookies(self, app: str = "tv") -> bool:
        """Auto-renew cookies without user interaction."""
        if self._mode != "cookie" or not self._cookies:
            return False
        try:
            http_headers = {"Cookie": self._cookies}
            resp = self._http.get(f"{QR_API}/api/1.0/{app}/1.0/token/")
            resp.raise_for_status()
            uid = resp.json().get("data", {}).get("uid")
            if not uid:
                return False

            resp = self._http.get(
                f"{QR_API}/api/2.0/prompt.php",
                params={"uid": uid},
                headers=http_headers,
            )
            resp.raise_for_status()

            resp = self._http.get(
                f"{QR_API}/api/2.0/slogin.php",
                params={"key": uid, "uid": uid, "client": 0},
                headers=http_headers,
            )
            resp.raise_for_status()

            resp = self._http.post(
                f"{PASSPORT_API}/app/1.0/{app}/1.0/login/qrcode",
                data={"account": uid, "app": app},
            )
            resp.raise_for_status()
            cookies = resp.json().get("data", {}).get("cookie", {})

            if isinstance(cookies, dict):
                cookie_str = "; ".join(f"{k}={v}" for k, v in cookies.items())
            else:
                cookie_str = str(cookies)

            if not cookie_str or len(cookie_str) < 10:
                return False

            self._cookies = cookie_str
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
        limiter: "RateLimiter | None" = None,
    ) -> dict:
        (limiter or self._limiter).acquire()
        resp = self._http.request(
            method,
            url,
            params=params,
            data=data,
            headers={"Cookie": self._cookies},
        )
        if resp.status_code == 429:
            self._limiter.set_cooldown(3600)
            self._download_limiter.set_cooldown(3600)
            raise RuntimeError("115 API rate limit hit (429). Cooling down for 1 hour.")
        if resp.status_code == 405:
            if self.renew_cookies():
                resp = self._http.request(
                    method,
                    url,
                    params=params,
                    data=data,
                    headers={"Cookie": self._cookies},
                )
        resp.raise_for_status()
        result = resp.json()
        if isinstance(result, dict) and "errNo" in result:
            err = result["errNo"]
            if err == 770004 or "访问上限" in str(result.get("error", "")):
                self._limiter.set_cooldown(3600)
                self._download_limiter.set_cooldown(3600)
                raise RuntimeError(
                    f"115 API rate limit hit (errNo={err}). Cooling down."
                )
        return result

    def _openapi_request(
        self,
        method: str,
        path: str,
        params: dict | None = None,
        data: dict | None = None,
    ) -> dict:
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
            is_dir = "cid" in item or (not item.get("sha") and not item.get("pc"))
            item["_is_dir"] = is_dir
            item["_parent_id"] = dir_id
            result.append(item)
            if is_dir:
                child_id = item.get("cid", item.get("fid", ""))
                if child_id:
                    time.sleep(0.5)
                    result.extend(self.list_files_recursive(dir_id=str(child_id)))
        return result

    def resolve_path(self, path: str) -> str:
        """Resolve a path like '/影音/电影/' to a dir_id."""
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

        self._download_limiter.acquire()
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
            raise NotImplementedError(
                "Rapid upload via cookie mode requires EC115 encryption (not yet implemented)."
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
