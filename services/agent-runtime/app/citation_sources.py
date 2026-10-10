"""Collect clickable citation URLs from retrieval tool calls.

Used when the bot fetched web/KB/MCP/memory content that exposes real http(s)
URLs. Deduplicated and ordered by first appearance. Never invents URLs.
"""

from __future__ import annotations

import json
import re
from typing import Any
from urllib.parse import urlparse, urlunparse

# Absolute http(s) URLs in free text / JSON string values.
_URL_RE = re.compile(r"https?://[^\s<>\"'\\)\]]+", re.IGNORECASE)

# Tool names (exact or substring) that typically retrieve external pages.
_RETRIEVAL_EXACT = frozenset(
    {
        "http_fetch",
        "secret_http",
        "web_search",
        "web_fetch",
        "browse",
        "browse_page",
        "fetch_url",
        "search_web",
    }
)
_RETRIEVAL_SUBSTR = (
    "search",
    "browse",
    "fetch",
    "http",
    "web_",
    "arxiv",
    "news",
    "crawl",
    "scrape",
    "xurl",
)


def is_http_url(value: str) -> bool:
    """Public http(s) URL suitable for a user-facing citation link."""
    s = (value or "").strip()
    if not s or len(s) > 2048:
        return False
    try:
        p = urlparse(s)
    except Exception:  # noqa: BLE001
        return False
    if p.scheme not in ("http", "https"):
        return False
    host = (p.hostname or "").lower().strip()
    if not host:
        return False
    # Never cite loopback / link-local / cloud metadata.
    if host in ("localhost", "127.0.0.1", "::1", "0.0.0.0"):
        return False
    if host.endswith(".localhost") or host.endswith(".local"):
        return False
    if host.startswith("169.254.") or host == "metadata.google.internal":
        return False
    # Require a dot in the hostname (example.com) — bare words are not URLs we want.
    if "." not in host:
        return False
    return True


def normalize_url(url: str) -> str:
    """Dedupe key: lowercase scheme/host, drop fragment, trim trailing slash on path."""
    raw = (url or "").strip()
    try:
        p = urlparse(raw)
    except Exception:  # noqa: BLE001
        return raw
    scheme = (p.scheme or "https").lower()
    netloc = (p.netloc or "").lower()
    path = p.path or ""
    if path != "/" and path.endswith("/"):
        path = path.rstrip("/")
    # Drop default ports
    if netloc.endswith(":80") and scheme == "http":
        netloc = netloc[:-3]
    if netloc.endswith(":443") and scheme == "https":
        netloc = netloc[:-4]
    return urlunparse((scheme, netloc, path, "", p.query, ""))


def _clean_url(raw: str) -> str | None:
    s = (raw or "").strip().rstrip(".,;:)]}>\"'")
    if not is_http_url(s):
        return None
    return s


def _title_from_obj(obj: dict[str, Any]) -> str:
    for key in ("title", "name", "label", "headline", "source_title"):
        v = obj.get(key)
        if isinstance(v, str) and v.strip():
            return v.strip()[:200]
    return ""


def _urls_from_obj(obj: Any, *, depth: int = 0, out: list[tuple[str, str]] | None = None) -> list[tuple[str, str]]:
    """Walk JSON-like structures for url/link/href fields and string URL values."""
    if out is None:
        out = []
    if depth > 6:
        return out
    if isinstance(obj, dict):
        title = _title_from_obj(obj)
        for key, val in obj.items():
            lk = str(key).lower()
            if lk in ("url", "link", "href", "source_url", "canonical_url", "page_url", "uri"):
                if isinstance(val, str):
                    u = _clean_url(val)
                    if u:
                        out.append((u, title))
                elif isinstance(val, list):
                    for item in val:
                        if isinstance(item, str):
                            u = _clean_url(item)
                            if u:
                                out.append((u, title))
            elif isinstance(val, (dict, list)):
                _urls_from_obj(val, depth=depth + 1, out=out)
            elif isinstance(val, str) and ("http://" in val or "https://" in val):
                for m in _URL_RE.findall(val):
                    u = _clean_url(m)
                    if u:
                        out.append((u, title if lk in ("content", "snippet", "body", "text", "summary") else ""))
    elif isinstance(obj, list):
        for item in obj:
            _urls_from_obj(item, depth=depth + 1, out=out)
    elif isinstance(obj, str):
        for m in _URL_RE.findall(obj):
            u = _clean_url(m)
            if u:
                out.append((u, ""))
    return out


