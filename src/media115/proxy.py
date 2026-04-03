"""strm-proxy: Jellyfin reverse proxy with STRM 302 redirect."""

import re
import time
from contextlib import asynccontextmanager

import httpx
from fastapi import FastAPI, Request
from fastapi.responses import RedirectResponse, Response

CACHE_TTL = 300  # 5 minutes
EXCLUDED_HEADERS = {"host", "transfer-encoding", "content-encoding", "content-length"}


def create_app(jellyfin_url: str, cloud115_client) -> FastAPI:
    @asynccontextmanager
    async def lifespan(app: FastAPI):
        app.state.http = httpx.AsyncClient(timeout=30)
        yield
        await app.state.http.aclose()

    app = FastAPI(title="115 strm-proxy", lifespan=lifespan)
    app.state.jellyfin_url = jellyfin_url.rstrip("/")
    app.state.cloud115 = cloud115_client
    app.state.url_cache: dict[str, tuple[str, float]] = {}

    @app.get("/play/{pick_code}")
    async def play_redirect(pick_code: str):
        url = _get_cached_url(app, pick_code)
        return RedirectResponse(url=url, status_code=302)

    @app.api_route("/Videos/{item_id}/{action}", methods=["GET", "HEAD"])
    async def video_stream(request: Request, item_id: str, action: str):
        action_base = action.lower().split(".")[0]
        if action_base not in ("stream", "original"):
            return await _proxy_to_jellyfin(app, request)

        media_source_id = request.query_params.get("mediasourceid", "")
        api_key = request.query_params.get("api_key", "")

        try:
            item_resp = await app.state.http.get(
                f"{app.state.jellyfin_url}/Items",
                params={
                    "Ids": media_source_id or item_id,
                    "Fields": "Path,MediaSources",
                    "api_key": api_key,
                },
            )
            item_resp.raise_for_status()
            items = item_resp.json().get("Items", [])
        except Exception:
            return await _proxy_to_jellyfin(app, request)

        if not items:
            return await _proxy_to_jellyfin(app, request)

        item = items[0]
        path = item.get("Path", "")

        if not path.lower().endswith(".strm"):
            return await _proxy_to_jellyfin(app, request)

        for ms in item.get("MediaSources", []):
            ms_path = ms.get("Path", "")
            pick_code = _extract_pick_code(ms_path)
            if pick_code:
                url = _get_cached_url(app, pick_code)
                return RedirectResponse(url=url, status_code=302)

        return await _proxy_to_jellyfin(app, request)

    @app.api_route("/{path:path}", methods=["GET", "POST", "PUT", "DELETE", "HEAD"])
    async def catchall(request: Request, path: str):
        return await _proxy_to_jellyfin(app, request)

    return app


def _get_cached_url(app: FastAPI, pick_code: str) -> str:
    cache = app.state.url_cache
    now = time.time()
    if pick_code in cache:
        url, ts = cache[pick_code]
        if now - ts < CACHE_TTL:
            return url

    url = app.state.cloud115.download_url(pick_code)
    cache[pick_code] = (url, now)
    return url


def _extract_pick_code(strm_url: str) -> str | None:
    match = re.search(r"/play/(\w+)", strm_url)
    return match.group(1) if match else None


async def _proxy_to_jellyfin(app: FastAPI, request: Request) -> Response:
    target = f"{app.state.jellyfin_url}{request.url.path}"
    if request.url.query:
        target += f"?{request.url.query}"

    fwd_headers = {
        k: v for k, v in request.headers.items() if k.lower() not in EXCLUDED_HEADERS
    }
    body = await request.body()

    resp = await app.state.http.request(
        method=request.method,
        url=target,
        headers=fwd_headers,
        content=body,
    )

    resp_headers = {
        k: v for k, v in resp.headers.items() if k.lower() not in EXCLUDED_HEADERS
    }
    return Response(
        content=resp.content, status_code=resp.status_code, headers=resp_headers
    )
