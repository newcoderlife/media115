"""NFO generation and parsing tests."""

import pytest
from lxml import etree

from media115.scraper.nfo import (
    generate_episode_nfo,
    generate_movie_nfo,
    generate_tvshow_nfo,
    parse_nfo,
)


@pytest.fixture
def movie_metadata():
    return {
        "title": "The Matrix",
        "originaltitle": "The Matrix",
        "year": 1999,
        "plot": "A computer hacker learns about the true nature of reality.",
        "runtime": 136,
        "genres": ["Action", "Science Fiction"],
        "rating": 8.2,
        "premiered": "1999-03-31",
        "studio": "Warner Bros.",
        "directors": ["Lana Wachowski", "Lilly Wachowski"],
        "actors": [
            {"name": "Keanu Reeves", "role": "Neo"},
            {"name": "Laurence Fishburne", "role": "Morpheus"},
        ],
        "uniqueids": {"tmdb": "603", "imdb": "tt0133093"},
    }


@pytest.fixture
def episode_metadata():
    return {
        "title": "Pilot",
        "season": 1,
        "episode": 1,
        "plot": "Walter White turns to crime.",
        "aired": "2008-01-20",
        "rating": 9.0,
        "uniqueids": {"tmdb": "62085"},
    }


class TestGenerateMovieNFO:
    def test_generates_valid_xml(self, movie_metadata, tmp_path):
        nfo_path = tmp_path / "movie.nfo"
        generate_movie_nfo(movie_metadata, nfo_path)
        assert nfo_path.exists()
        # Should be parseable XML
        tree = etree.parse(str(nfo_path))
        assert tree.getroot().tag == "movie"

    def test_contains_required_fields(self, movie_metadata, tmp_path):
        nfo_path = tmp_path / "movie.nfo"
        generate_movie_nfo(movie_metadata, nfo_path)
        tree = etree.parse(str(nfo_path))
        root = tree.getroot()

        assert root.findtext("title") == "The Matrix"
        assert root.findtext("originaltitle") == "The Matrix"
        assert root.findtext("year") == "1999"
        assert root.findtext("plot") is not None
        assert root.findtext("runtime") == "136"
        assert root.findtext("premiered") == "1999-03-31"
        assert root.findtext("studio") == "Warner Bros."
        assert root.findtext("rating") == "8.2"

    def test_contains_genres(self, movie_metadata, tmp_path):
        nfo_path = tmp_path / "movie.nfo"
        generate_movie_nfo(movie_metadata, nfo_path)
        tree = etree.parse(str(nfo_path))
        root = tree.getroot()
        genres = [el.text for el in root.findall("genre")]
        assert "Action" in genres
        assert "Science Fiction" in genres

    def test_contains_actors(self, movie_metadata, tmp_path):
        nfo_path = tmp_path / "movie.nfo"
        generate_movie_nfo(movie_metadata, nfo_path)
        tree = etree.parse(str(nfo_path))
        root = tree.getroot()
        actors = root.findall("actor")
        assert len(actors) == 2
        assert actors[0].findtext("name") == "Keanu Reeves"
        assert actors[0].findtext("role") == "Neo"

    def test_contains_uniqueids(self, movie_metadata, tmp_path):
        nfo_path = tmp_path / "movie.nfo"
        generate_movie_nfo(movie_metadata, nfo_path)
        tree = etree.parse(str(nfo_path))
        root = tree.getroot()
        uids = {el.get("type"): el.text for el in root.findall("uniqueid")}
        assert uids["tmdb"] == "603"
        assert uids["imdb"] == "tt0133093"

    def test_contains_directors(self, movie_metadata, tmp_path):
        nfo_path = tmp_path / "movie.nfo"
        generate_movie_nfo(movie_metadata, nfo_path)
        tree = etree.parse(str(nfo_path))
        root = tree.getroot()
        directors = [el.text for el in root.findall("director")]
        assert "Lana Wachowski" in directors


class TestGenerateEpisodeNFO:
    def test_generates_valid_xml(self, episode_metadata, tmp_path):
        nfo_path = tmp_path / "episode.nfo"
        generate_episode_nfo(episode_metadata, nfo_path)
        assert nfo_path.exists()
        tree = etree.parse(str(nfo_path))
        assert tree.getroot().tag == "episodedetails"

    def test_contains_episode_fields(self, episode_metadata, tmp_path):
        nfo_path = tmp_path / "episode.nfo"
        generate_episode_nfo(episode_metadata, nfo_path)
        tree = etree.parse(str(nfo_path))
        root = tree.getroot()
        assert root.findtext("title") == "Pilot"
        assert root.findtext("season") == "1"
        assert root.findtext("episode") == "1"
        assert root.findtext("aired") == "2008-01-20"


