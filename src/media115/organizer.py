"""File organizer: SHA1 hashing, rapid upload, STRM generation, 115 rename."""

from __future__ import annotations

import contextlib
import hashlib
import re
from pathlib import Path

CHUNK_SIZE = 1024 * 1024  # 1MB
PRE_SHA1_SIZE = 128 * 1024 * 1024  # 128MB


def compute_sha1(file_path: Path) -> str:
    h = hashlib.sha1()
    with open(file_path, "rb") as f:
        while chunk := f.read(CHUNK_SIZE):
            h.update(chunk)
    return h.hexdigest()


def compute_pre_sha1(file_path: Path) -> str:
    file_size = file_path.stat().st_size
    if file_size <= PRE_SHA1_SIZE:
        return compute_sha1(file_path)

    h = hashlib.sha1()
    bytes_read = 0
    with open(file_path, "rb") as f:
        while bytes_read < PRE_SHA1_SIZE:
            to_read = min(CHUNK_SIZE, PRE_SHA1_SIZE - bytes_read)
            chunk = f.read(to_read)
            if not chunk:
                break
            h.update(chunk)
            bytes_read += len(chunk)
    return h.hexdigest()


# ── 115 Cloud directory organization ─────────────────────────────────


def build_organize_plan(category: str, tree_entries: list[dict], cache_module) -> list[dict]:
    """Build a rename/move plan from file_map cache (written by batch-scrape).

    Uses scrape results for correct title/year, not filename parsing.
    Zero API calls.
    """
    from media115.utils import stem as _u_stem

    ops = []
    videos = [
        e for e in tree_entries
        if e["is_video"] and f"/{category}/" in f"/{e['path']}/"
    ]

    for item in videos:
        name = item["n"]
        parent = item["parent"]
        parent_leaf = parent.split("/")[-1] if "/" in parent else parent

        # Look up scrape result by filename stem
        scrape_info = cache_module.get("file_map", _u_stem(name))

        op = {
            "file": name,
            "path": item["path"],
            "parent": parent,
            "type": scrape_info.get("type", "unknown") if scrape_info else "unknown",
            "action": "skip",
            "new_folder": None,
            "new_name": None,
            "reason": "",
            "source_id": "",
        }

        if scrape_info:
            sid = scrape_info.get("tmdb_id", scrape_info.get("number", ""))
            op["source_id"] = str(sid) if sid else ""

        if not scrape_info:
            # Check if already in standard format: "Title (Year).ext"
            if re.match(r".+\(\d{4}\)\.\w{2,4}$", name):
                op["reason"] = "already in standard format"
            else:
                op["reason"] = "no scrape result cached"
            ops.append(op)
            continue

        media_type = scrape_info.get("type", "unknown")
        ext_m = re.search(r"(\.\w{2,4})$", name)
        ext = ext_m.group(1) if ext_m else ".mkv"

        if media_type == "movie":
            title = scrape_info.get("title", "")
            year = scrape_info.get("year")
            if title and year is not None:
                new_folder = f"{_sanitize(title)} ({year})"
                new_name = f"{_sanitize(title)} ({year}){ext}"
                if new_folder != parent_leaf or new_name != name:
                    op["new_folder"] = new_folder
                    op["new_name"] = new_name
                    op["action"] = "rename"

        elif media_type == "av":
            number = scrape_info.get("number", "")
            if number:
                new_folder = number
                # Preserve Part suffix for multi-part files
                part_m = re.search(r"[._](Part\d+)", name, re.IGNORECASE)
                part_suffix = f".{part_m.group(1)}" if part_m else ""
                new_name = f"{number}{part_suffix}{ext}"
                if new_folder != parent_leaf or new_name != name:
                    op["new_folder"] = new_folder if new_folder != parent_leaf else None
                    op["new_name"] = new_name if new_name != name else None
                    op["action"] = "rename"

        elif media_type == "tv":
            show_title = scrape_info.get("showtitle") or scrape_info.get("title", "")
            year = scrape_info.get("year", "")
            season = scrape_info.get("season")
            episode = scrape_info.get("episode")

            new_folder = None
            if show_title and year:
                new_folder = f"{_sanitize(show_title)} ({year})"
            elif show_title:
                new_folder = _sanitize(show_title)

            new_name = None
            if show_title and season is not None and episode is not None:
                new_name = f"{_sanitize(show_title)} S{int(season):02d}E{int(episode):02d}{ext}"

            needs_rename = False
            if new_folder and new_folder != parent_leaf:
                needs_rename = True
            else:
                new_folder = None  # already correct
            if new_name and new_name != name:
                needs_rename = True
            else:
                new_name = None

            if needs_rename:
                op["new_folder"] = new_folder
                op["new_name"] = new_name
                op["action"] = "rename"

        if op["action"] == "skip" and not op["reason"]:
            op["reason"] = "already correct"

        ops.append(op)

    return ops


