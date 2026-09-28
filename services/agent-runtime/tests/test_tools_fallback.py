"""Unit tests for vLLM auto-tool-choice fallback (no pytest — runnable as script)."""

from __future__ import annotations

import asyncio
import os
import sys
from pathlib import Path
from typing import Any
from unittest.mock import AsyncMock, patch

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

from app.llm import (  # noqa: E402
    AutoToolChoiceUnsupported,
    LLMOverride,
    is_auto_tool_choice_unsupported,
    run_tool_loop,
)


def _ok(cond: bool, msg: str) -> None:
    if not cond:
        raise AssertionError(msg)
    print(f"  OK  {msg}")


def test_detector() -> None:
    msg = (
        '"auto" tool choice requires --enable-auto-tool-choice '
        "and --tool-call-parser to be set"
    )
    _ok(is_auto_tool_choice_unsupported(msg), "vLLM classic auto tool choice 400")
    _ok(
        is_auto_tool_choice_unsupported(
            "upstream HTTP 400: enable_auto_tool_choice missing"
        ),
        "underscore form",
    )
    _ok(
        is_auto_tool_choice_unsupported("need --tool-call-parser hermes"),
        "tool-call-parser alone",
    )
    _ok(not is_auto_tool_choice_unsupported("model not found"), "unrelated error")
    _ok(not is_auto_tool_choice_unsupported(""), "empty")


async def test_run_tool_loop_fallback() -> None:
    os.environ["OPENAI_ENABLE_TOOLS"] = "1"
    os.environ.pop("OPENAI_TOOL_CHOICE_AUTO", None)

    calls: list[dict[str, Any]] = []
    statuses: list[dict[str, Any]] = []

    async def fake_chat(
        messages: list[dict[str, Any]],
        *,
        api_key: str,
        tools: list[dict[str, Any]] | None = None,
        tool_choice: str | dict | None = None,
        override: LLMOverride | None = None,
    ) -> dict[str, Any]:
        calls.append({"tools": tools, "tool_choice": tool_choice})
        if tools:
            raise AutoToolChoiceUnsupported(
                'upstream HTTP 400: "auto" tool choice requires '
                "--enable-auto-tool-choice and --tool-call-parser to be set"
            )
        return {
            "choices": [
                {"message": {"role": "assistant", "content": "你好，这是无 tools 回退答案"}}
            ],
            "usage": {"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
        }

    async def on_status(payload: dict[str, Any]) -> None:
        statuses.append(payload)

    async def tool_handler(name: str, args: dict[str, Any]) -> str:
        raise AssertionError(f"tools should not run, got {name}")

    with patch("app.llm.chat_completion", new=AsyncMock(side_effect=fake_chat)):
        with patch(
            "app.llm.openai_config",
            return_value=("sk-test", "http://192.168.5.34:30632/v1", "Qwen3-32B-AWQ"),
        ):
            final, used, usage = await run_tool_loop(
                [{"role": "user", "content": "hi"}],
                api_key="sk-test",
                tool_handler=tool_handler,
                override=LLMOverride(enable_tools=True),
                on_status=on_status,
            )

    _ok(len(calls) == 2, f"two chat_completion calls (got {len(calls)})")
    _ok(calls[0]["tools"] is not None, "first call sent tools")
    _ok(calls[1]["tools"] is None, "retry without tools")
    _ok(final == "你好，这是无 tools 回退答案", f"fallback text (got {final!r})")
    _ok(used == [], f"no tools used (got {used})")
    _ok(
        any(s.get("reason") == "auto_tool_choice_unsupported" for s in statuses),
        f"status note emitted (got {statuses})",
    )
    _ok(usage is not None and usage.get("total_tokens") == 8, f"usage accrued ({usage})")


def main() -> None:
    print("test_tools_fallback")
    test_detector()
    asyncio.run(test_run_tool_loop_fallback())
    print("all passed")


if __name__ == "__main__":
    main()
