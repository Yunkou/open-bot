"""Collect clickable citation URLs from retrieval tool calls.

Used when the bot fetched web/KB/MCP/memory content that exposes real http(s)
URLs. Deduplicated and ordered by first appearance. Never invents URLs.

The sources footer prefers URLs cited in the final assistant answer (or structured
citations). Soft host/title matches and a curated paraphrase fallback keep the
footer when the model used retrieval but did not paste raw links.
"""

from __future__ import annotations

import json
import re
from typing import Any
from urllib.parse import urlparse, urlunparse

# Absolute http(s) URLs in free text / JSON string values.
_URL_RE = re.compile(r"https?://[^\s<>\"'\\)\]]+", re.IGNORECASE)
_HOSTISH_RE = re.compile(r"\b(?:[a-z0-9-]+\.)+[a-z]{2,}\b", re.IGNORECASE)

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

# Shell tools often curl/wget pages; treat as retrieval when URLs appear in cmd/stdout.
_SHELL_RETRIEVAL = frozenset(
    {
        "sandbox_shell",
        "host_shell",
        "shell",
        "bash",
        "run_shell",
        "exec_shell",
    }
)
_SHELL_URL_LINE_RE = re.compile(
    r"(?im)^(?:URL|url|链接|地址)\s*[:=]\s*(https?://\S+)"
)
_CURL_URL_RE = re.compile(
    r'(?i)(?:curl|wget)\b[^\n"\']*?(https?://[^\s"\'\\]+)'
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
    if n in _RETRIEVAL_EXACT or n in _SHELL_RETRIEVAL:
        return True
    if n.endswith("_shell") and "shell" in n:
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


def _looks_like_html(content_type: str, body: str) -> bool:
    ct = (content_type or "").lower()
    if "html" in ct:
        return True
    b = (body or "").lstrip()[:200].lower()
    return b.startswith("<!doctype html") or b.startswith("<html")


def _looks_like_feed(content_type: str, body: str) -> bool:
    ct = (content_type or "").lower()
    if any(x in ct for x in ("xml", "rss", "atom", "rdf")):
        return True
    b = (body or "").lstrip()[:300].lower()
    return (
        b.startswith("<?xml")
        or b.startswith("<rss")
        or b.startswith("<feed")
        or "<channel>" in b[:500]
    )


_RSS_LINK_RE = re.compile(
    r"<link(?:\s[^>]*)?>(\s*https?://[^\s<]+)\s*</link>"
    r'|<link\b[^>]*\bhref=["\']?(https?://[^\s"\'>]+)',
    re.IGNORECASE,
)
_RSS_GUID_RE = re.compile(
    r'<guid[^>]*ispermalink=["\']?true["\']?[^>]*>(\s*https?://[^\s<]+)\s*</guid>',
    re.IGNORECASE,
)


def _urls_from_feed(body: str) -> list[tuple[str, str]]:
    """Item permalinks from RSS/Atom — skip xmlns / chrome URLs."""
    out: list[tuple[str, str]] = []
    seen: set[str] = set()
    if not body:
        return out
    for rx in (_RSS_LINK_RE, _RSS_GUID_RE):
        for m in rx.finditer(body):
            raw = (m.group(1) or m.group(2) or "").strip()
            u = _clean_url(raw)
            if not u or _is_noisy_citation_url(u):
                continue
            key = normalize_url(u)
            if key in seen:
                continue
            seen.add(key)
            out.append((u, ""))
            if len(out) >= 12:
                return out
    return out



def _extract_shell_citation_urls(
    args: dict[str, Any],
    result: Any,
) -> list[tuple[str, str]]:
    """Primary page URLs from sandbox/host shell curl/wget (not every HTML href)."""
    found: list[tuple[str, str]] = []
    seen: set[str] = set()

    def _add(raw: str, title: str = "") -> None:
        u = _clean_url(raw)
        if not u or _is_noisy_citation_url(u):
            return
        key = normalize_url(u)
        if key in seen:
            return
        seen.add(key)
        found.append((u, title))

    cmd = ""
    for key in ("cmd", "command", "script", "code"):
        v = args.get(key)
        if isinstance(v, str) and v.strip():
            cmd = v
            break
    if cmd:
        for m in _URL_RE.findall(cmd):
            # Prefer URLs that look like curl/wget targets; still accept bare https in cmd.
            _add(m)
        for m in _CURL_URL_RE.finditer(cmd):
            _add(m.group(1))

    body = ""
    if isinstance(result, dict):
        for key in ("stdout", "output", "body", "content"):
            v = result.get(key)
            if isinstance(v, str) and v.strip():
                body = v
                break
        if not body and isinstance(result.get("content"), str):
            body = str(result.get("content") or "")
    elif isinstance(result, str):
        text = result.strip()
        if text.startswith("{"):
            try:
                obj = json.loads(text)
            except json.JSONDecodeError:
                try:
                    obj = json.loads(text, strict=False)
                except json.JSONDecodeError:
                    obj = None
            if isinstance(obj, dict):
                for key in ("stdout", "output", "body", "content"):
                    v = obj.get(key)
                    if isinstance(v, str) and v.strip():
                        body = v
                        break
            else:
                body = text
        else:
            body = text

    if body:
        for m in _SHELL_URL_LINE_RE.finditer(body):
            _add(m.group(1))
        # Do not scrape every href out of HTML dumped to stdout — that floods
        # the 20-slot list. Cmd curl/wget targets + explicit "URL:" lines only.
    return found[:12]


def extract_from_tool(
    name: str,
    args: dict[str, Any] | None,
    result: str | dict[str, Any] | None,
) -> list[dict[str, str]]:
    """Return [{url, title?}] from one tool invocation when it looks like retrieval.

    Page-fetch tools (http_fetch etc.) keep the request/final URL only — never scrape
    every href out of an HTML body (that floods the 20-slot list with CDN/xmlns junk and
    drops the real pages the model later cites by host).
    """
    if not _is_retrieval_tool(name):
        return []

    args = args if isinstance(args, dict) else {}
    nlow = (name or "").strip().lower()
    if nlow in _SHELL_RETRIEVAL or nlow.endswith("_shell"):
        shell_found = _extract_shell_citation_urls(args, result)
        return merge_sources([], [{"url": u, "title": t} for u, t in shell_found if u])

    found: list[tuple[str, str]] = []
    arg_title = str(args.get("title") or "").strip()[:200]

    # Prefer explicit arg URLs (fetch/browse).
    for key in ("url", "link", "href", "source_url", "page_url"):
        v = args.get(key)
        if isinstance(v, str):
            u = _clean_url(v)
            if u:
                found.append((u, arg_title))

    # Query-only search tools rarely put the destination URL in args; rely on result.
    parsed_result: Any = result
    if isinstance(result, str):
        text = result.strip()
        if text.startswith("{") or text.startswith("["):
            try:
                parsed_result = json.loads(text)
            except json.JSONDecodeError:
                try:
                    # HTML bodies often embed raw control chars; strict JSON rejects them
                    # and we must not fall back to scraping the whole blob for hrefs.
                    parsed_result = json.loads(text, strict=False)
                except json.JSONDecodeError:
                    # Unparseable: keep arg/primary URLs only (do not regex the blob).
                    return merge_sources([], [{"url": u, "title": t} for u, t in found if u])
        else:
            parsed_result = text

    if parsed_result is not None:
        if isinstance(parsed_result, dict) and parsed_result.get("error"):
            err = str(parsed_result.get("error") or "").lower()
            if "blocked" in err or "not allowed" in err or "ssrf" in err:
                return merge_sources([], [{"url": u, "title": t} for u, t in found if u])
            # Timed-out / soft errors: still keep the requested URL.
            return merge_sources([], [{"url": u, "title": t} for u, t in found if u])

        if isinstance(parsed_result, dict):
            final_u = parsed_result.get("url")
            if isinstance(final_u, str):
                u = _clean_url(final_u)
                if u:
                    found.append((u, arg_title or _title_from_obj(parsed_result)))
            ct = str(parsed_result.get("content_type") or "")
            body = parsed_result.get("body")
            body_s = body if isinstance(body, str) else ""
            # HTML pages: primary URL only (arg + final). Scraping hrefs floods the cap.
            if _looks_like_html(ct, body_s):
                return merge_sources([], [{"url": u, "title": t} for u, t in found if u])
            # RSS/Atom: item links, not every URL in the XML prologue.
            # SERP feeds (Bing/Google search RSS) are full of unrelated junk — keep
            # primary only so they don't fill the 20-slot list before real pages.
            if _looks_like_feed(ct, body_s):
                primary_noisy = any(_is_noisy_citation_url(u) for u, _ in found)
                if not primary_noisy:
                    found.extend(_urls_from_feed(body_s))
                return merge_sources([], [{"url": u, "title": t} for u, t in found if u])
            # Structured JSON result (search APIs): walk url/link fields; skip raw HTML body.
            meta = {k: v for k, v in parsed_result.items() if k != "body"}
            found.extend(_urls_from_obj(meta))
            if body_s and (body_s.lstrip().startswith("{") or body_s.lstrip().startswith("[")):
                try:
                    found.extend(_urls_from_obj(json.loads(body_s)))
                except json.JSONDecodeError:
                    pass
        elif isinstance(parsed_result, (list, dict)):
            found.extend(_urls_from_obj(parsed_result))
        elif isinstance(parsed_result, str) and not _looks_like_html("", parsed_result):
            if _looks_like_feed("", parsed_result):
                found.extend(_urls_from_feed(parsed_result))
            else:
                found.extend(_urls_from_obj(parsed_result))

    return merge_sources([], [{"url": u, "title": t} for u, t in found if u])


def merge_sources(
    existing: list[dict[str, str]] | None,
    new: list[dict[str, str]] | None,
    *,
    limit: int = 20,
) -> list[dict[str, str]]:
    """Deduplicate by normalized URL; keep first title; cap length.

    When at capacity, a non-noisy URL may replace an earlier noisy slot so HTML/SERP
    chrome does not permanently block real pages fetched later in the run.
    """
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
            for prev in out:
                if normalize_url(prev["url"]) == key and not prev.get("title"):
                    title = str(src.get("title") or "").strip()[:200]
                    if title:
                        prev["title"] = title
                    break
            continue
        item: dict[str, str] = {"url": url}
        title = str(src.get("title") or "").strip()[:200]
        if title:
            item["title"] = title
        if len(out) >= limit:
            if _is_noisy_citation_url(url):
                continue
            replaced = False
            for i, prev in enumerate(out):
                if _is_noisy_citation_url(prev["url"]):
                    seen.discard(normalize_url(prev["url"]))
                    out[i] = item
                    seen.add(key)
                    replaced = True
                    break
            if not replaced:
                continue
            continue
        seen.add(key)
        out.append(item)
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



def _cite_match_keys(url: str) -> set[str]:
    """Normalized forms used to match a source URL against answer citations."""
    keys: set[str] = set()
    cleaned = _clean_url(url)
    if not cleaned:
        return keys
    full = normalize_url(cleaned)
    keys.add(full)
    try:
        p = urlparse(cleaned)
        scheme = (p.scheme or "https").lower()
        netloc = (p.netloc or "").lower()
        path = p.path or ""
        if path != "/" and path.endswith("/"):
            path = path.rstrip("/")
        if netloc.endswith(":80") and scheme == "http":
            netloc = netloc[:-3]
        if netloc.endswith(":443") and scheme == "https":
            netloc = netloc[:-4]
        # Also match without query (answers often drop tracking params).
        keys.add(urlunparse((scheme, netloc, path, "", "", "")))
    except Exception:  # noqa: BLE001
        pass
    return {k for k in keys if k}


def urls_from_answer_text(text: str | None) -> list[str]:
    """Extract http(s) URLs from assistant answer text (bare URLs / markdown links)."""
    if not text:
        return []
    found: list[str] = []
    seen: set[str] = set()
    for m in _URL_RE.findall(str(text)):
        u = _clean_url(m)
        if not u:
            continue
        key = normalize_url(u)
        if key in seen:
            continue
        seen.add(key)
        found.append(u)
    return found


def structured_citation_urls(citations: Any) -> list[str]:
    """URLs from structured citation payloads (strings or {url/link/href/...} objects)."""
    if citations is None:
        return []
    found: list[str] = []
    seen: set[str] = set()

    def _add(raw: str) -> None:
        u = _clean_url(raw)
        if not u:
            return
        key = normalize_url(u)
        if key in seen:
            return
        seen.add(key)
        found.append(u)

    if isinstance(citations, str):
        s = citations.strip()
        if s.startswith("{") or s.startswith("["):
            try:
                citations = json.loads(s)
            except json.JSONDecodeError:
                for m in _URL_RE.findall(s):
                    _add(m)
                return found
        else:
            for m in _URL_RE.findall(s):
                _add(m)
            return found

    if isinstance(citations, dict):
        citations = [citations]

    if not isinstance(citations, list):
        return found

    for item in citations:
        if isinstance(item, str):
            _add(item)
            continue
        if not isinstance(item, dict):
            continue
        for key in ("url", "link", "href", "source_url", "canonical_url", "page_url", "uri"):
            v = item.get(key)
            if isinstance(v, str):
                _add(v)
                break
        else:
            # Nested / free-text fields
            for m in _URL_RE.findall(json.dumps(item, ensure_ascii=False)):
                _add(m)
    return found


def _source_host(url: str) -> str:
    try:
        host = (urlparse(url).hostname or "").lower().strip()
    except Exception:  # noqa: BLE001
        return ""
    if host.startswith("www."):
        host = host[4:]
    return host


# Hosts / paths that are tooling noise — fine as fetch targets, poor as footer cites.
_FALLBACK_HOST_BLOCK = frozenset(
    {
        "duckduckgo.com",
        "lite.duckduckgo.com",
        "html.duckduckgo.com",
        "google.com",
        "bing.com",
        "cn.bing.com",
        "so.com",
        "sogou.com",
        "baidu.com",
        "mojeek.com",
        "purl.org",
        "w3.org",
        "xmlns.com",
        "schemas.live.com",
        "microsoft.com",
        "googletagmanager.com",
        "google-analytics.com",
        "doubleclick.net",
        "wpa.qq.com",
        "beian.miit.gov.cn",
    }
)

_FALLBACK_HOST_SUFFIXES = (
    ".bing.com",
    ".duckduckgo.com",
    ".google.com",
    ".baidu.com",
    ".sogou.com",
    ".microsoft.com",
    ".live.com",
    ".w3.org",
    ".360tres.com",
    ".bdstatic.com",
    ".googleusercontent.com",
    ".gstatic.com",
    ".ytimg.com",
    ".ggpht.com",
)


def _is_noisy_citation_url(url: str) -> bool:
    """API / RSS / search / xmlns / SERP chrome — exclude from paraphrase fallback footer."""
    try:
        p = urlparse(url)
    except Exception:  # noqa: BLE001
        return True
    host = (p.hostname or "").lower()
    if host.startswith("www."):
        host = host[4:]
    if not host or host in _FALLBACK_HOST_BLOCK:
        return True
    if host.startswith("api.") or host.startswith("search."):
        return True
    if any(host.endswith(suf) for suf in _FALLBACK_HOST_SUFFIXES):
        return True
    path = (p.path or "").lower()
    if "/api/" in path or path.startswith("/api"):
        return True
    if "/rss" in path or path.endswith(".xml") or "/feed" in path or "/atom" in path:
        return True
    if "outboundfeeds" in path:
        return True
    # Fetch plumbing — fine as tool targets, poor as user-facing cites.
    base = path.rstrip("/")
    if base in ("/robots.txt", "/favicon.ico") or base.endswith("/robots.txt"):
        return True
    if "sitemap" in base and (base.endswith(".xml") or base.endswith(".txt") or base.endswith("/sitemap")):
        return True
    # Search engine result pages (even on hosts not in the block list).
    if path.rstrip("/") in ("/search", "/web", "/s", "/html", "/lite") or path.startswith("/search"):
        q = (p.query or "").lower()
        if any(k in q for k in ("q=", "query=", "wd=", "p=")):
            return True
    q = (p.query or "").lower()
    if "format=rss" in q:
        return True
    if "format=json" in q and ("/q/" in path or "/charts/" in path):
        return True
    return False


def _hosts_mentioned_in_answer(answer: str) -> set[str]:
    """Host-like tokens in answer text (bare domains or inside URLs)."""
    found: set[str] = set()
    if not answer:
        return found
    for m in _HOSTISH_RE.findall(answer):
        h = m.lower().strip(".")
        if h.startswith("www."):
            h = h[4:]
        if "." in h:
            found.add(h)
    return found


def _host_soft_match(source_host: str, mentioned: set[str]) -> bool:
    """Match full host, or subdomain of a host named in the answer."""
    if not source_host or not mentioned:
        return False
    if source_host in mentioned:
        return True
    for m in mentioned:
        if source_host.endswith("." + m):
            return True
    return False


def _answer_mentions_host_label(answer_l: str, source_host: str) -> bool:
    """Match brand-like first label (thingjs.com ← "ThingJS 站") without requiring a bare domain."""
    label = (source_host or "").split(".", 1)[0].strip().lower()
    if len(label) < 5 or not label.isalnum():
        return False
    return re.search(rf"(?<![a-z0-9]){re.escape(label)}(?![a-z0-9])", answer_l) is not None


def _answer_mentions_title(answer_l: str, title: str) -> bool:
    t = (title or "").strip().lower()
    if len(t) < 6:
        return False
    return t in answer_l


def _source_soft_hit(url: str, title: str, answer_l: str, mentioned_hosts: set[str]) -> bool:
    host = _source_host(url)
    return (
        _host_soft_match(host, mentioned_hosts)
        or _answer_mentions_host_label(answer_l, host)
        or _answer_mentions_title(answer_l, title)
    )


def _fallback_retrieval_sources(
    sources: list[dict[str, str]],
    *,
    limit: int = 8,
) -> list[dict[str, str]]:
    """When the model paraphrased with no links, still surface readable retrieval URLs."""
    out: list[dict[str, str]] = []
    seen_host: set[str] = set()
    seen_url: set[str] = set()
    for src in sources:
        if not isinstance(src, dict):
            continue
        url = _clean_url(str(src.get("url") or ""))
        if not url or _is_noisy_citation_url(url):
            continue
        host = _source_host(url)
        if not host or host in seen_host:
            continue
        nkey = normalize_url(url)
        if nkey in seen_url:
            continue
        seen_host.add(host)
        seen_url.add(nkey)
        item: dict[str, str] = {"url": url}
        title = str(src.get("title") or "").strip()[:200]
        if title:
            item["title"] = title
        out.append(item)
        if len(out) >= limit:
            break
    return out


def filter_sources_cited(
    sources: list[dict[str, str]] | None,
    answer_text: str | None,
    *,
    structured_citations: Any = None,
    limit: int = 20,
) -> list[dict[str, str]]:
    """Keep retrieval sources that the answer actually used.

    Preference order:
    1. URLs appearing in the answer (bare / markdown) or structured citations
    2. Soft match: answer names the source host or title (no raw URL pasted)
    3. Fallback: answer has no URL cites but retrieval ran — curated readable URLs
       (drop API/RSS/search noise) so the footer still shows when the model paraphrased
    """
    if not sources:
        return []
    cited_keys: set[str] = set()
    for u in urls_from_answer_text(answer_text):
        cited_keys |= _cite_match_keys(u)
    for u in structured_citation_urls(structured_citations):
        cited_keys |= _cite_match_keys(u)

    answer_raw = str(answer_text or "")
    answer_l = answer_raw.lower()
    mentioned_hosts = _hosts_mentioned_in_answer(answer_raw)
    out: list[dict[str, str]] = []
    seen: set[str] = set()

    def _append(src: dict[str, str], url: str) -> None:
        nkey = normalize_url(url)
        if nkey in seen:
            return
        seen.add(nkey)
        item: dict[str, str] = {"url": url}
        title = str(src.get("title") or "").strip()[:200]
        if title:
            item["title"] = title
        out.append(item)

    # Pass 1: exact / query-tolerant URL citations.
    for src in sources:
        if not isinstance(src, dict):
            continue
        url = _clean_url(str(src.get("url") or ""))
        if not url:
            continue
        if cited_keys and _cite_match_keys(url).intersection(cited_keys):
            _append(src, url)
            if len(out) >= limit:
                return out

    # Pass 2: soft host / title mentions — only when the answer already has URL cites
    # (augment exact URL matches). Pure paraphrase goes to pass 3.
    if cited_keys and len(out) < limit and answer_l.strip():
        for src in sources:
            if not isinstance(src, dict):
                continue
            url = _clean_url(str(src.get("url") or ""))
            if not url:
                continue
            host = _source_host(url)
            title = str(src.get("title") or "")
            if _is_noisy_citation_url(url):
                continue
            if _source_soft_hit(url, title, answer_l, mentioned_hosts):
                _append(src, url)
                if len(out) >= limit:
                    return out

    if out:
        return out

    # Pass 3: no raw URL cites — answer clearly rests on retrieval (paraphrase).
    # Prefer soft host/title hits, then fill with curated readable URLs.
    if (answer_text or "").strip() and not cited_keys:
        soft: list[dict[str, str]] = []
        soft_seen: set[str] = set()
        soft_hosts: set[str] = set()
        for src in sources:
            if not isinstance(src, dict):
                continue
            url = _clean_url(str(src.get("url") or ""))
            if not url or _is_noisy_citation_url(url):
                continue
            title = str(src.get("title") or "")
            if not _source_soft_hit(url, title, answer_l, mentioned_hosts):
                continue
            host = _source_host(url)
            if host in soft_hosts:
                continue
            nkey = normalize_url(url)
            if nkey in soft_seen:
                continue
            soft_seen.add(nkey)
            if host:
                soft_hosts.add(host)
            item: dict[str, str] = {"url": url}
            if title.strip():
                item["title"] = title.strip()[:200]
            soft.append(item)
        fb_limit = min(limit, 8)
        # Named hosts/titles win alone; otherwise curated retrieval URLs.
        if soft:
            return soft[:fb_limit]
        return _fallback_retrieval_sources(sources, limit=fb_limit)
    return []


_CODE_FENCE_RE = re.compile(r"```.*?```", re.DOTALL)
_INLINE_CODE_RE = re.compile(r"`[^`]+`")
_EXISTING_CITE_RE = re.compile(r"\[(\d+)\]")


def _mask_code_spans(text: str) -> str:
    """Replace fenced/inline code with spaces so host/title search skips them."""
    masked = text

    def _blank(m: re.Match[str]) -> str:
        return " " * len(m.group(0))

    masked = _CODE_FENCE_RE.sub(_blank, masked)
    masked = _INLINE_CODE_RE.sub(_blank, masked)
    return masked


def inject_citation_markers(
    answer: str | None,
    sources: list[dict[str, str]] | None,
) -> str:
    """Insert [1][2]… after the first host/title mention for each footer source.

    Idempotent when markers already exist. Does not invent markers when the answer
    never names the source (footer alone still lists them).
    """
    text = str(answer or "")
    if not text.strip() or not sources:
        return text
    existing = {int(m.group(1)) for m in _EXISTING_CITE_RE.finditer(text) if m.group(1).isdigit()}
    for i, src in enumerate(sources):
        if not isinstance(src, dict):
            continue
        n = i + 1
        if n in existing:
            continue
        marker = f"[{n}]"
        if marker in text:
            continue
        url = _clean_url(str(src.get("url") or ""))
        if not url:
            continue
        host = _source_host(url)
        title = str(src.get("title") or "").strip()
        masked = _mask_code_spans(text)
        insert_at: int | None = None
        if host:
            # Match bare host, www.host, or a parent domain the answer named
            # (dt.uino.com ← "uino.com" / "www.uino.com").
            host_candidates = [host]
            parts = host.split(".")
            if len(parts) >= 2:
                parent = ".".join(parts[-2:])
                if parent != host:
                    host_candidates.append(parent)
            for hc in host_candidates:
                pat = re.compile(
                    rf"(?i)(?:https?://)?(?:www\.)?{re.escape(hc)}\b"
                )
                m = pat.search(masked)
                if m:
                    insert_at = m.end()
                    break
            if insert_at is None:
                label = host.split(".", 1)[0]
                if len(label) >= 5 and label.isalnum():
                    lm = re.search(
                        rf"(?i)(?<![a-z0-9]){re.escape(label)}(?![a-z0-9])",
                        masked,
                    )
                    if lm:
                        insert_at = lm.end()
        if insert_at is None and len(title) >= 6:
            idx = masked.lower().find(title.lower())
            if idx >= 0:
                insert_at = idx + len(title)
        if insert_at is None:
            continue
        # If a citation marker already sits right after the match, skip.
        after = text[insert_at : insert_at + 8].lstrip()
        if after.startswith("["):
            continue
        text = text[:insert_at] + marker + text[insert_at:]
        existing.add(n)
        # Remask after mutation for subsequent searches.
    return text
