"""Unit tests for token-aware compaction (pytest or script)."""

from __future__ import annotations

import asyncio
import os
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

from app import compact as c  # noqa: E402


def _clear_compact_env() -> None:
    for key in (
        "COMPACT_MAX_MESSAGES",
        "COMPACT_MAX_CHARS",
        "COMPACT_KEEP_RECENT",
        "COMPACT_DEFAULT_CONTEXT_WINDOW",
        "COMPACT_RESERVE_OUTPUT_TOKENS",
        "COMPACT_BUDGET_RATIO",
    ):
        os.environ.pop(key, None)


def test_guess_context_window() -> None:
    assert c.guess_context_window("gpt-4o") == 128_000
    assert c.guess_context_window("GPT-4.1-mini") == 128_000
    assert c.guess_context_window("claude-3-5-sonnet") == 200_000
    assert c.guess_context_window("claude-sonnet-4-20250514") == 200_000
    assert c.guess_context_window("qwen2.5-32k") == 32_768
    assert c.guess_context_window("foo-128k-bar") == 131_072
    assert c.guess_context_window("gpt-3.5-turbo") == 16_384
    assert c.guess_context_window("totally-unknown-xyz") == 32_768
    assert c.resolve_context_window(4096, "gpt-4o") == 4096
    assert c.resolve_context_window(None, "gpt-4o") == 128_000
    assert c.resolve_context_window(0, "gpt-4o") == 128_000
    assert c.clamp_context_window(100) == 4096
    assert c.clamp_context_window(9_999_999) == 1_000_000


def test_estimate_tokens_monotonic_nonzero() -> None:
    assert c.estimate_tokens("") == 0
    a = c.estimate_tokens("hi")
    b = c.estimate_tokens("hi" * 50)
    assert a > 0
    assert b > a
    cjk = c.estimate_tokens("你好世界" * 10)
    assert cjk > 0
    # CJK-heavy text should estimate more than naive ASCII chars/4 for same length
    ascii_same_len = c.estimate_tokens("abcd" * 10)  # 40 chars
    cjk_40 = c.estimate_tokens("你好" * 20)  # 40 chars
    assert cjk_40 >= ascii_same_len


def test_needs_compact_token_budget() -> None:
    _clear_compact_env()
    # Many short messages: under legacy thresholds if we keep counts high
    os.environ["COMPACT_MAX_MESSAGES"] = "100"
    os.environ["COMPACT_MAX_CHARS"] = "100000"
    msgs = [{"role": "user", "content": "hello world " * 20} for _ in range(30)]
    needed, reason = c.needs_compact(msgs, context_window=4096)
    assert needed is True
    assert reason == "token_budget"

    needed2, reason2 = c.needs_compact(msgs, context_window=200_000)
    assert needed2 is False
    assert reason2 == ""


def test_needs_compact_legacy_fallback() -> None:
    _clear_compact_env()
    os.environ["COMPACT_MAX_MESSAGES"] = "5"
    os.environ["COMPACT_MAX_CHARS"] = "100000"
    # Short msgs, huge window → legacy message_count fires
    msgs = [{"role": "user", "content": "x"} for _ in range(8)]
    needed, reason = c.needs_compact(msgs, context_window=200_000)
    assert needed is True
    assert reason == "message_count"


def test_reused_summary_path() -> None:
    _clear_compact_env()
    os.environ["COMPACT_MAX_MESSAGES"] = "100"
    os.environ["COMPACT_MAX_CHARS"] = "100000"

    async def _run() -> None:
        messages = [
            {"role": "summary", "content": "[对话摘要] 之前聊过天气。"},
            {"role": "user", "content": "继续"},
            {"role": "assistant", "content": "好的"},
        ]
        out, meta = await c.compact_messages(
            messages,
            context_window=200_000,
            model="gpt-4o",
        )
        assert meta["compacted"] is True
        assert meta["compact_reason"] == "reused_summary"
        assert meta.get("summary_new") is False
        assert out[0]["role"] == "system"
        assert "天气" in out[0]["content"]

    asyncio.run(_run())


def test_compact_config_exposes_token_knobs() -> None:
    _clear_compact_env()
    cfg = c.compact_config()
    assert cfg["token_mode"] is True
    assert cfg["default_context_window"] == 32_768
    assert cfg["reserve_output_tokens"] == 2048
    assert cfg["budget_ratio"] == 0.75
    assert cfg["token_budget"] > 0


def main() -> None:
    print("test_compact")
    _clear_compact_env()
    test_guess_context_window()
    print("  OK  guess_context_window")
    test_estimate_tokens_monotonic_nonzero()
    print("  OK  estimate_tokens")
    test_needs_compact_token_budget()
    print("  OK  needs_compact token_budget")
    test_needs_compact_legacy_fallback()
    print("  OK  needs_compact legacy")
    test_reused_summary_path()
    print("  OK  reused_summary")
    test_compact_config_exposes_token_knobs()
    print("  OK  compact_config")
    print("all passed")


if __name__ == "__main__":
    main()
