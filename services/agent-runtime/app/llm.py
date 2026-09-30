"""OpenAI-compatible chat + light tool loop."""

from __future__ import annotations

import asyncio
import json
import os
import re
from dataclasses import dataclass
from typing import Any, Awaitable, Callable

import httpx

from .builtin_tools import BUILTIN_TOOL_DEFS
from .openbot_api import ROUTINE_TOOL_DEFS
from .deferral import CONTINUE_WORK, host_followup_prompt, turn_unfinished
from .client_env import (
    ClientContext,
    format_environment_block,
    format_tools_routing_block,
    tool_display_label,
)
from .model_compat import (
    adapt_chat_payload,
    postprocess_text,
    profile_for,
    strip_think_tags,
)
from .tool_markup import (
    content_has_tool_markup,
    parse_tool_markup,
    strip_tool_markup,
    to_openai_tool_calls,
)

ToolHandler = Callable[[str, dict[str, Any]], Awaitable[str]]
StatusCallback = Callable[[dict[str, Any]], Awaitable[None]]


@dataclass
class LLMOverride:
    base_url: str | None = None
    api_key: str | None = None
    model: str | None = None
    enable_tools: bool | None = None


def strip_think(text: str) -> str:
    """Remove Qwen-style <think>...</think> blocks from model output."""
    return strip_think_tags(text)


SYSTEM_PERSONA_BASE = (
    "你是 open-bot 助手，回答简洁、有帮助，默认使用中文。"
)

# When tools are off, models often roleplay fake tool calls in plain text — block that.
TOOLS_DISABLED_RULE = (
    "当前会话未启用函数调用；禁止假装调用工具、禁止写「请稍等 / 正在调用 xxx / mcp__…」；"
    "禁止输出 <tool_call> / <function= 等 XML 伪调用；"
    "用已有知识直接完整回答；若缺实时信息请如实说明无法获取。"
)

