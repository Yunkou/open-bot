"""Unit/smoke tests for XML/Hermes tool-markup parser (no pytest required)."""

from __future__ import annotations

import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

from app.tool_markup import (  # noqa: E402
    content_has_tool_markup,
    parse_tool_markup,
    strip_tool_markup,
    to_openai_tool_calls,
)


def _ok(cond: bool, msg: str) -> None:
    if not cond:
        raise AssertionError(msg)
    print(f"  OK  {msg}")


def test_bug_format_sandbox_ls() -> None:
    """Exact shape from the user-visible leak."""
    raw = (
        "<tool_call> <function=sandbox_ls> "
        "<parameter=path> / </parameter> </function> </tool_call>"
    )
    _ok(content_has_tool_markup(raw), "detect bug-format markup")
    parsed = parse_tool_markup(raw)
    _ok(len(parsed) == 1, f"one call (got {len(parsed)})")
    _ok(parsed[0].name == "sandbox_ls", f"name sandbox_ls (got {parsed[0].name!r})")
    _ok(parsed[0].arguments.get("path") == "/", f"path=/ (got {parsed[0].arguments!r})")

    cleaned = strip_tool_markup(raw)
    _ok(cleaned == "", f"strip leaves empty (got {cleaned!r})")

    oai = to_openai_tool_calls(parsed)
    _ok(len(oai) == 1 and oai[0]["type"] == "function", "openai shape")
    _ok(oai[0]["function"]["name"] == "sandbox_ls", "openai name")
    args = json.loads(oai[0]["function"]["arguments"])
    _ok(args == {"path": "/"}, f"openai args {args!r}")


def test_multiline_and_mixed_prose() -> None:
    text = (
        "我来看看沙箱目录：\n"
        "<tool_call>\n"
        "<function=sandbox_ls>\n"
        "<parameter=path>/workspace</parameter>\n"
        "</function>\n"
        "</tool_call>\n"
        "（请稍等）"
    )
    parsed = parse_tool_markup(text)
    _ok(len(parsed) == 1, "one call amid prose")
    _ok(parsed[0].name == "sandbox_ls", "name")
    _ok(parsed[0].arguments.get("path") == "/workspace", "path /workspace")
    cleaned = strip_tool_markup(text)
    _ok("<tool_call" not in cleaned.lower(), "no tool_call left")
    _ok("<function=" not in cleaned.lower(), "no function= left")
    _ok("我来看看沙箱目录" in cleaned, "prose kept")


def test_hermes_json_inside_tool_call() -> None:
    text = (
        '<tool_call>\n'
        '{"name": "calculator", "arguments": {"expression": "1+2*3"}}\n'
        "</tool_call>"
    )
    parsed = parse_tool_markup(text)
    _ok(len(parsed) == 1, "json hermes one call")
    _ok(parsed[0].name == "calculator", "calculator")
    _ok(parsed[0].arguments.get("expression") == "1+2*3", "expression")


def test_parameter_name_attr_and_invoke() -> None:
    text = (
        "<tool_call>"
        '<invoke name="sandbox_read">'
        '<parameter name="path">/workspace/a.txt</parameter>'
        "</invoke>"
        "</tool_call>"
    )
    parsed = parse_tool_markup(text)
    _ok(len(parsed) == 1, "invoke one call")
    _ok(parsed[0].name == "sandbox_read", "sandbox_read")
    _ok(parsed[0].arguments.get("path") == "/workspace/a.txt", "path")


def test_fenced_xml() -> None:
    text = (
        "```xml\n"
        "<tool_call><function=get_current_time>"
        "<parameter=timezone>Asia/Shanghai</parameter>"
        "</function></tool_call>\n"
        "```"
    )
    parsed = parse_tool_markup(text)
    _ok(len(parsed) == 1, "fenced xml one call")
    _ok(parsed[0].name == "get_current_time", "get_current_time")
    _ok(parsed[0].arguments.get("timezone") == "Asia/Shanghai", "tz")
    _ok(strip_tool_markup(text) == "", "fence stripped to empty")


def test_bare_function_without_tool_call_wrapper() -> None:
    text = (
        "<function=memory_recall>"
        "<parameter=query>偏好</parameter>"
        "<parameter=top_k>3</parameter>"
        "</function>"
    )
    parsed = parse_tool_markup(text)
    _ok(len(parsed) == 1, "bare function")
    _ok(parsed[0].name == "memory_recall", "name")
    _ok(parsed[0].arguments.get("query") == "偏好", "query")
    _ok(parsed[0].arguments.get("top_k") == 3, f"top_k int (got {parsed[0].arguments!r})")


def test_hermes_line_form() -> None:
    text = "<tool_call>\nsandbox_shell\ncmd: ls -la\ntimeout_sec: 10\n</tool_call>"
    parsed = parse_tool_markup(text)
    _ok(len(parsed) == 1, "line form")
    _ok(parsed[0].name == "sandbox_shell", "name")
    _ok(parsed[0].arguments.get("cmd") == "ls -la", "cmd")
    _ok(parsed[0].arguments.get("timeout_sec") == 10, "timeout int")


def test_no_false_positive() -> None:
    text = "普通回答，没有工具。比较一下 a < b 与 c > d。"
    _ok(not content_has_tool_markup(text), "no markup")
    _ok(parse_tool_markup(text) == [], "empty parse")
    _ok(strip_tool_markup(text) == text.strip(), "strip noop")


def test_unclosed_strip() -> None:
    text = "前言 <tool_call> <function=sandbox_ls> <parameter=path>/"
    cleaned = strip_tool_markup(text)
    _ok("<tool_call" not in cleaned.lower(), f"unclosed stripped: {cleaned!r}")
    _ok("前言" in cleaned, "prose kept")


def main() -> None:
    print("test_tool_markup")
    test_bug_format_sandbox_ls()
    test_multiline_and_mixed_prose()
    test_hermes_json_inside_tool_call()
    test_parameter_name_attr_and_invoke()
    test_fenced_xml()
    test_bare_function_without_tool_call_wrapper()
    test_hermes_line_form()
    test_no_false_positive()
    test_unclosed_strip()
    print("ALL PASSED")


if __name__ == "__main__":
    main()
