"""JAV321 scraper for AV metadata. No Cloudflare, POST-based search."""

from lxml import html as lxml_html

from media115.scraper.base import ThrottledClient

BASE_URL = "https://www.jav321.com"
_client = ThrottledClient()


def fetch_metadata(number: str) -> dict | None:
    """Fetch AV metadata from JAV321 by number."""
    try:
        resp = _client.post(
            f"{BASE_URL}/search",
            data={"sn": number},
            follow_redirects=False,
        )
        if resp.status_code not in (301, 302):
            return None

        location = resp.headers.get("location", "")
        if "/video/" not in location:
            return None

        url = location if location.startswith("http") else f"{BASE_URL}{location}"
        detail_resp = _client.get(url, follow_redirects=True)
        if detail_resp.status_code != 200 or len(detail_resp.content) < 500:
            return None

        return _parse_detail(detail_resp.text, number)
    except Exception:
        return None


def _parse_detail(html_str: str, original_number: str) -> dict | None:
    doc = lxml_html.fromstring(html_str)
    result = {}

    titles = doc.xpath("//h3/text()")
    result["title"] = titles[0].strip() if titles else ""
    result["number"] = original_number

    info_div = doc.xpath('//div[@class="col-md-9"]')
    if info_div:
        info_text = info_div[0].text_content()
        result["release_date"] = _extract_field(
            info_text, "配信開始日", "發行日期", "发行日期"
        )
        result["runtime"] = _extract_digits(
            _extract_field(info_text, "収録時間", "長度")
        )
        result["studio"] = _extract_field(info_text, "メーカー", "製作商")
        result["label"] = _extract_field(info_text, "レーベル", "發行商")
        result["series"] = _extract_field(info_text, "シリーズ", "系列")

    result["actors"] = [
        a.strip()
        for a in doc.xpath(
            '//div[@class="col-md-9"]//a[contains(@href, "/star/")]/text()'
        )
        if a.strip()
    ]
    result["genres"] = [
        g.strip()
        for g in doc.xpath(
            '//div[@class="col-md-9"]//a[contains(@href, "/genre/")]/text()'
        )
        if g.strip()
    ]

    cover_els = doc.xpath('//img[contains(@class, "img-responsive")]/@src')
    if not cover_els:
        cover_els = doc.xpath('//div[@class="col-md-3"]//img/@src')
    result["cover_url"] = cover_els[0] if cover_els else ""

    return result if result.get("title") else None


def _extract_field(text: str, *labels: str) -> str:
    for label in labels:
        if label in text:
            parts = text.split(label)
            if len(parts) > 1:
                return parts[1].strip().split("\n")[0].lstrip(":：").strip()
    return ""


def _extract_digits(s: str) -> str:
    return "".join(c for c in s if c.isdigit()) if s else ""
