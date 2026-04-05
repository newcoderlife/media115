"""TMDB API client tests. Mocked HTTP, no network."""

from unittest.mock import MagicMock, patch

import pytest

from media115.scraper.tmdb import TMDBClient


@pytest.fixture
def tmdb():
    with patch("media115.scraper.tmdb.httpx.Client"):
        client = TMDBClient(read_access_token="fake_token_123")
        client._last_request = 0
        yield client
        client.close()


@pytest.fixture
def mock_http(tmdb):
    return tmdb._http


class TestTMDBInit:
    def test_init_sets_auth_header(self):
        with patch("media115.scraper.tmdb.httpx.Client") as MockClient:
            TMDBClient(read_access_token="my_token")
            call_kwargs = MockClient.call_args
            headers = call_kwargs.kwargs.get("headers", {})
            assert headers["Authorization"] == "Bearer my_token"

    def test_init_default_language(self):
        with patch("media115.scraper.tmdb.httpx.Client"):
            client = TMDBClient(read_access_token="tok")
            assert client._language == "zh-CN"

    def test_init_custom_language(self):
        with patch("media115.scraper.tmdb.httpx.Client"):
            client = TMDBClient(read_access_token="tok", language="en-US")
            assert client._language == "en-US"


class TestSearchMovie:
    def test_search_movie(self, tmdb, mock_http):
        mock_resp = MagicMock()
        mock_resp.status_code = 200
        mock_resp.json.return_value = {
            "results": [
                {"id": 603, "title": "The Matrix", "release_date": "1999-03-31"},
                {"id": 604, "title": "The Matrix Reloaded", "release_date": "2003-05-15"},
            ],
        }
        mock_resp.raise_for_status = MagicMock()
        mock_http.get.return_value = mock_resp

        results = tmdb.search_movie("The Matrix")
        assert len(results) == 2
        assert results[0]["id"] == 603

        call_args = mock_http.get.call_args
        assert call_args.args[0] == "/search/movie"
        params = call_args.kwargs.get("params", {})
        assert params["query"] == "The Matrix"
        assert params["language"] == "zh-CN"

    def test_search_movie_custom_language(self, tmdb, mock_http):
        mock_resp = MagicMock()
        mock_resp.status_code = 200
        mock_resp.json.return_value = {"results": []}
        mock_resp.raise_for_status = MagicMock()
        mock_http.get.return_value = mock_resp

        tmdb.search_movie("Matrix", language="en-US")
        call_args = mock_http.get.call_args
        params = call_args.kwargs.get("params", {})
        assert params["language"] == "en-US"

    def test_search_movie_no_results(self, tmdb, mock_http):
        mock_resp = MagicMock()
        mock_resp.status_code = 200
        mock_resp.json.return_value = {"results": []}
        mock_resp.raise_for_status = MagicMock()
        mock_http.get.return_value = mock_resp

        results = tmdb.search_movie("xyznonexistent12345")
        assert results == []


class TestMovieDetail:
    def test_movie_detail(self, tmdb, mock_http):
        mock_resp = MagicMock()
        mock_resp.status_code = 200
        mock_resp.json.return_value = {
            "id": 603,
            "title": "The Matrix",
            "release_date": "1999-03-31",
            "overview": "A computer hacker learns...",
            "genres": [{"id": 28, "name": "Action"}, {"id": 878, "name": "Sci-Fi"}],
            "runtime": 136,
        }
        mock_resp.raise_for_status = MagicMock()
        mock_http.get.return_value = mock_resp

        detail = tmdb.movie_detail(603)
        assert detail["id"] == 603
        assert detail["title"] == "The Matrix"
        assert len(detail["genres"]) == 2
        assert detail["runtime"] == 136

        call_args = mock_http.get.call_args
        assert call_args.args[0] == "/movie/603"


class TestMovieCredits:
    def test_movie_credits(self, tmdb, mock_http):
        mock_resp = MagicMock()
        mock_resp.status_code = 200
        mock_resp.json.return_value = {
            "id": 603,
            "cast": [
                {"id": 6384, "name": "Keanu Reeves", "character": "Neo"},
                {"id": 2975, "name": "Laurence Fishburne", "character": "Morpheus"},
            ],
            "crew": [
                {"id": 9340, "name": "Lana Wachowski", "job": "Director"},
            ],
        }
        mock_resp.raise_for_status = MagicMock()
        mock_http.get.return_value = mock_resp

        credits = tmdb.movie_credits(603)
        assert len(credits["cast"]) == 2
        assert credits["cast"][0]["name"] == "Keanu Reeves"
        assert len(credits["crew"]) == 1
        assert credits["crew"][0]["job"] == "Director"

        call_args = mock_http.get.call_args
        assert call_args.args[0] == "/movie/603/credits"


class TestMovieImages:
    def test_movie_images(self, tmdb, mock_http):
        mock_resp = MagicMock()
        mock_resp.status_code = 200
        mock_resp.json.return_value = {
            "id": 603,
            "posters": [
                {"file_path": "/poster1.jpg", "width": 500, "height": 750},
                {"file_path": "/poster2.jpg", "width": 500, "height": 750},
            ],
            "backdrops": [
                {"file_path": "/backdrop1.jpg", "width": 1920, "height": 1080},
            ],
        }
        mock_resp.raise_for_status = MagicMock()
        mock_http.get.return_value = mock_resp

        images = tmdb.movie_images(603)
        assert len(images["posters"]) == 2
        assert images["posters"][0]["file_path"] == "/poster1.jpg"
        assert len(images["backdrops"]) == 1

        call_args = mock_http.get.call_args
        assert call_args.args[0] == "/movie/603/images"
        params = call_args.kwargs.get("params", {})
        assert "include_image_language" in params


