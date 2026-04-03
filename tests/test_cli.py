"""CLI tests."""

import os

import pytest
from click.testing import CliRunner

from media115.cli import main


@pytest.fixture
def runner():
    return CliRunner()


has_tmdb = bool(os.environ.get("TMDB_READ_ACCESS_TOKEN"))
has_bangumi = bool(os.environ.get("BANGUMI_ACCESS_TOKEN"))


class TestCLI:
    def test_help(self, runner):
        result = runner.invoke(main, ["--help"])
        assert result.exit_code == 0
        assert "115-media" in result.output

    def test_auth_help(self, runner):
        result = runner.invoke(main, ["auth", "--help"])
        assert result.exit_code == 0
        assert "QR" in result.output or "login" in result.output

    @pytest.mark.skipif(not has_tmdb, reason="TMDB_READ_ACCESS_TOKEN not set")
    def test_scrape_tmdb(self, runner):
        result = runner.invoke(main, ["scrape", "The Matrix", "--source", "tmdb"])
        assert result.exit_code == 0
        assert "Matrix" in result.output or "603" in result.output

    @pytest.mark.skipif(not has_bangumi, reason="BANGUMI_ACCESS_TOKEN not set")
    def test_scrape_bangumi(self, runner):
        result = runner.invoke(main, ["scrape", "孤独摇滚", "--source", "bangumi"])
        assert result.exit_code == 0

    @pytest.mark.skipif(not has_tmdb, reason="TMDB_READ_ACCESS_TOKEN not set")
    def test_scrape_default_source(self, runner):
        result = runner.invoke(main, ["scrape", "The Matrix"])
        assert result.exit_code == 0
        assert "Matrix" in result.output
