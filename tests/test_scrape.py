"""Scrape orchestration tests (mock all external APIs)."""

from unittest.mock import MagicMock, patch

import pytest

from media115 import cache
from media115.scraper.scrape import _write_av_nfo, scrape_av, scrape_movie, scrape_tv


@pytest.fixture(autouse=True)
def _isolate_cache(tmp_path, monkeypatch):
    monkeypatch.setattr(cache, "CACHE_DIR", str(tmp_path / ".cache"))


class TestScrapeMovie:
    def test_success(self, tmp_path):
        mock_client = MagicMock()
        mock_client.search_movie.return_value = [{"id": 603, "release_date": "1999-03-31"}]
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
            "cast": [{"name": "Keanu Reeves", "character": "Neo", "profile_path": "/k.jpg"}],
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


def _make_tv_client():
    """Create a mock TMDBClient pre-configured for TV scraping."""
    client = MagicMock()
    client.search_tv.return_value = [
        {"id": 456, "name": "Test Show", "first_air_date": "2024-01-01"}
    ]
    client.tv_detail.return_value = {
        "name": "Test Show",
        "original_name": "Test Show Original",
        "overview": "Series plot summary",
        "first_air_date": "2024-01-01",
        "vote_average": 8.5,
        "vote_count": 1200,
        "status": "Returning Series",
        "genres": [{"name": "Drama"}, {"name": "Sci-Fi"}],
        "production_companies": [{"name": "Studio A"}],
        "keywords": {"results": [{"name": "time travel"}]},
        "poster_path": "/tv_poster.jpg",
        "backdrop_path": "/tv_backdrop.jpg",
        "id": 456,
    }
    client.tv_credits.return_value = {
        "crew": [{"name": "Jane Director", "job": "Director"}],
        "cast": [
            {"name": "Actor One", "character": "Hero", "profile_path": "/a1.jpg"},
            {"name": "Actor Two", "character": "Villain", "profile_path": None},
        ],
    }
    client.season_detail.return_value = {
        "episodes": [
            {
                "episode_number": 1,
                "name": "Pilot Episode",
                "overview": "episode specific plot",
                "air_date": "2024-01-15",
            },
            {
                "episode_number": 2,
                "name": "Second Episode",
                "overview": "ep2 plot",
                "air_date": "2024-01-22",
            },
        ]
    }
    client.tv_images.return_value = {"posters": []}
    client.close = MagicMock()
    return client


class TestScrapeTV:
    def test_scrape_tv_basic(self, tmp_path):
        """scrape_tv returns ok, writes episode NFO and file_map with type='tv'."""
        mock_client = _make_tv_client()
        out = tmp_path / "out"

        with patch("media115.scraper.tmdb.TMDBClient", return_value=mock_client):
            with patch.dict("os.environ", {"TMDB_READ_ACCESS_TOKEN": "test"}):
                result = scrape_tv("Test Show", 1, 1, "show.s01e01.mkv", out)

        assert result["status"] == "ok"
        assert result["tmdb_id"] == 456
        assert (out / "show.s01e01.nfo").exists()

        # Verify file_map was written with type=tv
        fm = cache.get("file_map", "show.s01e01")
        assert fm is not None
        assert fm["type"] == "tv"
        assert fm["tmdb_id"] == 456

    def test_scrape_tv_episode_metadata(self, tmp_path):
        """Episode-level plot/aired from season_detail overrides series-level data."""
        mock_client = _make_tv_client()
        out = tmp_path / "out"

        with patch("media115.scraper.tmdb.TMDBClient", return_value=mock_client):
            with patch.dict("os.environ", {"TMDB_READ_ACCESS_TOKEN": "test"}):
                scrape_tv("Test Show", 1, 1, "show.s01e01.mkv", out)

        from lxml import etree

        tree = etree.parse(str(out / "show.s01e01.nfo"))
        root = tree.getroot()
        # Episode-specific data, not series-level
        assert root.findtext("plot") == "episode specific plot"
        assert root.findtext("aired") == "2024-01-15"
        assert root.findtext("title") == "Pilot Episode"

    def test_scrape_tv_generates_tvshow_nfo(self, tmp_path):
        """tvshow.nfo is created alongside episode NFO."""
        mock_client = _make_tv_client()
        out = tmp_path / "out"

        with patch("media115.scraper.tmdb.TMDBClient", return_value=mock_client):
            with patch.dict("os.environ", {"TMDB_READ_ACCESS_TOKEN": "test"}):
                scrape_tv("Test Show", 1, 1, "show.s01e01.mkv", out)

        tvshow_nfo = out / "tvshow.nfo"
        assert tvshow_nfo.exists()

        from lxml import etree

        tree = etree.parse(str(tvshow_nfo))
        root = tree.getroot()
        assert root.tag == "tvshow"
        assert root.findtext("title") == "Test Show"
        assert root.findtext("year") == "2024"
        assert root.findtext("plot") == "Series plot summary"

    def test_scrape_tv_tvshow_nfo_only_once(self, tmp_path):
        """Calling scrape_tv twice for the same show does not overwrite tvshow.nfo."""
        mock_client = _make_tv_client()
        out = tmp_path / "out"
        out.mkdir(parents=True, exist_ok=True)

        with patch("media115.scraper.tmdb.TMDBClient", return_value=mock_client):
            with patch.dict("os.environ", {"TMDB_READ_ACCESS_TOKEN": "test"}):
                scrape_tv("Test Show", 1, 1, "show.s01e01.mkv", out)

        # Record the modification time of tvshow.nfo after first call
        tvshow_nfo = out / "tvshow.nfo"
        assert tvshow_nfo.exists()
        first_content = tvshow_nfo.read_bytes()

        # Alter detail so we can detect if it gets overwritten
        mock_client2 = _make_tv_client()
        mock_client2.tv_detail.return_value["name"] = "Changed Name"

        with patch("media115.scraper.tmdb.TMDBClient", return_value=mock_client2):
            with patch.dict("os.environ", {"TMDB_READ_ACCESS_TOKEN": "test"}):
                scrape_tv("Test Show", 1, 2, "show.s01e02.mkv", out)

        # tvshow.nfo should still have the original content
        assert tvshow_nfo.read_bytes() == first_content

    def test_scrape_tv_not_found(self, tmp_path):
        """search_tv returning [] yields not_found status."""
        mock_client = MagicMock()
        mock_client.search_tv.return_value = []
        mock_client.close = MagicMock()

        out = tmp_path / "out"
        with patch("media115.scraper.tmdb.TMDBClient", return_value=mock_client):
            with patch.dict("os.environ", {"TMDB_READ_ACCESS_TOKEN": "test"}):
                result = scrape_tv("Nonexistent", 1, 1, "nope.mkv", out)

        assert result["status"] == "not_found"

    def test_scrape_tv_no_token(self, tmp_path):
        """Missing TMDB token returns error."""
        out = tmp_path / "out"
        with patch.dict("os.environ", {}, clear=True):
            result = scrape_tv("Test", 1, 1, "x.mkv", out)
        assert result["status"] == "error"


