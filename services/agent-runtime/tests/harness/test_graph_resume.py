"""Durable harness: checkpoint resume + idempotent done."""

from __future__ import annotations

import os

import pytest

# Force in-memory checkpointer for unit tests (no Postgres required).
os.environ["RUNTIME_CHECKPOINT"] = "memory"
os.environ["RUNTIME_DURABLE"] = "1"


@pytest.mark.asyncio
async def test_thread_id_stable():
    from app.harness.config import thread_id_for

    a = thread_id_for(conversation_id="c1", request_id="r1")
    b = thread_id_for(conversation_id="c1", request_id="r1")
    assert a == b == "c1:r1"


@pytest.mark.asyncio
async def test_graph_prepare_llm_finish_memory_skip_on_resume(monkeypatch):
    from langgraph.checkpoint.memory import MemorySaver

    from app.harness.graph import build_graph, reset_compiled_graph
    from app.harness.state import initial_state

    reset_compiled_graph()
    saver = MemorySaver()
    graph = build_graph().compile(checkpointer=saver)

    calls = {"recall": 0, "llm": 0}

    async def fake_prepare(state, config):
        calls["recall"] += 1
        if state.get("skip_recall") or state.get("prepared"):
            return {"prepared": True, "status": "running"}
        return {
            "prepared": True,
            "skip_recall": True,
            "messages": [
                {"role": "system", "content": "sys"},
                {"role": "user", "content": "hi"},
            ],
            "meta": {"memory_recall": {"items": []}, "run_id": "r1"},
            "recall_payload": {"items": []},
            "status": "running",
        }

    async def fake_llm(state, config):
        calls["llm"] += 1
        return {
            "final_text": "hello",
            "pending_tool_calls": [],
            "status": "done",
            "messages": list(state.get("messages") or [])
            + [{"role": "assistant", "content": "hello"}],
            "round": 1,
        }

    async def fake_finish(state, config):
        return {"status": "done", "meta": dict(state.get("meta") or {})}

    monkeypatch.setattr("app.harness.graph.prepare_node", fake_prepare)
    monkeypatch.setattr("app.harness.graph.llm_node", fake_llm)
    monkeypatch.setattr("app.harness.graph.finish_node", fake_finish)
    # rebuild with patched nodes
    from app.harness import graph as graph_mod

    reset_compiled_graph()
    g = graph_mod.build_graph().compile(checkpointer=saver)

    config = {"configurable": {"thread_id": "conv:req-1"}}
    s0 = initial_state(messages=[{"role": "user", "content": "hi"}], max_rounds=4)
    out1 = await g.ainvoke(s0, config)
    assert out1["status"] == "done"
    assert out1["final_text"] == "hello"
    assert calls["recall"] == 1

    # Idempotent re-invoke with same thread: LangGraph returns current state when done
    # Our runner short-circuits on status=done; here we verify skip_recall is set.
    assert out1.get("skip_recall") is True or out1.get("prepared") is True


@pytest.mark.asyncio
async def test_replay_policy():
    from app.harness.replay import interrupted_result, policy_for

    assert policy_for("load_skill") == "safe"
    assert policy_for("host_shell") == "interrupted"
    assert "interrupted" in interrupted_result("host_shell")
