"""Unit tests for model_compat (no pytest required — runnable as script)."""

from __future__ import annotations

import os
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

from app.model_compat import (  # noqa: E402
    adapt_chat_payload,
    detect_family,
    postprocess_text,
    profile_for,
)


def _ok(cond: bool, msg: str) -> None:
    if not cond:
        raise AssertionError(msg)
    print(f"  OK  {msg}")


def _clear_env() -> None:
    for key in (
        "OPENAI_MODEL_FAMILY",
        "OPENAI_REASONING_EFFORT",
        "OPENAI_REASONING_EFFORT_WITH_TOOLS",
        "OPENAI_TOOL_CHOICE_AUTO",
    ):
        os.environ.pop(key, None)


def test_detect_family() -> None:
    _ok(detect_family("gpt-4o") == "openai_chat", "gpt-4o → openai_chat")
    _ok(detect_family("gpt-4.1") == "openai_chat", "gpt-4.1 → openai_chat")
    _ok(detect_family("gpt-3.5-turbo") == "openai_chat", "gpt-3.5 → openai_chat")
    _ok(detect_family("chatgpt-4o-latest") == "openai_chat", "chatgpt-* → openai_chat")

    _ok(detect_family("gpt-6-luna") == "openai_reasoning", "gpt-6-luna → openai_reasoning")
    _ok(detect_family("o1-preview") == "openai_reasoning", "o1 → openai_reasoning")
    _ok(detect_family("o3-mini") == "openai_reasoning", "o3 → openai_reasoning")
    _ok(detect_family("gpt-5") == "openai_reasoning", "gpt-5 → openai_reasoning")
    _ok(
        detect_family("my-model-thinking") == "openai_reasoning",
        "*-thinking* → openai_reasoning",
    )
    _ok(
        detect_family("foo-reasoning-bar") == "openai_reasoning",
        "reasoning → openai_reasoning",
    )

    _ok(detect_family("claude-3-5-sonnet") == "anthropic", "claude → anthropic")
    _ok(detect_family("Qwen3-32B-AWQ") == "qwen", "Qwen3-32B-AWQ → qwen")
    _ok(detect_family("qwq-32b") == "qwen", "qwq → qwen")
    _ok(detect_family("deepseek-chat") == "deepseek", "deepseek-chat → deepseek")

    _ok(
        detect_family("opaque", "https://api.anthropic.com/v1") == "anthropic",
        "URL anthropic.com → anthropic",
    )
    _ok(
        detect_family("opaque", "https://dashscope.aliyuncs.com/compatible-mode/v1")
        == "qwen",
        "URL dashscope → qwen",
    )
    _ok(
        detect_family("opaque", "https://api.deepseek.com/v1") == "deepseek",
        "URL deepseek.com → deepseek",
    )


def test_profiles_and_adapt() -> None:
    p = profile_for("gpt-6-luna")
    _ok(p.family == "openai_reasoning", "luna family")
    _ok(p.reasoning_effort_with_tools == "none", "luna reasoning_effort_with_tools=none")
    _ok(p.drop_temperature_with_tools is True, "luna drop_temperature_with_tools")
    _ok(p.supports_tools is True, "luna supports_tools")

    out = adapt_chat_payload(
        {
            "model": "gpt-6-luna",
            "messages": [],
            "tools": [{"type": "function"}],
            "tool_choice": "auto",
            "temperature": 0.7,
            "top_p": 0.9,
        },
        has_tools=True,
        profile=p,
    )
    _ok(out.get("reasoning_effort") == "none", "luna+tools → reasoning_effort none")
    _ok("temperature" not in out and "top_p" not in out, "luna+tools drops temperature/top_p")
    _ok(out.get("tool_choice") == "auto", "luna keeps tool_choice=auto")

    p4 = profile_for("gpt-4o")
    out4 = adapt_chat_payload(
        {"model": "gpt-4o", "messages": [], "tools": [{"type": "function"}]},
        has_tools=True,
        profile=p4,
    )
    _ok("reasoning_effort" not in out4, "gpt-4o+tools → no reasoning_effort key")

    pq = profile_for("Qwen3-32B-AWQ")
    _ok(pq.family == "qwen", "qwen family")
    _ok(pq.strip_think_tags is True, "qwen strip_think_tags")
    _ok(pq.tool_choice_auto is False, "qwen tool_choice_auto False")
    outq = adapt_chat_payload(
        {
            "model": "Qwen3-32B-AWQ",
            "messages": [],
            "tools": [{"type": "function"}],
            "tool_choice": "auto",
        },
        has_tools=True,
        profile=pq,
    )
    _ok("tool_choice" not in outq, "qwen omits tool_choice=auto")

    text = "hello <think>secret</think> world"
    cleaned = postprocess_text(text, pq)
    _ok("<think>" not in cleaned and "hello" in cleaned and "world" in cleaned, "strip think")
    _ok(postprocess_text(text, p4) == text, "gpt-4o does not strip think")


def test_env_overrides() -> None:
    os.environ["OPENAI_REASONING_EFFORT_WITH_TOOLS"] = ""
    p = profile_for("gpt-6-luna")
    _ok(p.reasoning_effort_with_tools is None, "empty WITH_TOOLS → omit")
    out = adapt_chat_payload(
        {"model": "gpt-6-luna", "messages": [], "tools": [{"type": "function"}]},
        has_tools=True,
        profile=p,
    )
    _ok("reasoning_effort" not in out, "never send empty reasoning_effort")
    os.environ.pop("OPENAI_REASONING_EFFORT_WITH_TOOLS", None)

    os.environ["OPENAI_MODEL_FAMILY"] = "openai_reasoning"
    p2 = profile_for("gpt-4o")
    _ok(p2.family == "openai_reasoning", "OPENAI_MODEL_FAMILY force")
    _ok(p2.reasoning_effort_with_tools == "none", "forced family gets luna defaults")
    os.environ.pop("OPENAI_MODEL_FAMILY", None)

    os.environ["OPENAI_TOOL_CHOICE_AUTO"] = "1"
    pq = profile_for("Qwen3-32B-AWQ")
    _ok(pq.tool_choice_auto is True, "OPENAI_TOOL_CHOICE_AUTO=1")
    os.environ.pop("OPENAI_TOOL_CHOICE_AUTO", None)

    os.environ["OPENAI_REASONING_EFFORT"] = "medium"
    p3 = profile_for("gpt-6-luna")
    out3 = adapt_chat_payload(
        {"model": "gpt-6-luna", "messages": []},
        has_tools=False,
        profile=p3,
    )
    _ok(out3.get("reasoning_effort") == "medium", "no-tools uses REASONING_EFFORT default")
    os.environ.pop("OPENAI_REASONING_EFFORT", None)


def main() -> None:
    print("test_model_compat")
    _clear_env()
    test_detect_family()
    _clear_env()
    test_profiles_and_adapt()
    _clear_env()
    test_env_overrides()
    _clear_env()
    print("all passed")


if __name__ == "__main__":
    main()
