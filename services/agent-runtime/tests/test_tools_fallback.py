"""Unit tests for vLLM auto-tool-choice fallback (no pytest — runnable as script)."""

from __future__ import annotations

import asyncio
import json
import os
import sys
from pathlib import Path
from typing import Any
from unittest.mock import AsyncMock, patch

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

from app.llm import (  # noqa: E402
    CHEAPER_HOST_RETRY,
    AutoToolChoiceUnsupported,
    LLMOverride,
    guide_tool_result,
    is_auto_tool_choice_unsupported,
    run_tool_loop,
    tool_result_fallback,
)
from app.machines import HOST_EXEC_TIMEOUT_SEC  # noqa: E402


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


async def test_run_tool_loop_markup_after_fallback() -> None:
    os.environ["OPENAI_ENABLE_TOOLS"] = "1"
    calls: list[dict[str, Any]] = []
    ran: list[str] = []
    n = {"i": 0}

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
                'upstream HTTP 400: "auto" tool choice requires --enable-auto-tool-choice'
            )
        n["i"] += 1
        if n["i"] == 1:
            return {
                "choices": [
                    {
                        "message": {
                            "role": "assistant",
                            "content": (
                                "<tool_call>\n"
                                "<function=list_machines>\n"
                                "</function>\n"
                                "</tool_call>"
                            ),
                        }
                    }
                ],
                "usage": {"prompt_tokens": 1, "completion_tokens": 2, "total_tokens": 3},
            }
        return {
            "choices": [{"message": {"role": "assistant", "content": "目前没有已连接电脑"}}],
            "usage": {"prompt_tokens": 4, "completion_tokens": 5, "total_tokens": 9},
        }

    async def tool_handler(name: str, args: dict[str, Any]) -> str:
        ran.append(name)
        return '{"machines":[],"count":0}'

    with patch("app.llm.chat_completion", new=AsyncMock(side_effect=fake_chat)):
        with patch(
            "app.llm.openai_config",
            return_value=("sk-test", "http://192.168.5.34:30632/v1", "Qwen3-32B-AWQ"),
        ):
            final, used, _usage = await run_tool_loop(
                [{"role": "user", "content": "我的设备有哪些"}],
                api_key="sk-test",
                tool_handler=tool_handler,
                override=LLMOverride(enable_tools=True),
            )

    _ok(ran == ["list_machines"], f"markup tool ran (got {ran})")
    _ok(used == ["list_machines"], f"used recorded (got {used})")
    _ok(final == "目前没有已连接电脑", f"final text (got {final!r})")
    _ok(calls[0]["tools"] is not None and calls[1]["tools"] is None, "fallback then markup")


async def test_tool_timeout_reasoning_only_is_visible() -> None:
    """A timed-out tool plus a think-only follow-up must not end the turn blank."""
    os.environ["OPENAI_ENABLE_TOOLS"] = "1"
    calls = {"n": 0}

    async def fake_chat(
        messages: list[dict[str, Any]],
        *,
        api_key: str,
        tools: list[dict[str, Any]] | None = None,
        tool_choice: str | dict | None = None,
        override: LLMOverride | None = None,
    ) -> dict[str, Any]:
        calls["n"] += 1
        if calls["n"] == 1:
            return {
                "choices": [
                    {
                        "message": {
                            "role": "assistant",
                            "content": None,
                            "tool_calls": [
                                {
                                    "id": "call_1",
                                    "type": "function",
                                    "function": {
                                        "name": "host_shell",
                                        "arguments": '{"command":"find ~/Downloads -exec stat {} \\;"}',
                                    },
                                }
                            ],
                        }
                    }
                ],
                "usage": {"prompt_tokens": 10, "completion_tokens": 8, "total_tokens": 18},
            }
        return {
            "choices": [
                {
                    "message": {
                        "role": "assistant",
                        "content": "<think>命令超时了，换更快的 find</think>",
                    }
                }
            ],
            "usage": {"prompt_tokens": 20, "completion_tokens": 12, "total_tokens": 32},
        }

    async def tool_handler(name: str, args: dict[str, Any]) -> str:
        _ok(name == "host_shell", f"tool name {name}")
        return '{"ok": false, "error": "命令超过 30 秒还没结束"}'

    with patch("app.llm.chat_completion", new=AsyncMock(side_effect=fake_chat)):
        with patch(
            "app.llm.openai_config",
            return_value=("sk-test", "http://127.0.0.1:9/v1", "Qwen3-32B-AWQ"),
        ):
            final, used, _usage = await run_tool_loop(
                [{"role": "user", "content": "看看最不常用的大文件"}],
                api_key="sk-test",
                tool_handler=tool_handler,
                max_rounds=4,
                override=LLMOverride(enable_tools=True),
            )

    _ok(used == ["host_shell"], f"used {used}")
    _ok(final and "<think>" not in final, f"visible fallback (got {final!r})")
    _ok("超过时限" in final, f"timeout strategy text (got {final!r})")
    _ok(calls["n"] == 2, f"no extra completion after think-only (got {calls['n']})")



