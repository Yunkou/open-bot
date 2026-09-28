"""Smoke tests for client_env + list_machines tool registration."""

from __future__ import annotations

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

from app.client_env import (  # noqa: E402
    ClientContext,
    format_environment_block,
    tool_display_label,
)
from app.llm import TOOL_DEFS, build_system_prompt  # noqa: E402


def _ok(cond: bool, msg: str) -> None:
    if not cond:
        raise AssertionError(msg)
    print(f"  OK  {msg}")


def test_from_any_and_env_block() -> None:
    c = ClientContext.from_any(
        {
            "platform": "web",
            "app": "browser",
            "os": "darwin",
            "capabilities": {"host_tools": False, "workspace_tools": True},
        }
    )
    _ok(c is not None and c.platform == "web", "parse web client")
    block = format_environment_block(c)
    _ok("环境" in block and "sandbox_*" in block, "env block mentions internal sandbox_*")
    _ok("工作电脑" not in block, "env block does not invent 工作电脑 for users")
    _ok("禁止提及" in block and "Docker" in block, "instructs model not to say Docker")


def test_tool_defs_include_list_machines() -> None:
    names = {str((t.get("function") or {}).get("name") or "") for t in TOOL_DEFS}
    _ok("list_machines" in names, "list_machines in TOOL_DEFS")
    _ok("host_ls" in names and "host_read" in names, "host stubs present")
    _ok(tool_display_label("sandbox_ls") == "列出目录", "sandbox_ls alias")
    _ok(tool_display_label("sandbox_write") == "写入文件", "sandbox_write alias")


def test_system_prompt_routing() -> None:
    client = ClientContext(platform="macos", app="tauri", os="darwin", arch="arm64")
    prompt = build_system_prompt(
        agent_id="open-bot",
        skills_catalog="（无）",
        memory_snippets=[],
        tools_enabled=True,
        available_tool_names=["list_machines", "sandbox_ls", "get_current_time", "load_skill"],
        client=client,
    )
    _ok("## 环境" in prompt, "has 环境 section")
    _ok("list_machines" in prompt, "mentions list_machines")
    _ok("sandbox_*" in prompt, "mentions internal sandbox_*")
    _ok("工作电脑" not in prompt, "no 工作电脑 in prompt")
    _ok("沙箱电脑" not in prompt, "no 沙箱电脑 in prompt")
    _ok("完全透明" in prompt or "只谈结果" in prompt, "instructs outcome-only user speech")


if __name__ == "__main__":
    test_from_any_and_env_block()
    test_tool_defs_include_list_machines()
    test_system_prompt_routing()
    print("all passed")
