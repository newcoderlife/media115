"""Scrape orchestration tests (mock all external APIs)."""

import pytest
from unittest.mock import patch, MagicMock
from pathlib import Path

from media115 import cache
from media115.scraper.scrape import scrape_movie, scrape_tv, scrape_av


@pytest.fixture(autouse=True)
def _isolate_cache(tmp_path, monkeypatch):
    monkeypatch.setattr(cache, "CACHE_DIR", str(tmp_path / ".cache"))


class TestScrapeMovie:
    def test_success(self, tmp_path):
        mock_client = MagicMock()
        mock_client.search_movie.return_value = [
            {"id": 603, "release_date": "1999-03-31"}
        ]
        mock_client.movie_detail.return_value = {
            "title": "The Matrix",
            "original_title": "The Matrix",
            "release_date": "1999-03-31",
            "overview": "A hacker...",
            "runtime": 136,
            "vote_average": 8.7,
            "vote_count": 27000,
            "genres": [{"name": "Action"}],
            "production_companies": [],
            "production_countries": [],
            "imdb_id": "tt0133093",
            "tagline": "Welcome to the real world",
            "belongs_to_collection": {"name": "The Matrix Collection"},
            "poster_path": "/poster.jpg",
            "backdrop_path": "/backdrop.jpg",
            "id": 603,
        }
        mock_client.movie_images.return_value = {"posters": [{"file_path": "/p.jpg"}]}
        mock_client.movie_credits.return_value = {
            "crew": [{"name": "Lana Wachowski", "job": "Director"}],
            "cast": [
                {"name": "Keanu Reeves", "character": "Neo", "profile_path": "/k.jpg"}
            ],
        }
        mock_client.close = MagicMock()

        out = tmp_path / "out"
        with patch("media115.scraper.tmdb.TMDBClient", return_value=mock_client):
            with patch.dict("os.environ", {"TMDB_READ_ACCESS_TOKEN": "test"}):
                result = scrape_movie("The Matrix", 1999, "matrix.mkv", out)

        assert result["status"] == "ok"
        assert result["tmdb_id"] == 603
        assert (out / "matrix.nfo").exists()

    def test_not_found_cached(self, tmp_path):
        mock_client = MagicMock()
        mock_client.search_movie.return_value = []
        mock_client.close = MagicMock()

        out = tmp_path / "out"
        with patch("media115.scraper.tmdb.TMDBClient", return_value=mock_client):
            with patch.dict("os.environ", {"TMDB_READ_ACCESS_TOKEN": "test"}):
                r1 = scrape_movie("Nonexistent", None, "x.mkv", out)
                assert r1["status"] == "not_found"

                # Second call should be cached
                r2 = scrape_movie("Nonexistent", None, "x.mkv", out)
                assert r2["status"] == "not_found"
                assert r2.get("cached") is True
                # search_movie should only be called once
                assert mock_client.search_movie.call_count == 1


class TestScrapeAV:
    def test_jav321_success(self, tmp_path):
        meta = {
            "title": "Test AV",
            "number": "ABC-123",
            "actors": [],
            "genres": [],
            "release_date": "",
            "runtime": "",
            "studio": "",
            "director": "",
            "cover_url": "",
        }

        out = tmp_path / "out"
        with patch("media115.scraper.jav321.fetch_metadata", return_value=meta):
            result = scrape_av("ABC-123", "ABC-123.mkv", out)

        assert result["status"] == "ok"
        assert (out / "ABC-123.nfo").exists()
        # Should be cached
        assert cache.get("av", "ABC-123") is not None

    def test_fallback_to_javfree(self, tmp_path):
        meta = {
            "title": "Found on JavFree",
            "number": "XYZ-999",
            "actors": [],
            "genres": [],
            "release_date": "",
            "runtime": "",
            "studio": "",
            "director": "",
            "cover_url": "",
        }

        out = tmp_path / "out"
        with patch("media115.scraper.jav321.fetch_metadata", return_value=None):
            with patch("media115.scraper.javfree.fetch_metadata", return_value=meta):
                result = scrape_av("XYZ-999", "XYZ-999.mkv", out)

        assert result["status"] == "ok"
        cached = cache.get("av", "XYZ-999")
        assert cached["_source"] == "javfree"

    def test_all_fail_caches_not_found(self, tmp_path):
        out = tmp_path / "out"
        with patch("media115.scraper.jav321.fetch_metadata", return_value=None):
            with patch("media115.scraper.javfree.fetch_metadata", return_value=None):
                r1 = scrape_av("GONE-999", "GONE-999.mkv", out)
                assert r1["status"] == "not_found"

                # Second call should be cached
                r2 = scrape_av("GONE-999", "GONE-999.mkv", out)
                assert r2["status"] == "not_found"
                assert r2.get("cached") is True

    def test_cache_hit_skips_network(self, tmp_path):
        cache.put(
            "av",
            "CACHED-001",
            {
                "title": "Cached",
                "number": "CACHED-001",
                "actors": [],
                "genres": [],
                "release_date": "",
                "runtime": "",
                "studio": "",
                "director": "",
                "cover_url": "",
                "_source": "jav321",
            },
        )

        out = tmp_path / "out"
        # No mocking needed — should not call any fetcher
        result = scrape_av("CACHED-001", "CACHED-001.mkv", out)
        assert result["status"] == "ok"
        assert result["match"] == "Cached"
