"""File organizer: SHA1 hashing, rapid upload, STRM generation."""

import hashlib
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
    # Skip if already a .strm
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
