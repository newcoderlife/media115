"""JavFree scraper for AV metadata. Simple, no anti-bot."""

import time

import httpx
from lxml import html as lxml_html

BASE_URL = "https://javfree.me"
_MIN_INTERVAL = 2.0
_last_request: float = 0


def fetch_metadata(number: str) -> dict | None:
    """Fetch AV metadata from JavFree by number."""
    global _last_request
    elapsed = time.monotonic() - _last_request
    if elapsed < _MIN_INTERVAL:
        time.sleep(_MIN_INTERVAL - elapsed)

    headers = {
        "User-Agent": (
            "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
            "AppleWebKit/537.36 (KHTML, like Gecko) "
            "Chrome/130.0.0.0 Safari/537.36"
        ),
    }

    try:
        # Search by number
        resp = httpx.get(
            f"{BASE_URL}/{number.lower()}/",
            headers=headers,
            timeout=15,
            follow_redirects=True,
        )
        _last_request = time.monotonic()

        if resp.status_code != 200 or len(resp.content) < 1000:
            return None

        doc = lxml_html.fromstring(resp.content)

        # Title from h1 or entry-title
        title = ""
        for xpath in [
            '//*[contains(@class,"entry-title")]/text()',
            "//article//h1/text()",
            "//h1/text()",
        ]:
            vals = doc.xpath(xpath)
            if vals:
                title = vals[0].strip()
                break

        if not title or number.upper() not in title.upper():
            return None

        # Clean title: remove [NUMBER] prefix
        clean_title = title
        if title.startswith(f"[{number.upper()}]"):
            clean_title = title[len(number) + 3 :].strip()

        # Cover image
        cover = ""
        for xpath in [
            '//meta[@property="og:image"]/@content',
            "//article//img/@src",
        ]:
            vals = doc.xpath(xpath)
            if vals:
                cover = vals[0]
                break

        return {
            "title": clean_title or title,
            "number": number.upper(),
            "cover_url": cover,
            "actors": [],
            "genres": [],
            "release_date": "",
            "runtime": "",
            "studio": "",
            "director": "",
        }
    except Exception:
        _last_request = time.monotonic()
        return None