def execute_organize_plan(
    ops: list[dict], client, category_path: str, resolver=None
) -> list[dict]:
    """Execute organize operations on 115 using batch APIs.

    Strategy:
    1. Resolve all fids (batch per parent dir)
    2. mkdir all target dirs
    3. Batch move files grouped by target dir
    4. Batch rename all files
    5. Upload NFO/poster for each target dir
    6. Update file_map cache
    """
    import sys
    from collections import defaultdict

    from media115 import cache as _cache
    from media115.fs import PathResolver
    from media115.log import get_logger
    from media115.utils import stem as _u_stem

    logger = get_logger()

    if resolver is None:
        resolver = PathResolver(client)

    results = []
    folder_cache: dict[str, tuple[str, dict[str, str]]] = {}
    created_dirs: dict[str, str] = {}

    try:
        category_cid = resolver.resolve_dir("/" + category_path)
    except FileNotFoundError:
        category_cid = None
    if not category_cid:
        return [
            {
                **op,
                "status": "error",
                "error": f"category dir not found: {category_path}",
            }
            for op in ops
            if op["action"] != "skip"
        ]

    active_ops = [op for op in ops if op["action"] != "skip"]
    for op in ops:
        if op["action"] == "skip":
            results.append({**op, "status": "skipped"})

    # Phase 1: Resolve all fids
    # Pre-populate sub-dir cids from category listing to avoid N get_dir_id calls
    logger.info("  Resolving %d files...", len(active_ops))
    cat_items = client.list_files_all(dir_id=category_cid)
    subdir_cids = {}  # subdir_name → cid
    for item in cat_items:
        if "fid" not in item:
            subdir_cids[item.get("n", "")] = str(item.get("cid", ""))

    resolved = []  # (op, fid)
    for op in active_ops:
        parent_path = op["parent"]
        if parent_path not in folder_cache:
            # Try to resolve from pre-fetched subdir list
            parent_leaf = parent_path.split("/")[-1] if "/" in parent_path else parent_path
            cid = subdir_cids.get(parent_leaf)
            if not cid:
                try:
                    cid = resolver.resolve_dir("/" + parent_path)
                except FileNotFoundError:
                    cid = None
            if not cid:
                results.append(
                    {
                        **op,
                        "status": "not_found",
                        "error": f"dir not found: {parent_path}",
                    }
                )
                continue
            files = client.list_files_all(dir_id=cid)
            fid_map = {f.get("n", ""): f.get("fid", "") for f in files if "fid" in f}
            folder_cache[parent_path] = (cid, fid_map)

        _, fid_map = folder_cache[parent_path]
        fid = fid_map.get(op["file"])
        if not fid:
            results.append(
                {**op, "status": "not_found", "error": f"file not in dir: {op['file']}"}
            )
            continue
        resolved.append((op, fid))

    logger.info("  Resolved %d/%d files", len(resolved), len(active_ops))

    # Phase 2: Create all target directories
    target_folders = {op.get("new_folder") for op, _ in resolved if op.get("new_folder")}
    logger.info("  Creating %d directories...", len(target_folders))
    for folder in target_folders:
        if folder not in created_dirs:
            try:
                result = client.mkdir(category_cid, folder)
                new_cid = str(result.get("cid", result.get("aid", "")))
                if new_cid:
                    created_dirs[folder] = new_cid
                    resolver.update_dir_entry("/" + category_path, category_cid, folder, new_cid)
                else:
                    try:
                        existing = resolver.resolve_dir(f"/{category_path}/{folder}")
                    except FileNotFoundError:
                        existing = None
                    if existing:
                        created_dirs[folder] = existing
            except Exception:
                try:
                    existing = resolver.resolve_dir(f"/{category_path}/{folder}")
                except FileNotFoundError:
                    existing = None
                if existing:
                    created_dirs[folder] = existing

    # Phase 3: Batch move (grouped by target dir)
    move_groups: dict[str, list[str]] = defaultdict(list)
    move_fid_to_op: dict[str, dict] = {}  # fid → op
    for op, fid in resolved:
        target_folder = op.get("new_folder")
        if target_folder and target_folder in created_dirs:
            move_groups[created_dirs[target_folder]].append(fid)
            move_fid_to_op[fid] = op

    failed_fids: set[str] = set()
    total_moves = sum(len(fids) for fids in move_groups.values())
    logger.info("  Moving %d files to %d directories...", total_moves, len(move_groups))
    for target_cid, fids in move_groups.items():
        try:
            client.move(fids, target_cid)
            # Mark source dirs stale; target dir is new so its listing is already fresh
            for fid in fids:
                op = move_fid_to_op.get(fid)
                if op:
                    parent_path = op["parent"]
                    if parent_path in folder_cache:
                        src_cid, _ = folder_cache[parent_path]
                        resolver.mark_stale(src_cid)
            resolver.mark_stale(target_cid)
        except Exception as e:
            logger.error("  Move failed: %s", e)
            failed_fids.update(fids)

    # Phase 4: Batch rename
    rename_map: dict[str, str] = {}  # fid → new_name
    for op, fid in resolved:
        if fid in failed_fids:
            continue
        new_name = op.get("new_name")
        if new_name and new_name != op["file"]:
            rename_map[fid] = new_name

    if rename_map:
        logger.info("  Renaming %d files...", len(rename_map))
        try:
            client.batch_rename(rename_map)
            # Mark dirs containing renamed files as stale
            renamed_cids: set[str] = set()
            for op, fid in resolved:
                if fid in rename_map:
                    target_folder = op.get("new_folder")
                    if target_folder and target_folder in created_dirs:
                        renamed_cids.add(created_dirs[target_folder])
                    elif op["parent"] in folder_cache:
                        renamed_cids.add(folder_cache[op["parent"]][0])
            for cid in renamed_cids:
                resolver.mark_stale(cid)
        except Exception as e:
            logger.error("  Rename failed: %s", e)
            failed_fids.update(rename_map.keys())

    # Phase 5: Upload NFO/poster + update file_map (skip failed files)
    upload_total = len(resolved)
    logger.info("  Uploading NFO/poster (%d files)...", upload_total)
    for idx, (op, fid) in enumerate(resolved, 1):
        if fid in failed_fids:
            results.append({**op, "status": "error", "error": "move or rename failed"})
            continue

        target_folder = op.get("new_folder")
        if target_folder and target_folder in created_dirs:
            upload_cid = created_dirs[target_folder]
        else:
            parent = op["parent"]
            parent_leaf = parent.split("/")[-1] if "/" in parent else parent
            upload_cid = subdir_cids.get(parent_leaf)

        display_name = (op.get("new_name") or op["file"])[:50]
        print(
            f"\r  [{idx}/{upload_total}] {display_name}",
            end="", file=sys.stderr, flush=True,
        )

        if upload_cid:
            _upload_scrape_output(client, op, upload_cid, op.get("new_name"))

        # Update file_map cache
        new_name = op.get("new_name")
        old_stem = _u_stem(op["file"])
        new_stem = _u_stem(new_name) if new_name else old_stem
        if new_stem != old_stem:
            old_data = _cache.get("file_map", old_stem)
            if old_data:
                _cache.put("file_map", new_stem, old_data)

        results.append({**op, "status": "ok"})
    print("", file=sys.stderr)  # 换行
    logger.info("  Uploaded %d NFO/poster files", upload_total)

    # Phase 6: Delete old source directories (now empty or metadata-only)
    source_dirs = set()
    for op, _fid in resolved:
        if op.get("new_folder"):
            parent = op["parent"]
            parent_leaf = parent.split("/")[-1] if "/" in parent else parent
            # Only clean up if file moved to a DIFFERENT directory
            if parent_leaf != op["new_folder"]:
                source_dirs.add(parent_leaf)

    if source_dirs:
        logger.info("  Cleaning up %d old directories...", len(source_dirs))
        # Refresh category listing to get current cids
        cat_items = client.list_files_all(dir_id=category_cid)
        for item in cat_items:
            if "fid" in item:
                continue
            name = item.get("n", "")
            if name not in source_dirs:
                continue
            cid = str(item.get("cid", ""))
            # Check if dir still has video files
            contents = client.list_files_all(dir_id=cid)
            has_video = any(
                "fid" in f and re.search(r"\.(mkv|mp4|avi|ts|rmvb|flv|wmv)$", f.get("n", ""), re.I)
                for f in contents
            )
            if not has_video:
                try:
                    client.delete([cid])
                    logger.debug("    Deleted: %s", name)
                    resolver.invalidate(f"/{category_path}/{name}")
                except Exception as e:
                    logger.error("    Failed to delete %s: %s", name, e)

    return results


