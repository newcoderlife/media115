"""File organizer: SHA1 hashing, rapid upload, STRM generation, 115 rename."""

from __future__ import annotations

import hashlib
import re
from pathlib import Path

_CHUNK_SIZE = 1024 * 1024  # 1MB
_PRE_SHA1_SIZE = 128 * 1024 * 1024  # 128MB


def compute_sha1(file_path: Path) -> str:
    h = hashlib.sha1()
    with open(file_path, "rb") as f:
        while chunk := f.read(_CHUNK_SIZE):
            h.update(chunk)
    return h.hexdigest()


def compute_pre_sha1(file_path: Path) -> str:
    file_size = file_path.stat().st_size
    if file_size <= _PRE_SHA1_SIZE:
        return compute_sha1(file_path)

    h = hashlib.sha1()
    bytes_read = 0
    with open(file_path, "rb") as f:
        while bytes_read < _PRE_SHA1_SIZE:
            to_read = min(_CHUNK_SIZE, _PRE_SHA1_SIZE - bytes_read)
            chunk = f.read(to_read)
            if not chunk:
                break
            h.update(chunk)
            bytes_read += len(chunk)
    return h.hexdigest()


# ── 115 Cloud directory organization ─────────────────────────────────


def _extract_av_suffix(after_number: str) -> str:
    """Extract content-discriminating suffix from the part of filename after the AV number.

    Preserves: Part1, A/B/C/D (disc), -C (cut version), CD1/CD2
    Strips: HD, FHD, 4K, quality markers, team names, codec info

    Examples:
        "-C.mp4 stuff" -> "-C"
        "A.FHD" -> ".A"
        ".Part1" -> ".Part1"
        ".FHD" -> ""
        ".HD" -> ""
        "B_4K^WM" -> ".B"
        ".2160p.DMM.WEB-DL..." -> ""
    """
    if not after_number:
        return ""

    # Normalize separators
    s = after_number

    # Pattern 1: -C (cut/censored version)
    if s.startswith("-C") and (len(s) == 2 or not s[2].isalpha()):
        return "-C"

    # Pattern 2: Single letter A-D immediately after number (multi-disc)
    # e.g., "A.FHD", "B_4K", "A_4K^WM"
    if s and s[0] in "ABCDabcd" and (len(s) == 1 or not s[1].isalnum() or s[1].isdigit()):
        return "." + s[0].upper()

    # Pattern 3: .Part1, _Part2 etc
    part_m = re.match(r'[._-]?(Part\d+)', s, re.IGNORECASE)
    if part_m:
        return "." + part_m.group(1)

    # Pattern 4: .CD1, _CD2
    cd_m = re.match(r'[._-]?(CD\d+)', s, re.IGNORECASE)
    if cd_m:
        return "." + cd_m.group(1)

    # No content suffix found — everything else is quality/encode markers
    return ""


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
                # Extract content-discriminating suffix from original filename
                # Strip the extension first, then search for the number prefix
                name_stem = re.sub(r'\.\w{2,4}$', '', name)  # remove extension
                # Build a regex from the number that tolerates optional separators
                # e.g. "ABP-123" should match "abp123", "ABP-123", "ABP_123"
                # Escape each char, then replace escaped separator sequences with optional sep
                num_parts = re.split(r'[-_.]', number)
                num_re = r'[-_.]?'.join(re.escape(p) for p in num_parts)
                m = re.search(num_re + r'(.*)', name_stem, re.IGNORECASE)
                after_number = m.group(1) if m else ""

                # Extract meaningful suffix from what follows the number
                suffix = _extract_av_suffix(after_number)

                new_name = f"{number}{suffix}{ext}"
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
    ops: list[dict], client, category_path: str,
) -> list[dict]:
    """Execute organize operations on 115 using CachedClient batch APIs.

    *client* must be a :class:`cloud115.CachedClient`.

    Strategy:
    1. Resolve files via ``client.list_dir``
    2. mkdir all target dirs via ``client.mkdir``
    3. Batch move files via ``client.move`` (path-based)
    4. Batch rename files via ``client.batch_rename`` (path-based)
    5. Upload NFO/poster for each target dir
    6. Cleanup old source directories
    """
    import sys
    from collections import defaultdict

    from media115 import cache as _cache
    from media115.log import get_logger
    from media115.utils import stem as _u_stem

    logger = get_logger()

    results = []
    cat_path = "/" + category_path

    try:
        client.resolve_path(cat_path)
    except FileNotFoundError:
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

    # Phase 1: Resolve files by listing parent directories
    logger.info("  Resolving %d files...", len(active_ops))

    # Cache of parent_path → {filename → True} for existence checks
    dir_listing_cache: dict[str, dict[str, bool]] = {}

    resolved = []  # (op, parent_path)
    for op in active_ops:
        parent_path = op["parent"]
        if parent_path not in dir_listing_cache:
            try:
                items = client.list_dir("/" + parent_path)
                file_set = {
                    item["name"]: True
                    for item in items
                    if item.get("type") == "file"
                }
                dir_listing_cache[parent_path] = file_set
            except FileNotFoundError:
                results.append(
                    {
                        **op,
                        "status": "not_found",
                        "error": f"dir not found: {parent_path}",
                    }
                )
                continue

        file_set = dir_listing_cache[parent_path]
        if op["file"] not in file_set:
            results.append(
                {**op, "status": "not_found", "error": f"file not in dir: {op['file']}"}
            )
            continue
        resolved.append((op, parent_path))

    logger.info("  Resolved %d/%d files", len(resolved), len(active_ops))

    # Phase 2: Create all target directories
    target_folders = {op.get("new_folder") for op, _ in resolved if op.get("new_folder")}
    created_folders: set[str] = set()
    logger.info("  Creating %d directories...", len(target_folders))
    for folder in target_folders:
        try:
            client.mkdir(cat_path + "/" + folder)
            created_folders.add(folder)
        except Exception:
            # mkdir may fail if already exists in some edge cases;
            # try to verify it exists
            try:
                client.resolve_path(cat_path + "/" + folder)
                created_folders.add(folder)
            except FileNotFoundError:
                pass

    # Phase 3: Batch move (grouped by target dir)
    move_groups: dict[str, list[str]] = defaultdict(list)  # target_folder → [src_paths]
    move_path_to_op: dict[str, dict] = {}  # src_path → op
    for op, parent_path in resolved:
        target_folder = op.get("new_folder")
        if target_folder and target_folder in created_folders:
            src_path = "/" + parent_path + "/" + op["file"]
            move_groups[target_folder].append(src_path)
            move_path_to_op[src_path] = op

    failed_ops: set[int] = set()  # indices into resolved
    total_moves = sum(len(paths) for paths in move_groups.values())
    logger.info("  Moving %d files to %d directories...", total_moves, len(move_groups))
    for target_folder, src_paths in move_groups.items():
        target_dir = cat_path + "/" + target_folder
        try:
            client.move(src_paths, target_dir)
        except Exception as e:
            logger.error("  Move failed: %s", e)
            for sp in src_paths:
                op = move_path_to_op.get(sp)
                if op:
                    for i, (r_op, _) in enumerate(resolved):
                        if r_op is op:
                            failed_ops.add(i)

    # Phase 4: Batch rename
    rename_list: list[tuple[str, str]] = []  # (path, new_name)
    rename_indices: list[int] = []
    for i, (op, parent_path) in enumerate(resolved):
        if i in failed_ops:
            continue
        new_name = op.get("new_name")
        if new_name and new_name != op["file"]:
            target_folder = op.get("new_folder")
            if target_folder and target_folder in created_folders:
                file_path = cat_path + "/" + target_folder + "/" + op["file"]
            else:
                file_path = "/" + parent_path + "/" + op["file"]
            rename_list.append((file_path, new_name))
            rename_indices.append(i)

    if rename_list:
        logger.info("  Renaming %d files...", len(rename_list))
        try:
            client.batch_rename(rename_list)
        except Exception as e:
            logger.error("  Rename failed: %s", e)
            failed_ops.update(rename_indices)

    # Phase 5: Upload NFO/poster grouped by target directory (skip failed files)
    # Collect (local_path, remote_name) pairs per target dir
    upload_groups: dict[str, list[tuple]] = {}  # target_dir → [(local_path, remote_name)]
    successful_ops: list[tuple[int, dict]] = []  # (resolved_index, op) for status tracking

    for i, (op, parent_path) in enumerate(resolved):
        if i in failed_ops:
            results.append({**op, "status": "error", "error": "move or rename failed"})
            continue

        target_folder = op.get("new_folder")
        if target_folder and target_folder in created_folders:
            upload_dir = cat_path + "/" + target_folder
        else:
            upload_dir = "/" + parent_path

        pairs = _plan_scrape_upload(op, op.get("new_name"))
        if pairs:
            existing = upload_groups.setdefault(upload_dir, [])
            existing_names = {name for _, name in existing}
            for local_path, remote_name in pairs:
                if remote_name not in existing_names:
                    existing.append((local_path, remote_name))
                    existing_names.add(remote_name)

        # Update file_map cache
        new_name = op.get("new_name")
        old_stem = _u_stem(op["file"])
        new_stem = _u_stem(new_name) if new_name else old_stem
        if new_stem != old_stem:
            old_data = _cache.get("file_map", old_stem)
            if old_data:
                _cache.put("file_map", new_stem, old_data)

        successful_ops.append((i, op))
        results.append({**op, "status": "ok"})

    # One list_dir + one delete call per target directory
    num_groups = len(upload_groups)
    logger.info(
        "  Uploading NFO/poster: %d sidecar file(s) across %d director(ies)...",
        sum(len(v) for v in upload_groups.values()),
        num_groups,
    )
    for idx, (upload_dir, uploads) in enumerate(upload_groups.items(), 1):
        display_dir = upload_dir.rsplit("/", 1)[-1][:50]
        print(
            f"\r  [{idx}/{num_groups}] {display_dir}",
            end="", file=sys.stderr, flush=True,
        )

        # One list_dir per directory
        existing_names: dict[str, bool] = {}
        try:
            items = client.list_dir(upload_dir)
            existing_names = {
                item["name"]: True for item in items if item.get("type") == "file"
            }
        except Exception:
            pass

        # Batch delete stale sidecars
        to_delete = [
            upload_dir + "/" + remote_name
            for _, remote_name in uploads
            if existing_names.get(remote_name)
        ]
        if to_delete:
            try:
                client.delete(to_delete)
                logger.debug("  deleted %d old sidecar(s) in %s", len(to_delete), upload_dir)
            except Exception as e:
                logger.warning("  delete old sidecars failed in %s: %s", upload_dir, e)

        # Upload new sidecar files
        for local_path, remote_name in uploads:
            try:
                client.upload(str(local_path), upload_dir, remote_name)
                logger.debug("  uploaded %s → %s", remote_name, upload_dir)
            except Exception as e:
                logger.warning("  upload failed %s: %s", remote_name, e)

    print("", file=sys.stderr)  # newline
    logger.info(
        "  Uploaded NFO/poster for %d director(ies)",
        num_groups,
    )

    # Phase 6: Delete old source directories (now empty or metadata-only)
    source_dirs = set()
    for op, _ in resolved:
        if op.get("new_folder"):
            parent = op["parent"]
            parent_leaf = parent.split("/")[-1] if "/" in parent else parent
            if parent_leaf != op["new_folder"]:
                source_dirs.add(parent_leaf)

    if source_dirs:
        logger.info("  Cleaning up %d old directories...", len(source_dirs))
        cat_items = client.list_dir(cat_path)
        for item in cat_items:
            if item.get("type") != "dir":
                continue
            name = item["name"]
            if name not in source_dirs:
                continue
            # Check if dir still has video files
            contents = client.list_dir(cat_path + "/" + name)
            has_video = any(
                f.get("type") == "file"
                and re.search(r"\.(mkv|mp4|avi|ts|rmvb|flv|wmv)$", f["name"], re.I)
                for f in contents
            )
            if not has_video:
                try:
                    client.delete([cat_path + "/" + name])
                    logger.debug("    Deleted: %s", name)
                except Exception as e:
                    logger.error("    Failed to delete %s: %s", name, e)

    return results


