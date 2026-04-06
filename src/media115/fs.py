"""PathResolver: 路径 → ID 翻译层，带两层缓存。

缓存结构（~/.cache/media115/fs/）：
  path_index.json        路径 → cid 映射
  dir/{cid}.json         目录 listing 缓存
"""

from __future__ import annotations

import json
import time
from pathlib import Path

from media115.cache import _cache_dir


# ── 内部辅助 ─────────────────────────────────────────────────────────────────


def _normalize(path: str) -> str:
    """确保以 / 开头，去掉尾部 /，空路径返回 '/'。"""
    if not path:
        return "/"
    path = path if path.startswith("/") else "/" + path
    path = path.rstrip("/") or "/"
    return path


def _normalize_item(item: dict) -> dict:
    """把 115 API 返回的 dict 转换为缓存格式。"""
    name = item.get("fn", item.get("n", ""))
    is_file = "fid" in item and "cid" not in item
    if is_file:
        return {
            "name": name,
            "fid": str(item["fid"]),
            "size": item.get("s", 0),
            "pick_code": item.get("pc", ""),
        }
    else:
        cid = item.get("cid", item.get("fid", ""))
        return {
            "name": name,
            "cid": str(cid),
        }


# ── PathResolver ─────────────────────────────────────────────────────────────


