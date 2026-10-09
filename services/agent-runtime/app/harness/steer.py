"""Steer / abort / approve against a durable thread."""

from __future__ import annotations

from typing import Any

from langgraph.types import Command

from .graph import get_compiled_graph
from . import tracing


async def steer_thread(
    thread_id: str,
    *,
    text: str,
    mode: str = "follow_up",
) -> dict[str, Any]:
    """Queue steer/follow-up text onto graph state for the next llm turn."""
    graph = await get_compiled_graph()
    config = {"configurable": {"thread_id": thread_id}}
    t = (text or "").strip()
    if not t:
        return {"ok": False, "error": "empty_text"}
    field = "steer_queue" if mode == "steer" else "follow_up_queue"
    await graph.aupdate_state(config, {field: [t]})
    return {"ok": True, "thread_id": thread_id, "mode": mode}


async def abort_thread(thread_id: str) -> dict[str, Any]:
    graph = await get_compiled_graph()
    config = {"configurable": {"thread_id": thread_id}}
    await graph.aupdate_state(
        config,
        {
            "status": "aborted",
            "pending_tool_calls": [],
            "interrupted_reason": "aborted",
        },
    )
    return {"ok": True, "thread_id": thread_id, "status": "aborted"}


async def approve_thread(
    thread_id: str,
    *,
    approve: bool = True,
    reason: str = "",
    root_obs: Any | None = None,
) -> dict[str, Any]:
    """Resume an interrupt with an approval decision."""
    graph = await get_compiled_graph()
    config = {"configurable": {"thread_id": thread_id}}
    payload = {"approve": bool(approve), "reason": reason or ""}
    if approve:
        tracing.mark_resumed(root_obs)
    result = await graph.ainvoke(Command(resume=payload), config)
    status = str((result or {}).get("status") or "")
    return {
        "ok": True,
        "thread_id": thread_id,
        "status": status,
        "final_text": str((result or {}).get("final_text") or ""),
    }


async def get_thread_state(thread_id: str) -> dict[str, Any]:
    graph = await get_compiled_graph()
    config = {"configurable": {"thread_id": thread_id}}
    snap = await graph.aget_state(config)
    values = getattr(snap, "values", None) or {}
    tasks = getattr(snap, "tasks", None) or ()
    interrupted = bool(tasks)
    return {
        "thread_id": thread_id,
        "status": values.get("status"),
        "interrupted": interrupted,
        "next": list(getattr(snap, "next", None) or []),
        "final_text": values.get("final_text") or "",
        "langfuse_trace_id": values.get("langfuse_trace_id") or "",
        "run_id": (values.get("meta") or {}).get("run_id") or "",
    }
