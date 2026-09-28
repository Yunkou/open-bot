"""ListMachines helper: call Go internal API (same auth as sandbox/agent_bus)."""

from __future__ import annotations

import json
import os
import urllib.error
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


def list_machines(user_id: str, timeout: float = 15.0) -> dict[str, Any]:
    """Return {"machines": [...], "count": N} for the user."""
    uid = (user_id or "").strip()
    if not uid:
        return {"error": "user_id required", "machines": [], "count": 0}
    url = f"{_api_base()}/internal/machines/list"
    data = json.dumps({"user_id": uid}).encode("utf-8")
    req = urllib.request.Request(
        url,
        data=data,
        method="POST",
        headers={
            "Content-Type": "application/json",
            "X-Internal-Token": _internal_token(),
            "Accept": "application/json",
        },
    )
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read().decode("utf-8")
            return json.loads(raw) if raw else {"machines": [], "count": 0}
    except urllib.error.HTTPError as e:
        detail = e.read().decode("utf-8", errors="replace")
        return {"error": f"machines api HTTP {e.code}: {detail}", "machines": [], "count": 0}
    except urllib.error.URLError as e:
        return {"error": f"machines api unreachable: {e}", "machines": [], "count": 0}
