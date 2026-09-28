"""Built-in tools: get_current_time, calculator, http_fetch.

Available when OPENAI_ENABLE_TOOLS=1 / tools_enabled. Keep handlers self-contained
so main.tool_handler can dispatch by name.
"""

from __future__ import annotations

import ast
import ipaddress
import json
import os
import socket
from datetime import datetime
from typing import Any
from urllib.parse import urlparse
from zoneinfo import ZoneInfo, ZoneInfoNotFoundError

import httpx

# ---------------------------------------------------------------------------
# OpenAI tool schemas
# ---------------------------------------------------------------------------

BUILTIN_TOOL_DEFS: list[dict[str, Any]] = [
    {
        "type": "function",
        "function": {
            "name": "get_current_time",
            "description": (
                "Return the current date/time in a timezone. "
                "Default timezone is Asia/Shanghai (or TZ / OPENBOT_TZ env)."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "timezone": {
                        "type": "string",
                        "description": "IANA timezone name, e.g. Asia/Shanghai, UTC, America/New_York",
                    }
                },
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "calculator",
            "description": (
                "Safely evaluate a numeric arithmetic expression. "
                "Supports + - * / ** % //, parentheses, and unary minus. No names/calls."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "expression": {
                        "type": "string",
                        "description": "Arithmetic expression, e.g. (1+2)*3 or 2**10",
                    }
                },
                "required": ["expression"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "http_fetch",
            "description": (
                "HTTP GET a public URL and return truncated text body. "
                "Blocks private/loopback/link-local/metadata IPs (SSRF guard) unless "
                "HTTP_FETCH_ALLOW_PRIVATE=1."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "url": {
                        "type": "string",
                        "description": "http or https URL",
                    },
                    "max_bytes": {
                        "type": "integer",
                        "description": "Max response body bytes (default 100000)",
                    },
                    "timeout_sec": {
                        "type": "number",
                        "description": "Request timeout seconds (default 10, max 30)",
                    },
                },
                "required": ["url"],
            },
        },
    },
]

BUILTIN_TOOL_NAMES = frozenset(
    str((t.get("function") or {}).get("name") or "") for t in BUILTIN_TOOL_DEFS
) - {""}


# ---------------------------------------------------------------------------
# get_current_time
# ---------------------------------------------------------------------------

_WEEKDAY_ZH = ("周一", "周二", "周三", "周四", "周五", "周六", "周日")
_WEEKDAY_EN = (
    "Monday",
    "Tuesday",
    "Wednesday",
    "Thursday",
    "Friday",
    "Saturday",
    "Sunday",
)


def _default_timezone() -> str:
    for key in ("OPENBOT_TZ", "TZ"):
        v = (os.getenv(key) or "").strip()
        if v:
            return v
    return "Asia/Shanghai"


async def get_current_time(args: dict[str, Any]) -> str:
    tz_name = str(args.get("timezone") or "").strip() or _default_timezone()
    try:
        tz = ZoneInfo(tz_name)
    except (ZoneInfoNotFoundError, KeyError, ValueError) as e:
        return json.dumps(
            {
                "error": f"invalid timezone: {tz_name}",
                "detail": str(e),
                "hint": "Use an IANA name like Asia/Shanghai or UTC",
            },
            ensure_ascii=False,
        )
    now = datetime.now(tz)
    wd = now.weekday()  # Mon=0
    payload = {
        "timezone": tz_name,
        "iso8601": now.isoformat(),
        "local": now.strftime("%Y-%m-%d %H:%M:%S %Z"),
        "weekday": _WEEKDAY_EN[wd],
        "weekday_zh": _WEEKDAY_ZH[wd],
        "unix": int(now.timestamp()),
    }
    return json.dumps(payload, ensure_ascii=False)


# ---------------------------------------------------------------------------
# calculator — AST-only safe eval
# ---------------------------------------------------------------------------

_BIN_OPS: dict[type, Any] = {
    ast.Add: lambda a, b: a + b,
    ast.Sub: lambda a, b: a - b,
    ast.Mult: lambda a, b: a * b,
    ast.Div: lambda a, b: a / b,
    ast.FloorDiv: lambda a, b: a // b,
    ast.Mod: lambda a, b: a % b,
    ast.Pow: lambda a, b: a**b,
}
_UNARY_OPS: dict[type, Any] = {
    ast.UAdd: lambda a: +a,
    ast.USub: lambda a: -a,
}


