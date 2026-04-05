"""TMDB scraper tests. Hits real TMDB API (read-only, free)."""

import os

import pytest

from media115.scraper.tmdb import TMDBClient

pytestmark = pytest.mark.live


@pytest.fixture
def tmdb():
    token = os.environ.get("TMDB_READ_ACCESS_TOKEN")
    if not token:
        pytest.skip("TMDB_READ_ACCESS_TOKEN not set")
    return TMDBClient(read_access_token=token)


class TestTMDBSearchMovie:
    def test_search_the_matrix(self, tmdb):
        results = tmdb.search_movie("The Matrix")
        assert len(results) > 0
        # The Matrix (1999) should be in results
        ids = [r["id"] for r in results]
        assert 603 in ids

    def test_search_chinese_title(self, tmdb):
        results = tmdb.search_movie("黑客帝国")
        assert len(results) > 0

    def test_search_no_results(self, tmdb):
        results = tmdb.search_movie("xyznonexistentmovie12345")
        assert len(results) == 0


class TestTMDBMovieDetail:
    def test_get_movie_detail(self, tmdb):
        detail = tmdb.movie_detail(603)  # The Matrix
        assert detail["title"] is not None
        assert detail["release_date"].startswith("1999-03")
        assert detail["id"] == 603
        assert "genres" in detail
        assert len(detail["genres"]) > 0

    def test_get_movie_detail_chinese(self, tmdb):
        detail = tmdb.movie_detail(603, language="zh-CN")
        # Chinese title should be present
        assert detail["title"] is not None


class TestTMDBSearchTV:
    def test_search_breaking_bad(self, tmdb):
        results = tmdb.search_tv("Breaking Bad")
        assert len(results) > 0
        ids = [r["id"] for r in results]
        assert 1396 in ids

    def test_search_chinese_drama(self, tmdb):
        results = tmdb.search_tv("漫长的季节")
        assert len(results) > 0


class TestTMDBTVDetail:
    def test_get_tv_detail(self, tmdb):
        detail = tmdb.tv_detail(1396)  # Breaking Bad
        assert detail["name"] is not None
        assert detail["id"] == 1396
        assert "seasons" in detail

    def test_get_season_detail(self, tmdb):
        detail = tmdb.season_detail(1396, 1)  # Breaking Bad S01
        assert "episodes" in detail
        assert len(detail["episodes"]) == 7  # S01 has 7 episodes


class TestTMDBImages:
    def test_get_movie_images(self, tmdb):
        images = tmdb.movie_images(603)
        assert "posters" in images
        assert len(images["posters"]) > 0
        # Each poster should have file_path
        assert "file_path" in images["posters"][0]

    def test_image_url(self, tmdb):
        url = tmdb.image_url("/poster.jpg", size="w500")
        assert url == "https://image.tmdb.org/t/p/w500/poster.jpg"
