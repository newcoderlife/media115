"""JavFree scraper for AV metadata. Simple, no anti-bot."""

from lxml import html as lxml_html

from media115.scraper.base import ThrottledClient

BASE_URL = "https://javfree.me"
_client = ThrottledClient()


def fetch_metadata(number: str) -> dict | None:
    """Fetch AV metadata from JavFree by number."""
    try:
        resp = _client.get(f"{BASE_URL}/{number.lower()}/", follow_redirects=True)
        if resp.status_code != 200 or len(resp.content) < 1000:
            return None

        doc = lxml_html.fromstring(resp.content)

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

        clean_title = title
        if title.startswith(f"[{number.upper()}]"):
            clean_title = title[len(number) + 3 :].strip()

        cover = ""
        for xpath in ['//meta[@property="og:image"]/@content', "//article//img/@src"]:
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
        return None