def _plan_scrape_upload(
    op: dict, new_video_name: str | None = None,
) -> list[tuple]:
    """Plan sidecar upload for a single video file.

    Returns [(local_path, remote_name), ...].
    Only includes NFOs matching this video's stem (not all NFOs in the dir).
    """
    from media115.cache import _cache_root
    from media115.utils import split_ext

    scrape_dir = _cache_root() / "scrape_output"
    if not scrape_dir.exists():
        return []

    nfo_name = split_ext(new_video_name)[0] + ".nfo" if new_video_name else None
    video_stem = split_ext(op.get("file", ""))[0]  # original video filename stem

    parent = op.get("parent", "")
    search_name = parent.replace("/", "_")

    for category_dir in scrape_dir.iterdir():
        if not category_dir.is_dir():
            continue
        for out_dir in category_dir.iterdir():
            if not out_dir.is_dir():
                continue
            if out_dir.name == search_name:
                result = []
                for f in out_dir.iterdir():
                    if f.suffix == ".nfo":
                        f_stem = split_ext(f.name)[0]
                        if f_stem == video_stem:
                            # This NFO matches our video — rename to new video name
                            remote_name = nfo_name if nfo_name else f.name
                            result.append((f, remote_name))
                        elif f.name == "tvshow.nfo":
                            # Shared tvshow.nfo — keep original name
                            result.append((f, "tvshow.nfo"))
                        # Skip NFOs for other videos in the same directory
                    elif f.suffix in (".jpg", ".png"):
                        # poster.jpg, fanart.jpg — shared, keep original name
                        result.append((f, f.name))
                return result

    return []


