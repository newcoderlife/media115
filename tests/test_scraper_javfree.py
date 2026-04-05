"""JavFree scraper tests. Uses mocked HTTP, no network."""

from unittest.mock import MagicMock, patch

from media115.scraper import javfree
from media115.scraper.javfree import fetch_metadata

# Realistic detail HTML with title matching the number
# Padded to exceed the 1000-byte minimum content check in fetch_metadata
_PADDING = "<!-- " + "x" * 1200 + " -->"

DETAIL_HTML = f"""\
<html>
<head>
  <meta property="og:image" content="https://javfree.me/wp-content/uploads/abp001_cover.jpg">
</head>
<body>
<article>
  <h1 class="entry-title">[ABP-001] Beautiful Title Here</h1>
  <div class="entry-content">
    <img class="attachment-post-thumbnail" src="https://javfree.me/thumb/abp001.jpg">
    <p>Some description text.</p>
  </div>
</article>
{_PADDING}
</body>
</html>
"""

# Title without the bracket prefix
DETAIL_HTML_NO_BRACKET = f"""\
<html>
<head>
  <meta property="og:image" content="https://javfree.me/cover2.jpg">
</head>
<body>
<article>
  <h1 class="entry-title">ABP-002 Another Title</h1>
</article>
{_PADDING}
</body>
</html>
"""

# Title that does not match the number
DETAIL_HTML_NO_MATCH = """\
<html>
<body>
<article>
  <h1 class="entry-title">Completely Unrelated Title</h1>
</article>
</body>
</html>
"""

# No title at all
EMPTY_HTML = """\
<html><body><div>Nothing here</div></body></html>
"""


class TestFetchMetadata:
    def _make_mock_client(self):
        return MagicMock()

    def test_fetch_metadata_success(self):
        mock_client = self._make_mock_client()

        resp = MagicMock()
        resp.status_code = 200
        resp.content = DETAIL_HTML.encode()

        mock_client.get.return_value = resp

        with patch.object(javfree, "_client", mock_client):
            result = fetch_metadata("ABP-001")

        assert result is not None
        assert result["title"] == "Beautiful Title Here"
        assert result["number"] == "ABP-001"
        assert result["cover_url"] == "https://javfree.me/wp-content/uploads/abp001_cover.jpg"

    def test_fetch_metadata_title_without_bracket_prefix(self):
        mock_client = self._make_mock_client()

        resp = MagicMock()
        resp.status_code = 200
        resp.content = DETAIL_HTML_NO_BRACKET.encode()

        mock_client.get.return_value = resp

        with patch.object(javfree, "_client", mock_client):
            result = fetch_metadata("ABP-002")

        assert result is not None
        assert result["number"] == "ABP-002"
        # Title doesn't start with bracket prefix, so clean_title == title
        assert "ABP-002" in result["title"]
        assert result["cover_url"] == "https://javfree.me/cover2.jpg"

    def test_fetch_metadata_number_not_in_title(self):
        mock_client = self._make_mock_client()

        resp = MagicMock()
        resp.status_code = 200
        resp.content = DETAIL_HTML_NO_MATCH.encode()

        mock_client.get.return_value = resp

        with patch.object(javfree, "_client", mock_client):
            result = fetch_metadata("ABP-003")

        assert result is None

    def test_fetch_metadata_no_title(self):
        mock_client = self._make_mock_client()

        resp = MagicMock()
        resp.status_code = 200
        resp.content = EMPTY_HTML.encode()

        mock_client.get.return_value = resp

        with patch.object(javfree, "_client", mock_client):
            result = fetch_metadata("ABP-004")

        assert result is None

    def test_fetch_metadata_not_found_status(self):
        mock_client = self._make_mock_client()

        resp = MagicMock()
        resp.status_code = 404
        resp.content = b"Not found"

        mock_client.get.return_value = resp

        with patch.object(javfree, "_client", mock_client):
            result = fetch_metadata("ABP-005")

        assert result is None

    def test_fetch_metadata_too_small_response(self):
        mock_client = self._make_mock_client()

        resp = MagicMock()
        resp.status_code = 200
        resp.content = b"<html>short</html>"  # Less than 1000 bytes

        mock_client.get.return_value = resp

        with patch.object(javfree, "_client", mock_client):
            result = fetch_metadata("ABP-006")

        assert result is None

    def test_fetch_metadata_exception_returns_none(self):
        mock_client = self._make_mock_client()
        mock_client.get.side_effect = Exception("connection error")

        with patch.object(javfree, "_client", mock_client):
            result = fetch_metadata("ABP-007")

        assert result is None

    def test_fetch_metadata_url_uses_lowercase(self):
        mock_client = self._make_mock_client()

        resp = MagicMock()
        resp.status_code = 200
        resp.content = DETAIL_HTML.encode()

        mock_client.get.return_value = resp

        with patch.object(javfree, "_client", mock_client):
            fetch_metadata("ABP-001")

        call_url = mock_client.get.call_args[0][0]
        assert "abp-001" in call_url

    def test_fetch_metadata_result_has_all_fields(self):
        mock_client = self._make_mock_client()

        resp = MagicMock()
        resp.status_code = 200
        resp.content = DETAIL_HTML.encode()

        mock_client.get.return_value = resp

        with patch.object(javfree, "_client", mock_client):
            result = fetch_metadata("ABP-001")

        expected_keys = {
            "title",
            "number",
            "cover_url",
            "actors",
            "genres",
            "release_date",
            "runtime",
            "studio",
            "director",
        }
        assert set(result.keys()) == expected_keys
        assert result["actors"] == []
        assert result["genres"] == []
        assert result["release_date"] == ""
        assert result["runtime"] == ""
        assert result["studio"] == ""
        assert result["director"] == ""
