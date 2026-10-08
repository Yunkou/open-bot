"""Request-scoped tool handler binding for durable harness + openai_path."""

from __future__ import annotations

import json
from contextvars import ContextVar, Token
from typing import Any, Awaitable, Callable

ToolHandler = Callable[[str, dict[str, Any]], Awaitable[str]]

_active: ContextVar[ToolHandler | None] = ContextVar("openbot_tool_handler", default=None)


def bind_tool_handler(handler: ToolHandler) -> Token:
    return _active.set(handler)


def reset_tool_handler(token: Token) -> None:
    _active.reset(token)


async def dispatch_bound_tool(name: str, args: dict[str, Any]) -> str:
    handler = _active.get()
    if handler is None:
        return json.dumps({"error": "no tool_handler bound"}, ensure_ascii=False)
    return await handler(name, args)
