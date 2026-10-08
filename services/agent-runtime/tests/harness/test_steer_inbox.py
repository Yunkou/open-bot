"""Steer queue updates graph state."""

from __future__ import annotations

import os

import pytest

os.environ["RUNTIME_CHECKPOINT"] = "memory"
os.environ["RUNTIME_DURABLE"] = "1"


@pytest.mark.asyncio
async def test_steer_updates_follow_up_queue():
    from langgraph.checkpoint.memory import MemorySaver

    from app.harness.graph import build_graph, reset_compiled_graph
    from app.harness.state import initial_state
    from app.harness import steer as steer_mod
    from app.harness import graph as graph_mod

    reset_compiled_graph()
    saver = MemorySaver()

    async def fake_compiled(checkpointer=None):
        return build_graph().compile(checkpointer=saver)

    monkeypatch = pytest.MonkeyPatch()
    monkeypatch.setattr(steer_mod, "get_compiled_graph", fake_compiled)
    try:
        g = await fake_compiled()
        config = {"configurable": {"thread_id": "c:s1"}}
        st = initial_state(messages=[{"role": "user", "content": "x"}], max_rounds=2)
        st["prepared"] = True
        st["skip_recall"] = True
        st["status"] = "running"
        await g.aupdate_state(config, st, as_node="prepare")
        res = await steer_mod.steer_thread("c:s1", text="also do Y", mode="follow_up")
        assert res["ok"] is True
        snap = await g.aget_state(config)
        values = snap.values or {}
        assert "also do Y" in (values.get("follow_up_queue") or [])
    finally:
        monkeypatch.undo()
        reset_compiled_graph()
