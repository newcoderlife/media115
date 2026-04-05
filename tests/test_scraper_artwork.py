"""Artwork download tests. Hits real TMDB image CDN (read-only, free)."""

import pytest

pytestmark = pytest.mark.live

from media115.scraper.artwork import download_image, save_poster


class TestDownloadImage:
    def test_download_real_image(self, tmp_path):
        # TMDB logo, always available
        url = "https://image.tmdb.org/t/p/w92/wwemzKWzjKYJFfCeiB57q3r4Bcm.png"
        out = tmp_path / "test.png"
        download_image(url, out)
        assert out.exists()
        assert out.stat().st_size > 100
        # Should start with PNG magic bytes
        assert out.read_bytes()[:4] == b"\x89PNG"

    def test_skips_if_exists(self, tmp_path):
        out = tmp_path / "existing.jpg"
        out.write_bytes(b"already here")
        download_image("https://example.com/should-not-fetch", out)
        # Should not overwrite
        assert out.read_bytes() == b"already here"


class TestSavePoster:
    def test_save_poster_from_tmdb(self, tmp_path):
        # The Matrix poster path from TMDB
        save_poster("/f89U3ADr1oiB1s9GkdPOEpXUk5H.jpg", tmp_path, size="w92")
        poster = tmp_path / "poster.jpg"
        assert poster.exists()
        assert poster.stat().st_size > 100

    def test_save_custom_filename(self, tmp_path):
        save_poster(
            "/f89U3ADr1oiB1s9GkdPOEpXUk5H.jpg",
            tmp_path,
            size="w92",
            filename="fanart.jpg",
        )
        assert (tmp_path / "fanart.jpg").exists()
