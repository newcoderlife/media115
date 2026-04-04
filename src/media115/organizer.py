"""File organizer: SHA1 hashing, rapid upload, STRM generation, 115 rename."""

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


def build_organize_plan(
    category: str, tree_entries: list[dict], cache_module
) -> list[dict]:
    """Build a rename/move plan from file_map cache (written by batch-scrape).

    Uses scrape results for correct title/year, not filename parsing.
    Zero API calls.
    """
    from media115.utils import stem as _u_stem

    ops = []
    videos = [e for e in tree_entries if e["is_video"] and category in e["path"]]

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
        }

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
            if title and year:
                new_folder = f"{_sanitize(title)} ({year})"
                new_name = f"{_sanitize(title)} ({year}){ext}"
                if new_folder != parent_leaf or new_name != name:
                    op["new_folder"] = new_folder
                    op["new_name"] = new_name
                    op["action"] = "rename"

        elif media_type == "av":
            number = scrape_info.get("number", "")
            if number and number != parent_leaf:
                op["new_folder"] = number
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


def execute_organize_plan(ops: list[dict], client, category_path: str) -> list[dict]:
    """Execute organize operations on 115.

    Strategy: always mkdir target → move file → rename file.
    Never rename existing directories. Clean up empty dirs after.

    category_path: e.g. "影音/电影" — the parent where new folders are created.
    """
    results = []
    # Cache: original folder path → (cid, {filename: fid})
    folder_cache: dict[str, tuple[str, dict[str, str]]] = {}
    # Cache: new folder name → cid (avoid creating same folder twice)
    created_dirs: dict[str, str] = {}

    # Get category dir cid
    category_cid = client.get_dir_id("/" + category_path)
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

    import sys

    total = len([o for o in ops if o["action"] != "skip"])
    done = 0

    for op in ops:
        if op["action"] == "skip":
            results.append({**op, "status": "skipped"})
            continue

        done += 1
        print(
            f"  [{done}/{total}] {op['file'][:60]} ...",
            end="",
            flush=True,
            file=sys.stderr,
        )

        try:
            parent_path = op["parent"]

            # Get file's fid from its current directory
            if parent_path not in folder_cache:
                cid = client.get_dir_id("/" + parent_path)
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
                fid_map = {
                    f.get("n", ""): f.get("fid", "") for f in files if "fid" in f
                }
                folder_cache[parent_path] = (cid, fid_map)

            _, fid_map = folder_cache[parent_path]
            fid = fid_map.get(op["file"])
            if not fid:
                results.append(
                    {
                        **op,
                        "status": "not_found",
                        "error": f"file not in dir: {op['file']}",
                    }
                )
                continue

            target_folder = op.get("new_folder")
            new_name = op.get("new_name")

            # Step 1: Ensure target directory exists
            if target_folder:
                if target_folder not in created_dirs:
                    result = client.mkdir(category_cid, target_folder)
                    new_cid = str(result.get("cid", result.get("aid", "")))
                    if new_cid:
                        created_dirs[target_folder] = new_cid
                    else:
                        # mkdir might fail if dir already exists, try get_dir_id
                        existing = client.get_dir_id(
                            f"/{category_path}/{target_folder}"
                        )
                        if existing:
                            created_dirs[target_folder] = existing

                target_cid = created_dirs.get(target_folder)
                if target_cid:
                    # Step 2: Move file to target directory
                    client.move([fid], target_cid)

            # Step 3: Rename file
            if new_name and new_name != op["file"]:
                client.rename(fid, new_name)

            # Step 4: Upload NFO + poster if available
            if target_folder:
                upload_cid = created_dirs.get(target_folder) or category_cid
                _upload_scrape_output(client, op, upload_cid)

            # Step 5: Update file_map cache with new filename
            from media115 import cache as _cache
            from media115.utils import stem as _u_stem

            old_stem = _u_stem(op["file"])
            new_stem = _u_stem(new_name) if new_name else old_stem
            if new_stem != old_stem:
                old_data = _cache.get("file_map", old_stem)
                if old_data:
                    _cache.put("file_map", new_stem, old_data)

            results.append({**op, "status": "ok"})
            print(" ok", file=sys.stderr)
        except Exception as e:
            results.append({**op, "status": "error", "error": str(e)})
            print(f" error: {e}", file=sys.stderr)

    return results


def _upload_scrape_output(client, op: dict, target_cid: str):
    """Upload NFO + poster for an organized file if they exist locally."""
    import os

    scrape_dir = Path(os.getcwd()) / ".cache" / "scrape_output"
    if not scrape_dir.exists():
        return

    # Find matching output directory (by original parent path)
    parent = op.get("parent", "")
    search_name = parent.replace("/", "_")

    for category_dir in scrape_dir.iterdir():
        if not category_dir.is_dir():
            continue
        for out_dir in category_dir.iterdir():
            if not out_dir.is_dir():
                continue
            if search_name in out_dir.name:
                # Upload all NFO and image files
                for f in out_dir.iterdir():
                    if f.suffix in (".nfo", ".jpg", ".png"):
                        try:
                            client.upload_file(f, target_cid, f.name)
                        except Exception:
                            pass
                return


def _sanitize(name: str) -> str:
    """Remove characters not allowed in filenames."""
    return re.sub(r'[<>:"/\\|?*]', "", name).strip()
