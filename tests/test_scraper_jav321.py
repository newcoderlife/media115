"""JAV321 scraper tests. Uses mocked HTTP, no network."""

from unittest.mock import MagicMock, patch

from media115.scraper import jav321
from media115.scraper.jav321 import _parse_detail, fetch_metadata

# Realistic detail HTML for _parse_detail tests
DETAIL_HTML = """\
<html>
<body>
<div class="col-md-9">
  <div class="panel-heading"><h3>ABP-001 Beautiful Title Here</h3></div>
  <div class="col-md-3">
    <img class="img-responsive" src="https://pic.jav321.com/cover/abp001.jpg">
  </div>
  <div class="col-md-9">
    <b>女优</b>: <a href="/star/abc">Actress One</a> <a href="/star/def">Actress Two</a><br>
    <b>メーカー</b>: Studio Name<br>
    <b>レーベル</b>: Label Name<br>
    <b>シリーズ</b>: Series Name<br>
    <b>配信開始日</b>: 2024/01/15<br>
    <b>収録時間</b>: 120分<br>
    <a href="/genre/1">Drama</a> <a href="/genre/2">Romance</a>
  </div>
</div>
</body>
</html>
"""

DETAIL_HTML_MINIMAL = """\
<html>
<body>
<div class="col-md-9">
  <div class="panel-heading"><h3>TEST-002 Minimal</h3></div>
  <div class="col-md-3"><img src="https://pic.jav321.com/minimal.jpg"></div>
  <div class="col-md-9">
    <b>配信開始日</b>: 2023/06/01<br>
  </div>
</div>
</body>
</html>
"""

NO_TITLE_HTML = """\
<html><body><div class="col-md-9"></div></body></html>
"""


class TestParseDetail:
    def test_title(self):
        result = _parse_detail(DETAIL_HTML, "ABP-001")
        assert result["title"] == "ABP-001 Beautiful Title Here"

    def test_number_preserved(self):
        result = _parse_detail(DETAIL_HTML, "ABP-001")
        assert result["number"] == "ABP-001"

    def test_actors(self):
        result = _parse_detail(DETAIL_HTML, "ABP-001")
        assert "Actress One" in result["actors"]
        assert "Actress Two" in result["actors"]
        assert len(result["actors"]) == 2

    def test_genres(self):
        result = _parse_detail(DETAIL_HTML, "ABP-001")
        assert "Drama" in result["genres"]
        assert "Romance" in result["genres"]

    def test_cover_url_img_responsive(self):
        result = _parse_detail(DETAIL_HTML, "ABP-001")
        assert result["cover_url"] == "https://pic.jav321.com/cover/abp001.jpg"

    def test_cover_url_fallback(self):
        result = _parse_detail(DETAIL_HTML_MINIMAL, "TEST-002")
        assert result["cover_url"] == "https://pic.jav321.com/minimal.jpg"

    def test_release_date(self):
        result = _parse_detail(DETAIL_HTML, "ABP-001")
        assert "2024/01/15" in result["release_date"]

    def test_runtime(self):
        result = _parse_detail(DETAIL_HTML, "ABP-001")
        assert result["runtime"] == "120"

    def test_studio(self):
        result = _parse_detail(DETAIL_HTML, "ABP-001")
        assert "Studio Name" in result["studio"]

    def test_label(self):
        result = _parse_detail(DETAIL_HTML, "ABP-001")
        assert "Label Name" in result["label"]

    def test_series(self):
        result = _parse_detail(DETAIL_HTML, "ABP-001")
        assert "Series Name" in result["series"]

    def test_no_title_returns_none(self):
        result = _parse_detail(NO_TITLE_HTML, "XXX-999")
        assert result is None

    def test_minimal_html_empty_fields(self):
        result = _parse_detail(DETAIL_HTML_MINIMAL, "TEST-002")
        assert result["title"] == "TEST-002 Minimal"
        assert result["actors"] == []
        assert result["genres"] == []
        assert result["studio"] == ""
        assert result["label"] == ""
        assert result["series"] == ""


