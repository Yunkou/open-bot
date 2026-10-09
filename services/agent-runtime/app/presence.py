"""Publish bot_presence to Go API during a run (thinking vs working).

POST /internal/bot-presence with X-Internal-Token. Fail soft — never break the run.
Enforces a minimum hold so short tool calls do not flicker the client status.
"""

from __future__ import annotations

import asyncio
import json
import logging
import os
import time
import urllib.error
import urllib.request
from typing import Any

DEFAULT_API_URL = "http://127.0.0.1:18080"
DEFAULT_INTERNAL_TOKEN = "open-bot-dev-internal"
DEFAULT_MIN_HOLD_MS = 300

logger = logging.getLogger(__name__)

ALLOWED = frozenset({"idle", "thinking", "working", "awaiting_approval", "error"})


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


def _post_presence(payload: dict[str, Any], timeout: float = 5.0) -> None:
    url = f"{_api_base()}/internal/bot-presence"
    data = json.dumps(payload).encode("utf-8")
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
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        resp.read()


class PresencePublisher:
    """Async status publisher with ≥min_hold_ms dwell before switching away."""

    def __init__(
        self,
        conversation_id: str | None,
        agent_id: str | None,
        user_id: str | None = None,
        min_hold_ms: int = DEFAULT_MIN_HOLD_MS,
        *,
        post_fn: Any | None = None,
    ) -> None:
        self.conversation_id = (conversation_id or "").strip()
        self.agent_id = (agent_id or "").strip() or "open-bot"
        self.user_id = (user_id or "").strip() or None
        self.min_hold_ms = max(0, int(min_hold_ms))
        self._post_fn = post_fn or _post_presence
        self._current: str | None = None
        self._set_at: float = 0.0
        self._lock = asyncio.Lock()

    def _payload(self, status: str) -> dict[str, Any] | None:
        if status not in ALLOWED:
            return None
        if not self.conversation_id and not self.user_id:
            return None
        body: dict[str, Any] = {
            "agent_id": self.agent_id,
            "status": status,
        }
        if self.conversation_id:
            body["conversation_id"] = self.conversation_id
        if self.user_id:
            body["user_id"] = self.user_id
        return body

    async def set(self, status: str) -> None:
        status = (status or "").strip()
        if not status:
            return
        async with self._lock:
            if status == self._current:
                return
            if self._current is not None and self.min_hold_ms > 0:
                elapsed_ms = (time.monotonic() - self._set_at) * 1000.0
                wait_ms = self.min_hold_ms - elapsed_ms
                if wait_ms > 0:
                    await asyncio.sleep(wait_ms / 1000.0)
            payload = self._payload(status)
            if payload is not None:
                try:
                    await asyncio.to_thread(self._post_fn, payload)
                except (urllib.error.URLError, urllib.error.HTTPError, TimeoutError, OSError) as e:
                    logger.debug("bot-presence post failed: %s", e)
                except Exception as e:  # noqa: BLE001
                    logger.debug("bot-presence post failed: %s", e)
            self._current = status
            self._set_at = time.monotonic()
