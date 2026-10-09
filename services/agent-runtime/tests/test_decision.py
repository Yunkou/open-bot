"""Decision adapter: disabled by default; Jev and Laya share one result shape."""

from __future__ import annotations

import sys
from pathlib import Path

import httpx

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

from app.decision.factory import build_client, try_decide  # noqa: E402
from app.decision.http_client import SystemOneHTTPClient  # noqa: E402
from app.decision.laya_local import LayaLocalClient  # noqa: E402
from app.decision.protocol import DecisionError, DecisionSettings  # noqa: E402


def test_off_skips_the_network() -> None:
    client = build_client(DecisionSettings(provider="off"))
    assert client.enabled is False
    assert try_decide("hello", {"ok": {"type": "noul", "instructions": "non-empty?"}}, client) is None
    assert build_client(None).enabled is False
    assert build_client(DecisionSettings(provider="nope")).enabled is False


def test_jev_posts_systemone() -> None:
    captured: dict = {}

    def handler(request: httpx.Request) -> httpx.Response:
        captured["url"] = str(request.url)
        captured["auth"] = request.headers.get("authorization")
        captured["body"] = request.content.decode()
        return httpx.Response(
            200,
            json={
                "model": "jev-1.13.0",
                "answers": {"ok": {"type": "noul", "noul": 0.91}},
                "usage": {"input_tokens": 4, "output_tokens": 0},
            },
        )

    client = SystemOneHTTPClient(
        "jev",
        "https://api.typesafe.ai",
        "secret",
        "jev-latest",
        transport=httpx.MockTransport(handler),
    )
    result = client.decide("hello", {"ok": {"type": "noul", "instructions": "Is this non-empty?"}})
    assert result.provider == "jev"
    assert result.model == "jev-1.13.0"
    assert result.answers["ok"]["noul"] == 0.91
    assert captured["url"] == "https://api.typesafe.ai/v1/systemone"
    assert captured["auth"] == "Bearer secret"
    assert '"model": "jev-latest"' in captured["body"] or '"model":"jev-latest"' in captured["body"].replace(" ", "")


def test_laya_http_uses_the_same_contract() -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        assert str(request.url) == "http://127.0.0.1:8081/v1/systemone"
        assert "authorization" not in request.headers
        return httpx.Response(
            200,
            json={"model": "laya", "answers": {"dept": {"choice": "billing", "probabilities": {"billing": 0.8}, "confidence": 0.7}}},
        )

    client = build_client(
        DecisionSettings(provider="laya", base_url="http://127.0.0.1:8081/", model="convaiinnovations/laya")
    )
    assert isinstance(client, SystemOneHTTPClient)
    client._transport = httpx.MockTransport(handler)
    result = client.decide(
        "refund please",
        {
            "dept": {
                "type": "choice",
                "instructions": "Which desk?",
                "criteria": {"billing": "payments", "other": "else"},
            }
        },
    )
    assert result.answers["dept"]["choice"] == "billing"
    assert result.answers["dept"]["type"] == "choice"


def test_laya_without_base_url_stays_local() -> None:
    client = build_client(DecisionSettings(provider="laya", model=""))
    assert isinstance(client, LayaLocalClient)
    assert client.model == "convaiinnovations/laya"


def test_invalid_question_does_not_call_http() -> None:
    called = {"n": 0}

    def handler(request: httpx.Request) -> httpx.Response:
        called["n"] += 1
        return httpx.Response(500, json={"detail": "nope"})

    client = SystemOneHTTPClient(
        "jev",
        "https://api.typesafe.ai",
        "secret",
        "jev-latest",
        transport=httpx.MockTransport(handler),
    )
    try:
        client.decide("x", {"bad": {"type": "choice", "instructions": "pick"}})
    except DecisionError as exc:
        assert "criteria" in str(exc)
    else:
        raise AssertionError("expected DecisionError")
    assert called["n"] == 0


def test_jev_requires_api_key() -> None:
    client = build_client(DecisionSettings(provider="jev", api_key="", model="jev-latest"))
    try:
        client.decide("x", {"ok": {"type": "noul", "instructions": "yes?"}})
    except DecisionError as exc:
        assert "API key" in str(exc)
    else:
        raise AssertionError("expected DecisionError")
