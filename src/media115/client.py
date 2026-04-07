"""115 cloud client (cookie mode only).

Cookie mode: works immediately, no approval needed. Based on py115/p115client.
"""

from __future__ import annotations

import contextlib
import hashlib
import json
import time
from pathlib import Path
from urllib.parse import urlencode

import httpx

from media115._crypto import generate_m115_key, m115_decode, m115_encode
from media115.cache import _cache_dir
from media115.log import get_logger
from media115.rate_limit import RateLimiter, _read_state, _write_state

# API endpoints
WEB_API = "https://webapi.115.com"
PRO_API = "https://proapi.115.com"
QR_API = "https://qrcodeapi.115.com"
PASSPORT_API = "https://passportapi.115.com"


class Cloud115Client:
    """115 client (cookie mode). Use `from_cookies` to create."""

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
        self._limiter = RateLimiter(qps=0.5, qpm=20, state_path=_cache_dir() / "rate_limit.json")
        self._download_limiter = RateLimiter(qps=0.5, qpm=20, state_path=_cache_dir() / "download_rate_limit.json")
        # Cookie mode
        self._cookies: str = ""
        self._user_id: str = ""

    def close(self):
        self._http.close()

    def __enter__(self):
        return self

    def __exit__(self, *args):
        self.close()

    # ── Factory methods ──────────────────────────────────────────────

    @classmethod
    def from_cookies(cls, cookies: str) -> Cloud115Client:
        """Create client from cookie string (UID=...; CID=...; SEID=...)."""
        client = cls()
        client._cookies = cookies
        for part in cookies.split(";"):
            part = part.strip()
            if part.startswith("UID="):
                client._user_id = part[4:].split("_")[0]
                break
        return client

    @classmethod
    def from_cookie_file(cls, path: Path) -> Cloud115Client:
        """Load cookies from a text file."""
        return cls.from_cookies(path.read_text().strip())

    # ── QR code login ────────────────────────────────────────────────

    @classmethod
    def qr_login(cls, app: str = "tv") -> Cloud115Client:
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
            resp = self._http.get(
                "https://my.115.com/?ct=guide&ac=status",
                headers={"Cookie": self._cookies},
                timeout=10,
            )
            if resp.status_code == 200:
                return resp.json().get("state", False)
            return False
        except Exception:
            return False

    def renew_cookies(self, app: str = "tv") -> bool:
        """Auto-renew cookies without user interaction."""
        if not self._cookies:
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
        limiter: RateLimiter | None = None,
    ) -> dict:
        (limiter or self._limiter).acquire()
        logger = get_logger()
        # 构建可读的日志：方法 + 路径 + 关键参数
        api_path = url.split(".com")[-1][:40]
        key_params = ""
        if params:
            parts = []
            for k in ("cid", "path", "search_value", "pick_code", "pickcode"):
                if k in params:
                    parts.append(f"{k}={params[k]}")
            if parts:
                key_params = " " + " ".join(parts)
        if data:
            parts = []
            for k in ("pid", "cname", "fid", "file_name", "filename", "target"):
                if k in data:
                    parts.append(f"{k}={data[k]}")
            if parts:
                key_params += " " + " ".join(parts)
        logger.debug("115 %s %s%s", method, api_path, key_params)
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
                logger.warning("115 network error (attempt %d/%d), retrying...", attempt + 1, max_retries)
                time.sleep(3 * (attempt + 1))
                continue
        if resp.status_code == 429:
            self._limiter.set_cooldown(3600)
            self._download_limiter.set_cooldown(3600)
            logger.warning("115 rate limit (429), cooldown 3600s")
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
                logger.warning("115 rate limit (errNo=%d), cooldown 3600s", err)
                raise RuntimeError(f"115 API rate limit hit (errNo={err}). Cooling down.")
        return result

    # ── Public API ───────────────────────────────────────────────────

    def list_files(self, dir_id: str = "0", limit: int = 100, offset: int = 0) -> list[dict]:
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
                raise FileNotFoundError(f"Directory not found: '{part}' in path '{path}'")
        return current_id

    def get_dir_id(self, path: str) -> str | None:
        """Get directory ID by absolute path. One API call, very reliable."""
        result = self._cookie_request(
            "GET",
            f"{WEB_API}/files/getid",
            params={"path": path},
        )
        if result.get("state"):
            return str(result.get("id", ""))
        return None

    def search(self, keyword: str, dir_id: str = "0") -> list[dict]:
        result = self._cookie_request(
            "GET",
            f"{WEB_API}/files/search",
            params={"search_value": keyword, "cid": dir_id, "format": "json"},
        )
        return result.get("data", [])

    def download_url(self, pick_code: str) -> str:
        return self._download_url_cookie(pick_code)

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

    def mkdir(self, parent_id: str, name: str) -> dict:
        return self._cookie_request(
            "POST",
            f"{WEB_API}/files/add",
            data={"pid": parent_id, "cname": name},
        )

    def move(self, file_ids: list[str], target_dir_id: str) -> dict:
        data = {"pid": target_dir_id}
        for i, fid in enumerate(file_ids):
            data[f"fid[{i}]"] = fid
        return self._cookie_request("POST", f"{WEB_API}/files/move", data=data)

    def rename(self, file_id: str, new_name: str) -> dict:
        return self._cookie_request(
            "POST",
            f"{WEB_API}/files/edit",
            data={"fid": file_id, "file_name": new_name},
        )

    def delete(self, file_ids: list[str]) -> dict:
        data = {}
        for i, fid in enumerate(file_ids):
            data[f"fid[{i}]"] = fid
        return self._cookie_request("POST", f"{WEB_API}/rb/delete", data=data)

    def upload_file(self, local_path: Path, target_dir_id: str, filename: str = "") -> dict | None:
        """Upload a small file (NFO, image) to 115 via OSS.

        No encryption needed. Works for files up to ~500MB.
        Retries up to 3 times on network errors.
        """
        import httpx as _httpx

        logger = get_logger()
        fname = filename or local_path.name
        content = local_path.read_bytes()

        self._limiter.acquire()

        # Step 1: Init upload
        for attempt in range(3):
            try:
                resp = self._http.post(
                    "https://uplb.115.com/3.0/sampleinitupload.php",
                    data={"filename": fname, "target": f"U_1_{target_dir_id}"},
                    headers={"Cookie": self._cookies},
                )
                resp.raise_for_status()
                break
            except (httpx.ReadTimeout, httpx.ConnectError, httpx.RemoteProtocolError):
                if attempt == 2:
                    raise
                logger.warning("upload init failed (attempt %d/3), retrying...", attempt + 1)
                time.sleep(3 * (attempt + 1))

        init = resp.json()

        # Step 2: Upload to OSS (separate client, longer timeout, with retry)
        self._limiter.acquire()
        for attempt in range(3):
            try:
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
                    timeout=60,
                )
                break
            except (httpx.ReadTimeout, httpx.ConnectError, httpx.RemoteProtocolError):
                if attempt == 2:
                    raise
                logger.warning("OSS upload failed (attempt %d/3), retrying...", attempt + 1)
                time.sleep(3 * (attempt + 1))

        if oss_resp.status_code == 200:
            logger.debug("115 upload %s → dir=%s", fname, target_dir_id)
            return oss_resp.json().get("data")
        logger.warning("115 upload %s failed: HTTP %d", fname, oss_resp.status_code)
        return None

    def upload_info(self) -> dict:
        """获取上传所需的 user_id 和 user_key。"""
        self._limiter.acquire()
        resp = self._http.get(
            f"{PRO_API}/app/uploadinfo",
            headers={"Cookie": self._cookies},
        )
        resp.raise_for_status()
        result = resp.json()
        return {
            "user_id": str(result.get("user_id", "")),
            "user_key": result.get("userkey", ""),
        }

    def rapid_upload(
        self,
        dir_id: str,
        filename: str,
        file_size: int,
        file_sha1: str,
        file_stream=None,
    ) -> dict:
        """Cookie 模式秒传。

        file_stream: 可选的文件流（seekable），用于 sign_check 验证。
        返回 {"status": 2, "pickcode": "..."} 表示成功，
               {"status": 1} 表示 115 上没有此文件。
        """
        from media115._ec115 import EC115Cipher, TOKEN_SALT

        info = self.upload_info()
        user_id = info["user_id"]
        user_key = info["user_key"]
        user_hash = hashlib.md5(user_id.encode()).hexdigest()
        app_ver = "2.0.3.6"

        ec = EC115Cipher()
        target = f"U_1_{dir_id}"

        sign_key = ""
        sign_val = ""

        for attempt in range(3):  # 最多重试 3 次（sign_check）
            get_logger().debug("115 rapid_upload %s (attempt %d)", filename, attempt)
            timestamp = str(int(time.time()))

            # sig
            h1 = hashlib.sha1(
                f"{user_id}{file_sha1}{target}0".encode()
            ).hexdigest()
            sig = hashlib.sha1(
                f"{user_key}{h1}000000".encode()
            ).hexdigest().upper()

            # token
            token_data = (
                TOKEN_SALT
                + file_sha1
                + str(file_size)
                + sign_key
                + sign_val
                + user_id
                + timestamp
                + user_hash
                + app_ver
            )
            token = hashlib.md5(token_data.encode()).hexdigest()

            form: dict[str, str] = {
                "appid": "0",
                "appversion": app_ver,
                "userid": user_id,
                "filename": filename,
                "filesize": str(file_size),
                "fileid": file_sha1,
                "target": target,
                "sig": sig,
                "t": timestamp,
                "token": token,
            }
            if sign_key:
                form["sign_key"] = sign_key
                form["sign_val"] = sign_val

            # EC115 加密请求
            form_data = urlencode(form).encode()
            encrypted = ec.encode(form_data)

            self._limiter.acquire()
            resp = self._http.post(
                "https://uplb.115.com/4.0/initupload.php",
                params={"k_ec": ec.encode_token()},
                content=encrypted,
                headers={
                    "Cookie": self._cookies,
                    "Content-Type": "application/x-www-form-urlencoded",
                },
            )
            resp.raise_for_status()

            # EC115 解密响应
            result = json.loads(ec.decode(resp.content))

            status = result.get("status")
            if status == 2:
                return {"status": 2, "pickcode": result.get("pickcode", "")}
            elif status == 7 and result.get("statuscode") == 701 and file_stream:
                # sign_check: 计算指定范围 SHA1
                sign_key = result["sign_key"]
                sign_range = result["sign_check"]
                start_s, end_s = sign_range.split("-")
                start, end = int(start_s), int(end_s)
                file_stream.seek(start)
                sign_val = hashlib.sha1(
                    file_stream.read(end - start + 1)
                ).hexdigest().upper()
                continue
            else:
                return {"status": result.get("status", 0)}

        return {"status": 0}  # 超过重试次数

    def batch_rename(self, renames: dict[str, str]) -> dict:
        """Rename multiple files in one API call.

        renames: {file_id: new_name, ...}
        """
        data = {f"files_new_name[{fid}]": name for fid, name in renames.items()}
        return self._cookie_request("POST", f"{WEB_API}/files/batch_rename", data=data)

    def export_tree(self, dir_id: str) -> str | None:
        """Export 115 directory tree. Returns tree text (UTF-8) or None on failure.

        Only uses 2-3 API calls regardless of directory size.
        """
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
