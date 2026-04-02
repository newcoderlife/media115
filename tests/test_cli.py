"""CLI tests."""

import pytest
from click.testing import CliRunner
from media115.cli import main


@pytest.fixture
def runner():
    return CliRunner()


class TestCLI:
    def test_help(self, runner):
        result = runner.invoke(main, ["--help"])
        assert result.exit_code == 0
        assert "115-media" in result.output

    def test_auth_no_app_id(self, runner):
        result = runner.invoke(main, ["auth"])
        assert "not set" in result.output or "not yet" in result.output

    def test_scrape_tmdb(self, runner):
        result = runner.invoke(main, ["scrape", "The Matrix", "--source", "tmdb"])
        assert result.exit_code == 0
        assert "Matrix" in result.output or "603" in result.output

    def test_scrape_bangumi(self, runner):
        result = runner.invoke(main, ["scrape", "孤独摇滚", "--source", "bangumi"])
        assert result.exit_code == 0

    def test_scrape_default_source(self, runner):
        result = runner.invoke(main, ["scrape", "The Matrix"])
        assert result.exit_code == 0
        assert "Matrix" in result.output