def _is_retrieval_tool(name: str) -> bool:
    n = (name or "").strip().lower()
    if not n:
        return False
    if n in _RETRIEVAL_EXACT:
        return True
    if n.startswith("mcp__"):
        # MCP search / browse / fetch connectors often return URLs.
        leaf = n.rsplit("__", 1)[-1]
        if any(s in leaf for s in _RETRIEVAL_SUBSTR):
            return True
        # Also accept any mcp tool whose leaf looks like a search/read helper.
        if any(s in leaf for s in ("duck", "google", "bing", "brave", "wiki", "doc")):
            return True
        return False
    if n.startswith("memory_") or n in ("memory_recall", "memory_write", "memory_search"):
        return True
    return any(s in n for s in _RETRIEVAL_SUBSTR)


def extract_from_tool(
    name: str,
    args: dict[str, Any] | None,
    result: str | dict[str, Any] | None,
) -> list[dict[str, str]]:
    """Return [{url, title?}] from one tool invocation when it looks like retrieval."""
    if not _is_retrieval_tool(name):
        return []

    found: list[tuple[str, str]] = []
    args = args if isinstance(args, dict) else {}

    # Prefer explicit arg URLs (fetch/browse).
    for key in ("url", "link", "href", "source_url", "page_url"):
        v = args.get(key)
        if isinstance(v, str):
            u = _clean_url(v)
            if u:
                found.append((u, str(args.get("title") or "").strip()[:200]))

    # Query-only search tools rarely put the destination URL in args; rely on result.
    parsed_result: Any = result
    if isinstance(result, str):
        text = result.strip()
        if text.startswith("{") or text.startswith("["):
            try:
                parsed_result = json.loads(text)
            except json.JSONDecodeError:
                parsed_result = text
        else:
            parsed_result = text

    if parsed_result is not None:
        # Successful http_fetch prefers final url from body.
        if isinstance(parsed_result, dict) and parsed_result.get("error"):
            # Still allow URL from error payloads only if it's the requested page
            # (user may want to open it); skip if clearly a blocked SSRF/etc.
            err = str(parsed_result.get("error") or "").lower()
            if "blocked" in err or "not allowed" in err or "ssrf" in err:
                return merge_sources([], [{"url": u, "title": t} for u, t in found if u])
        found.extend(_urls_from_obj(parsed_result))

    return merge_sources([], [{"url": u, "title": t} for u, t in found if u])


def merge_sources(
    existing: list[dict[str, str]] | None,
    new: list[dict[str, str]] | None,
    *,
    limit: int = 20,
) -> list[dict[str, str]]:
    """Deduplicate by normalized URL; keep first title; cap length."""
    out: list[dict[str, str]] = []
    seen: set[str] = set()
    for src in list(existing or []) + list(new or []):
        if not isinstance(src, dict):
            continue
        url = _clean_url(str(src.get("url") or ""))
        if not url:
            continue
        key = normalize_url(url)
        if key in seen:
            # Fill missing title on earlier entry.
            if not out:
                continue
            for prev in out:
                if normalize_url(prev["url"]) == key and not prev.get("title"):
                    title = str(src.get("title") or "").strip()[:200]
                    if title:
                        prev["title"] = title
                    break
            continue
        seen.add(key)
        item: dict[str, str] = {"url": url}
        title = str(src.get("title") or "").strip()[:200]
        if title:
            item["title"] = title
        out.append(item)
        if len(out) >= limit:
            break
    return out


def sources_from_messages(messages: list[dict[str, Any]] | None) -> list[dict[str, str]]:
    """Scan a chat/tool transcript for retrieval tool results (legacy loop fallback)."""
    acc: list[dict[str, str]] = []
    if not messages:
        return acc
    # Pair assistant tool_calls args with subsequent tool results when possible.
    pending_args: dict[str, tuple[str, dict[str, Any]]] = {}
    for msg in messages:
        if not isinstance(msg, dict):
            continue
        role = str(msg.get("role") or "")
        if role == "assistant":
            for tc in msg.get("tool_calls") or []:
                if not isinstance(tc, dict):
                    continue
                fn = tc.get("function") or {}
                name = str(fn.get("name") or "")
                tc_id = str(tc.get("id") or "")
                raw_args = fn.get("arguments") or {}
                if isinstance(raw_args, str):
                    try:
                        args = json.loads(raw_args)
                    except json.JSONDecodeError:
                        args = {}
                elif isinstance(raw_args, dict):
                    args = raw_args
                else:
                    args = {}
                if tc_id:
                    pending_args[tc_id] = (name, args if isinstance(args, dict) else {})
        elif role == "tool":
            tc_id = str(msg.get("tool_call_id") or "")
            name = str(msg.get("name") or "")
            args: dict[str, Any] = {}
            if tc_id and tc_id in pending_args:
                name = pending_args[tc_id][0] or name
                args = pending_args[tc_id][1]
            content = msg.get("content")
            acc = merge_sources(acc, extract_from_tool(name, args, content))
    return acc
