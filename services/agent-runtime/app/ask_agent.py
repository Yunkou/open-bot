"""Synchronous colleague ask: wait for the other bot and return the reply."""

from __future__ import annotations

from typing import Any

import httpx

from .agent_bus import _api_base, _internal_token


async def ask_colleague(
    *,
    user_id: str,
    from_agent_id: str,
    to_agent_id: str,
    question: str,
    parent_conversation_id: str,
    parent_thread_id: str,
) -> dict[str, Any]:
    question = (question or "").strip()
    to_agent_id = (to_agent_id or "").strip()
    if not question or not to_agent_id:
        return {"ok": False, "error": "to_agent_id and question are required"}
    url = f"{_api_base()}/internal/agents/ask"
    payload = {
        "user_id": user_id,
        "from_agent_id": from_agent_id,
        "to_agent_id": to_agent_id,
        "question": question,
        "parent_conversation_id": parent_conversation_id,
        "parent_thread_id": parent_thread_id,
    }
    timeout = httpx.Timeout(480.0, connect=10.0)
    try:
        async with httpx.AsyncClient(timeout=timeout) as client:
            resp = await client.post(
                url,
                json=payload,
                headers={
                    "Content-Type": "application/json",
                    "X-Internal-Token": _internal_token(),
                    "Accept": "application/json",
                },
            )
    except httpx.TimeoutException:
        return {"ok": False, "error": "询问同事超时"}
    except httpx.HTTPError as exc:
        return {"ok": False, "error": str(exc)}
    try:
        data = resp.json()
    except ValueError:
        data = {"ok": False, "error": resp.text[:500] or f"HTTP {resp.status_code}"}
    if resp.status_code >= 300 and isinstance(data, dict) and not data.get("error"):
        data = {"ok": False, "error": f"HTTP {resp.status_code}"}
    if isinstance(data, dict):
        return data
    return {"ok": False, "error": "bad ask response"}
