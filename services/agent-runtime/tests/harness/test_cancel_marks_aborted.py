"""ASGI CancelledError on durable astream must leave harness status aborted."""

from __future__ import annotations

import asyncio
from typing import Any
from unittest.mock import AsyncMock, MagicMock

import pytest

from app.harness import runner as runner_mod


class _FakeJournal:
    def __init__(self) -> None:
        self.status = "running"
        self.statuses: list[str] = []

    async def set_status(self, status: str) -> dict[str, Any]:
        self.status = status
        self.statuses.append(status)
        return {"status": status}

    async def commit_assistant(self, *args: Any, **kwargs: Any) -> None:
        return None

    async def put_live(self, *args: Any, **kwargs: Any) -> None:
        return None

    async def put_usage(self, *args: Any, **kwargs: Any) -> None:
        return None


@pytest.mark.asyncio
async def test_durable_cancelled_error_marks_aborted(monkeypatch: pytest.MonkeyPatch) -> None:
    journal = _FakeJournal()

    class _FakeSnap:
        values: dict[str, Any] = {}
        tasks = ()
        next = ()

    class _FakeGraph:
        async def aget_state(self, config: Any) -> _FakeSnap:
            return _FakeSnap()

        async def astream(self, inp: Any, config: Any, stream_mode: str = "updates"):
            yield {"prepare": {"meta": {"run_id": "r1"}}}
            raise asyncio.CancelledError()

    async def fake_get_compiled_graph() -> _FakeGraph:
        return _FakeGraph()

    monkeypatch.setattr(runner_mod, "get_compiled_graph", fake_get_compiled_graph)
    monkeypatch.setattr(runner_mod, "JournalSession", lambda **kwargs: journal)
    monkeypatch.setattr(runner_mod, "mark_live", lambda tid: None)
    monkeypatch.setattr(runner_mod, "release_live", lambda tid: None)
    monkeypatch.setattr(runner_mod.tracing, "attach_run_metadata", lambda *a, **k: None)
    monkeypatch.setattr(runner_mod.lf, "update_obs", lambda *a, **k: None)

    body = MagicMock()
    body.conversation_id = "c1"
    body.user_id = "u1"
    body.agent_id = "a1"
    body.request_id = "r1"

    agen = runner_mod.run_durable_events(
        body=body,
        history=[{"role": "user", "content": "hi"}],
        api_key="sk-test",
        override=None,
        run_id="r1",
        request_id="r1",
    )
    with pytest.raises(asyncio.CancelledError):
        async for _ in agen:
            pass

    assert journal.status == "aborted"
    assert "aborted" in journal.statuses
