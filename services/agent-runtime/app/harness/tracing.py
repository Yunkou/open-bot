"""Langfuse helpers scoped to the durable harness."""

from __future__ import annotations

from typing import Any

from .. import langfuse_trace as lf


def attach_run_metadata(root_obs: Any, *, run_id: str, thread_id: str, trace_id: str) -> None:
    if root_obs is None:
        return
    meta: dict[str, Any] = {"run_id": run_id, "thread_id": thread_id}
    if trace_id:
        meta["langfuse_trace_id"] = trace_id
    lf.update_obs(root_obs, metadata=meta)


def mark_interrupt(root_obs: Any, reason: str) -> None:
    if root_obs is None:
        return
    lf.update_obs(
        root_obs,
        metadata={"waiting_approval": True, "interrupt_reason": (reason or "")[:300]},
    )


def mark_resumed(root_obs: Any) -> None:
    if root_obs is None:
        return
    lf.update_obs(root_obs, metadata={"waiting_approval": False, "resumed": True})


def generation_span(*, model: str | None, messages: list[dict[str, Any]]):
    return lf.observation_generation(
        name="open-bot.llm",
        model=model,
        input_messages=messages,
    )


def tool_span(name: str, args: dict[str, Any]):
    return lf.observation_tool(name, args)