def test_host_waits_stay_above_desktop_shell() -> None:
    """Runtime HTTP must outlast API hostExecTimeout (150s), which outlasts desktop 120s."""
    _ok(HOST_EXEC_TIMEOUT_SEC > 150, f"runtime wait {HOST_EXEC_TIMEOUT_SEC} cuts before the API")


def test_guide_on_timeout_and_shell_error() -> None:
    timed = guide_tool_result("host_shell", '{"ok": false, "error": "命令超过 120 秒还没结束"}')
    _ok("不要重复同一条命令" in timed, "timeout says do not repeat")
    _ok("只重试一次" in timed, "timeout says retry once")
    _ok("host-file-query" in timed, "timeout names the skill")
    _ok("-exec stat" in timed, "timeout forbids per-file stat")
    failed = guide_tool_result("host_shell", '{"ok": false, "error": "启动失败"}')
    _ok(CHEAPER_HOST_RETRY in failed, "shell error gets the same guidance")
    ok = guide_tool_result("host_shell", '{"ok": true, "output": "a"}')
    _ok("不要重复" not in ok, "success is unchanged")
    denied = guide_tool_result("host_shell", '{"ok": false, "denied": true, "error": "用户拒绝了这次操作"}')
    _ok("host-file-query" not in denied, "a refusal is not a cheaper-find retry")
    offline = guide_tool_result("host_shell", '{"ok": false, "error": "应用没开着"}')
    _ok("host-file-query" not in offline, "offline machine is not a find retry")
    ls = guide_tool_result("host_ls", '{"ok": false, "error": "没有在时限内完成或确认这次操作"}')
    _ok("不要重复同一条命令" in ls, "host timeout other than shell still guides")
    other = guide_tool_result("sandbox_shell", '{"ok": false, "error": "timeout"}')
    _ok("host-file-query" not in other, "non-host tools are not given the file-query hint")


def _shell_call(call_id: str, command: str) -> dict[str, Any]:
    return {
        "choices": [
            {
                "message": {
                    "role": "assistant",
                    "content": None,
                    "tool_calls": [
                        {
                            "id": call_id,
                            "type": "function",
                            "function": {
                                "name": "host_shell",
                                "arguments": '{"command": ' + __import__('json').dumps(command) + '}',
                            },
                        }
                    ],
                }
            }
        ]
    }