class TestFetchMetadata:
    def _make_mock_client(self):
        return MagicMock()

    def test_fetch_metadata_success(self):
        mock_client = self._make_mock_client()

        # POST /search returns 302 redirect to /video/xxx
        redirect_resp = MagicMock()
        redirect_resp.status_code = 302
        redirect_resp.headers = {"location": "/video/abp001"}

        # GET /video/abp001 returns detail page
        detail_resp = MagicMock()
        detail_resp.status_code = 200
        detail_resp.content = DETAIL_HTML.encode()
        detail_resp.text = DETAIL_HTML

        mock_client.post.return_value = redirect_resp
        mock_client.get.return_value = detail_resp

        with patch.object(jav321, "_client", mock_client):
            result = fetch_metadata("ABP-001")

        assert result is not None
        assert result["title"] == "ABP-001 Beautiful Title Here"
        assert result["number"] == "ABP-001"
        assert len(result["actors"]) == 2

    def test_fetch_metadata_redirect_absolute_url(self):
        mock_client = self._make_mock_client()

        redirect_resp = MagicMock()
        redirect_resp.status_code = 301
        redirect_resp.headers = {"location": "https://www.jav321.com/video/abp001"}

        detail_resp = MagicMock()
        detail_resp.status_code = 200
        detail_resp.content = DETAIL_HTML.encode()
        detail_resp.text = DETAIL_HTML

        mock_client.post.return_value = redirect_resp
        mock_client.get.return_value = detail_resp

        with patch.object(jav321, "_client", mock_client):
            result = fetch_metadata("ABP-001")

        assert result is not None
        # Verify get was called with the absolute URL directly
        mock_client.get.assert_called_once()
        call_url = mock_client.get.call_args[0][0]
        assert call_url.startswith("https://")

    def test_fetch_metadata_not_found_no_redirect(self):
        mock_client = self._make_mock_client()

        resp = MagicMock()
        resp.status_code = 200  # No redirect
        mock_client.post.return_value = resp

        with patch.object(jav321, "_client", mock_client):
            result = fetch_metadata("NONEXIST-999")

        assert result is None

    def test_fetch_metadata_not_found_no_video_in_location(self):
        mock_client = self._make_mock_client()

        resp = MagicMock()
        resp.status_code = 302
        resp.headers = {"location": "/search/results"}  # No /video/ in path
        mock_client.post.return_value = resp

        with patch.object(jav321, "_client", mock_client):
            result = fetch_metadata("NONEXIST-999")

        assert result is None

    def test_fetch_metadata_detail_page_too_small(self):
        mock_client = self._make_mock_client()

        redirect_resp = MagicMock()
        redirect_resp.status_code = 302
        redirect_resp.headers = {"location": "/video/xxx"}

        detail_resp = MagicMock()
        detail_resp.status_code = 200
        detail_resp.content = b"<html></html>"  # Less than 500 bytes

        mock_client.post.return_value = redirect_resp
        mock_client.get.return_value = detail_resp

        with patch.object(jav321, "_client", mock_client):
            result = fetch_metadata("SHORT-001")

        assert result is None

    def test_fetch_metadata_detail_page_error_status(self):
        mock_client = self._make_mock_client()

        redirect_resp = MagicMock()
        redirect_resp.status_code = 302
        redirect_resp.headers = {"location": "/video/xxx"}

        detail_resp = MagicMock()
        detail_resp.status_code = 404
        detail_resp.content = b"x" * 1000

        mock_client.post.return_value = redirect_resp
        mock_client.get.return_value = detail_resp

        with patch.object(jav321, "_client", mock_client):
            result = fetch_metadata("ERR-001")

        assert result is None

    def test_fetch_metadata_exception_returns_none(self):
        mock_client = self._make_mock_client()
        mock_client.post.side_effect = Exception("network error")

        with patch.object(jav321, "_client", mock_client):
            result = fetch_metadata("EXC-001")

        assert result is None