class TestSearchTV:
    def test_search_tv(self, tmdb, mock_http):
        mock_resp = MagicMock()
        mock_resp.status_code = 200
        mock_resp.json.return_value = {
            "results": [
                {"id": 1396, "name": "Breaking Bad", "first_air_date": "2008-01-20"},
            ],
        }
        mock_resp.raise_for_status = MagicMock()
        mock_http.get.return_value = mock_resp

        results = tmdb.search_tv("Breaking Bad")
        assert len(results) == 1
        assert results[0]["id"] == 1396

        call_args = mock_http.get.call_args
        assert call_args.args[0] == "/search/tv"

    def test_search_tv_no_results(self, tmdb, mock_http):
        mock_resp = MagicMock()
        mock_resp.status_code = 200
        mock_resp.json.return_value = {"results": []}
        mock_resp.raise_for_status = MagicMock()
        mock_http.get.return_value = mock_resp

        results = tmdb.search_tv("zzzznonexistent")
        assert results == []


class TestTVDetail:
    def test_tv_detail(self, tmdb, mock_http):
        mock_resp = MagicMock()
        mock_resp.status_code = 200
        mock_resp.json.return_value = {
            "id": 1396,
            "name": "Breaking Bad",
            "first_air_date": "2008-01-20",
            "seasons": [
                {"season_number": 1, "episode_count": 7},
                {"season_number": 2, "episode_count": 13},
            ],
            "number_of_seasons": 5,
        }
        mock_resp.raise_for_status = MagicMock()
        mock_http.get.return_value = mock_resp

        detail = tmdb.tv_detail(1396)
        assert detail["id"] == 1396
        assert detail["name"] == "Breaking Bad"
        assert len(detail["seasons"]) == 2

        call_args = mock_http.get.call_args
        assert call_args.args[0] == "/tv/1396"


class TestSeasonDetail:
    def test_season_detail(self, tmdb, mock_http):
        mock_resp = MagicMock()
        mock_resp.status_code = 200
        mock_resp.json.return_value = {
            "id": 3572,
            "season_number": 1,
            "episodes": [
                {"episode_number": 1, "name": "Pilot", "air_date": "2008-01-20"},
                {"episode_number": 2, "name": "Cat's in the Bag...", "air_date": "2008-01-27"},
            ],
        }
        mock_resp.raise_for_status = MagicMock()
        mock_http.get.return_value = mock_resp

        detail = tmdb.season_detail(1396, 1)
        assert detail["season_number"] == 1
        assert len(detail["episodes"]) == 2
        assert detail["episodes"][0]["name"] == "Pilot"

        call_args = mock_http.get.call_args
        assert call_args.args[0] == "/tv/1396/season/1"


class TestTVCredits:
    def test_tv_credits(self, tmdb, mock_http):
        mock_resp = MagicMock()
        mock_resp.status_code = 200
        mock_resp.json.return_value = {
            "id": 1396,
            "cast": [
                {"id": 17419, "name": "Bryan Cranston", "character": "Walter White"},
            ],
            "crew": [],
        }
        mock_resp.raise_for_status = MagicMock()
        mock_http.get.return_value = mock_resp

        credits = tmdb.tv_credits(1396)
        assert len(credits["cast"]) == 1
        assert credits["cast"][0]["name"] == "Bryan Cranston"

        call_args = mock_http.get.call_args
        assert call_args.args[0] == "/tv/1396/credits"


class TestImageURL:
    def test_image_url_default_size(self):
        url = TMDBClient.image_url("/poster.jpg")
        assert url == "https://image.tmdb.org/t/p/original/poster.jpg"

    def test_image_url_custom_size(self):
        url = TMDBClient.image_url("/poster.jpg", size="w500")
        assert url == "https://image.tmdb.org/t/p/w500/poster.jpg"


class TestClose:
    def test_close(self):
        with patch("media115.scraper.tmdb.httpx.Client") as MockClient:
            mock_http = MockClient.return_value
            client = TMDBClient(read_access_token="tok")
            client.close()
            mock_http.close.assert_called_once()

    def test_context_manager_calls_close(self):
        with patch("media115.scraper.tmdb.httpx.Client") as MockClient:
            mock_http = MockClient.return_value
            with TMDBClient(read_access_token="tok") as _client:  # noqa: F841
                pass
            mock_http.close.assert_called_once()


class TestRateLimitRetry:
    def test_429_retry(self, tmdb, mock_http):
        """When server returns 429, client should retry after Retry-After."""
        resp_429 = MagicMock()
        resp_429.status_code = 429
        resp_429.headers = {"Retry-After": "0"}

        resp_ok = MagicMock()
        resp_ok.status_code = 200
        resp_ok.json.return_value = {"results": [{"id": 1}]}
        resp_ok.raise_for_status = MagicMock()

        mock_http.get.side_effect = [resp_429, resp_ok]

        results = tmdb.search_movie("test")
        assert len(results) == 1
        assert mock_http.get.call_count == 2
