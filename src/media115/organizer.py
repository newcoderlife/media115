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


def generate_strm(strm_path: Path, pick_code: str, proxy_base: str):
    url = f"{proxy_base.rstrip('/')}/play/{pick_code}"
    strm_path.write_text(url + "\n")


def process_file_to_strm(
    video_path: Path,
    cloud_client,
    remote_dir_id: str,
    proxy_base: str,
) -> dict:
    if video_path.suffix == ".strm":
        return {"success": True, "skipped": True}

    sha1 = compute_sha1(video_path)
    pre_sha1 = compute_pre_sha1(video_path)
    file_size = video_path.stat().st_size

    result = cloud_client.rapid_upload(
        dir_id=remote_dir_id,
        filename=video_path.name,
        file_size=file_size,
        sha1=sha1,
        pre_sha1=pre_sha1,
    )

    if result.get("status") != 2:
        return {
            "success": False,
            "reason": "rapid_upload_failed",
            "status": result.get("status"),
        }

    pick_code = result["data"]["pick_code"]
    strm_path = video_path.with_suffix(".strm")
    generate_strm(strm_path, pick_code, proxy_base)
    video_path.unlink()

    return {"success": True, "pick_code": pick_code, "strm_path": str(strm_path)}


# ── 115 Cloud directory organization ─────────────────────────────────


def plan_movie_rename(folder_name: str, metadata: dict | None) -> str | None:
    """Generate Jellyfin-standard folder name for a movie.

    Input:  'A.Better.Tomorrow.II.1987.BluRay.2160p...'
    Output: '英雄本色2 (1987)' (if metadata available)
    Or:     'A Better Tomorrow II (1987)' (parsed from filename)
    """
    if metadata:
        title = metadata.get("title", "")
        year = metadata.get("year")
        if title and year:
            clean = _sanitize(title)
            return f"{clean} ({year})"

    # Fallback: parse from folder name
    m = re.match(r"^(.+?)[.\s](\d{4})[.\s]", folder_name)
    if m:
        title = m.group(1).replace(".", " ").strip()
        year = m.group(2)
        return f"{title} ({year})"

    return None


def plan_tv_rename(folder_name: str, metadata: dict | None) -> str | None:
    """Generate Jellyfin-standard folder name for a TV show.

    Input:  '沧元图.The.Demon.Hunter.S01.2026...'
    Output: '沧元图 (2026)'
    """
    if metadata:
        title = metadata.get("title", "")
        year = (metadata.get("aired", "") or metadata.get("premiered", ""))[:4]
        if title and year:
            clean = _sanitize(title)
            return f"{clean} ({year})"

    # Fallback: parse
    m = re.match(r"^(.+?)[.\s]S\d{2}", folder_name, re.IGNORECASE)
    if m:
        title = m.group(1).replace(".", " ").strip()
        # Try to find year
        year_m = re.search(r"\.(\d{4})\.", folder_name)
        year = year_m.group(1) if year_m else ""
        if year:
            return f"{title} ({year})"
        return title

    return None


def plan_av_rename(folder_name: str) -> str | None:
    """Generate clean AV folder name.

    Input:  'DANDY-992.2026.2160p.DMM.WEB-DL...'
    Output: 'DANDY-992'
    """
    m = re.match(r"^([A-Z]{2,10}-\d{3,5})", folder_name, re.IGNORECASE)
    if m:
        return m.group(1).upper()
    return None


def plan_episode_rename(filename: str, show_title: str) -> str | None:
    """Generate Jellyfin-standard episode filename.

    Input:  'The.Demon.Hunter.S01E67.2026.2160p...'
    Output: '沧元图 S01E67.mkv'
    """
    m = re.search(r"(S\d{2}E\d{2,3})", filename, re.IGNORECASE)
    if m:
        ep_tag = m.group(1).upper()
        ext_m = re.search(r"(\.\w{2,4})$", filename)
        ext = ext_m.group(1) if ext_m else ".mkv"
        clean = _sanitize(show_title)
        return f"{clean} {ep_tag}{ext}"
    return None


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
        ext_m = re.search(r"(\\.\w{2,4})$", name)
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

    for op in ops:
        if op["action"] == "skip":
            results.append({**op, "status": "skipped"})
            continue

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

            results.append({**op, "status": "ok"})
        except Exception as e:
            results.append({**op, "status": "error", "error": str(e)})

    return results


def _sanitize(name: str) -> str:
    """Remove characters not allowed in filenames."""
    return re.sub(r'[<>:"/\\|?*]', "", name).strip()