class TestParseNFO:
    def test_parse_movie_nfo(self, movie_metadata, tmp_path):
        nfo_path = tmp_path / "movie.nfo"
        generate_movie_nfo(movie_metadata, nfo_path)
        parsed = parse_nfo(nfo_path)
        assert parsed["title"] == "The Matrix"
        assert parsed["year"] == "1999"
        assert parsed["uniqueids"]["tmdb"] == "603"
        assert parsed["uniqueids"]["imdb"] == "tt0133093"

    def test_parse_episode_nfo(self, episode_metadata, tmp_path):
        nfo_path = tmp_path / "episode.nfo"
        generate_episode_nfo(episode_metadata, nfo_path)
        parsed = parse_nfo(nfo_path)
        assert parsed["title"] == "Pilot"
        assert parsed["season"] == "1"
        assert parsed["episode"] == "1"

    def test_roundtrip(self, movie_metadata, tmp_path):
        """Generate NFO then parse it back — fields should match."""
        nfo_path = tmp_path / "movie.nfo"
        generate_movie_nfo(movie_metadata, nfo_path)
        parsed = parse_nfo(nfo_path)
        assert parsed["title"] == movie_metadata["title"]
        assert parsed["year"] == str(movie_metadata["year"])
        assert parsed["uniqueids"] == movie_metadata["uniqueids"]

    def test_parse_tolerant_of_bad_xml(self, tmp_path):
        """NFO files in the wild are often malformed. Parser should be tolerant."""
        nfo_path = tmp_path / "bad.nfo"
        nfo_path.write_text("<movie><title>Test</title><plot>a & b</plot></movie>")
        parsed = parse_nfo(nfo_path)
        assert parsed["title"] == "Test"


@pytest.fixture
def tvshow_metadata():
    return {
        "title": "Breaking Bad",
        "originaltitle": "Breaking Bad",
        "showtitle": "Breaking Bad",
        "year": 2008,
        "plot": "A chemistry teacher diagnosed with cancer turns to cooking meth.",
        "premiered": "2008-01-20",
        "rating": 9.5,
        "votes": 8000,
        "status": "Ended",
        "genres": ["Drama", "Crime"],
        "studios": ["AMC"],
        "tags": ["crime", "drugs"],
        "actors": [
            {"name": "Bryan Cranston", "role": "Walter White", "thumb": "https://example.com/bc.jpg"},
            {"name": "Aaron Paul", "role": "Jesse Pinkman", "thumb": ""},
        ],
        "uniqueids": {"tmdb": "1396", "imdb": "tt0903747"},
        "thumb": "https://image.tmdb.org/t/p/original/poster.jpg",
        "fanart": "https://image.tmdb.org/t/p/original/fanart.jpg",
    }


class TestGenerateTvshowNFO:
    def test_generate_tvshow_nfo(self, tvshow_metadata, tmp_path):
        """Full metadata produces valid XML with all expected elements."""
        nfo_path = tmp_path / "tvshow.nfo"
        generate_tvshow_nfo(tvshow_metadata, nfo_path)
        assert nfo_path.exists()

        tree = etree.parse(str(nfo_path))
        root = tree.getroot()
        assert root.tag == "tvshow"

        # Core fields
        assert root.findtext("title") == "Breaking Bad"
        assert root.findtext("showtitle") == "Breaking Bad"
        assert root.findtext("originaltitle") == "Breaking Bad"
        assert root.findtext("year") == "2008"
        assert root.findtext("plot") is not None
        assert "chemistry teacher" in root.findtext("plot")
        assert root.findtext("rating") == "9.5"
        assert root.findtext("votes") == "8000"
        assert root.findtext("premiered") == "2008-01-20"
        assert root.findtext("status") == "Ended"

        # Studios
        studios = [el.text for el in root.findall("studio")]
        assert "AMC" in studios

        # Genres
        genres = [el.text for el in root.findall("genre")]
        assert "Drama" in genres
        assert "Crime" in genres

        # Tags
        tags = [el.text for el in root.findall("tag")]
        assert "crime" in tags
        assert "drugs" in tags

        # Actors
        actors = root.findall("actor")
        assert len(actors) == 2
        assert actors[0].findtext("name") == "Bryan Cranston"
        assert actors[0].findtext("role") == "Walter White"
        assert actors[0].findtext("thumb") == "https://example.com/bc.jpg"

        # Unique IDs
        uids = {el.get("type"): el.text for el in root.findall("uniqueid")}
        assert uids["tmdb"] == "1396"
        assert uids["imdb"] == "tt0903747"

        # Thumb / fanart
        thumb_el = root.find("thumb[@aspect='poster']")
        assert thumb_el is not None
        assert "poster.jpg" in thumb_el.text
        fanart_el = root.find("fanart")
        assert fanart_el is not None
        assert fanart_el.findtext("thumb") is not None

    def test_generate_tvshow_nfo_minimal(self, tmp_path):
        """Only title and year -- should not crash, should produce valid XML."""
        minimal = {"title": "Minimal Show", "year": 2025}
        nfo_path = tmp_path / "tvshow.nfo"
        generate_tvshow_nfo(minimal, nfo_path)
        assert nfo_path.exists()

        tree = etree.parse(str(nfo_path))
        root = tree.getroot()
        assert root.tag == "tvshow"
        assert root.findtext("title") == "Minimal Show"
        assert root.findtext("year") == "2025"
        # showtitle falls back to title when not provided
        assert root.findtext("showtitle") == "Minimal Show"

    def test_parse_tvshow_nfo(self, tvshow_metadata, tmp_path):
        """Generate tvshow.nfo then parse it back -- round-trip fidelity."""
        nfo_path = tmp_path / "tvshow.nfo"
        generate_tvshow_nfo(tvshow_metadata, nfo_path)
        parsed = parse_nfo(nfo_path)

        assert parsed["title"] == "Breaking Bad"
        assert parsed["showtitle"] == "Breaking Bad"
        assert parsed["year"] == "2008"
        assert parsed["status"] == "Ended"
        assert parsed["uniqueids"]["tmdb"] == "1396"
        assert parsed["uniqueids"]["imdb"] == "tt0903747"
        assert "Drama" in parsed["genres"]
        assert "Crime" in parsed["genres"]
        assert len(parsed["actors"]) == 2
        assert parsed["actors"][0]["name"] == "Bryan Cranston"
