"""Decision path emits Langfuse open-bot.decision when tracing is on."""

from __future__ import annotations

import sys
from pathlib import Path
from types import SimpleNamespace

import httpx

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

from app import langfuse_trace as lf  # noqa: E402
from app.decision.factory import try_decide  # noqa: E402
from app.decision.http_client import SystemOneHTTPClient  # noqa: E402
from app.main import DecisionTestRequest, _run_decision  # noqa: E402


class _FakeObs:
    def __init__(self) -> None:
        self.updates: list[dict] = []
        self.trace_id = "trace-decision-test"

    def update(self, **kwargs):  # noqa: ANN003
        self.updates.append(kwargs)


class _FakeCM:
    def __init__(self, kwargs: dict) -> None:
        self.kwargs = kwargs
        self.obs = _FakeObs()
        self.entered = False
        self.exited = False

    def __enter__(self):
        self.entered = True
        return self.obs

    def __exit__(self, *args):
        self.exited = True
        return False


class _FakeClient:
    def __init__(self) -> None:
        self.cms: list[_FakeCM] = []
        self.flushed = 0

    def start_as_current_observation(self, **kwargs):  # noqa: ANN003
        cm = _FakeCM(kwargs)
        self.cms.append(cm)
        return cm

    def flush(self) -> None:
        self.flushed += 1


def _install(fake: _FakeClient, monkeypatch=None) -> None:
    lf._client = fake
    lf._init_attempted = True
    lf._disabled_reason = None


def _uninstall() -> None:
    lf._client = None
    lf._init_attempted = False
    lf._disabled_reason = None


def test_run_decision_emits_generation() -> None:
    fake = _FakeClient()
    _install(fake)
    try:
        def handler(request: httpx.Request) -> httpx.Response:
            return httpx.Response(
                200,
                json={
                    "model": "laya-multilingual",
                    "answers": {"ok": {"type": "noul", "noul": 0.9}},
                    "usage": {"input_tokens": 5, "output_tokens": 1},
                },
            )

        body = DecisionTestRequest(
            provider="unsloth",
            base_url="http://localhost:8888",
            model="laya",
            api_key="",
            state="ping",
            questions={"ok": {"type": "noul", "instructions": "Is the state non-empty?"}},
        )
        # Patch build path: SystemOneHTTPClient uses transport — inject via build then transport
        # _run_decision uses build_client; replace decide by wrapping after build.
        from app.decision import factory as fac

        real_build = fac.build_client

        def build_with_transport(settings):
            client = real_build(settings)
            if isinstance(client, SystemOneHTTPClient):
                client._transport = httpx.MockTransport(handler)
            return client

        fac.build_client = build_with_transport  # type: ignore[assignment]
        # main imported build_client at module level — patch there too
        import app.main as main_mod

        main_mod.build_client = build_with_transport  # type: ignore[assignment]
        try:
            out = _run_decision(body, default_probe=False)
        finally:
            fac.build_client = real_build  # type: ignore[assignment]
            main_mod.build_client = real_build  # type: ignore[assignment]

        assert out["answers"]["ok"]["noul"] == 0.9
        assert len(fake.cms) == 1
        kw = fake.cms[0].kwargs
        assert kw["as_type"] == "generation"
        assert kw["name"] == "open-bot.decision"
        assert kw["model"] == "laya"
        assert kw["metadata"]["provider"] == "unsloth"
        assert kw["metadata"]["endpoint"] == "decide"
        updates = fake.cms[0].obs.updates
        assert updates, "expected obs.update with output/usage"
        last = updates[-1]
        assert last.get("usage_details") == {
            "prompt_tokens": 5,
            "completion_tokens": 1,
            "total_tokens": 6,
        }
        assert fake.flushed >= 1
        assert fake.cms[0].exited
    finally:
        _uninstall()


def test_run_decision_probe_endpoint_metadata() -> None:
    fake = _FakeClient()
    _install(fake)
    try:
        def handler(request: httpx.Request) -> httpx.Response:
            return httpx.Response(
                200,
                json={"model": "jev-1", "answers": {"ok": {"type": "noul", "noul": 1.0}}},
            )

        body = DecisionTestRequest(
            provider="jev",
            base_url="https://api.typesafe.ai",
            model="jev-latest",
            api_key="secret",
            state="ping",
            questions=None,  # default probe questions
        )
        from app.decision import factory as fac
        import app.main as main_mod

        real_build = fac.build_client

        def build_with_transport(settings):
            client = real_build(settings)
            if isinstance(client, SystemOneHTTPClient):
                client._transport = httpx.MockTransport(handler)
            return client

        fac.build_client = build_with_transport  # type: ignore[assignment]
        main_mod.build_client = build_with_transport  # type: ignore[assignment]
        try:
            out = _run_decision(body, default_probe=True)
        finally:
            fac.build_client = real_build  # type: ignore[assignment]
            main_mod.build_client = real_build  # type: ignore[assignment]

        assert out["enabled"] is True
        assert fake.cms[0].kwargs["metadata"]["endpoint"] == "test"
        assert fake.cms[0].kwargs["metadata"]["probe"] is True
    finally:
        _uninstall()


def test_try_decide_emits_generation() -> None:
    fake = _FakeClient()
    _install(fake)
    try:
        def handler(request: httpx.Request) -> httpx.Response:
            return httpx.Response(
                200,
                json={
                    "model": "laya",
                    "answers": {"pick": {"type": "choice", "choice": "a", "probabilities": {"a": 1.0}}},
                    "usage": {"prompt_tokens": 3, "completion_tokens": 0},
                },
            )

        client = SystemOneHTTPClient(
            "unsloth",
            "http://localhost:8888",
            "",
            "laya",
            transport=httpx.MockTransport(handler),
        )
        out = try_decide(
            "who?",
            {
                "pick": {
                    "type": "choice",
                    "instructions": "choose",
                    "criteria": {"a": "A", "b": "B"},
                }
            },
            client,
        )
        assert out is not None
        assert out["answers"]["pick"]["choice"] == "a"
        assert fake.cms[0].kwargs["name"] == "open-bot.decision"
        assert fake.cms[0].kwargs["metadata"]["endpoint"] == "try_decide"
        assert fake.cms[0].obs.updates[-1].get("usage_details", {}).get("prompt_tokens") == 3
    finally:
        _uninstall()


def test_try_decide_off_skips_observation() -> None:
    fake = _FakeClient()
    _install(fake)
    try:
        from app.decision.factory import build_client
        from app.decision.protocol import DecisionSettings

        client = build_client(DecisionSettings(provider="off"))
        assert try_decide("x", {"ok": {"type": "noul", "instructions": "y"}}, client) is None
        assert fake.cms == []
    finally:
        _uninstall()


if __name__ == "__main__":
    test_run_decision_emits_generation()
    test_run_decision_probe_endpoint_metadata()
    test_try_decide_emits_generation()
    test_try_decide_off_skips_observation()
    print("test_decision_langfuse OK")
