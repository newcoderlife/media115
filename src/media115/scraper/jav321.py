"""JAV321 scraper for AV metadata. No Cloudflare, POST-based search."""

import time

import httpx
from lxml import html as lxml_html

BASE_URL = "https://www.jav321.com"
_MIN_INTERVAL = 2.0
_last_request: float = 0


def fetch_metadata(number: str) -> dict | None:
    """Fetch AV metadata from JAV321 by number. Returns None if not found."""
    global _last_request
    elapsed = time.monotonic() - _last_request
    if elapsed < _MIN_INTERVAL:
        time.sleep(_MIN_INTERVAL - elapsed)

    headers = {
        "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
    }

    try:
        # POST search → 301 redirect to /video/{id}
        resp = httpx.post(
            f"{BASE_URL}/search",
            data={"sn": number},
            headers=headers,
            timeout=15,
            follow_redirects=False,
        )
        _last_request = time.monotonic()

        if resp.status_code not in (301, 302):
            return None

        location = resp.headers.get("location", "")
        if "/video/" not in location:
            return None

        url = location if location.startswith("http") else f"{BASE_URL}{location}"

        # Throttle before detail request
        time.sleep(_MIN_INTERVAL)

        detail_resp = httpx.get(url, headers=headers, timeout=15, follow_redirects=True)
        _last_request = time.monotonic()

        if detail_resp.status_code != 200 or len(detail_resp.content) < 500:
            return None

        return _parse_detail(detail_resp.text, number)
    except Exception:
        _last_request = time.monotonic()
        return None


def _parse_detail(html_str: str, original_number: str) -> dict | None:
    doc = lxml_html.fromstring(html_str)

    result = {}

    # Title
    titles = doc.xpath("//h3/text()")
    result["title"] = titles[0].strip() if titles else ""

    # Number (from page content or use original)
    result["number"] = original_number

    # Info panel - parse key-value pairs from the info div
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

    # Actors
    actor_els = doc.xpath(
        '//div[@class="col-md-9"]//a[contains(@href, "/star/")]/text()'
    )
    result["actors"] = [a.strip() for a in actor_els if a.strip()]

    # Genres
    genre_els = doc.xpath(
        '//div[@class="col-md-9"]//a[contains(@href, "/genre/")]/text()'
    )
    result["genres"] = [g.strip() for g in genre_els if g.strip()]

    # Cover image
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
                line = parts[1].strip().split("\n")[0].strip()
                # Remove leading colon/space
                line = line.lstrip(":：").strip()
                return line
    return ""


def _extract_digits(s: str) -> str:
    return "".join(c for c in s if c.isdigit()) if s else ""