async def test_timeout_allows_another_tool_round() -> None:
    """A timed-out host_shell is a result, not the end of the turn."""
    os.environ["OPENAI_ENABLE_TOOLS"] = "1"
    seen: list[str] = []
    ran: list[str] = []

    async def fake_chat(
        messages: list[dict[str, Any]],
        *,
        api_key: str,
        tools: list[dict[str, Any]] | None = None,
        tool_choice: str | dict | None = None,
        override: LLMOverride | None = None,
    ) -> dict[str, Any]:
        n = len(seen) + 1
        seen.append("tools" if tools else "plain")
        if n == 1:
            return _shell_call("call_slow", "find ~/Downloads -exec stat {} \\;")
        if n == 2:
            tool_body = next(m["content"] for m in reversed(messages) if m.get("role") == "tool")
            _ok("不要重复同一条命令" in tool_body, "model sees do-not-repeat")
            _ok("host-file-query" in tool_body, "model sees the skill")
            _ok("-exec stat" in tool_body, "model sees no per-file stat")
            _ok(tools is not None, "follow-up round still has tools")
            return _shell_call("call_cheap", "find ~/Downloads -type f -print")
        return {"choices": [{"message": {"role": "assistant", "content": "最大的是 a.mp4"}}]}

    async def tool_handler(name: str, args: dict[str, Any]) -> str:
        ran.append(str(args.get("command") or ""))
        if len(ran) == 1:
            return '{"ok": false, "error": "命令超过 120 秒还没结束"}'
        return '{"ok": true, "output": "a.mp4"}'

    with patch("app.llm.chat_completion", new=AsyncMock(side_effect=fake_chat)):
        with patch(
            "app.llm.openai_config",
            return_value=("sk-test", "http://127.0.0.1:9/v1", "Qwen3-32B-AWQ"),
        ):
            final, used, _usage = await run_tool_loop(
                [{"role": "user", "content": "看看 Downloads 里最大的文件"}],
                api_key="sk-test",
                tool_handler=tool_handler,
                max_rounds=4,
                override=LLMOverride(enable_tools=True),
            )

    _ok(used == ["host_shell", "host_shell"], f"second tool ran (got {used})")
    _ok(ran[1] == "find ~/Downloads -type f -print", f"cheaper command (got {ran})")
    _ok(final == "最大的是 a.mp4", f"turn finished after the retry (got {final!r})")
    _ok(seen == ["tools", "tools", "tools"], f"did not drop tools after timeout (got {seen})")


async def test_tool_exception_still_continues() -> None:
    os.environ["OPENAI_ENABLE_TOOLS"] = "1"
    calls = {"n": 0}

    async def fake_chat(
        messages: list[dict[str, Any]],
        *,
        api_key: str,
        tools: list[dict[str, Any]] | None = None,
        tool_choice: str | dict | None = None,
        override: LLMOverride | None = None,
    ) -> dict[str, Any]:
        calls["n"] += 1
        if calls["n"] == 1:
            return _shell_call("call_err", "find ~/Downloads -exec stat {} \\;")
        tool_body = next(m["content"] for m in reversed(messages) if m.get("role") == "tool")
        _ok("boom" in tool_body and "不要重复同一条命令" in tool_body, "exception is a guided result")
        return {"choices": [{"message": {"role": "assistant", "content": "换个查法失败了，先停一下"}}]}

    async def tool_handler(name: str, args: dict[str, Any]) -> str:
        raise RuntimeError("boom")

    with patch("app.llm.chat_completion", new=AsyncMock(side_effect=fake_chat)):
        with patch(
            "app.llm.openai_config",
            return_value=("sk-test", "http://127.0.0.1:9/v1", "Qwen3-32B-AWQ"),
        ):
            final, used, _usage = await run_tool_loop(
                [{"role": "user", "content": "查一下"}],
                api_key="sk-test",
                tool_handler=tool_handler,
                max_rounds=4,
                override=LLMOverride(enable_tools=True),
            )

    _ok(used == ["host_shell"], f"used {used}")
    _ok(final == "换个查法失败了，先停一下", f"loop survived the exception (got {final!r})")
    _ok(calls["n"] == 2, f"second round happened (got {calls['n']})")



