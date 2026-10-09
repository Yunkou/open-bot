"""Confirmed bot lessons: fetch active only, inject into system prompt.

Pending / ignored never load. Draft generation lives in API after feedback submit.
"""

from __future__ import annotations

import json
import os
import urllib.error
import urllib.parse
import urllib.request
from typing import Any

DEFAULT_API_URL = "http://127.0.0.1:18080"
DEFAULT_INTERNAL_TOKEN = "open-bot-dev-internal"


def _api_base() -> str:
    return (os.getenv("OPENBOT_API_URL") or os.getenv("API_BASE_URL") or DEFAULT_API_URL).rstrip(
        "/"
    )


def _internal_token() -> str:
    return (
        (os.getenv("INTERNAL_TOKEN") or os.getenv("OPENBOT_INTERNAL_TOKEN") or DEFAULT_INTERNAL_TOKEN)
        .strip()
        or DEFAULT_INTERNAL_TOKEN
    )


def fetch_active_lessons(user_id: str, agent_id: str) -> list[dict[str, Any]]:
    """GET /internal/lessons/active — only status=active rows."""
    user_id = (user_id or "").strip()
    agent_id = (agent_id or "").strip() or "open-bot"
    if not user_id:
        return []
    q = urllib.parse.urlencode({"user_id": user_id, "agent_id": agent_id})
    url = f"{_api_base()}/internal/lessons/active?{q}"
    req = urllib.request.Request(
        url,
        method="GET",
        headers={
            "X-Internal-Token": _internal_token(),
            "Accept": "application/json",
        },
    )
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            raw = resp.read().decode("utf-8")
            data = json.loads(raw) if raw else {}
            lessons = data.get("lessons") or []
            out: list[dict[str, Any]] = []
            for item in lessons:
                if not isinstance(item, dict):
                    continue
                if str(item.get("status") or "active") != "active":
                    continue
                title = str(item.get("title") or "").strip()
                body = str(item.get("body") or "").strip()
                if not title or not body:
                    continue
                out.append(item)
            return out
    except (urllib.error.URLError, urllib.error.HTTPError, TimeoutError, json.JSONDecodeError):
        return []
    except Exception:  # noqa: BLE001
        return []


def format_lessons_for_prompt(lessons: list[dict[str, Any]], *, limit: int = 12) -> str:
    """Format confirmed lessons as a system-prompt section (empty if none)."""
    if not lessons:
        return ""
    lines = [
        "## 用户已确认的经验（必须遵守；未确认的经验不会出现在此）",
    ]
    for item in lessons[: max(1, limit)]:
        title = str(item.get("title") or "").strip()
        body = str(item.get("body") or "").strip()
        tags = item.get("tags") or []
        tag_s = ""
        if isinstance(tags, list) and tags:
            tag_s = " [" + ", ".join(str(t) for t in tags if t) + "]"
        lines.append(f"- **{title}**{tag_s}：{body}")
    return "\n".join(lines)


def active_lesson_block(user_id: str, agent_id: str) -> str:
    return format_lessons_for_prompt(fetch_active_lessons(user_id, agent_id))


def active_lessons_with_block(user_id: str, agent_id: str) -> tuple[list[dict[str, Any]], str]:
    """Return (active lessons, prompt block) so callers can record what was injected."""
    lessons = fetch_active_lessons(user_id, agent_id)
    return lessons, format_lessons_for_prompt(lessons)