class _SafeCalc(ast.NodeVisitor):
    def visit(self, node: ast.AST) -> Any:  # type: ignore[override]
        if isinstance(node, ast.Expression):
            return self.visit(node.body)
        if isinstance(node, ast.Constant) and isinstance(node.value, (int, float)):
            return node.value
        if isinstance(node, ast.UnaryOp) and type(node.op) in _UNARY_OPS:
            return _UNARY_OPS[type(node.op)](self.visit(node.operand))
        if isinstance(node, ast.BinOp) and type(node.op) in _BIN_OPS:
            left = self.visit(node.left)
            right = self.visit(node.right)
            return _BIN_OPS[type(node.op)](left, right)
        raise ValueError(f"disallowed expression node: {type(node).__name__}")


def _eval_expression(expr: str) -> float | int:
    tree = ast.parse(expr, mode="eval")
    return _SafeCalc().visit(tree)


async def calculator(args: dict[str, Any]) -> str:
    expr = str(args.get("expression") or "").strip()
    if not expr:
        return json.dumps({"error": "expression is required"})
    if len(expr) > 500:
        return json.dumps({"error": "expression too long (max 500 chars)"})
    try:
        result = _eval_expression(expr)
    except Exception as e:  # noqa: BLE001
        return json.dumps(
            {
                "error": "invalid or unsafe expression",
                "detail": str(e),
                "expression": expr,
            },
            ensure_ascii=False,
        )
    # Prefer plain int string when exact.
    if isinstance(result, float) and result.is_integer():
        out: Any = int(result)
    else:
        out = result
    return str(out)


# ---------------------------------------------------------------------------
# http_fetch — constrained GET with SSRF guard
# ---------------------------------------------------------------------------

_DEFAULT_MAX_BYTES = 100_000
_DEFAULT_TIMEOUT = 10.0
_MAX_TIMEOUT = 30.0
_MAX_REDIRECTS = 5


def _allow_private() -> bool:
    v = (os.getenv("HTTP_FETCH_ALLOW_PRIVATE") or "0").strip().lower()
    return v in ("1", "true", "yes", "on")


def _is_blocked_ip(ip: ipaddress.IPv4Address | ipaddress.IPv6Address) -> bool:
    """Reject private, loopback, link-local, multicast, reserved, and metadata IPs."""
    if ip.is_private or ip.is_loopback or ip.is_link_local:
        return True
    if ip.is_multicast or ip.is_reserved or ip.is_unspecified:
        return True
    # Explicit cloud metadata / CGNAT-ish ranges often used in SSRF.
    if isinstance(ip, ipaddress.IPv4Address):
        # 169.254.0.0/16 already link-local; 169.254.169.254 is metadata.
        if ip in ipaddress.ip_network("169.254.0.0/16"):
            return True
        if ip in ipaddress.ip_network("100.64.0.0/10"):  # CGNAT
            return True
        if ip in ipaddress.ip_network("0.0.0.0/8"):
            return True
    if isinstance(ip, ipaddress.IPv6Address):
        if ip.ipv4_mapped is not None:
            return _is_blocked_ip(ip.ipv4_mapped)
        # Unique local fc00::/7
        if ip in ipaddress.ip_network("fc00::/7"):
            return True
    return False


def _resolve_and_check_host(hostname: str) -> list[str]:
    """Resolve hostname and raise ValueError if any address is blocked."""
    if not hostname:
        raise ValueError("empty hostname")
    # Literal IP in hostname
    try:
        lit = ipaddress.ip_address(hostname)
        if not _allow_private() and _is_blocked_ip(lit):
            raise ValueError(f"blocked address: {hostname}")
        return [hostname]
    except ValueError as e:
        if "blocked address" in str(e):
            raise
        # not a literal IP — fall through to DNS
        pass

    try:
        infos = socket.getaddrinfo(hostname, None, type=socket.SOCK_STREAM)
    except socket.gaierror as e:
        raise ValueError(f"DNS resolve failed for {hostname}: {e}") from e
    addrs: list[str] = []
    for info in infos:
        addr = info[4][0]
        try:
            ip = ipaddress.ip_address(addr)
        except ValueError:
            continue
        if not _allow_private() and _is_blocked_ip(ip):
            raise ValueError(f"blocked address for host {hostname}: {addr}")
        addrs.append(addr)
    if not addrs:
        raise ValueError(f"no usable addresses for {hostname}")
    return addrs


def _validate_url(url: str) -> str:
    parsed = urlparse(url)
    scheme = (parsed.scheme or "").lower()
    if scheme not in ("http", "https"):
        raise ValueError("only http/https URLs are allowed")
    if not parsed.hostname:
        raise ValueError("URL missing hostname")
    if parsed.username or parsed.password:
        raise ValueError("URLs with userinfo are not allowed")
    _resolve_and_check_host(parsed.hostname)
    return url