async def test_success_stdout_think_only_is_quoted() -> None:
    """A successful tool plus a think-only follow-up must show the stdout, not an apology."""
    os.environ["OPENAI_ENABLE_TOOLS"] = "1"
    calls = {"n": 0}
    listing = "440M output.wav\n422M 择日飞升 第05集.ts\n"

    async def fake_chat(
        messages: list[dict[str, Any]],
        *,
        api_key: str,
        tools: list[dict[str, Any]] | None = None,
        tool_choice: str | dict | None = None,
        override: LLMOverride | None = None,
    ) -> dict[str, Any]:
        calls["n"] += 1
        if calls["n"] == 1:
            return _shell_call("call_ok", "find ~/Downloads -name '*.ts' -o -name '*.mp4'")
        return {
            "choices": [
                {
                    "message": {
                        "role": "assistant",
                        "content": "<think>结果已经有了，整理成列表</think>",
                    }
                }
            ]
        }

    async def tool_handler(name: str, args: dict[str, Any]) -> str:
        return json.dumps(
            {
                "ok": True,
                "exit_code": 0,
                "command": "find ~/Downloads -name '*.ts'",
                "output": "440M output.wav\n422M 择日飞升 第05集.ts\n",
            },
            ensure_ascii=False,
        )
    with patch("app.llm.chat_completion", new=AsyncMock(side_effect=fake_chat)):
        with patch(
            "app.llm.openai_config",
            return_value=("sk-test", "http://127.0.0.1:9/v1", "Qwen3-32B-AWQ"),
        ):
            final, used, _usage = await run_tool_loop(
                [{"role": "user", "content": "看看我电脑上有哪些视频文件"}],
                api_key="sk-test",
                tool_handler=tool_handler,
                max_rounds=4,
                override=LLMOverride(enable_tools=True),
            )

    _ok(used == ["host_shell"], f"used {used}")
    _ok(final and "<think>" not in final, f"think stripped (got {final!r})")
    _ok("output.wav" in final and "择日飞升" in final, f"stdout quoted (got {final!r})")
    _ok("没能整理成可见回复" not in final, f"no apology (got {final!r})")
    _ok("更窄的命令" not in final, f"no narrower-retry on success (got {final!r})")
    _ok(calls["n"] == 2, f"no extra completion (got {calls['n']})")
    _ok(listing.splitlines()[0] in final, "first line kept")


def test_quote_success_failure_and_truncation() -> None:
    success = tool_result_fallback(
        [
            {
                "role": "tool",
                "content": json.dumps(
                    {
                        "ok": True,
                        "output": "find: $: unknown primary or operator\n",
                        "stderr": "find: $: unknown primary or operator",
                    },
                    ensure_ascii=False,
                ),
            }
        ]
    )
    _ok("unknown primary" in success, f"quotes stdout (got {success!r})")
    _ok("更窄" not in success and "没能整理" not in success, f"success is not an apology (got {success!r})")
    _ok(success.count("unknown primary") == 1, "stderr duplicate dropped")

    empty = tool_result_fallback([{"role": "tool", "content": '{"ok": true, "output": ""}'}])
    _ok("没有可显示的输出" in empty and "更窄" not in empty, f"empty success (got {empty!r})")

    failed = tool_result_fallback(
        [{"role": "tool", "content": '{"ok": false, "error": "启动失败", "output": "partial"}'}]
    )
    _ok("工具没有成功" in failed and "更窄的命令" in failed, f"failure still offers a narrower retry (got {failed!r})")
    _ok("partial" in failed, f"failure still shows output (got {failed!r})")

    timed = tool_result_fallback(
        [{"role": "tool", "content": '{"ok": false, "error": "命令超过 30 秒还没结束", "output": "half"}'}]
    )
    _ok("超过时限" in timed and "half" not in timed, f"timeout stays the strategy text (got {timed!r})")

    huge = "x" * 5000
    quoted = tool_result_fallback(
        [{"role": "tool", "content": '{"ok": true, "command": "find ~", "output": "' + huge + '"}'}]
    )
    _ok("输出过长，已截断" in quoted, f"huge stdout truncated (got len {len(quoted)})")
    _ok(len(quoted) < 4500, f"quote bounded (got {len(quoted)})")
    _ok("更窄" not in quoted, "truncation is not a retry hint")


def main() -> None:
    print("test_tools_fallback")
    test_detector()
    asyncio.run(test_run_tool_loop_fallback())
    asyncio.run(test_run_tool_loop_markup_after_fallback())
    asyncio.run(test_tool_timeout_reasoning_only_is_visible())
    asyncio.run(test_success_stdout_think_only_is_quoted())
    test_quote_success_failure_and_truncation()
    test_host_waits_stay_above_desktop_shell()
    test_guide_on_timeout_and_shell_error()
    asyncio.run(test_timeout_allows_another_tool_round())
    asyncio.run(test_tool_exception_still_continues())
    print("all passed")


if __name__ == "__main__":
    main()
