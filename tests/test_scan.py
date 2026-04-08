"""Scan command tests (all mocked, using new CachedClient API)."""

from unittest.mock import MagicMock, patch

import pytest
from click.testing import CliRunner

from media115.cli import main


@pytest.fixture
def runner():
    return CliRunner()


class TestScanCommand:
    def test_scan_dry_run(self, runner):
        mock_client = MagicMock()
        mock_client.walk.return_value = [
            {
                "path": "/影音",
                "entries": [
                    {
                        "name": "The.Matrix.1999.BluRay.mkv",
                        "type": "file",
                        "fid": "1",
                        "size": 1024,
                        "pick_code": "p1",
                    },
                    {
                        "name": "ABC-123.mp4",
                        "type": "file",
                        "fid": "2",
                        "size": 2048,
                        "pick_code": "p2",
                    },
                    {
                        "name": "The.Matrix.1999.BluRay.nfo",
                        "type": "file",
                        "fid": "3",
                        "size": 512,
                        "pick_code": "p3",
                    },
                ],
            }
        ]

        with patch("media115.cli._get_client", return_value=mock_client):
            result = runner.invoke(main, ["scan", "/影音"])
            assert result.exit_code == 0
            assert "2 video files" in result.output
            assert "Matrix" in result.output
            assert "ABC-123" in result.output

    def test_scan_detects_nfo(self, runner):
        mock_client = MagicMock()
        mock_client.walk.return_value = [
            {
                "path": "/影音",
                "entries": [
                    {
                        "name": "movie.mkv",
                        "type": "file",
                        "fid": "1",
                        "size": 1024,
                        "pick_code": "p1",
                    },
                    {
                        "name": "movie.nfo",
                        "type": "file",
                        "fid": "2",
                        "size": 256,
                        "pick_code": "p2",
                    },
                ],
            }
        ]

        with patch("media115.cli._get_client", return_value=mock_client):
            result = runner.invoke(main, ["scan", "/影音"])
            assert result.exit_code == 0

    def test_scan_no_credentials(self, runner):
        import click
        with patch("media115.cli._get_client", side_effect=click.ClickException("未登录。请先运行: media115 auth")):
            result = runner.invoke(main, ["scan", "/影音"])
            # Should exit with ClickException (code 1)
            assert result.exit_code == 1
