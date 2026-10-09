"""Build a decision client and keep it on the current run."""

from __future__ import annotations

import logging
from contextvars import ContextVar, Token
from typing import Any

from .http_client import JEV_DEFAULT_BASE, JEV_DEFAULT_MODEL, SystemOneHTTPClient
from .laya_local import LAYA_DEFAULT_MODEL, LayaLocalClient
from .protocol import DecisionDisabled, DecisionError, DecisionResult, DecisionSettings, normalize_provider

logger = logging.getLogger(__name__)


class DisabledDecision:
    provider = "off"
    enabled = False

    def decide(self, state: Any, questions: dict[str, Any]) -> DecisionResult:
        raise DecisionDisabled("决策层已关闭")


_disabled = DisabledDecision()
_current: ContextVar[Any] = ContextVar("openbot_decision", default=_disabled)


def build_client(settings: DecisionSettings | None):
    if settings is None:
        return _disabled
    provider = normalize_provider(settings.provider)
    base_url = (settings.base_url or "").strip().rstrip("/")
    model = (settings.model or "").strip()
    api_key = settings.api_key or ""
    if provider == "off":
        return _disabled
    if provider == "jev":
        return SystemOneHTTPClient(
            "jev",
            base_url or JEV_DEFAULT_BASE,
            api_key,
            model or JEV_DEFAULT_MODEL,
        )
    if base_url:
        return SystemOneHTTPClient(
            "laya",
            base_url,
            api_key,
            model or LAYA_DEFAULT_MODEL,
        )
    return LayaLocalClient(model or LAYA_DEFAULT_MODEL)


def bind_decision(settings: DecisionSettings | None) -> Token:
    return _current.set(build_client(settings))


def reset_decision(token: Token) -> None:
    _current.reset(token)


def current_decision():
    return _current.get()


def try_decide(state: Any, questions: dict[str, Any], client: Any | None = None) -> dict[str, Any] | None:
    """Return a decision dict, or None when the layer is off or the call fails.

    Failures are logged and swallowed so a decision outage does not stop the chat turn.
    """
    active = client if client is not None else current_decision()
    if not getattr(active, "enabled", False):
        return None
    try:
        result = active.decide(state, questions)
    except DecisionError as exc:
        logger.warning("decision skipped: %s", exc)
        return None
    if not isinstance(result, DecisionResult):
        return None
    return result.to_dict()
