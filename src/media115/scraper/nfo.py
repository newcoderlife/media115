"""NFO file generation (Kodi format) and parsing."""

from pathlib import Path
from lxml import etree


def _add_text(parent: etree._Element, tag: str, text: str | None):
    if text is not None:
        el = etree.SubElement(parent, tag)
        el.text = str(text)


def generate_movie_nfo(metadata: dict, output_path: Path):
    root = etree.Element("movie")

    _add_text(root, "title", metadata.get("title"))
    _add_text(root, "originaltitle", metadata.get("originaltitle"))
    _add_text(root, "year", metadata.get("year"))
    _add_text(root, "plot", metadata.get("plot"))
    _add_text(root, "runtime", metadata.get("runtime"))
    _add_text(root, "rating", metadata.get("rating"))
    _add_text(root, "premiered", metadata.get("premiered"))
    _add_text(root, "studio", metadata.get("studio"))

    for genre in metadata.get("genres", []):
        _add_text(root, "genre", genre)

    for director in metadata.get("directors", []):
        _add_text(root, "director", director)

    for actor_data in metadata.get("actors", []):
        actor_el = etree.SubElement(root, "actor")
        _add_text(actor_el, "name", actor_data.get("name"))
        _add_text(actor_el, "role", actor_data.get("role"))

    for uid_type, uid_value in metadata.get("uniqueids", {}).items():
        uid_el = etree.SubElement(root, "uniqueid", type=uid_type)
        uid_el.text = str(uid_value)

    tree = etree.ElementTree(root)
    tree.write(
        str(output_path),
        encoding="UTF-8",
        xml_declaration=True,
        pretty_print=True,
    )


def generate_episode_nfo(metadata: dict, output_path: Path):
    root = etree.Element("episodedetails")

    _add_text(root, "title", metadata.get("title"))
    _add_text(root, "season", metadata.get("season"))
    _add_text(root, "episode", metadata.get("episode"))
    _add_text(root, "plot", metadata.get("plot"))
    _add_text(root, "aired", metadata.get("aired"))
    _add_text(root, "rating", metadata.get("rating"))

    for uid_type, uid_value in metadata.get("uniqueids", {}).items():
        uid_el = etree.SubElement(root, "uniqueid", type=uid_type)
        uid_el.text = str(uid_value)

    tree = etree.ElementTree(root)
    tree.write(
        str(output_path),
        encoding="UTF-8",
        xml_declaration=True,
        pretty_print=True,
    )


def parse_nfo(nfo_path: Path) -> dict:
    content = nfo_path.read_bytes()
    # Use HTMLParser for tolerance of malformed XML
    parser = etree.HTMLParser(encoding="utf-8")
    doc = etree.fromstring(content, parser)

    # Find the root element (movie, episodedetails, tvshow, etc.)
    # HTMLParser wraps in <html><body>, so look for the actual root
    root = None
    for tag in ("movie", "episodedetails", "tvshow"):
        found = doc.find(f".//{tag}")
        if found is not None:
            root = found
            break
    if root is None:
        root = doc

    result = {}

    for field in (
        "title",
        "originaltitle",
        "year",
        "plot",
        "runtime",
        "rating",
        "premiered",
        "studio",
        "season",
        "episode",
        "aired",
    ):
        val = root.findtext(field)
        if val:
            result[field] = val

    genres = [el.text for el in root.findall("genre") if el.text]
    if genres:
        result["genres"] = genres

    directors = [el.text for el in root.findall("director") if el.text]
    if directors:
        result["directors"] = directors

    actors = []
    for actor_el in root.findall("actor"):
        actor = {}
        name = actor_el.findtext("name")
        if name:
            actor["name"] = name
        role = actor_el.findtext("role")
        if role:
            actor["role"] = role
        if actor:
            actors.append(actor)
    if actors:
        result["actors"] = actors

    uniqueids = {}
    for uid_el in root.findall("uniqueid"):
        uid_type = uid_el.get("type")
        if uid_type and uid_el.text:
            uniqueids[uid_type] = uid_el.text
    if uniqueids:
        result["uniqueids"] = uniqueids

    return result
