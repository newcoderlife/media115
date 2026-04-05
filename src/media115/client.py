"""115 cloud client supporting both cookie mode and OpenAPI mode.

Cookie mode: works immediately, no approval needed. Based on py115/p115client.
OpenAPI mode: requires approved app_id/app_secret from open.115.com.
"""

import contextlib
import json
import sys
import threading
import time
from pathlib import Path

import httpx

from media115._crypto import generate_m115_key, m115_decode, m115_encode
from media115.cache import rate_limit_path as _rate_limit_path

# API endpoints
WEB_API = "https://webapi.115.com"
PRO_API = "https://proapi.115.com"
QR_API = "https://qrcodeapi.115.com"
PASSPORT_API = "https://passportapi.115.com"
OPEN_API = "https://proapi.115.com"


def _read_state(state_path: Path) -> dict:
    """Read rate limit state from a dedicated state file (not .env)."""
    if not state_path.exists():
        return {}
    try:
        return json.loads(state_path.read_text())
    except (json.JSONDecodeError, ValueError):
        return {}


def _write_state(state_path: Path, state: dict):
    """Write rate limit state atomically."""
    state_path.write_text(json.dumps(state))


class RateLimiter:
    """Rate limiter with QPS + QPM control and cross-process persistence.

    State persisted to .115_rate_limit (JSON):
    - cooldown_until: timestamp, 429 triggers 1-hour global cooldown
    - last_request: timestamp of last API request
    - minute_start: start of current minute window
    - minute_count: requests in current minute window
    """

    def __init__(self, qps: float = 0.5, qpm: int = 20, use_state: bool = True):
        self._qps = qps
        self._qpm = qpm
        self._state_path = _rate_limit_path() if use_state else None
        self._lock = threading.Lock()
        self.request_count = 0

    def acquire(self):
        with self._lock:
            self._wait_for_slot()
            self.request_count += 1

    def _wait_for_slot(self):
        if not self._state_path:
            return

        while True:
            now = time.time()
            state = _read_state(self._state_path)

            # Check cooldown (429 ban)
            cooldown_until = state.get("cooldown_until", 0)
            if now < cooldown_until:
                remaining = int(cooldown_until - now)
                mins, secs = divmod(remaining, 60)
                until_str = time.strftime("%H:%M", time.localtime(cooldown_until))
                raise RuntimeError(
                    f"Rate limit cooldown: {mins}m{secs:02d}s remaining "
                    f"(until {until_str})"
                )

            # Check QPS
            last_req = state.get("last_request", 0)
            min_interval = 1.0 / self._qps
            wait = min_interval - (now - last_req)
            if wait > 0:
                time.sleep(wait)
                now = time.time()

            # Check QPM
            minute_start = state.get("minute_start", 0)
            minute_count = state.get("minute_count", 0)

            if now - minute_start > 60:
                minute_start = now
                minute_count = 0

            if minute_count >= self._qpm:
                wait = 60 - (now - minute_start)
                if wait > 0:
                    time.sleep(wait)
                    continue

            # All checks passed — record this request
            state["last_request"] = now
            state["minute_start"] = minute_start
            state["minute_count"] = minute_count + 1
            _write_state(self._state_path, state)
            break

    def set_cooldown(self, seconds: float):
        """Set a global cooldown, persisted for cross-process enforcement."""
        if self._state_path:
            until = time.time() + seconds
            state = _read_state(self._state_path)
            state["cooldown_until"] = until
            _write_state(self._state_path, state)
            mins, secs = divmod(int(seconds), 60)
            hours, mins = divmod(mins, 60)
            until_str = time.strftime("%H:%M", time.localtime(until))
            dur = f"{hours}h" if hours else f"{mins}m{secs:02d}s"
            print(
                f"  429 received: entering {dur} cooldown (until {until_str})",
                file=sys.stderr, flush=True,
            )