class PathResolver:
    """路径 → ID 的翻译层，带 path_index + dir listing 两层缓存。"""

    def __init__(self, client):
        self.client = client
        self._fs_dir = _cache_dir("fs")
        self._dir_dir = _cache_dir("fs/dir")

    # ── 缓存文件路径 ──────────────────────────────────────────────────────────

    @property
    def _path_index_file(self) -> Path:
        return self._fs_dir / "path_index.json"

    def _dir_listing_file(self, cid: str) -> Path:
        return self._dir_dir / f"{cid}.json"

    # ── 底层读写（内部 + 测试用） ─────────────────────────────────────────────

    def _read_path_index(self) -> dict:
        p = self._path_index_file
        if not p.exists():
            return {}
        try:
            return json.loads(p.read_text(encoding="utf-8"))
        except (json.JSONDecodeError, ValueError):
            return {}

    def _write_path_index(self, path: str, cid: str):
        """把单条路径写入 path_index（合并更新）。"""
        index = self._read_path_index()
        index[path] = {"cid": cid, "ts": int(time.time())}
        self._path_index_file.write_text(
            json.dumps(index, ensure_ascii=False, indent=2), encoding="utf-8"
        )

    def _read_dir_listing(self, cid: str) -> dict | None:
        p = self._dir_listing_file(cid)
        if not p.exists():
            return None
        try:
            return json.loads(p.read_text(encoding="utf-8"))
        except (json.JSONDecodeError, ValueError):
            return None

    def _write_dir_listing(self, cid: str, items: list[dict], stale: bool = False):
        """把 items 列表写入 dir/{cid}.json（已经是缓存格式）。"""
        data = {
            "ts": int(time.time()),
            "stale": stale,
            "items": items,
        }
        self._dir_listing_file(cid).write_text(
            json.dumps(data, ensure_ascii=False, indent=2), encoding="utf-8"
        )

    def _save_dir_listing(self, cid: str, raw_items: list[dict]):
        """把 API 返回的原始 items 规范化后写入缓存。"""
        normalized = [_normalize_item(item) for item in raw_items]
        self._write_dir_listing(cid, normalized)

    # ── 公共 API ──────────────────────────────────────────────────────────────

    def resolve_dir(self, path: str) -> str:
        """路径 → cid。/ 返回 "0"。未找到抛 FileNotFoundError。"""
        path = _normalize(path)
        if path == "/":
            return "0"

        # 查 path_index
        index = self._read_path_index()
        entry = index.get(path)
        if entry:
            return entry["cid"]

        # cache miss → API
        cid = self.client.get_dir_id(path)
        if cid is None:
            raise FileNotFoundError(f"Directory not found: {path!r}")

        # 写入缓存
        self._write_path_index(path, str(cid))
        return str(cid)

    def resolve_file(self, path: str) -> tuple[str, str, dict]:
        """路径 → (fid, parent_cid, metadata)。未找到抛 FileNotFoundError。"""
        path = _normalize(path)

        # 拆出父目录和文件名
        parent, _, name = path.rpartition("/")
        parent = parent or "/"

        parent_cid = self.resolve_dir(parent)

        # 在 listing 中查找
        listing_data = self._read_dir_listing(parent_cid)
        if listing_data and not listing_data.get("stale", False):
            item = self._find_file_in_items(listing_data["items"], name)
            if item:
                return item["fid"], parent_cid, item

        # cache miss 或 stale → API
        raw_items = self.client.list_files_all(parent_cid)
        self._save_dir_listing(parent_cid, raw_items)

        listing_data = self._read_dir_listing(parent_cid)
        if listing_data:
            item = self._find_file_in_items(listing_data["items"], name)
            if item:
                return item["fid"], parent_cid, item

        raise FileNotFoundError(f"File not found: {name!r} in {parent!r}")

    def listing(self, cid: str) -> list[dict]:
        """获取目录 listing（缓存优先，stale 时刷新）。"""
        data = self._read_dir_listing(cid)
        if data and not data.get("stale", False):
            return data["items"]

        # 刷新
        raw_items = self.client.list_files_all(cid)
        self._save_dir_listing(cid, raw_items)
        data = self._read_dir_listing(cid)
        return data["items"] if data else []

    def invalidate(self, path: str):
        """从 path_index 和 dir listing 中移除路径及所有子路径。"""
        path = _normalize(path)
        index = self._read_path_index()
        # 收集要删除的路径：精确匹配 + 所有子路径
        prefix = path + "/"
        to_remove = [p for p in index if p == path or p.startswith(prefix)]
        for p in to_remove:
            entry = index.pop(p, None)
            if entry:
                listing_file = self._dir_listing_file(entry["cid"])
                listing_file.unlink(missing_ok=True)
        self._path_index_file.write_text(
            json.dumps(index, ensure_ascii=False, indent=2), encoding="utf-8"
        )

    def mark_stale(self, cid: str):
        """标记 dir listing 为 stale。"""
        data = self._read_dir_listing(cid)
        if data is None:
            return
        data["stale"] = True
        self._dir_listing_file(cid).write_text(
            json.dumps(data, ensure_ascii=False, indent=2), encoding="utf-8"
        )

    def update_dir_entry(self, parent_path: str, parent_cid: str, name: str, cid: str):
        """把新目录加入 path_index 和父 listing。"""
        parent_path = _normalize(parent_path)
        new_path = f"{parent_path}/{name}" if parent_path != "/" else f"/{name}"

        # 更新 path_index
        self._write_path_index(new_path, cid)

        # 更新父 listing
        data = self._read_dir_listing(parent_cid)
        if data is None:
            data = {"ts": int(time.time()), "stale": False, "items": []}
        # 避免重复
        data["items"] = [i for i in data["items"] if i.get("name") != name]
        data["items"].append({"name": name, "cid": cid})
        self._dir_listing_file(parent_cid).write_text(
            json.dumps(data, ensure_ascii=False, indent=2), encoding="utf-8"
        )

    def remove_from_listing(self, parent_cid: str, name: str):
        """从 listing 中移除条目。"""
        data = self._read_dir_listing(parent_cid)
        if data is None:
            return
        data["items"] = [i for i in data["items"] if i.get("name") != name]
        self._dir_listing_file(parent_cid).write_text(
            json.dumps(data, ensure_ascii=False, indent=2), encoding="utf-8"
        )

    # ── 内部辅助 ──────────────────────────────────────────────────────────────

    def _find_file_in_items(self, items: list[dict], name: str) -> dict | None:
        """在 items 中查找匹配 name 且有 fid 的条目。"""
        for item in items:
            if item.get("name") == name and "fid" in item:
                return item
        return None