class TestScrapeAVDetailed:
    def test_scrape_av_jav321_hit(self, tmp_path):
        """jav321 returning metadata produces NFO + file_map entry."""
        meta = {
            "title": "JAV321 Title",
            "number": "JAV-001",
            "actors": ["Actress A"],
            "genres": ["Drama"],
            "release_date": "2024-05-10",
            "runtime": "120",
            "studio": "StudioX",
            "director": "Director Y",
            "cover_url": "",
        }

        out = tmp_path / "out"
        with patch("media115.scraper.jav321.fetch_metadata", return_value=meta):
            result = scrape_av("JAV-001", "JAV-001.mp4", out)

        assert result["status"] == "ok"
        assert result["match"] == "JAV321 Title"
        assert (out / "JAV-001.nfo").exists()

        fm = cache.get("file_map", "JAV-001")
        assert fm["type"] == "av"
        assert fm["number"] == "JAV-001"

    def test_scrape_av_jav321_miss_javfree_hit(self, tmp_path):
        """jav321 returning None falls through to javfree."""
        meta = {
            "title": "JavFree Title",
            "number": "JF-002",
            "actors": [],
            "genres": ["Action"],
            "release_date": "2023-12-01",
            "runtime": "90",
            "studio": "StudioZ",
            "director": "",
            "cover_url": "",
        }

        out = tmp_path / "out"
        with patch("media115.scraper.jav321.fetch_metadata", return_value=None):
            with patch("media115.scraper.javfree.fetch_metadata", return_value=meta):
                result = scrape_av("JF-002", "JF-002.mkv", out)

        assert result["status"] == "ok"
        assert result["match"] == "JavFree Title"
        assert (out / "JF-002.nfo").exists()

        cached = cache.get("av", "JF-002")
        assert cached["_source"] == "javfree"

        fm = cache.get("file_map", "JF-002")
        assert fm["type"] == "av"
        assert fm["title"] == "JavFree Title"


class TestWriteAvNfo:
    def test_write_av_nfo(self, tmp_path):
        """_write_av_nfo generates correct NFO content from sample metadata."""
        from lxml import etree

        meta = {
            "title": "Test AV Title",
            "number": "TEST-999",
            "actors": ["Star A", "Star B"],
            "genres": ["Genre1", "Genre2"],
            "release_date": "2024-03-15",
            "runtime": "95",
            "studio": "TestStudio",
            "director": "Dir Name",
            "cover_url": "",
        }

        out = tmp_path / "out"
        out.mkdir(parents=True, exist_ok=True)
        _write_av_nfo(meta, "TEST-999.mp4", out, "jav321")

        nfo_path = out / "TEST-999.nfo"
        assert nfo_path.exists()

        tree = etree.parse(str(nfo_path))
        root = tree.getroot()
        assert root.tag == "movie"
        assert root.findtext("title") == "Test AV Title"
        assert root.findtext("year") == "2024"
        assert root.findtext("runtime") == "95"
        assert root.findtext("studio") == "TestStudio"
        assert root.findtext("director") == "Dir Name"

        # Verify genres
        genres = [el.text for el in root.findall("genre")]
        assert "Genre1" in genres
        assert "Genre2" in genres

        # Verify actors
        actors = root.findall("actor")
        actor_names = [a.findtext("name") for a in actors]
        assert "Star A" in actor_names
        assert "Star B" in actor_names

        # Verify uniqueid with source
        uids = {el.get("type"): el.text for el in root.findall("uniqueid")}
        assert uids["jav321"] == "TEST-999"

    def test_write_av_nfo_minimal(self, tmp_path):
        """_write_av_nfo works with minimal metadata (no release_date, etc.)."""
        meta = {
            "title": "Minimal",
            "number": "MIN-001",
            "actors": [],
            "genres": [],
        }

        out = tmp_path / "out"
        out.mkdir(parents=True, exist_ok=True)
        _write_av_nfo(meta, "MIN-001.mp4", out, "jav321")

        nfo_path = out / "MIN-001.nfo"
        assert nfo_path.exists()