def _upload_scrape_output(client, op: dict, target_cid: str, new_video_name: str | None = None):
    """Upload NFO + poster for an organized file if they exist locally.

    NFO is renamed to match the new video filename so Jellyfin can pair them.
    """
    from media115.cache import _cache_root
    from media115.utils import split_ext

    scrape_dir = _cache_root() / "scrape_output"
    if not scrape_dir.exists():
        return

    # Compute the NFO name that matches the new video
    nfo_name = split_ext(new_video_name)[0] + ".nfo" if new_video_name else None

    # Find matching output directory (by original parent path)
    parent = op.get("parent", "")
    search_name = parent.replace("/", "_")

    for category_dir in scrape_dir.iterdir():
        if not category_dir.is_dir():
            continue
        for out_dir in category_dir.iterdir():
            if not out_dir.is_dir():
                continue
            if out_dir.name == search_name:
                for f in out_dir.iterdir():
                    if f.suffix in (".nfo", ".jpg", ".png"):
                        # Rename NFO to match video; keep image names as-is
                        remote_name = nfo_name if f.suffix == ".nfo" and nfo_name else f.name
                        with contextlib.suppress(Exception):
                            client.upload_file(f, target_cid, remote_name)
                return


def _sanitize(name: str) -> str:
    """Remove characters not allowed in filenames."""
    return re.sub(r'[<>:"/\\|?*]', "", name).strip()