TOOL_DEFS: list[dict[str, Any]] = [
    {
        "type": "function",
        "function": {
            "name": "load_skill",
            "description": (
                "Load a skill package. Without path: returns SKILL.md body and the list of "
                "package files (references/, scripts/, …). With path: returns that file's content."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "name": {
                        "type": "string",
                        "description": "Skill name (directory / frontmatter name)",
                    },
                    "path": {
                        "type": "string",
                        "description": (
                            "Optional package-relative file (e.g. references/examples.md). "
                            "Omit to load SKILL.md + file list."
                        ),
                    },
                },
                "required": ["name"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "memory_write",
            "description": (
                "Persist a memory item. tier: profile|log|note. "
                "scope defaults to the current bot, or the current group when this run is in a channel. "
                "Use user for facts about the person, agent_pair for a fact shared by two bots "
                "(requires peer_agent_id)."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "content": {"type": "string"},
                    "tier": {
                        "type": "string",
                        "enum": ["profile", "log", "note"],
                    },
                    "scope": {
                        "type": "string",
                        "enum": ["user", "bot", "channel", "agent_pair"],
                    },
                    "peer_agent_id": {
                        "type": "string",
                        "description": "The other bot when scope is agent_pair",
                    },
                    "tags": {
                        "type": "array",
                        "items": {"type": "string"},
                    },
                },
                "required": ["content"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "memory_recall",
            "description": (
                "Recall memories for this conversation. More specific scopes "
                "(this pair, this group, this bot) are filled before the user's shared memories."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "query": {"type": "string"},
                    "tier": {
                        "type": "string",
                        "enum": ["profile", "log", "note"],
                    },
                    "top_k": {"type": "integer"},
                },
                "required": ["query"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "send_to_agent",
            "description": "Send a message to another agent (or channel) via the agent bus.",
            "parameters": {
                "type": "object",
                "properties": {
                    "to_agent_id": {
                        "type": "string",
                        "description": "Target agent id (optional if channel_id set)",
                    },
                    "channel_id": {
                        "type": "string",
                        "description": "Target channel id (optional if to_agent_id set)",
                    },
                    "body": {"type": "string", "description": "Message body"},
                    "priority": {"type": "boolean"},
                },
                "required": ["body"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "sandbox_ensure",
            "description": (
                "Ensure the internal execution environment is running (tool name sandbox_*). "
                "Never mention sandbox/Docker/containers//workspace or invent a second PC for the user; speak only about outcomes."
            ),
            "parameters": {"type": "object", "properties": {}},
        },
    },
    {
        "type": "function",
        "function": {
            "name": "sandbox_shell",
            "description": (
                "Run a shell command in the internal execution environment (paths under /workspace), "
                "NOT the user's personal PC. Call sandbox_ensure first if unsure. "
                "Never claim this is the user's Downloads/Desktop; never mention sandbox//workspace to the user."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "cmd": {"type": "string", "description": "Shell command to run"},
                    "workdir": {
                        "type": "string",
                        "description": "Working directory inside the execution environment (optional)",
                    },
                    "timeout_sec": {"type": "integer", "description": "Timeout seconds (default 30)"},
                },
                "required": ["cmd"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "sandbox_read",
            "description": (
                "Read a text file in the internal execution environment (under /workspace), "
                "not the user's personal PC Downloads/Desktop. Do not expose raw paths to the user."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "path": {"type": "string", "description": "File path inside the execution environment"},
                },
                "required": ["path"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "sandbox_write",
            "description": (
                "Write a text file in the internal execution environment. "
                "In team mode bare paths land in bots/{agent_id}/; use shared/ for shared files. "
                "Not the user's personal PC. Tell the user only about the result (preview/download), not paths."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "path": {"type": "string"},
                    "content": {"type": "string"},
                },
                "required": ["path", "content"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "sandbox_ls",
            "description": (
                "List a directory in the internal execution environment. "
                "This is NOT the user's personal PC Downloads/Desktop. "
                "Never say sandbox/Docker//workspace or invent a second PC for the user."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "path": {
                        "type": "string",
                        "description": "Directory path inside the execution environment",
                    },
                },
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "request_secret",
            "description": "Ask the user to provide a secret (API key/token). Never receives plaintext back to the model; Web shows a prompt.",
            "parameters": {
                "type": "object",
                "properties": {
                    "name": {"type": "string", "description": "Secret name, e.g. github_token"},
                    "origin": {"type": "string", "description": "HTTPS origin this secret may be used with"},
                    "auth_type": {"type": "string", "description": "bearer|basic|header"},
                    "reason": {"type": "string"},
                },
                "required": ["name"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "secret_http",
            "description": "HTTPS request to a saved secret origin with auth injected server-side. Plaintext never returned.",
            "parameters": {
                "type": "object",
                "properties": {
                    "name": {"type": "string"},
                    "method": {"type": "string"},
                    "url": {"type": "string", "description": "Must be https and match secret origin host"},
                    "body": {"type": "string"},
                },
                "required": ["name"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "list_machines",
            "description": (
                "List the user's registered computers. Each row has id, label, platform, connected, "
                "and file_op_count. connected means that desktop app can run file operations now. "
                "Call this when the user names a computer or asks which machines they have."
            ),
            "parameters": {"type": "object", "properties": {}},
        },
    },
    {
        "type": "function",
        "function": {
            "name": "host_ls",
            "description": (
                "List a directory on a connected computer. Absolute paths and ~/... are allowed anywhere "
                "the OS user can read. Pass machine_id from list_machines. If the user did not name a computer, "
                "omit machine_id so the most-used work computer is chosen. Empty path or ~ lists the home directory."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "machine_id": {"type": "string", "description": "Machine id from list_machines"},
                    "path": {
                        "type": "string",
                        "description": "Path such as /tmp, ~/Projects, Downloads, or /var/log",
                    },
                },
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "host_read",
            "description": (
                "Read a text file on a connected computer. Absolute paths and ~/... are allowed. "
                "Pass machine_id when the user named a computer; otherwise omit it to use the usual work computer."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "machine_id": {"type": "string"},
                    "path": {"type": "string"},
                },
                "required": ["path"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "host_write",
            "description": (
                "Write a text file on a connected computer. Absolute paths and ~/... are allowed. "
                "Creating a new file under the home directory runs immediately. "
                "Overwriting an existing file, or any write outside the home directory, waits for confirmation "
                "in the chat. Any logged-in device can allow or deny; the write still happens on the target computer."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "machine_id": {"type": "string"},
                    "path": {"type": "string"},
                    "content": {"type": "string"},
                },
                "required": ["path", "content"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "host_delete",
            "description": (
                "Delete one or more files on a connected computer. Absolute paths and ~/... are allowed. "
                "When deleting multiple files, pass them all in paths (one confirmation card). "
                "Do not call host_delete once per file — that creates multiple cards. "
                "Call immediately when the user asks to delete; the chat shows an allow/deny card "
                "(any logged-in client including the browser can click). Do not ask the user in plain text "
                "to confirm on the computer — wait for the tool result after they click."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "machine_id": {"type": "string"},
                    "path": {
                        "type": "string",
                        "description": "Single file path when deleting one file.",
                    },
                    "paths": {
                        "type": "array",
                        "items": {"type": "string"},
                        "description": "All file paths to delete together (preferred for batch delete).",
                    },
                },
                "required": [],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "host_move",
            "description": (
                "Move or rename a file on a connected computer. Absolute paths and ~/... are allowed. "
                "Always waits for confirmation in the chat. Any logged-in device can allow or deny."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "machine_id": {"type": "string"},
                    "path": {"type": "string"},
                    "dest": {"type": "string"},
                },
                "required": ["path", "dest"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "host_open",
            "description": (
                "Open an application on a connected computer, such as 微信, Safari, or Visual Studio Code. "
                "Pass the app name the user said. Runs immediately on that computer. "
                "Pass machine_id when the user named a computer; otherwise omit it to use the usual work computer."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "machine_id": {"type": "string"},
                    "name": {"type": "string", "description": "Application name, for example 微信 or Terminal"},
                },
                "required": ["name"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "host_shell",
            "description": (
                "Run a local command on a connected computer. Not for ssh/scp/sftp "
                "(use host_ssh_* after load_skill host-ssh). "
                "terminal=true only for a local interactive UI. Always confirms in chat."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "machine_id": {"type": "string"},
                    "command": {"type": "string", "description": "Local command only"},
                    "terminal": {
                        "type": "boolean",
                        "description": "Open a visible local terminal",
                    },
                },
                "required": ["command"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "host_ssh_ls",
            "description": (
                "List a remote directory over SSH from a connected desktop app (headless). "
                "Prefer load_skill host-ssh first. host: IP, domain, or ~/.ssh/config Host."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "machine_id": {"type": "string"},
                    "host": {"type": "string"},
                    "user": {"type": "string"},
                    "port": {"type": "integer"},
                    "path": {"type": "string"},
                },
                "required": ["host"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "host_ssh_read",
            "description": "Read a remote text file over SSH/SFTP. Prefer load_skill host-ssh first.",
            "parameters": {
                "type": "object",
                "properties": {
                    "machine_id": {"type": "string"},
                    "host": {"type": "string"},
                    "user": {"type": "string"},
                    "port": {"type": "integer"},
                    "path": {"type": "string"},
                },
                "required": ["host", "path"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "host_ssh_write",
            "description": (
                "Write a remote text file over SSH/SFTP. Confirms in chat. Prefer load_skill host-ssh first."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "machine_id": {"type": "string"},
                    "host": {"type": "string"},
                    "user": {"type": "string"},
                    "port": {"type": "integer"},
                    "path": {"type": "string"},
                    "content": {"type": "string"},
                },
                "required": ["host", "path", "content"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "host_ssh_delete",
            "description": (
                "Delete one or more remote files over SSH/SFTP. Confirms in chat. "
                "For multiple files use paths once; do not call repeatedly. Prefer load_skill host-ssh first."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "machine_id": {"type": "string"},
                    "host": {"type": "string"},
                    "user": {"type": "string"},
                    "port": {"type": "integer"},
                    "path": {"type": "string"},
                    "paths": {
                        "type": "array",
                        "items": {"type": "string"},
                        "description": "All remote paths to delete together.",
                    },
                },
                "required": ["host"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "host_ssh_exec",
            "description": (
                "Run one remote command over SSH (headless desktop client). Confirms in chat. "
                "Prefer load_skill host-ssh first. Not for opening a system terminal."
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "machine_id": {"type": "string"},
                    "host": {"type": "string"},
                    "user": {"type": "string"},
                    "port": {"type": "integer"},
                    "command": {"type": "string"},
                },
                "required": ["host", "command"],
            },
        },
    },
]
# Append built-in utility tools (time / calculator / http_fetch).
TOOL_DEFS = list(TOOL_DEFS) + list(BUILTIN_TOOL_DEFS) + list(ROUTINE_TOOL_DEFS)


def openai_config(override: LLMOverride | None = None) -> tuple[str, str, str]:
    api_key = (os.getenv("OPENAI_API_KEY") or "").strip()
    base = (os.getenv("OPENAI_BASE_URL") or "https://api.openai.com/v1").rstrip("/")
    model = os.getenv("OPENAI_MODEL") or "gpt-4o-mini"
    if override:
        if override.api_key is not None and str(override.api_key).strip() != "":
            api_key = str(override.api_key).strip()
        elif override.api_key is not None:
            # explicit empty api_key still overrides to empty
            api_key = str(override.api_key).strip()
        if override.base_url is not None and str(override.base_url).strip() != "":
            base = str(override.base_url).strip().rstrip("/")
        if override.model is not None and str(override.model).strip() != "":
            model = str(override.model).strip()
    return api_key, base, model


def build_system_prompt(
    *,
    agent_id: str,
    skills_catalog: str,
    memory_snippets: list[str],
    tools_enabled: bool = False,
    available_tool_names: list[str] | None = None,
    client: ClientContext | None = None,
    machines: list[dict[str, Any]] | None = None,
) -> str:
    parts = [
        SYSTEM_PERSONA_BASE,
        f"当前 agent_id: {agent_id or 'open-bot'}。",
        format_environment_block(client, machines),
    ]
    if tools_enabled:
        names = [n for n in (available_tool_names or []) if n]
        builtin = [n for n in names if not n.startswith("mcp__")]
        mcp_names = [n for n in names if n.startswith("mcp__")]
        tool_bits = [
            "当前会话已启用真实的 OpenAI function-calling 工具；"
            "请**仅**通过 API 的 tool_calls / function calling 调用工具，"
            "禁止在正文里写 <tool_call>、<function= 或任何 XML/Hermes 风格伪调用（那些不会自动执行且会泄漏给用户）。"
        ]
        if builtin:
            tool_bits.append("内置工具：" + "、".join(builtin) + "。")
        if mcp_names:
            tool_bits.append("可用 MCP 工具：" + "、".join(mcp_names) + "。")
        if any(n.startswith("sandbox_") for n in names):
            tool_bits.append(
                "长任务：优先调用 defer_work(goal=…) 先确认再后台做完，同一会话交付；"
                "例行任务用 create_routine/update_routine/pause_routine/resume_routine/delete_routine/list_routines"
                "（可设 cron 与 Slack/GitHub triggers）。"
            )
            tool_bits.append(
                "重要：sandbox_* 仅在内部执行环境操作，不是用户本机 Downloads/Desktop。"
                "对用户完全透明：禁止提及 sandbox / Docker / 容器 / /workspace，"
                "禁止把内部环境说成用户的另一台电脑；只谈结果（已生成、可预览、可下载）。"
                "若用户要看「我的电脑 / Downloads / 桌面」，先 list_machines；"
                "没有在线主机时如实说明要打开桌面应用，不要用 sandbox_ls 假装那是用户的 Downloads，"
                "也不要凭记忆编造文件名或大小。"
            )
        parts.append("".join(tool_bits))
        routing = format_tools_routing_block(
            tools_enabled=True, available_tool_names=available_tool_names
        )
        if routing:
            parts.append(routing)
        skills_header = "## 可用技能（仅目录；需要全文时调用 load_skill）"
    else:
        parts.append(TOOLS_DISABLED_RULE)
        skills_header = "## 可用技能（仅目录；当前未启用工具，勿调用 load_skill）"
    parts.append(skills_header)
    parts.append(skills_catalog or "（无）")
    if memory_snippets:
        parts.append("## 相关记忆（自动召回）")
        parts.append(
            "更具体的记忆档优先于更泛的档。"
            "本会话近期消息和压缩摘要优先于长期记忆；冲突时以本会话为准。"
        )
        parts.extend(f"- {s}" for s in memory_snippets)
    return "\n".join(parts)


_FAKE_TOOL_LINE = re.compile(
    r"(?m)^.*(?:"
    r"调用\s*\S*\s*工具|"
    r"mcp__\w+|"
    r"(?:请稍等|稍等|正在调用|正在使用).{0,40}(?:工具|mcp__)"
    r").*$"
)
_FALLBACK_NO_FAKE = (
    "当前会话未启用函数调用，我无法真正调用工具。"
    "请直接说明你的问题，我会用已有知识回答；若需要实时信息，请开启工具或另行提供。"
)


def sanitize_fake_tool_narration(text: str) -> str:
    """Strip obvious fake tool-call stall lines when tools are disabled."""
    if not text or not text.strip():
        return text
    text = strip_tool_markup(text)
    if not text or not text.strip():
        return _FALLBACK_NO_FAKE
    lines = text.splitlines()
    kept: list[str] = []
    removed = 0
    for line in lines:
        if _FAKE_TOOL_LINE.search(line):
            removed += 1
            continue
        kept.append(line)
    if removed == 0:
        return text
    cleaned = "\n".join(kept).strip()
    if not cleaned:
        return _FALLBACK_NO_FAKE
    return cleaned


def normalize_messages(
    messages: list[dict[str, Any]] | None,
    content: str,
) -> list[dict[str, str]]:
    out: list[dict[str, str]] = []
    if messages:
        for m in messages:
            role = str(m.get("role") or "").strip()
            text = str(m.get("content") or "")
            if role in ("system", "user", "assistant", "summary") and text != "":
                out.append({"role": role, "content": text})
    if not out and content.strip():
        out.append({"role": "user", "content": content.strip()})
    elif content.strip():
        last = out[-1] if out else None
        if not (last and last["role"] == "user" and last["content"] == content.strip()):
            if not (last and last["role"] == "user"):
                out.append({"role": "user", "content": content.strip()})
    return out



def _raw_usage_from_response(data: dict[str, Any] | None) -> Any | None:
    """Return upstream `usage` object/dict if present; else None."""
    if not isinstance(data, dict):
        return None
    usage = data.get("usage")
    if usage is None:
        return None
    if isinstance(usage, dict) and not usage:
        return None
    return usage


async def chat_completion(
    messages: list[dict[str, Any]],
    *,
    api_key: str,
    tools: list[dict[str, Any]] | None = None,
    tool_choice: str | dict | None = None,
    override: LLMOverride | None = None,
) -> dict[str, Any]:
    _, base, model = openai_config(override)
    profile = profile_for(model, base)
    url = f"{base}/chat/completions"
    payload: dict[str, Any] = {
        "model": model,
        "stream": False,
        "messages": messages,
    }
    if tools:
        payload["tools"] = tools
        if tool_choice is not None:
            payload["tool_choice"] = tool_choice
    payload = adapt_chat_payload(payload, has_tools=bool(tools), profile=profile)
    headers = {
        "Authorization": f"Bearer {api_key}",
        "Content-Type": "application/json",
    }
    async with httpx.AsyncClient(timeout=httpx.Timeout(90.0, connect=10.0)) as client:
        resp = await client.post(url, headers=headers, json=payload)
        if resp.status_code >= 300:
            body = resp.text[:2000]
            # Some gateways inject a non-none default; retry once with none + tools.
            if (
                tools
                and resp.status_code == 400
                and "reasoning_effort" in body
                and payload.get("reasoning_effort") != "none"
            ):
                payload = {**payload, "reasoning_effort": "none"}
                resp2 = await client.post(url, headers=headers, json=payload)
                if resp2.status_code >= 300:
                    body2 = resp2.text[:2000]
                    if (
                        tools
                        and resp2.status_code == 400
                        and is_auto_tool_choice_unsupported(body2)
                    ):
                        raise AutoToolChoiceUnsupported(
                            f"upstream HTTP {resp2.status_code}: {body2}"
                        )
                    raise RuntimeError(
                        f"upstream HTTP {resp2.status_code}: {body2}"
                    )
                return resp2.json()
            if (
                tools
                and resp.status_code == 400
                and is_auto_tool_choice_unsupported(body)
            ):
                raise AutoToolChoiceUnsupported(
                    f"upstream HTTP {resp.status_code}: {body}"
                )
            raise RuntimeError(f"upstream HTTP {resp.status_code}: {body}")
        return resp.json()


async def chat_text(
    messages: list[dict[str, Any]],
    *,
    api_key: str,
    override: LLMOverride | None = None,
) -> str:
    data = await chat_completion(messages, api_key=api_key, override=override)
    choices = data.get("choices") or []
    if not choices:
        return ""
    msg = choices[0].get("message") or {}
    return str(msg.get("content") or "")


def tools_enabled(override: LLMOverride | None = None) -> bool:
    """Local vLLM often lacks --enable-auto-tool-choice; default off."""
    if override is not None and override.enable_tools is not None:
        return bool(override.enable_tools)
    v = (os.getenv("OPENAI_ENABLE_TOOLS") or "0").strip().lower()
    return v in ("1", "true", "yes", "on")


class AutoToolChoiceUnsupported(RuntimeError):
    """Upstream rejects tools without --enable-auto-tool-choice / tool-call-parser."""


def is_auto_tool_choice_unsupported(message: str) -> bool:
    """Detect vLLM / gateway 400s that require auto tool choice flags."""
    m = (message or "").lower()
    needles = (
        "enable-auto-tool-choice",
        "enable_auto_tool_choice",
        "tool-call-parser",
        "tool_call_parser",
        '"auto" tool choice requires',
        "auto tool choice requires",
    )
    return any(n in m for n in needles)


async def run_tool_loop(
    messages: list[dict[str, Any]],
    *,
    api_key: str,
    tool_handler: ToolHandler,
    max_rounds: int = 4,
    override: LLMOverride | None = None,
    extra_tools: list[dict[str, Any]] | None = None,
    on_status: StatusCallback | None = None,
) -> tuple[str, list[str], Any | None]:
    """Non-stream tool loop; returns final text, tool names used, and usage_details.

    The third value is aggregated Langfuse-shaped usage_details
    (prompt_tokens / completion_tokens / total_tokens) across all upstream
    chat completions in this loop, or None if the gateway omitted usage.

    Optional on_status receives dicts like
    {"phase":"tool","label":"正在运行命令","tool":"<name>"} while tools run.
    """
    from .langfuse_trace import merge_usage_details, parse_usage_details

    used: list[str] = []
    msgs: list[dict[str, Any]] = list(messages)
    final = ""
    usage_acc: dict[str, int] | None = None
    _, base, model = openai_config(override)
    profile = profile_for(model, base)

    def _accumulate(data: dict[str, Any]) -> None:
        nonlocal usage_acc
        usage_acc = merge_usage_details(
            usage_acc, parse_usage_details(_raw_usage_from_response(data))
        )

    async def _checkpoint() -> None:
        # Let CancelledError surface between LLM/tool awaits.
        await asyncio.sleep(0)

    if not tools_enabled(override):
        await _checkpoint()
        data = await chat_completion(msgs, api_key=api_key, tools=None, override=override)
        _accumulate(data)
        choices = data.get("choices") or []
        if choices:
            raw = str((choices[0].get("message") or {}).get("content") or "")
            # Never leak XML/Hermes tool markup when tools are off — strip only.
            final = postprocess_text(strip_think(strip_tool_markup(raw)), profile)
            final = sanitize_fake_tool_narration(final)
        return final, used, usage_acc
    tools = list(TOOL_DEFS)
    if extra_tools:
        tools.extend(extra_tools)
    choice: str | None = "auto" if profile.tool_choice_auto else None
    # When upstream rejects tools+auto, keep running via XML/Hermes text tool calls
    # instead of silently chatting with a "tools enabled" system prompt (hallucinates).
    tools_via_markup = False
    markup_hint_added = False
    continued = False
    for _ in range(max_rounds):
        await _checkpoint()
        try:
            data = await chat_completion(
                msgs,
                api_key=api_key,
                tools=None if tools_via_markup else tools,
                tool_choice=None if tools_via_markup else choice,
                override=override,
            )
        except AutoToolChoiceUnsupported:
            if tools_via_markup:
                raise
            tools_via_markup = True
            if on_status is not None:
                await on_status(
                    {
                        "phase": "thinking",
                        "label": "上游未启用 auto tool choice，改用文本工具协议",
                        "tools_disabled": False,
                        "reason": "auto_tool_choice_unsupported",
                        "tools_via_markup": True,
                    }
                )
            if not markup_hint_added:
                markup_hint_added = True
                msgs.append(
                    {
                        "role": "system",
                        "content": (
                            "当前上游不支持 OpenAI 原生 tool_calls（缺 enable-auto-tool-choice）。"
                            "需要工具时，在回复里只输出如下 XML（系统会执行），"
                            "不要编造工具结果，尤其不要编造本机文件名、大小或删除成功：\n"
                            "<tool_call>\n"
                            "<function=工具名>\n"
                            "<parameter=参数名>参数值</parameter>\n"
                            "</function>\n"
                            "</tool_call>\n"
                            "查本机文件先 list_machines 再 host_ls；删除用 host_delete（可传 paths）。"
                        ),
                    }
                )
            continue
        _accumulate(data)
        choices = data.get("choices") or []
        if not choices:
            break
        msg = choices[0].get("message") or {}
        tool_calls = list(msg.get("tool_calls") or [])
        content = msg.get("content")
        content_str = str(content or "")

        # Recover XML/Hermes/YAML-style tool calls leaked as plain text.
        if not tool_calls and content_has_tool_markup(content_str):
            parsed = parse_tool_markup(content_str)
            if parsed:
                tool_calls = to_openai_tool_calls(parsed)
                cleaned = strip_tool_markup(content_str)
                content = cleaned if cleaned else None
                content_str = str(content or "")

        if tool_calls:
            # Persist assistant turn without leaking raw markup into history.
            msgs.append(
                {
                    "role": "assistant",
                    "content": content,
                    "tool_calls": tool_calls,
                }
            )
            for tc in tool_calls:
                fn = tc.get("function") or {}
                name = str(fn.get("name") or "")
                raw_args = fn.get("arguments") or "{}"
                try:
                    args = json.loads(raw_args) if isinstance(raw_args, str) else dict(raw_args)
                except json.JSONDecodeError:
                    args = {}
                used.append(name)
                if on_status is not None:
                    await on_status(
                        {
                            "phase": "tool",
                            "label": tool_display_label(name),
                            "tool": name,
                        }
                    )
                await _checkpoint()
                result = await tool_handler(name, args)
                if on_status is not None:
                    await on_status(
                        {
                            "phase": "tool_done",
                            "label": "正在思考…",
                            "tool": name,
                        }
                    )
                msgs.append(
                    {
                        "role": "tool",
                        "tool_call_id": tc.get("id"),
                        "content": result,
                    }
                )
            continue
        # Text with no tool call. If this thread already has a file and this
        # run has not written or executed anything, the agent is not done.
        # Keep the same run going. Do not classify the sentence.
        final = postprocess_text(strip_think(strip_tool_markup(content_str)), profile)
        if not continued and turn_unfinished(used, msgs):
            continued = True
            if on_status is not None:
                await on_status(
                    {
                        "phase": "thinking",
                        "label": "正在做，做好会发在这里",
                    }
                )
            msgs.append({"role": "assistant", "content": final or ""})
            msgs.append({"role": "user", "content": CONTINUE_WORK})
            final = ""
            continue
        host_nudge = host_followup_prompt(used, msgs) if not continued else None
        if host_nudge:
            continued = True
            if on_status is not None:
                await on_status(
                    {
                        "phase": "thinking",
                        "label": "需要先调用本机工具",
                    }
                )
            msgs.append({"role": "assistant", "content": final or ""})
            msgs.append({"role": "user", "content": host_nudge})
            final = ""
            continue
        break
    if not final and msgs:
        data = await chat_completion(msgs, api_key=api_key, tools=None, override=override)
        _accumulate(data)
        choices = data.get("choices") or []
        if choices:
            final = postprocess_text(
                strip_think(
                    strip_tool_markup(
                        str((choices[0].get("message") or {}).get("content") or "")
                    )
                ),
                profile,
            )
    return final, used, usage_acc


async def stream_chat_tokens(
    messages: list[dict[str, Any]],
    *,
    api_key: str,
    override: LLMOverride | None = None,
):
    """Yield text deltas from streaming completions (no tools)."""
    _, base, model = openai_config(override)
    profile = profile_for(model, base)
    url = f"{base}/chat/completions"
    payload: dict[str, Any] = {"model": model, "stream": True, "messages": messages}
    payload = adapt_chat_payload(payload, has_tools=False, profile=profile)
    headers = {
        "Authorization": f"Bearer {api_key}",
        "Content-Type": "application/json",
        "Accept": "text/event-stream",
    }
    async with httpx.AsyncClient(timeout=httpx.Timeout(90.0, connect=10.0)) as client:
        async with client.stream("POST", url, headers=headers, json=payload) as resp:
            if resp.status_code >= 300:
                err = (await resp.aread()).decode("utf-8", errors="replace")
                raise RuntimeError(f"upstream HTTP {resp.status_code}: {err[:2000]}")
            async for line in resp.aiter_lines():
                if not line or line.startswith(":") or not line.startswith("data:"):
                    continue
                data = line[5:].strip()
                if data == "[DONE]":
                    break
                try:
                    obj = json.loads(data)
                except json.JSONDecodeError:
                    continue
                choices = obj.get("choices") or []
                if not choices:
                    continue
                delta = choices[0].get("delta") or {}
                text = delta.get("content")
                if text:
                    yield text
