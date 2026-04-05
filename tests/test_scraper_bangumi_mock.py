"""Bangumi API client tests. Mocked HTTP, no network."""

from unittest.mock import MagicMock, patch

import pytest

from media115.scraper.bangumi import BangumiClient


class TestBangumiClientInit:
    def test_init_with_token(self):
        with patch("media115.scraper.bangumi.httpx.Client") as MockClient:
            client = BangumiClient(access_token="test_token_123")
            call_kwargs = MockClient.call_args
            headers = call_kwargs.kwargs.get("headers", {})
            assert headers["Authorization"] == "Bearer test_token_123"
            assert "User-Agent" in headers
            client.close()

    def test_init_without_token(self):
        with patch("media115.scraper.bangumi.httpx.Client") as MockClient:
            client = BangumiClient(access_token=None)
            call_kwargs = MockClient.call_args
            headers = call_kwargs.kwargs.get("headers", {})
            assert "Authorization" not in headers
            assert "User-Agent" in headers
            client.close()

    def test_context_manager(self):
        with patch("media115.scraper.bangumi.httpx.Client"):
            with BangumiClient() as client:
                assert client is not None
            # close was called via __exit__


class TestBangumiSearch:
    @pytest.fixture
    def client(self):
        with patch("media115.scraper.bangumi.httpx.Client") as MockClient:
            mock_http = MockClient.return_value
            c = BangumiClient()
            c._last_request = 0
            yield c, mock_http
            c.close()

    def test_search_returns_results(self, client):
        bgm, mock_http = client
        mock_resp = MagicMock()
        mock_resp.json.return_value = {
            "data": [
                {"id": 328609, "name": "BanG Dream! It's MyGO!!!!!"},
                {"id": 12345, "name": "Another Anime"},
            ],
            "total": 2,
        }
        mock_resp.raise_for_status = MagicMock()
        mock_http.post.return_value = mock_resp

        results = bgm.search("MyGO", subject_type=2)
        assert len(results) == 2
        assert results[0]["id"] == 328609

        # Verify POST body
        call_kwargs = mock_http.post.call_args
        assert call_kwargs.args[0] == "/v0/search/subjects"
        body = call_kwargs.kwargs.get("json", {})
        assert body["keyword"] == "MyGO"
        assert body["filter"]["type"] == [2]

    def test_search_empty_results(self, client):
        bgm, mock_http = client
        mock_resp = MagicMock()
        mock_resp.json.return_value = {"data": [], "total": 0}
        mock_resp.raise_for_status = MagicMock()
        mock_http.post.return_value = mock_resp

        results = bgm.search("nonexistent12345")
        assert results == []

    def test_search_custom_limit(self, client):
        bgm, mock_http = client
        mock_resp = MagicMock()
        mock_resp.json.return_value = {"data": []}
        mock_resp.raise_for_status = MagicMock()
        mock_http.post.return_value = mock_resp

        bgm.search("test", limit=5)
        call_kwargs = mock_http.post.call_args
        params = call_kwargs.kwargs.get("params", {})
        assert params["limit"] == 5


class TestBangumiSubject:
    @pytest.fixture
    def client(self):
        with patch("media115.scraper.bangumi.httpx.Client") as MockClient:
            mock_http = MockClient.return_value
            c = BangumiClient()
            c._last_request = 0
            yield c, mock_http
            c.close()

    def test_subject_detail(self, client):
        bgm, mock_http = client
        mock_resp = MagicMock()
        mock_resp.json.return_value = {
            "id": 328609,
            "name": "BanG Dream! It's MyGO!!!!!",
            "name_cn": "BanG Dream! It's MyGO!!!!!",
            "summary": "A story about a band.",
            "images": {"large": "https://lain.bgm.tv/pic/cover/l/abc.jpg"},
            "date": "2023-06-29",
            "eps": 13,
        }
        mock_resp.raise_for_status = MagicMock()
        mock_http.get.return_value = mock_resp

        result = bgm.subject(328609)
        assert result["id"] == 328609
        assert result["name"] is not None
        assert "images" in result
        assert result["eps"] == 13

        # Verify URL path
        call_args = mock_http.get.call_args
        assert call_args.args[0] == "/v0/subjects/328609"


