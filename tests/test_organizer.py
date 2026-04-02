"""Organizer tests: SHA1, pre-SHA1, STRM generation."""

import hashlib
import pytest
from unittest.mock import MagicMock

from media115.organizer import (
    compute_sha1,
    compute_pre_sha1,
    generate_strm,
    process_file_to_strm,
)


@pytest.fixture
def sample_file(tmp_path):
    f = tmp_path / "test_video.mkv"
    # 1MB of predictable data
    data = b"x" * (1024 * 1024)
    f.write_bytes(data)
    return f


@pytest.fixture
def large_file(tmp_path):
    f = tmp_path / "large_video.mkv"
    # 200MB - larger than 128MB pre_sha1 boundary
    chunk = b"a" * (1024 * 1024)  # 1MB
    with open(f, "wb") as fh:
        for _ in range(200):
            fh.write(chunk)
    return f


class TestComputeSha1:
    def test_small_file(self, sample_file):
        result = compute_sha1(sample_file)
        expected = hashlib.sha1(sample_file.read_bytes()).hexdigest()
        assert result == expected

    def test_consistent(self, sample_file):
        r1 = compute_sha1(sample_file)
        r2 = compute_sha1(sample_file)
        assert r1 == r2

    def test_hex_format(self, sample_file):
        result = compute_sha1(sample_file)
        assert len(result) == 40
        assert all(c in "0123456789abcdef" for c in result)


class TestComputePreSha1:
    def test_small_file_equals_full_sha1(self, sample_file):
        """Files smaller than 128MB: pre_sha1 == sha1."""
        pre = compute_pre_sha1(sample_file)
        full = compute_sha1(sample_file)
        assert pre == full

    def test_large_file_differs_from_full(self, large_file):
        """Files larger than 128MB: pre_sha1 != sha1."""
        pre = compute_pre_sha1(large_file)
        full = compute_sha1(large_file)
        assert pre != full

    def test_large_file_only_reads_128mb(self, large_file):
        """pre_sha1 should be SHA1 of first 128MB."""
        pre = compute_pre_sha1(large_file)
        with open(large_file, "rb") as f:
            data = f.read(128 * 1024 * 1024)
        expected = hashlib.sha1(data).hexdigest()
        assert pre == expected


class TestGenerateStrm:
    def test_creates_strm_file(self, tmp_path):
        strm_path = tmp_path / "movie.strm"
        generate_strm(strm_path, pick_code="abc123", proxy_base="http://proxy:9000")
        assert strm_path.exists()
        assert strm_path.read_text().strip() == "http://proxy:9000/play/abc123"

    def test_strm_content_format(self, tmp_path):
        strm_path = tmp_path / "test.strm"
        generate_strm(strm_path, pick_code="xyz", proxy_base="http://10.0.0.1:9000")
        content = strm_path.read_text().strip()
        assert content.startswith("http://")
        assert "/play/xyz" in content


class TestProcessFileToStrm:
    def test_replaces_video_with_strm(self, sample_file):
        mock_client = MagicMock()
        mock_client.rapid_upload.return_value = {
            "status": 2,
            "data": {"pick_code": "uploaded123"},
        }

        result = process_file_to_strm(
            video_path=sample_file,
            cloud_client=mock_client,
            remote_dir_id="0",
            proxy_base="http://proxy:9000",
        )
        assert result["success"] is True
        assert result["pick_code"] == "uploaded123"
        # Original video should be deleted
        assert not sample_file.exists()
        # STRM should exist
        strm_path = sample_file.with_suffix(".strm")
        assert strm_path.exists()
        assert "uploaded123" in strm_path.read_text()

    def test_keeps_video_if_upload_fails(self, sample_file):
        mock_client = MagicMock()
        mock_client.rapid_upload.return_value = {"status": 1}  # not rapid

        result = process_file_to_strm(
            video_path=sample_file,
            cloud_client=mock_client,
            remote_dir_id="0",
            proxy_base="http://proxy:9000",
        )
        assert result["success"] is False
        # Original should still exist
        assert sample_file.exists()

    def test_skips_existing_strm(self, tmp_path):
        strm = tmp_path / "already.strm"
        strm.write_text("http://proxy:9000/play/old")

        result = process_file_to_strm(
            video_path=strm,
            cloud_client=MagicMock(),
            remote_dir_id="0",
            proxy_base="http://proxy:9000",
        )
        assert result["success"] is True
        assert result["skipped"] is True
