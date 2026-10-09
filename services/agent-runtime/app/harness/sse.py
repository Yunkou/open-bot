"""Map harness state updates to existing SSE event shapes."""

from __future__ import annotations

import json
from typing import Any


def sse(event: str, data: dict[str, Any]) -> str:
    return f"event: {event}\ndata: {json.dumps(data, ensure_ascii=False)}\n\n"


def tokens_from_text(text: str, chunk: int = 24) -> list[str]:
    t = text or ""
    if not t:
        return []
    return [t[i : i + chunk] for i in range(0, len(t), chunk)]
