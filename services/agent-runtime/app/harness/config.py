"""Feature flags and thread-id helpers for the durable harness."""

from __future__ import annotations

import os


def _truthy(name: str, default: str = "0") -> bool:
    raw = os.getenv(name)
    if raw is None:
        raw = default
    return str(raw).strip().lower() in ("1", "true", "yes", "on")


def durable_enabled() -> bool:
    """RUNTIME_DURABLE defaults on so the LangGraph path is primary."""
    return _truthy("RUNTIME_DURABLE", "1")


def checkpoint_backend() -> str:
    """postgres | memory — memory used for tests / DB unavailable."""
    return (os.getenv("RUNTIME_CHECKPOINT") or "postgres").strip().lower()


def require_approval_tools() -> frozenset[str]:
    """Tools that interrupt for human approval before execute."""
    raw = (os.getenv("RUNTIME_APPROVAL_TOOLS") or "request_secret").strip()
    if not raw:
        return frozenset()
    return frozenset(x.strip() for x in raw.split(",") if x.strip())


def thread_id_for(
    *,
    conversation_id: str,
    request_id: str | None = None,
    run_id: str | None = None,
) -> str:
    """Stable LangGraph thread_id for idempotent submit / resume."""
    rid = (request_id or run_id or "").strip()
    cid = (conversation_id or "").strip() or "unknown"
    if rid:
        return f"{cid}:{rid}"
    return f"{cid}:ephemeral"