class TestBangumiSubjectPersons:
    @pytest.fixture
    def client(self):
        with patch("media115.scraper.bangumi.httpx.Client") as MockClient:
            mock_http = MockClient.return_value
            c = BangumiClient()
            c._last_request = 0
            yield c, mock_http
            c.close()

    def test_subject_persons(self, client):
        bgm, mock_http = client
        mock_resp = MagicMock()
        mock_resp.json.return_value = [
            {"id": 1, "name": "Director Name", "relation": "director"},
            {"id": 2, "name": "Producer Name", "relation": "producer"},
        ]
        mock_resp.raise_for_status = MagicMock()
        mock_http.get.return_value = mock_resp

        persons = bgm.subject_persons(328609)
        assert len(persons) == 2
        assert persons[0]["name"] == "Director Name"

        call_args = mock_http.get.call_args
        assert call_args.args[0] == "/v0/subjects/328609/persons"


class TestBangumiEpisodes:
    @pytest.fixture
    def client(self):
        with patch("media115.scraper.bangumi.httpx.Client") as MockClient:
            mock_http = MockClient.return_value
            c = BangumiClient()
            c._last_request = 0
            yield c, mock_http
            c.close()

    def test_episodes(self, client):
        bgm, mock_http = client
        mock_resp = MagicMock()
        mock_resp.json.return_value = {
            "data": [
                {"id": 1001, "sort": 1, "name": "Episode 1", "ep": 1},
                {"id": 1002, "sort": 2, "name": "Episode 2", "ep": 2},
            ],
            "total": 2,
        }
        mock_resp.raise_for_status = MagicMock()
        mock_http.get.return_value = mock_resp

        episodes = bgm.episodes(328609)
        assert len(episodes) == 2
        assert episodes[0]["ep"] == 1

        call_args = mock_http.get.call_args
        assert call_args.args[0] == "/v0/episodes"
        params = call_args.kwargs.get("params", {})
        assert params["subject_id"] == 328609

    def test_episodes_with_type(self, client):
        bgm, mock_http = client
        mock_resp = MagicMock()
        mock_resp.json.return_value = {"data": [{"ep": 1}]}
        mock_resp.raise_for_status = MagicMock()
        mock_http.get.return_value = mock_resp

        bgm.episodes(328609, episode_type=0)

        call_args = mock_http.get.call_args
        params = call_args.kwargs.get("params", {})
        assert params["type"] == 0

    def test_episodes_without_type(self, client):
        bgm, mock_http = client
        mock_resp = MagicMock()
        mock_resp.json.return_value = {"data": []}
        mock_resp.raise_for_status = MagicMock()
        mock_http.get.return_value = mock_resp

        bgm.episodes(328609)

        call_args = mock_http.get.call_args
        params = call_args.kwargs.get("params", {})
        assert "type" not in params

    def test_episodes_with_offset(self, client):
        bgm, mock_http = client
        mock_resp = MagicMock()
        mock_resp.json.return_value = {"data": []}
        mock_resp.raise_for_status = MagicMock()
        mock_http.get.return_value = mock_resp

        bgm.episodes(328609, limit=50, offset=10)

        call_args = mock_http.get.call_args
        params = call_args.kwargs.get("params", {})
        assert params["limit"] == 50
        assert params["offset"] == 10


class TestBangumiAnonymousMode:
    def test_works_without_token(self):
        """Verify the client can be used without an access token."""
        with patch("media115.scraper.bangumi.httpx.Client") as MockClient:
            mock_http = MockClient.return_value
            mock_resp = MagicMock()
            mock_resp.json.return_value = {"data": [{"id": 1, "name": "Test"}]}
            mock_resp.raise_for_status = MagicMock()
            mock_http.post.return_value = mock_resp

            client = BangumiClient()  # No token
            client._last_request = 0
            results = client.search("test")
            assert len(results) == 1

            # Verify no Authorization header was set
            init_kwargs = MockClient.call_args
            headers = init_kwargs.kwargs.get("headers", {})
            assert "Authorization" not in headers
            client.close()
