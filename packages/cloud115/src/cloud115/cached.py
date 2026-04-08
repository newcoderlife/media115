"""CachedClient — cache-first reads + write-through writes over CloudAPI.

Wraps :class:`CloudAPI` and :class:`FileCache` into a single high-level
client.  Every read checks the SQLite cache first (respecting TTL); every
write calls the API then updates the cache so subsequent reads stay warm.
"""
from __future__ import annotations

import hashlib
import time
from pathlib import Path
from typing import Optional

from cloud115.api import CloudAPI
from cloud115.cache import FileCache, _default_db_path
from cloud115.log import get_logger

# ---------------------------------------------------------------------------
# helpers
# ---------------------------------------------------------------------------


def _normalize(path: str) -> str:
    """Ensure *path* starts with ``/``, strip trailing ``/``, empty -> ``/``."""
    path = path.strip()
    if not path or path == "/":
        return "/"
    if not path.startswith("/"):
        path = "/" + path
    return path.rstrip("/")


def _normalize_item(item: dict) -> dict:
    """Convert a raw 115 API dict to the cache entry format.

    API uses ``n`` or ``fn`` for name, ``fid`` for file id, ``cid`` for
    directory id, ``s`` for size, ``pc`` for pick_code.

    Cache format: ``{name, type, node_id, size, pick_code}``.
    A *file* has ``fid`` in the raw dict. A *dir* has ``cid``.
    """
    name = item.get("n") or item.get("fn") or item.get("name", "")
    if "fid" in item:
        # file
        return {
            "name": name,
            "type": "file",
            "node_id": str(item["fid"]),
            "size": item.get("s", 0),
            "pick_code": item.get("pc", ""),
        }
    # directory
    cid = item.get("cid", item.get("fid", ""))
    return {
        "name": name,
        "type": "dir",
        "node_id": str(cid),
        "size": 0,
        "pick_code": "",
    }


def _entry_to_public(entry: dict) -> dict:
    """Convert a cache ``dir_entry`` row to the public format.

    For dirs:  ``{name, type='dir', cid}``
    For files: ``{name, type='file', fid, size, pick_code}``
    """
    if entry["type"] == "dir":
        return {
            "name": entry["name"],
            "type": "dir",
            "cid": entry["node_id"],
        }
    return {
        "name": entry["name"],
        "type": "file",
        "fid": entry["node_id"],
        "size": entry.get("size", 0),
        "pick_code": entry.get("pick_code", ""),
    }


# ---------------------------------------------------------------------------
# CachedClient
# ---------------------------------------------------------------------------


