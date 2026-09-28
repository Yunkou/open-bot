"""Multi-model compatibility for OpenAI-compatible /v1/chat/completions gateways.

Detects model family (GPT chat / reasoning, Claude, Qwen, DeepSeek, generic compat)
and adapts payloads (reasoning_effort, tool_choice, temperature) plus light
post-processing (strip <think> tags).
"""

from __future__ import annotations

import os
import re
from copy import deepcopy
from dataclasses import dataclass
from typing import Any


def strip_think_tags(text: str) -> str:
    """Remove Qwen-style <think>...</think> blocks from model output."""
    if not text:
        return text
    cleaned = re.sub(r"<think>[\s\S]*?</think>", "", text, flags=re.IGNORECASE)
    return cleaned.strip() or text.strip()


def detect_family(model: str, base_url: str = "") -> str:
    """Normalize model + base_url to a family key.

    Families: openai_chat | openai_reasoning | anthropic | qwen | deepseek | compat
    """
    m = (model or "").strip().lower()
    u = (base_url or "").strip().lower()

    # URL hints (opaque model ids on known gateways)
    if "anthropic.com" in u:
        return "anthropic"
    if "dashscope" in u or "aliyuncs.com" in u:
        return "qwen"
    if "deepseek.com" in u:
        return "deepseek"
    if "api.openai.com" in u and not m:
        return "openai_chat"

    if not m:
        return "compat"

    # Vendor substrings
    if m.startswith("claude") or "claude" in m:
        return "anthropic"
    if m.startswith("qwen") or m.startswith("qwq") or "qwen" in m or "qwq" in m:
        return "qwen"
    if m.startswith("deepseek") or "deepseek" in m:
        return "deepseek"

    # OpenAI reasoning / thinking (before generic gpt-*)
    if (
        m.startswith(("o1", "o3", "o4"))
        or m.startswith("gpt-5")
        or "luna" in m
        or "-thinking" in m
        or "reasoning" in m
    ):
        return "openai_reasoning"

    # OpenAI chat
    if (
        m.startswith("gpt-4")
        or m.startswith("gpt-3.5")
        or m.startswith("gpt-3")
        or m.startswith("chatgpt")
        or (m.startswith("gpt-") and "luna" not in m and not m.startswith("gpt-5"))
    ):
        return "openai_chat"

    return "compat"


@dataclass
class ModelProfile:
    family: str
    supports_tools: bool = True
    tool_choice_auto: bool = True
    reasoning_effort_with_tools: str | None = None
    reasoning_effort_default: str | None = None
    strip_think_tags: bool = False
    drop_temperature_with_tools: bool = False
    notes: str = ""


def _default_profile(family: str) -> ModelProfile:
    if family == "openai_chat":
        return ModelProfile(
            family=family,
            supports_tools=True,
            tool_choice_auto=True,
            reasoning_effort_with_tools=None,
            reasoning_effort_default=None,
            strip_think_tags=False,
            drop_temperature_with_tools=False,
            notes="OpenAI chat (gpt-4o / gpt-4.1 / chatgpt)",
        )
    if family == "openai_reasoning":
        return ModelProfile(
            family=family,
            supports_tools=True,
            tool_choice_auto=True,
            reasoning_effort_with_tools="none",
            reasoning_effort_default=None,
            strip_think_tags=False,
            drop_temperature_with_tools=True,
            notes="OpenAI reasoning (o1/o3/gpt-5/luna); tools need reasoning_effort=none",
        )
    if family == "anthropic":
        return ModelProfile(
            family=family,
            supports_tools=True,
            tool_choice_auto=True,
            reasoning_effort_with_tools=None,
            reasoning_effort_default=None,
            strip_think_tags=False,
            drop_temperature_with_tools=False,
            notes="Claude via OpenAI-compatible gateway (not native Messages API)",
        )
    if family == "qwen":
        return ModelProfile(
            family=family,
            supports_tools=True,
            tool_choice_auto=False,
            reasoning_effort_with_tools=None,
            reasoning_effort_default=None,
            strip_think_tags=True,
            drop_temperature_with_tools=False,
            notes="Qwen / DashScope / vLLM; omit tool_choice=auto; strip <think>",
        )
    if family == "deepseek":
        return ModelProfile(
            family=family,
            supports_tools=True,
            tool_choice_auto=True,
            reasoning_effort_with_tools=None,
            reasoning_effort_default=None,
            strip_think_tags=True,
            drop_temperature_with_tools=False,
            notes="DeepSeek; strip <think> if present",
        )
    return ModelProfile(
        family="compat",
        supports_tools=True,
        tool_choice_auto=False,
        reasoning_effort_with_tools=None,
        reasoning_effort_default=None,
        strip_think_tags=True,
        drop_temperature_with_tools=False,
        notes="Generic OpenAI-compatible / vLLM; omit tool_choice=auto",
    )


def profile_for(model: str, base_url: str = "") -> ModelProfile:
    """Build a ModelProfile with env overrides applied."""
    forced = (os.getenv("OPENAI_MODEL_FAMILY") or "").strip().lower()
    if forced in (
        "openai_chat",
        "openai_reasoning",
        "anthropic",
        "qwen",
        "deepseek",
        "compat",
    ):
        family = forced
    else:
        family = detect_family(model, base_url)

    profile = _default_profile(family)

    if "OPENAI_REASONING_EFFORT" in os.environ:
        general = (os.getenv("OPENAI_REASONING_EFFORT") or "").strip()
        profile.reasoning_effort_default = general or None
    if "OPENAI_REASONING_EFFORT_WITH_TOOLS" in os.environ:
        with_tools = (os.getenv("OPENAI_REASONING_EFFORT_WITH_TOOLS") or "").strip()
        profile.reasoning_effort_with_tools = with_tools or None

    if "OPENAI_TOOL_CHOICE_AUTO" in os.environ:
        v = (os.getenv("OPENAI_TOOL_CHOICE_AUTO") or "").strip().lower()
        if v in ("0", "false", "no", "off"):
            profile.tool_choice_auto = False
        elif v in ("1", "true", "yes", "on"):
            profile.tool_choice_auto = True

    return profile


def adapt_chat_payload(
    payload: dict[str, Any],
    *,
    has_tools: bool,
    profile: ModelProfile,
) -> dict[str, Any]:
    """Copy and adapt a chat/completions payload for the model profile."""
    out = deepcopy(payload)

    if has_tools and profile.reasoning_effort_with_tools:
        out["reasoning_effort"] = profile.reasoning_effort_with_tools
    elif (not has_tools) and profile.reasoning_effort_default:
        out["reasoning_effort"] = profile.reasoning_effort_default

    # Never send empty reasoning_effort
    if "reasoning_effort" in out and not str(out.get("reasoning_effort") or "").strip():
        out.pop("reasoning_effort", None)

    if has_tools and not profile.tool_choice_auto:
        if out.get("tool_choice") == "auto":
            out.pop("tool_choice", None)

    if has_tools and profile.drop_temperature_with_tools:
        out.pop("temperature", None)
        out.pop("top_p", None)

    return out


def postprocess_text(text: str, profile: ModelProfile) -> str:
    """Post-process assistant text according to profile flags."""
    if not text:
        return text
    if profile.strip_think_tags:
        return strip_think_tags(text)
    return text
