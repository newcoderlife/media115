"""JavBus scraper tests. Uses HTML fixtures, no network."""

from pathlib import Path

import pytest

from media115.scraper.javbus import parse_detail_page

FIXTURES = Path(__file__).parent / "fixtures"


class TestParseDetailPage:
    @pytest.fixture
    def detail_html(self):
        return (FIXTURES / "javbus_detail.html").read_text()

    def test_parse_number(self, detail_html):
        result = parse_detail_page(detail_html)
        assert result["number"] == "TEST-001"

    def test_parse_title(self, detail_html):
        result = parse_detail_page(detail_html)
        assert "テスト" in result["title"]

    def test_parse_release_date(self, detail_html):
        result = parse_detail_page(detail_html)
        assert result["release_date"] == "2024-01-15"

    def test_parse_runtime(self, detail_html):
        result = parse_detail_page(detail_html)
        assert result["runtime"] == "120"

    def test_parse_director(self, detail_html):
        result = parse_detail_page(detail_html)
        assert result["director"] == "テスト監督"

    def test_parse_studio(self, detail_html):
        result = parse_detail_page(detail_html)
        assert result["studio"] == "テストスタジオ"

    def test_parse_label(self, detail_html):
        result = parse_detail_page(detail_html)
        assert result["label"] == "テストレーベル"

    def test_parse_series(self, detail_html):
        result = parse_detail_page(detail_html)
        assert result["series"] == "テストシリーズ"

    def test_parse_actors(self, detail_html):
        result = parse_detail_page(detail_html)
        assert len(result["actors"]) == 2
        assert "テスト女優A" in result["actors"]
        assert "テスト女優B" in result["actors"]

    def test_parse_genres(self, detail_html):
        result = parse_detail_page(detail_html)
        assert "ドラマ" in result["genres"]
        assert "単体作品" in result["genres"]

    def test_parse_cover(self, detail_html):
        result = parse_detail_page(detail_html)
        assert "test001_b.jpg" in result["cover_url"]

    def test_parse_screenshots(self, detail_html):
        result = parse_detail_page(detail_html)
        assert len(result["screenshots"]) == 2