def _looks_binary(content_type: str, sample: bytes) -> bool:
    ct = (content_type or "").lower()
    if any(
        x in ct
        for x in (
            "application/octet-stream",
            "image/",
            "audio/",
            "video/",
            "application/pdf",
            "application/zip",
            "application/gzip",
            "font/",
        )
    ):
        return True
    if "text/" in ct or "json" in ct or "xml" in ct or "javascript" in ct or "csv" in ct:
        return False
    # Heuristic: high ratio of null / non-text bytes
    if not sample:
        return False
    nontext = sum(1 for b in sample if b < 9 or (13 < b < 32) or b == 127)
    return (nontext / max(len(sample), 1)) > 0.30


async def http_fetch(args: dict[str, Any]) -> str:
    url = str(args.get("url") or "").strip()
    if not url:
        return json.dumps({"error": "url is required"})

    try:
        max_bytes = int(args.get("max_bytes") if args.get("max_bytes") is not None else _DEFAULT_MAX_BYTES)
    except (TypeError, ValueError):
        max_bytes = _DEFAULT_MAX_BYTES
    max_bytes = max(1, min(max_bytes, 2_000_000))

    try:
        timeout_sec = float(
            args.get("timeout_sec") if args.get("timeout_sec") is not None else _DEFAULT_TIMEOUT
        )
    except (TypeError, ValueError):
        timeout_sec = _DEFAULT_TIMEOUT
    timeout_sec = max(0.5, min(timeout_sec, _MAX_TIMEOUT))

    try:
        _validate_url(url)
    except ValueError as e:
        return json.dumps({"error": str(e), "url": url}, ensure_ascii=False)

    current = url
    status = 0
    content_type = ""
    body_bytes = b""
    final_url = url

    timeout = httpx.Timeout(timeout_sec, connect=min(10.0, timeout_sec))
    try:
        async with httpx.AsyncClient(
            timeout=timeout,
            follow_redirects=False,
            trust_env=False,  # ignore HTTP_PROXY for SSRF surface
        ) as client:
            for _hop in range(_MAX_REDIRECTS + 1):
                try:
                    _validate_url(current)
                except ValueError as e:
                    return json.dumps(
                        {"error": f"redirect target blocked: {e}", "url": current},
                        ensure_ascii=False,
                    )
                resp = await client.get(current, headers={"User-Agent": "open-bot-http_fetch/1.0"})
                status = resp.status_code
                # Manual redirect follow with re-check
                if status in (301, 302, 303, 307, 308):
                    loc = resp.headers.get("location")
                    if not loc:
                        break
                    next_url = str(httpx.URL(current).join(loc))
                    current = next_url
                    continue
                content_type = resp.headers.get("content-type") or ""
                # Stream only up to max_bytes + 1 to detect truncation
                chunks: list[bytes] = []
                total = 0
                async for chunk in resp.aiter_bytes():
                    if not chunk:
                        continue
                    remain = max_bytes + 1 - total
                    if remain <= 0:
                        break
                    chunks.append(chunk[:remain])
                    total += len(chunks[-1])
                    if total > max_bytes:
                        break
                body_bytes = b"".join(chunks)
                final_url = str(resp.url)
                break
            else:
                return json.dumps(
                    {
                        "error": f"too many redirects (max {_MAX_REDIRECTS})",
                        "url": url,
                        "last": current,
                    },
                    ensure_ascii=False,
                )
    except httpx.TimeoutException:
        return json.dumps(
            {"error": "request timed out", "url": url, "timeout_sec": timeout_sec},
            ensure_ascii=False,
        )
    except Exception as e:  # noqa: BLE001
        return json.dumps(
            {"error": f"fetch failed: {e}", "url": url},
            ensure_ascii=False,
        )

    truncated = len(body_bytes) > max_bytes
    if truncated:
        body_bytes = body_bytes[:max_bytes]

    if _looks_binary(content_type, body_bytes[:512]):
        return json.dumps(
            {
                "status": status,
                "url": final_url,
                "content_type": content_type,
                "binary": True,
                "bytes": len(body_bytes),
                "truncated": truncated,
                "body": None,
                "message": "response looks binary; body omitted",
            },
            ensure_ascii=False,
        )

    text = body_bytes.decode("utf-8", errors="replace")
    return json.dumps(
        {
            "status": status,
            "url": final_url,
            "content_type": content_type,
            "binary": False,
            "bytes": len(body_bytes),
            "truncated": truncated,
            "body": text,
        },
        ensure_ascii=False,
    )


# ---------------------------------------------------------------------------
# Dispatch helper
# ---------------------------------------------------------------------------

async def dispatch(name: str, args: dict[str, Any]) -> str | None:
    """Handle a builtin tool by name. Returns None if name is not a builtin."""
    if name == "get_current_time":
        return await get_current_time(args)
    if name == "calculator":
        return await calculator(args)
    if name == "http_fetch":
        return await http_fetch(args)
    return None