class Cloud115Client:
    """Unified 115 client. Use `from_cookies` or `from_openapi` to create."""

    _USER_AGENT = (
        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
        "AppleWebKit/537.36 (KHTML, like Gecko) "
        "Chrome/130.0.0.0 Safari/537.36"
    )

    def __init__(self):
        self._http = httpx.Client(
            timeout=30,
            headers={
                "User-Agent": self._USER_AGENT,
                "Origin": "https://115.com",
                "Referer": "https://115.com/",
            },
        )
        self._env_path = Path.cwd() / ".env"
        self._limiter = RateLimiter(qps=0.5, qpm=20)
        self._download_limiter = RateLimiter(qps=0.5, qpm=20)
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
        max_retries = 3
        for attempt in range(max_retries):
            try:
                resp = self._http.request(
                    method,
                    url,
                    params=params,
                    data=data,
                    headers={"Cookie": self._cookies},
                )
                break
            except (httpx.ReadTimeout, httpx.RemoteProtocolError, httpx.ConnectError):
                if attempt == max_retries - 1:
                    raise
                time.sleep(3 * (attempt + 1))
                continue
        if resp.status_code == 429:
            self._limiter.set_cooldown(3600)
            self._download_limiter.set_cooldown(3600)
            raise RuntimeError("115 API rate limit hit (429). Cooling down for 1 hour.")
        if resp.status_code == 405 and self.renew_cookies():
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
                    "aid": 1,
                    "cid": dir_id,
                    "limit": limit,
                    "offset": offset,
                    "show_dir": 1,
                    "o": "user_ptime",
                    "asc": 1,
                    "natsort": 1,
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

    def list_files_recursive(
        self, dir_id: str = "0", max_depth: int = 5, _depth: int = 0
    ) -> list[dict]:
        """Recursively list all files and directories."""
        if _depth > max_depth:
            return []
        result = []
        items = self.list_files_all(dir_id=dir_id)
        for item in items:
            is_dir = "fid" not in item
            item["_is_dir"] = is_dir
            item["_parent_id"] = dir_id
            result.append(item)
            if is_dir:
                child_id = item.get("cid", item.get("fid", ""))
                if child_id:
                    result.extend(
                        self.list_files_recursive(
                            dir_id=str(child_id),
                            max_depth=max_depth,
                            _depth=_depth + 1,
                        )
                    )
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
                is_dir = "fid" not in item
                if is_dir and name == part:
                    current_id = str(item.get("cid", item.get("fid", "")))
                    found = True
                    break
            if not found:
                raise FileNotFoundError(
                    f"Directory not found: '{part}' in path '{path}'"
                )
        return current_id

    def get_dir_id(self, path: str) -> str | None:
        """Get directory ID by absolute path. One API call, very reliable."""
        if self._mode == "cookie":
            result = self._cookie_request(
                "GET",
                f"{WEB_API}/files/getid",
                params={"path": path},
            )
            if result.get("state"):
                return str(result.get("id", ""))
            return None
        else:
            return self.resolve_path(path)

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

    def upload_file(
        self, local_path: Path, target_dir_id: str, filename: str = ""
    ) -> dict | None:
        """Upload a small file (NFO, image) to 115 via OSS.

        No encryption needed. Works for files up to ~500MB.
        """
        if self._mode != "cookie":
            raise NotImplementedError("upload_file requires cookie mode")

        import httpx as _httpx

        fname = filename or local_path.name
        content = local_path.read_bytes()

        self._limiter.acquire()

        # Step 1: Init upload
        resp = self._http.post(
            "https://uplb.115.com/3.0/sampleinitupload.php",
            data={"filename": fname, "target": f"U_1_{target_dir_id}"},
            headers={"Cookie": self._cookies},
        )
        resp.raise_for_status()
        init = resp.json()

        # Step 2: Upload to OSS
        self._limiter.acquire()
        oss_resp = _httpx.post(
            init["host"],
            data={
                "key": init["object"],
                "OSSAccessKeyId": init["accessid"],
                "policy": init["policy"],
                "signature": init["signature"],
                "callback": init["callback"],
            },
            files={"file": (fname, content)},
            timeout=30,
        )
        if oss_resp.status_code == 200:
            return oss_resp.json().get("data")
        return None

    def batch_rename(self, renames: dict[str, str]) -> dict:
        """Rename multiple files in one API call.

        renames: {file_id: new_name, ...}
        """
        if self._mode == "cookie":
            data = {f"files_new_name[{fid}]": name for fid, name in renames.items()}
            return self._cookie_request(
                "POST", f"{WEB_API}/files/batch_rename", data=data
            )
        else:
            # OpenAPI doesn't have batch_rename, fall back to individual
            for fid, name in renames.items():
                self.rename(fid, name)
            return {"state": True}

    def export_tree(self, dir_id: str) -> str | None:
        """Export 115 directory tree. Returns tree text (UTF-8) or None on failure.

        Only uses 2-3 API calls regardless of directory size.
        """
        if self._mode != "cookie":
            raise NotImplementedError("export_tree requires cookie mode")

        import httpx as _httpx

        # Start export
        resp = self._cookie_request(
            "POST",
            f"{WEB_API}/files/export_dir",
            data={"file_ids": dir_id, "target": "U_1_0"},
        )
        export_id = resp.get("data", {}).get("export_id")
        if not export_id:
            # Previous export might still be running — try polling with id=0
            export_id = 0

        # Poll status
        pick_code = None
        file_id = None
        for _ in range(60):
            time.sleep(3)
            status = self._cookie_request(
                "GET",
                f"{WEB_API}/files/export_dir",
                params={"export_id": export_id},
            )
            data = status.get("data", {})
            if isinstance(data, list):
                data = data[0] if data else {}
            pc = data.get("pick_code") if isinstance(data, dict) else None
            if pc:
                pick_code = pc
                file_id = data.get("file_id")
                break

        if not pick_code:
            return None

        # Download the tree file
        cookie_dict = {}
        for part in self._cookies.split(";"):
            p = part.strip()
            if "=" in p:
                k, v = p.split("=", 1)
                cookie_dict[k.strip()] = v.strip()

        dl = _httpx.get(
            "https://115.com/",
            params={"ct": "download", "ac": "video", "pickcode": pick_code},
            cookies=cookie_dict,
            headers={"User-Agent": self._USER_AGENT},
            follow_redirects=True,
            timeout=30,
        )
        if dl.status_code != 200 or len(dl.content) < 100:
            return None

        # Clean up: delete the tree txt from 115
        if file_id:
            with contextlib.suppress(Exception):
                self.delete([str(file_id)])

        return dl.content.decode("utf-16-le", errors="replace")


def _print_qr(content: str, image_url: str):
    """Print QR login URL for the user to open in browser."""
    print(f"Scan QR: {image_url}")
