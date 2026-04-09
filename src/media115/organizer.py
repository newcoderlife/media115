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

    # Pattern 2: Single letter A-D (multi-disc)
    # e.g., "A.FHD", "B_4K", "A_4K^WM", ".A", ".B" (already organized)
    if s and s[0] in "ABCDabcd" and (len(s) == 1 or not s[1].isalnum() or s[1].isdigit()):
        return "." + s[0].upper()
    # Also match ".A", ".B" etc (after previous organize added the dot)
    if len(s) >= 2 and s[0] in "._" and s[1] in "ABCDabcd" and (len(s) == 2 or not s[2].isalnum()):
        return "." + s[1].upper()

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

    # Phase 2: Create target directories (only those that don't exist)
    target_folders = {op.get("new_folder") for op, _ in resolved if op.get("new_folder")}
    created_folders: set[str] = set()
    newly_created: set[str] = set()
    logger.info("  Creating %d directories...", len(target_folders))

    # List category dir once to find existing subdirs — avoids blind mkdir calls
    try:
        existing_items = client.list_dir(cat_path)
        existing_dirs = {item["name"] for item in existing_items if item.get("type") == "dir"}
    except Exception:
        existing_dirs = set()

    for folder in target_folders:
        if folder in existing_dirs:
            # Already exists — treat as created so moves/renames proceed
            created_folders.add(folder)
            continue
        try:
            client.mkdir(cat_path + "/" + folder)
            created_folders.add(folder)
            newly_created.add(folder)
        except Exception:
            # Last-resort: mkdir may still fail for other reasons; verify existence
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
    # Group video_file dicts by target directory
    target_groups: dict[str, list[dict]] = {}  # target_dir → [video_file_dicts]

    for i, (op, parent_path) in enumerate(resolved):
        if i in failed_ops:
            results.append({**op, "status": "error", "error": "move or rename failed"})
            continue

        target_folder = op.get("new_folder")
        if target_folder and target_folder in created_folders:
            upload_dir = cat_path + "/" + target_folder
        else:
            upload_dir = "/" + parent_path

        video_info = {
            "file": op["file"],
            "parent": op.get("parent", parent_path),
            "new_name": op.get("new_name"),
        }
        target_groups.setdefault(upload_dir, []).append(video_info)

        # Update file_map cache
        new_name = op.get("new_name")
        old_stem = _u_stem(op["file"])
        new_stem = _u_stem(new_name) if new_name else old_stem
        if new_stem != old_stem:
            old_data = _cache.get("file_map", old_stem)
            if old_data:
                _cache.put("file_map", new_stem, old_data)

        results.append({**op, "status": "ok"})

    num_groups = len(target_groups)
    logger.info("  Uploading NFO/poster across %d director(ies)...", num_groups)
    total_uploaded = 0
    for idx, (upload_dir, vf_list) in enumerate(target_groups.items(), 1):
        display_dir = upload_dir.rsplit("/", 1)[-1][:50]
        print(
            f"\r  [{idx}/{num_groups}] {display_dir}",
            end="", file=sys.stderr, flush=True,
        )
        dir_name = upload_dir.rsplit("/", 1)[-1]
        is_new = dir_name in newly_created
        total_uploaded += sync_sidecars(client, upload_dir, vf_list, skip_listing=is_new)

    print("", file=sys.stderr)  # newline
    logger.info(
        "  Uploaded %d NFO/poster file(s) across %d director(ies)",
        total_uploaded,
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


def verify_organize(client, ops: list[dict], cat_path: str) -> tuple[int, int]:
    """Verify organize results by refreshing touched directories.

    Returns (verified_count, mismatch_count).
    """
    # Collect unique target directories
    touched = set()
    for op in ops:
        # Target directory (where file should end up)
        if op.get("new_folder"):
            touched.add(cat_path + "/" + op["new_folder"])
        else:
            touched.add("/" + op["parent"])  # in-place rename stays in original dir

    if not touched:
        return 0, 0

    # Refresh only touched dirs
    client.refresh_paths(list(touched))

    # Verify: check each op's expected file exists
    verified = 0
    mismatches = 0
    for op in ops:
        # Determine expected file name and directory
        expected_name = op.get("new_name") or op["file"]  # if no rename, original name
        if op.get("new_folder"):
            target_dir = cat_path + "/" + op["new_folder"]
        else:
            target_dir = "/" + op["parent"]

        try:
            items = client.list_dir(target_dir)
            names = {item["name"] for item in items}
            if expected_name in names:
                verified += 1
            else:
                mismatches += 1
        except FileNotFoundError:
            mismatches += 1

    return verified, mismatches


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


def sync_sidecars(
    client,
    target_dir: str,
    video_files: list[dict],
    category: str = "",
    skip_listing: bool = False,
) -> int:
    """Sync local scrape_output (NFO/poster) to a 115 directory.

    For each video_file, finds matching local sidecar files and uploads them.
    Handles: per-video NFOs (matched by stem), shared files (tvshow.nfo, poster.jpg).
    Deduplicates shared files across multiple videos in the same directory.
    Deletes old remote files before uploading to avoid 115 duplicates.

    Args:
        client: CachedClient instance
        target_dir: remote 115 path like "/影音/电影/Movie (2024)"
        video_files: list of dicts, each with at least {"file": "name.mkv",
            "parent": "category/dir"} and optionally {"new_name": "optional_new.mkv"}
        category: optional category string (unused, reserved for future filtering)
        skip_listing: if True, skip the initial list_dir call (for newly created
            empty directories where no stale sidecars can exist)

    Returns:
        number of files uploaded
    """
    from media115.log import get_logger
    _log = get_logger()

    # Step 1: Collect all sidecar pairs, deduplicating by remote_name
    uploads: list[tuple] = []  # [(local_path, remote_name), ...]
    seen_names: set[str] = set()
    for vf in video_files:
        new_name = vf.get("new_name")
        pairs = _plan_scrape_upload(vf, new_name)
        for local_path, remote_name in pairs:
            if remote_name not in seen_names:
                uploads.append((local_path, remote_name))
                seen_names.add(remote_name)

    if not uploads:
        return 0

    # Step 2: List target directory once (skip for newly created empty dirs)
    existing_names: dict[str, bool] = {}
    if not skip_listing:
        try:
            items = client.list_dir(target_dir)
            existing_names = {
                item["name"]: True for item in items if item.get("type") == "file"
            }
        except Exception:
            pass

        # Step 3: Batch delete stale sidecars
        to_delete = [
            target_dir + "/" + remote_name
            for _, remote_name in uploads
            if existing_names.get(remote_name)
        ]
        if to_delete:
            try:
                client.delete(to_delete)
                _log.debug("  deleted %d old sidecar(s) in %s", len(to_delete), target_dir)
            except Exception as e:
                _log.warning("  delete old sidecars failed in %s: %s", target_dir, e)

    # Step 4: Upload new sidecar files
    uploaded = 0
    for local_path, remote_name in uploads:
        try:
            client.upload(str(local_path), target_dir, remote_name)
            _log.debug("  uploaded %s → %s", remote_name, target_dir)
            uploaded += 1
        except Exception as e:
            _log.warning("  upload failed %s: %s", remote_name, e)

    return uploaded


def _sanitize(name: str) -> str:
    """Remove characters not allowed in filenames."""
    return re.sub(r'[<>:"/\\|?*]', "", name).strip()
