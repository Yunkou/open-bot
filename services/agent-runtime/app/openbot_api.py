"""Internal HTTP helpers for defer_work / routine tools (same auth as sandbox)."""

from __future__ import annotations

import json
import os
import urllib.error
import urllib.request
from typing import Any

DEFAULT_API_URL = "http://127.0.0.1:18080"
DEFAULT_INTERNAL_TOKEN = "open-bot-dev-internal"


def _api_base() -> str:
    return (os.getenv("OPENBOT_API_URL") or os.getenv("API_BASE_URL") or DEFAULT_API_URL).rstrip(
        "/"
    )


def _internal_token() -> str:
    return (
        (os.getenv("INTERNAL_TOKEN") or os.getenv("OPENBOT_INTERNAL_TOKEN") or DEFAULT_INTERNAL_TOKEN)
        .strip()
        or DEFAULT_INTERNAL_TOKEN
    )


def _post(path: str, payload: dict[str, Any], timeout: float = 60.0) -> dict[str, Any]:
    url = f"{_api_base()}{path}"
    data = json.dumps(payload, ensure_ascii=False).encode("utf-8")
    req = urllib.request.Request(
        url,
        data=data,
        method="POST",
        headers={
            "Content-Type": "application/json",
            "X-Internal-Token": _internal_token(),
            "Accept": "application/json",
        },
    )
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read().decode("utf-8")
            return json.loads(raw) if raw else {}
    except urllib.error.HTTPError as e:
        detail = e.read().decode("utf-8", errors="replace")
        raise RuntimeError(f"openbot api HTTP {e.code}: {detail}") from e
    except urllib.error.URLError as e:
        raise RuntimeError(f"openbot api unreachable: {e}") from e


def defer_work(
    user_id: str,
    conversation_id: str,
    goal: str,
    *,
    agent_id: str | None = None,
) -> dict[str, Any]:
    return _post(
        "/internal/conversation-tasks/enqueue",
        {
            "user_id": user_id,
            "conversation_id": conversation_id,
            "agent_id": agent_id or "",
            "goal": goal,
        },
    )


def list_routines(user_id: str) -> dict[str, Any]:
    return _post("/internal/routines/list", {"user_id": user_id})


def create_routine(user_id: str, **fields: Any) -> dict[str, Any]:
    payload = {"user_id": user_id, **fields}
    return _post("/internal/routines/create", payload)


def update_routine(user_id: str, routine_id: str, **fields: Any) -> dict[str, Any]:
    payload = {"user_id": user_id, "id": routine_id, **fields}
    return _post("/internal/routines/update", payload)


def delete_routine(user_id: str, routine_id: str) -> dict[str, Any]:
    return _post("/internal/routines/delete", {"user_id": user_id, "id": routine_id})


ROUTINE_TOOL_DEFS: list[dict[str, Any]] = [
    {
        "type": "function",
        "function": {
            "name": "defer_work",
            "description": (
                "把耗时工作放到后台队列，本轮先简短确认「还在做，做好会发在这里」。"
                "长任务（多步改文件、调研、例行之外的重活）优先用本工具，不要只靠口头「稍后」。"
                "调用后立即用中文简短确认并停止本轮重活；后台会在同一会话交付结果。"
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "goal": {
                        "type": "string",
                        "description": "后台要完成的目标（完整、可执行）",
                    },
                },
                "required": ["goal"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "list_routines",
            "description": "列出当前用户的例行任务（cron 与事件触发器）。",
            "parameters": {"type": "object", "properties": {}},
        },
    },
    {
        "type": "function",
        "function": {
            "name": "create_routine",
            "description": (
                "创建例行任务。可设 schedule_cron（5 字段）和/或 triggers（Slack/GitHub 事件）。"
                "默认绑定当前会话 conversation_id，后续触发会复用该会话并注入 [routine] 提示。"
            ),
            "parameters": {
                "type": "object",
                "properties": {
                    "name": {"type": "string"},
                    "prompt": {"type": "string", "description": "每次触发时交给助手的任务说明"},
                    "schedule_cron": {
                        "type": "string",
                        "description": "5 字段 cron，如 0 9 * * *；纯事件触发可留空",
                    },
                    "timezone": {
                        "type": "string",
                        "description": "IANA 时区，默认 Asia/Shanghai",
                    },
                    "enabled": {"type": "boolean"},
                    "agent_id": {"type": "string"},
                    "triggers": {
                        "type": "array",
                        "description": (
                            "事件监听器，例如 "
                            '[{"source":"slack","type":"app_mention"},'
                            '{"source":"slack","type":"keyword","keywords":["日报"]},'
                            '{"source":"github","type":"pull_request","actions":["opened"],"repo":"org/repo"}]'
                        ),
                        "items": {"type": "object"},
                    },
                    "max_retries": {"type": "integer"},
                    "quiet_unchanged": {
                        "type": "boolean",
                        "description": "结果与上次相同时静默（ok_quiet）",
                    },
                    "pin_current_conversation": {
                        "type": "boolean",
                        "description": "默认 true：绑定当前会话",
                    },
                },
                "required": ["name", "prompt"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "update_routine",
            "description": "更新例行任务字段（含 enabled 暂停/恢复）。",
            "parameters": {
                "type": "object",
                "properties": {
                    "id": {"type": "string"},
                    "name": {"type": "string"},
                    "prompt": {"type": "string"},
                    "schedule_cron": {"type": "string"},
                    "timezone": {"type": "string"},
                    "enabled": {"type": "boolean"},
                    "agent_id": {"type": "string"},
                    "triggers": {"type": "array", "items": {"type": "object"}},
                    "max_retries": {"type": "integer"},
                    "quiet_unchanged": {"type": "boolean"},
                    "conversation_id": {"type": "string"},
                },
                "required": ["id"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "pause_routine",
            "description": "暂停例行任务（enabled=false）。",
            "parameters": {
                "type": "object",
                "properties": {"id": {"type": "string"}},
                "required": ["id"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "resume_routine",
            "description": "恢复例行任务（enabled=true）。",
            "parameters": {
                "type": "object",
                "properties": {"id": {"type": "string"}},
                "required": ["id"],
            },
        },
    },
    {
        "type": "function",
        "function": {
            "name": "delete_routine",
            "description": "删除例行任务。",
            "parameters": {
                "type": "object",
                "properties": {"id": {"type": "string"}},
                "required": ["id"],
            },
        },
    },
]

ROUTINE_TOOL_NAMES = frozenset(
    str((t.get("function") or {}).get("name") or "") for t in ROUTINE_TOOL_DEFS
)
