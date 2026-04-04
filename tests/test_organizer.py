"""Organizer tests: SHA1, pre-SHA1."""

import hashlib

import pytest

from media115.organizer import (
    compute_sha1,
    compute_pre_sha1,
)


@pytest.fixture
def sample_file(tmp_path):
    f = tmp_path / "test_video.mkv"
    data = b"x" * (1024 * 1024)
    f.write_bytes(data)
    return f


@pytest.fixture
def large_file(tmp_path):
    f = tmp_path / "large_video.mkv"
    chunk = b"a" * (1024 * 1024)
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
        pre = compute_pre_sha1(sample_file)
        full = compute_sha1(sample_file)
        assert pre == full

    def test_large_file_differs_from_full(self, large_file):
        pre = compute_pre_sha1(large_file)
        full = compute_sha1(large_file)
        assert pre != full

    def test_large_file_only_reads_128mb(self, large_file):
        pre = compute_pre_sha1(large_file)
        with open(large_file, "rb") as f:
            data = f.read(128 * 1024 * 1024)
        expected = hashlib.sha1(data).hexdigest()
        assert pre == expected
