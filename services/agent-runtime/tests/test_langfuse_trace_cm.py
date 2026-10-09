"""Throw-safe Langfuse contextmanager helpers (no double-yield after throw)."""

from __future__ import annotations

import sys
from pathlib import Path
from types import SimpleNamespace

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

from app import langfuse_trace as lf  # noqa: E402


def _ok(cond: bool, msg: str) -> None:
    if not cond:
        raise AssertionError(msg)
    print(f"  OK  {msg}")


class _FakeObs:
    def __init__(self) -> None:
        self.updates: list[dict] = []

    def update(self, **kwargs):  # noqa: ANN003
        self.updates.append(kwargs)


class _FakeCM:
    def __init__(self, *, exit_raises: bool = False) -> None:
        self.entered = False
        self.exited = False
        self.exit_args = None
        self.exit_raises = exit_raises
        self.obs = _FakeObs()

    def __enter__(self):
        self.entered = True
        return self.obs

    def __exit__(self, *args):
        self.exited = True
        self.exit_args = args
        if self.exit_raises:
            raise RuntimeError("cleanup boom")
        return False


class _FakeClient:
    def __init__(self, *, exit_raises: bool = False) -> None:
        self.cms: list[_FakeCM] = []
        self.exit_raises = exit_raises

    def start_as_current_observation(self, **kwargs):  # noqa: ANN003
        cm = _FakeCM(exit_raises=self.exit_raises)
        self.cms.append(cm)
        return cm


def test_observation_generation_body_raise_no_double_yield(monkeypatch_client: _FakeClient) -> None:
    """Raising after yield must re-raise original error, not 'generator didn't stop'."""
    raised = None
    try:
        with lf.observation_generation(name="t", model="m") as obs:
            _ok(isinstance(obs, _FakeObs), "got real obs from fake client")
            raise RuntimeError("body boom")
    except RuntimeError as e:
        raised = e
    _ok(raised is not None and str(raised) == "body boom", "original body error preserved")
    _ok(monkeypatch_client.cms[0].exited, "observation __exit__ called")
    _ok(
        monkeypatch_client.cms[0].exit_args is not None
        and monkeypatch_client.cms[0].exit_args[0] is RuntimeError,
        "exit saw RuntimeError",
    )


def test_observation_generation_exit_raise_swallowed(monkeypatch_client: _FakeClient) -> None:
    monkeypatch_client.exit_raises = True
    # recreate cms with exit_raises — client flag already set for new cms
    with lf.observation_generation(name="t") as obs:
        _ok(obs is not None, "enter ok")
    _ok(monkeypatch_client.cms[0].exited, "exit attempted despite raise")


def test_observation_tool_body_raise_no_double_yield(monkeypatch_client: _FakeClient) -> None:
    try:
        with lf.observation_tool("calc", {"x": 1}) as obs:
            _ok(isinstance(obs, _FakeObs), "tool obs entered")
            raise ValueError("tool body")
    except ValueError as e:
        _ok(str(e) == "tool body", "tool body error preserved")
    else:
        raise AssertionError("expected ValueError")


def test_trace_run_body_raise_no_double_yield(monkeypatch_client: _FakeClient) -> None:
    # stub propagate_attributes as a nested CM
    import types

    prop_cms: list[_FakeCM] = []

    def fake_propagate(**kwargs):  # noqa: ANN003
        cm = _FakeCM()
        prop_cms.append(cm)
        return cm

    fake_mod = types.ModuleType("langfuse")
    fake_mod.propagate_attributes = fake_propagate  # type: ignore[attr-defined]
    sys.modules["langfuse"] = fake_mod

    try:
        with lf.trace_run("c1", "a1", "u1", "hello") as span:
            _ok(isinstance(span, _FakeObs), "trace span entered")
            raise RuntimeError("run boom")
    except RuntimeError as e:
        _ok(str(e) == "run boom", "trace body error preserved")
    else:
        raise AssertionError("expected RuntimeError")

    _ok(len(prop_cms) == 1 and prop_cms[0].exited, "propagate exited")
    _ok(monkeypatch_client.cms[0].exited, "obs exited")
    # ERROR level marked
    _ok(
        any(u.get("level") == "ERROR" for u in monkeypatch_client.cms[0].obs.updates),
        "span marked ERROR",
    )


def test_disabled_yields_noop_once() -> None:
    # Force disabled client
    lf._client = None  # noqa: SLF001
    lf._init_attempted = True  # noqa: SLF001
    lf._disabled_reason = "test"  # noqa: SLF001
    with lf.observation_generation() as obs:
        _ok(isinstance(obs, lf._Noop), "noop when disabled")  # noqa: SLF001
    with lf.trace_run("c", "a", None, "x") as span:
        _ok(isinstance(span, lf._Noop), "noop trace when disabled")  # noqa: SLF001
    with lf.observation_tool("t") as t:
        _ok(isinstance(t, lf._Noop), "noop tool when disabled")  # noqa: SLF001


def test_async_gen_close_no_double_yield(monkeypatch_client: _FakeClient) -> None:
    """Closing an async generator that spans observation_generation must not double-yield."""
    import asyncio

    async def agen():
        with lf.observation_generation(name="llm") as obs:
            _ok(isinstance(obs, _FakeObs), "async gen entered obs")
            yield "chunk1"
            yield "chunk2"

    async def run():
        gen = agen()
        first = await gen.__anext__()
        _ok(first == "chunk1", "got first chunk")
        await gen.aclose()  # throws GeneratorExit into sync CM
        _ok(monkeypatch_client.cms[0].exited, "cleaned up on aclose")

    asyncio.run(run())


def main() -> None:
    print("test_langfuse_trace_cm")
    client = _FakeClient()

    def _patch():
        lf._client = client  # noqa: SLF001
        lf._init_attempted = True  # noqa: SLF001
        lf._disabled_reason = None  # noqa: SLF001
        # get_langfuse returns our client
        orig = lf.get_langfuse
        lf.get_langfuse = lambda: client  # type: ignore[assignment]
        return orig

    orig = _patch()
    try:
        client.cms.clear()
        test_observation_generation_body_raise_no_double_yield(client)
        client.cms.clear()
        test_observation_generation_exit_raise_swallowed(client)
        client.cms.clear()
        client.exit_raises = False
        test_observation_tool_body_raise_no_double_yield(client)
        client.cms.clear()
        test_trace_run_body_raise_no_double_yield(client)
        client.cms.clear()
        test_async_gen_close_no_double_yield(client)
    finally:
        lf.get_langfuse = orig  # type: ignore[assignment]

    test_disabled_yields_noop_once()
    print("all passed")


if __name__ == "__main__":
    main()

def test_trace_id_of_noop_and_attr():
    assert lf.trace_id_of(None) == ""
    assert lf.trace_id_of(lf._Noop()) == ""

    class _Obs:
        trace_id = "abc123"

    assert lf.trace_id_of(_Obs()) == "abc123"