def _upload_scrape_output(
    client, op: dict, target_dir: str, new_video_name: str | None = None,
):
    """Upload NFO + poster for an organized file if they exist locally.

    *target_dir* is a cloud path like ``/影音/电影/Title (2024)``.
    NFO is renamed to match the new video filename so Jellyfin can pair them.

    Backward-compatible wrapper used by ``_upload_missing_nfo`` in cli.py.
    For bulk uploads prefer ``_plan_scrape_upload`` + grouped execution.
    """
    from media115.log import get_logger
    _log = get_logger()

    pairs = _plan_scrape_upload(op, new_video_name)
    if not pairs:
        return

    # 115 does not support overwrite upload — same-name files create duplicates.
    # Use rclone strategy: delete old file then upload new file.
    existing_files: dict[str, bool] = {}
    try:
        items = client.list_dir(target_dir)
        for item in items:
            if item.get("type") == "file":
                existing_files[item["name"]] = True
    except Exception:
        pass

    # Batch delete stale sidecars, then upload
    to_delete = [
        target_dir + "/" + remote_name
        for _, remote_name in pairs
        if existing_files.get(remote_name)
    ]
    if to_delete:
        try:
            client.delete(to_delete)
            for path in to_delete:
                _log.debug("  deleted old %s", path.rsplit("/", 1)[-1])
        except Exception as e:
            _log.warning("  delete old sidecars failed: %s", e)

    for f, remote_name in pairs:
        try:
            client.upload(str(f), target_dir, remote_name)
            _log.debug("  uploaded %s → %s", remote_name, target_dir)
        except Exception as e:
            _log.warning("  upload failed %s: %s", remote_name, e)


def _sanitize(name: str) -> str:
    """Remove characters not allowed in filenames."""
    return re.sub(r'[<>:"/\\|?*]', "", name).strip()