class CachedClient:
    """High-level 115 client with transparent SQLite caching."""

    def __init__(
        self,
        cookies: str,
        cache_dir: Optional[str] = None,
        listing_ttl: int = 3600,
        path_ttl: int = 86400,
    ) -> None:
        cd = Path(cache_dir) if cache_dir else None
        db_path = (cd / "cache.db") if cd else _default_db_path()
        self._cache = FileCache(db_path)
        self._api = CloudAPI(cookies=cookies, file_cache=self._cache)
        self._listing_ttl = listing_ttl
        self._path_ttl = path_ttl

    # ── context manager ─────────────────────────────────────────────

    def close(self) -> None:
        self._api.close()
        self._cache.close()

    def __enter__(self) -> CachedClient:
        return self

    def __exit__(self, *args: object) -> None:
        self.close()

    # ================================================================
    # READ OPERATIONS (cache-first)
    # ================================================================

    def resolve_path(self, path: str) -> str:
        """Resolve a cloud path to a directory cid.

        Returns ``"0"`` for the root. Raises :class:`FileNotFoundError` if
        the path does not exist on 115.
        """
        logger = get_logger()
        path = _normalize(path)
        if path == "/":
            return "0"

        cached = self._cache.get_path(path)
        if cached is not None:
            cid, ts = cached
            age = int(time.time()) - ts
            if age < self._path_ttl:
                logger.debug(
                    "CACHE HIT  resolve  %s -> cid=%s age=%ds", path, cid, age
                )
                return cid

        cid = self._api.get_dir_id(path)
        if cid is None:
            raise FileNotFoundError(f"Path not found on 115: {path}")
        self._cache.set_path(path, cid)
        logger.debug("CACHE MISS resolve  %s -> API cid=%s", path, cid)
        return cid

    # ── list_dir ─────────────────────────────────────────────────────

    def list_dir(self, path: str) -> list:
        """List a directory by path (cache-first)."""
        path = _normalize(path)
        cid = self.resolve_path(path)
        return self._list_dir_by_cid(cid, label=path)

    def _list_dir_by_cid(self, cid: str, label: str = "") -> list:
        """Internal: list directory *cid*, returning public-format dicts."""
        logger = get_logger()
        ts = self._cache.get_dir_ts(cid)
        if ts is not None:
            age = int(time.time()) - ts
            if age < self._listing_ttl:
                entries = self._cache.get_dir_entries(cid)
                logger.debug(
                    "CACHE HIT  list_dir %s cid=%s age=%ds",
                    label, cid, age,
                )
                return [_entry_to_public(e) for e in entries]

        raw_items = self._api.list_files_all(cid)
        normalized = [_normalize_item(it) for it in raw_items]
        self._cache.set_dir_listing(cid, normalized)
        logger.debug(
            "CACHE MISS list_dir %s cid=%s -> API %d items",
            label, cid, len(normalized),
        )
        return [_entry_to_public(e) for e in normalized]

    # ── find_file ────────────────────────────────────────────────────

    def find_file(self, path: str) -> dict:
        """Find a file by full path. Returns metadata dict.

        Raises :class:`FileNotFoundError` if the file does not exist.
        """
        path = _normalize(path)
        if path == "/":
            raise FileNotFoundError("Root is not a file")

        parent_path = path.rsplit("/", 1)[0] or "/"
        filename = path.rsplit("/", 1)[1]
        parent_cid = self.resolve_path(parent_path)

        # Trust rule: only use cached entry if dir_meta is fresh
        ts = self._cache.get_dir_ts(parent_cid)
        if ts is None or (int(time.time()) - ts) >= self._listing_ttl:
            self._list_dir_by_cid(parent_cid, label=parent_path)

        entry = self._cache.find_entry(parent_cid, filename)
        if entry is None:
            raise FileNotFoundError(f"File not found: {path}")

        result = _entry_to_public(entry)
        result["parent_cid"] = parent_cid
        return result

    # ── passthrough / simple reads ───────────────────────────────────

    def download_url(self, pick_code: str, user_agent: str | None = None) -> str:
        """Get a download URL for a file (direct API call, no caching)."""
        get_logger().debug(
            "API        download pick_code=%s ua=%s",
            pick_code, (user_agent or "default")[:30],
        )
        return self._api.download_url(pick_code, user_agent)

    def stream_url(self, path: str, user_agent: str | None = None) -> str:
        """路径 → CDN 下载链接。一步完成 find_file + download_url。
        proxy 和 get 命令的统一入口。"""
        meta = self.find_file(path)
        return self.download_url(meta["pick_code"], user_agent=user_agent)

    def search(self, keyword: str, path: str = "/") -> list:
        """Search for files by keyword under *path*."""
        cid = self.resolve_path(path)
        return self._api.search(keyword, dir_id=cid)

    def export_tree(self, path: str) -> Optional[str]:
        """Export 115 directory tree as text."""
        cid = self.resolve_path(path)
        return self._api.export_tree(cid)

    def walk(self, path: str, max_depth: int = 5) -> list:
        """Recursively list directory tree, using cache at each level.

        Returns a flat list of ``{path, entries}`` dicts, one per directory.
        """
        path = _normalize(path)
        result = []
        self._walk_recursive(path, max_depth, 0, result)
        return result

    def _walk_recursive(
        self, path: str, max_depth: int, depth: int, acc: list
    ) -> None:
        if depth > max_depth:
            return
        entries = self.list_dir(path)
        acc.append({"path": path, "entries": entries})
        for entry in entries:
            if entry["type"] == "dir":
                child = path.rstrip("/") + "/" + entry["name"]
                self._walk_recursive(child, max_depth, depth + 1, acc)

    def stat(self, path: str) -> dict:
        """Return metadata for *path* (file or directory)."""
        path = _normalize(path)
        if path == "/":
            return {"name": "/", "type": "dir", "cid": "0"}
        try:
            return self.find_file(path)
        except FileNotFoundError:
            pass
        # Might be a directory
        cid = self.resolve_path(path)
        name = path.rsplit("/", 1)[-1]
        return {"name": name, "type": "dir", "cid": cid}

    # ================================================================
    # WRITE OPERATIONS (write-through)
    # ================================================================

    def mkdir(self, path: str, parents: bool = False) -> str:
        """Create a directory. Returns the new cid.

        If *parents* is True, create intermediate directories as needed.
        """
        logger = get_logger()
        path = _normalize(path)

        if parents:
            return self._mkdir_parents(path)

        parent_path = path.rsplit("/", 1)[0] or "/"
        name = path.rsplit("/", 1)[1]
        parent_cid = self.resolve_path(parent_path)

        result = self._api.mkdir(parent_cid, name)
        new_cid = str(result.get("cid", result.get("aid", "")))
        logger.debug(
            "API        mkdir    %s parent_cid=%s -> new_cid=%s",
            path, parent_cid, new_cid,
        )

        # write-through
        entry = {
            "name": name,
            "type": "dir",
            "node_id": new_cid,
            "size": 0,
            "pick_code": "",
        }
        self._cache.add_entry(parent_cid, entry)
        self._cache.set_path(path, new_cid)
        logger.debug(
            "WRITE-THRU add_entry cid=%s name=%s type=dir new_cid=%s",
            parent_cid, name, new_cid,
        )
        return new_cid

    def _mkdir_parents(self, path: str) -> str:
        """Create all segments in *path*, returning the final cid."""
        parts = [p for p in path.split("/") if p]
        current_path = ""
        current_cid = "0"
        for part in parts:
            current_path += "/" + part
            try:
                current_cid = self.resolve_path(current_path)
            except FileNotFoundError:
                result = self._api.mkdir(current_cid, part)
                new_cid = str(result.get("cid", result.get("aid", "")))
                entry = {
                    "name": part,
                    "type": "dir",
                    "node_id": new_cid,
                    "size": 0,
                    "pick_code": "",
                }
                self._cache.add_entry(current_cid, entry)
                self._cache.set_path(current_path, new_cid)
                get_logger().debug(
                    "WRITE-THRU add_entry cid=%s name=%s type=dir new_cid=%s",
                    current_cid, part, new_cid,
                )
                current_cid = new_cid
        return current_cid

    def rename(self, path: str, new_name: str) -> None:
        """Rename a file or directory at *path* to *new_name*."""
        logger = get_logger()
        path = _normalize(path)
        parent_path = path.rsplit("/", 1)[0] or "/"
        old_name = path.rsplit("/", 1)[1]
        parent_cid = self.resolve_path(parent_path)

        # Refresh listing if stale to get node_id
        ts = self._cache.get_dir_ts(parent_cid)
        if ts is None or (int(time.time()) - ts) >= self._listing_ttl:
            self._list_dir_by_cid(parent_cid, label=parent_path)

        entry = self._cache.find_entry(parent_cid, old_name)
        if entry is None:
            raise FileNotFoundError(f"Not found: {path}")

        node_id = entry["node_id"]
        self._api.rename(node_id, new_name)
        logger.debug(
            "API        rename   %s node_id=%s -> '%s'", path, node_id, new_name
        )

        # write-through
        self._cache.rename_entry(parent_cid, node_id, new_name)
        if entry["type"] == "dir":
            self._cache.delete_path_prefix(path)
            new_path = parent_path.rstrip("/") + "/" + new_name
            self._cache.set_path(new_path, node_id)
        logger.debug(
            "WRITE-THRU rename_entry cid=%s node_id=%s new_name=%s",
            parent_cid, node_id, new_name,
        )

    def delete(self, paths: list) -> None:
        """Delete one or more files/directories."""
        logger = get_logger()
        ids = []
        resolved = []  # (path, parent_cid, name, node_id, is_dir)

        for p in paths:
            p = _normalize(p)
            parent_path = p.rsplit("/", 1)[0] or "/"
            name = p.rsplit("/", 1)[1]
            parent_cid = self.resolve_path(parent_path)

            # Try as file entry first
            entry = self._cache.find_entry(parent_cid, name)
            if entry is None:
                # Refresh listing
                self._list_dir_by_cid(parent_cid, label=parent_path)
                entry = self._cache.find_entry(parent_cid, name)

            if entry is None:
                # Maybe a directory itself
                try:
                    cid = self.resolve_path(p)
                    ids.append(cid)
                    resolved.append((p, parent_cid, name, cid, True))
                    continue
                except FileNotFoundError:
                    raise FileNotFoundError(f"Not found: {p}")

            ids.append(entry["node_id"])
            resolved.append(
                (p, parent_cid, name, entry["node_id"], entry["type"] == "dir")
            )

        self._api.delete(ids)
        logger.debug("API        delete   %d items", len(ids))

        # write-through
        for p, parent_cid, name, node_id, is_dir in resolved:
            self._cache.remove_entry(parent_cid, name)
            self._cache.delete_path_prefix(p)
            logger.debug(
                "WRITE-THRU remove_entry cid=%s name=%s", parent_cid, name
            )

    def move(self, src_paths: list, dest_path: str) -> None:
        """Move files/directories to *dest_path*."""
        logger = get_logger()
        dest_path = _normalize(dest_path)
        dest_cid = self.resolve_path(dest_path)

        fids = []
        sources = []  # (path, parent_cid, name, entry)

        for sp in src_paths:
            sp = _normalize(sp)
            parent_path = sp.rsplit("/", 1)[0] or "/"
            name = sp.rsplit("/", 1)[1]
            parent_cid = self.resolve_path(parent_path)

            entry = self._cache.find_entry(parent_cid, name)
            if entry is None:
                self._list_dir_by_cid(parent_cid, label=parent_path)
                entry = self._cache.find_entry(parent_cid, name)
            if entry is None:
                raise FileNotFoundError(f"Not found: {sp}")

            fids.append(entry["node_id"])
            sources.append((sp, parent_cid, name, entry))

        self._api.move(fids, dest_cid)
        logger.debug(
            "API        move     %d items -> cid=%s", len(fids), dest_cid
        )

        # write-through
        for sp, parent_cid, name, entry in sources:
            self._cache.remove_entry(parent_cid, name)
            self._cache.delete_path_prefix(sp)
            # Add to destination
            self._cache.add_entry(dest_cid, entry)
            if entry["type"] == "dir":
                new_path = dest_path.rstrip("/") + "/" + name
                self._cache.set_path(new_path, entry["node_id"])
            logger.debug(
                "WRITE-THRU move_entry %s -> cid=%s name=%s",
                sp, dest_cid, name,
            )

    def batch_rename(self, renames: list) -> None:
        """Rename multiple files in one API call.

        *renames*: list of ``(path, new_name)`` tuples.
        """
        logger = get_logger()
        api_renames = {}  # {node_id: new_name}
        entries_info = []  # (parent_cid, node_id, new_name)

        for path, new_name in renames:
            path = _normalize(path)
            parent_path = path.rsplit("/", 1)[0] or "/"
            old_name = path.rsplit("/", 1)[1]
            parent_cid = self.resolve_path(parent_path)

            entry = self._cache.find_entry(parent_cid, old_name)
            if entry is None:
                self._list_dir_by_cid(parent_cid, label=parent_path)
                entry = self._cache.find_entry(parent_cid, old_name)
            if entry is None:
                raise FileNotFoundError(f"Not found: {path}")

            api_renames[entry["node_id"]] = new_name
            entries_info.append((parent_cid, entry["node_id"], new_name))

        self._api.batch_rename(api_renames)
        logger.debug("API        batch_rename %d files", len(api_renames))

        # write-through
        for parent_cid, node_id, new_name in entries_info:
            self._cache.rename_entry(parent_cid, node_id, new_name)
            logger.debug(
                "WRITE-THRU rename_entry cid=%s node_id=%s new_name=%s",
                parent_cid, node_id, new_name,
            )

    def upload(
        self,
        local_path: str,
        remote_dir: str,
        filename: str = "",
    ) -> Optional[dict]:
        """Upload a file to *remote_dir*. Returns API response data or None."""
        logger = get_logger()
        remote_dir = _normalize(remote_dir)
        cid = self.resolve_path(remote_dir)
        local = Path(local_path)
        fname = filename or local.name

        data = self._api.upload_file(local, cid, filename=fname)
        logger.debug("API        upload   '%s' -> cid=%s", fname, cid)

        if data and isinstance(data, dict) and data.get("fid"):
            entry = {
                "name": fname,
                "type": "file",
                "node_id": str(data["fid"]),
                "size": data.get("file_size", local.stat().st_size),
                "pick_code": data.get("pick_code", data.get("pickcode", "")),
            }
            self._cache.add_entry(cid, entry)
            logger.debug(
                "WRITE-THRU add_entry cid=%s name=%s type=file fid=%s",
                cid, fname, data["fid"],
            )
        else:
            # Cannot determine exact entry; mark stale
            self._cache.delete_dir_meta(cid)
            logger.debug("WRITE-THRU delete_dir_meta cid=%s (stale)", cid)

        return data

    def rapid_upload(self, local_path: str, remote_dir: str) -> dict:
        """Attempt rapid (instant) upload via SHA1 matching.

        Returns API result dict (status=2 on success).
        """
        logger = get_logger()
        remote_dir = _normalize(remote_dir)
        cid = self.resolve_path(remote_dir)
        local = Path(local_path)

        file_size = local.stat().st_size

        # Streaming SHA1 — no full-file memory load
        h = hashlib.sha1()
        with open(local, "rb") as f:
            while True:
                chunk = f.read(1024 * 1024)
                if not chunk:
                    break
                h.update(chunk)
        sha1 = h.hexdigest().upper()

        with open(local, "rb") as f:
            result = self._api.rapid_upload(
                dir_id=cid,
                filename=local.name,
                file_size=file_size,
                file_sha1=sha1,
                file_stream=f,
            )

        logger.debug(
            "API        rapid_upload '%s' -> cid=%s status=%s",
            local.name, cid, result.get("status"),
        )

        if result.get("status") == 2:
            self._cache.delete_dir_meta(cid)
            logger.debug("WRITE-THRU delete_dir_meta cid=%s (stale)", cid)

        return result

    # ================================================================
    # AUTH (transparent delegation)
    # ================================================================

    @classmethod
    def qr_login(cls, app: str = "tv") -> CachedClient:
        """Interactive QR-code login. Returns a CachedClient with fresh cookies."""
        api = CloudAPI.qr_login(app)
        client = cls.__new__(cls)
        client._api = api
        db_path = _default_db_path()
        client._cache = FileCache(db_path)
        client._listing_ttl = 3600
        client._path_ttl = 86400
        return client

    def check_login(self) -> bool:
        """Check if current credentials are valid."""
        return self._api.check_login()

    def renew_cookies(self, app: str = "tv") -> bool:
        """Auto-renew cookies without user interaction."""
        return self._api.renew_cookies(app)

    def save_cookies(self, path: str) -> None:
        """Save cookies to an env file."""
        self._api.save_cookies_to_env(Path(path))

    # ================================================================
    # CACHE MANAGEMENT
    # ================================================================

    def invalidate(self, path: str) -> None:
        """Invalidate cache for *path* and everything below it."""
        path = _normalize(path)
        # Get cid BEFORE deleting from path_index
        cached = self._cache.get_path(path)
        self._cache.delete_path_prefix(path)
        # Also invalidate the directory listing if it is one
        if cached is not None:
            self._cache.invalidate_dir(cached[0])

    def cache_status(self) -> dict:
        """Return cache statistics."""
        return self._cache.stats()

    def cache_clear(self) -> None:
        """清除缓存元数据。不清除限流状态。"""
        self._cache.clear_metadata()

    def warm(self, path, depth=3):
        """主动预热：递归 list_dir 到指定深度，填充全部缓存。

        每层 list_dir 自动写入 SQLite（path_index + dir_meta + dir_entry）。
        """
        path = _normalize(path)
        items = self.list_dir(path)
        if depth > 0:
            for item in items:
                if item["type"] == "dir":
                    child_path = path.rstrip("/") + "/" + item["name"]
                    self.warm(child_path, depth - 1)

    def refresh_dir(self, path):
        """强制刷新目录缓存，忽略 TTL。"""
        path = _normalize(path)
        cid = self.resolve_path(path)
        raw = self._api.list_files_all(cid)
        normalized = [_normalize_item(item) for item in raw]
        self._cache.set_dir_listing(cid, normalized)
        get_logger().debug("REFRESH    list_dir %s cid=%s → %d items", path, cid, len(normalized))
        return [_entry_to_public(e) for e in normalized]
