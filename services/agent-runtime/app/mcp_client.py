"""MCP client helpers: connect on demand, list/call tools, safety checks."""

from __future__ import annotations

import asyncio
import json
import os
import re
from contextlib import asynccontextmanager
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, AsyncIterator

from .memory import database_url

DEFAULT_DATABASE_URL = "postgres://openbot:openbot@127.0.0.1:5432/openbot?sslmode=disable"
DEFAULT_TIMEOUT_S = float(os.getenv("MCP_TIMEOUT_SECONDS") or "25")

_SAFE_CMD_NAMES = {
    "python",
    "python3",
    "node",
    "npx",
    "uv",
    "uvx",
    "deno",
    "bun",
}
_DANGEROUS_RE = re.compile(r"[;&|`$<>\n\r]")
_TOOL_PREFIX = "mcp__"


@dataclass
class MCPServerConfig:
    id: str = ""
    name: str = ""
    transport: str = "stdio"
    command: str = ""
    args: list[str] = field(default_factory=list)
    url: str = ""
    env: dict[str, str] = field(default_factory=dict)
    enabled: bool = True

    @classmethod
    def from_dict(cls, d: dict[str, Any] | None) -> "MCPServerConfig":
        d = d or {}
        args = d.get("args") or []
        if not isinstance(args, list):
            args = []
        env = d.get("env") or {}
        if not isinstance(env, dict):
            env = {}
        return cls(
            id=str(d.get("id") or ""),
            name=str(d.get("name") or ""),
            transport=str(d.get("transport") or "stdio").lower().strip() or "stdio",
            command=str(d.get("command") or "").strip(),
            args=[str(a) for a in args],
            url=str(d.get("url") or "").strip(),
            env={str(k): str(v) for k, v in env.items()},
            enabled=bool(d.get("enabled", True)),
        )


def validate_stdio(cfg: MCPServerConfig) -> None:
    if not cfg.command:
        raise ValueError("stdio command required")
    if _DANGEROUS_RE.search(cfg.command):
        raise ValueError("command contains unsafe shell characters")
    for a in cfg.args:
        if _DANGEROUS_RE.search(a):
            raise ValueError(f"arg contains unsafe shell characters: {a!r}")
    base = Path(cfg.command).name.lower()
    # Allow absolute/relative interpreters or known package runners.
    if base not in _SAFE_CMD_NAMES and not os.path.isabs(cfg.command):
        # relative path to a script runner is ok if it looks like a path
        if "/" not in cfg.command and "\\" not in cfg.command:
            raise ValueError(
                f"command {cfg.command!r} not in allowlist "
                f"({', '.join(sorted(_SAFE_CMD_NAMES))}) and not absolute"
            )
    # Block obvious dangerous binaries even if absolute
    blocked = {"rm", "sudo", "chmod", "chown", "mkfs", "dd", "shutdown", "reboot"}
    if base in blocked:
        raise ValueError(f"command {base!r} is blocked")


def validate_url(cfg: MCPServerConfig) -> None:
    if not cfg.url:
        raise ValueError("url required for sse/http transport")
    u = cfg.url.lower()
    if not (u.startswith("http://") or u.startswith("https://")):
        raise ValueError("url must start with http:// or https://")


def validate_config(cfg: MCPServerConfig) -> None:
    if cfg.transport == "stdio":
        validate_stdio(cfg)
    elif cfg.transport in ("sse", "http"):
        validate_url(cfg)
    else:
        raise ValueError(f"unsupported transport: {cfg.transport}")


def sanitize_token(s: str) -> str:
    s = re.sub(r"[^a-zA-Z0-9_]+", "_", (s or "").strip())
    s = re.sub(r"_+", "_", s).strip("_")
    return s or "x"


def tool_qualified_name(server: MCPServerConfig, tool_name: str) -> str:
    sid = sanitize_token(server.id[:8] or server.name or "srv")
    t = sanitize_token(tool_name)
    return f"{_TOOL_PREFIX}{sid}__{t}"


def parse_qualified_tool(name: str) -> tuple[str, str] | None:
    """Return (server_key, tool_name) where server_key is sanitized id prefix."""
    if not name.startswith(_TOOL_PREFIX):
        return None
    rest = name[len(_TOOL_PREFIX) :]
    if "__" not in rest:
        return None
    sid, tool = rest.split("__", 1)
    if not sid or not tool:
        return None
    return sid, tool


@asynccontextmanager
async def open_session(cfg: MCPServerConfig) -> AsyncIterator[Any]:
    """Yield an initialized MCP ClientSession for the given server config."""
    validate_config(cfg)
    from mcp import ClientSession, StdioServerParameters
    from mcp.client.stdio import stdio_client

    if cfg.transport == "stdio":
        params = StdioServerParameters(
            command=cfg.command,
            args=list(cfg.args),
            env=cfg.env or None,
        )
        async with stdio_client(params) as (read, write):
            async with ClientSession(read, write) as session:
                await session.initialize()
                yield session
        return

    if cfg.transport == "sse":
        from mcp.client.sse import sse_client

        async with sse_client(cfg.url) as (read, write):
            async with ClientSession(read, write) as session:
                await session.initialize()
                yield session
        return

    # http = streamable HTTP
    from mcp.client.streamable_http import streamablehttp_client

    async with streamablehttp_client(cfg.url) as streams:
        read, write = streams[0], streams[1]
        async with ClientSession(read, write) as session:
            await session.initialize()
            yield session


async def _with_timeout(coro, timeout: float):
    return await asyncio.wait_for(coro, timeout=timeout)


async def list_tools_for_server(
    cfg: MCPServerConfig,
    *,
    timeout: float = DEFAULT_TIMEOUT_S,
) -> list[dict[str, Any]]:
    async def _run() -> list[dict[str, Any]]:
        async with open_session(cfg) as session:
            result = await session.list_tools()
            tools = []
            for t in result.tools or []:
                schema = getattr(t, "inputSchema", None) or getattr(t, "input_schema", None) or {}
                if hasattr(schema, "model_dump"):
                    schema = schema.model_dump()
                tools.append(
                    {
                        "name": t.name,
                        "description": getattr(t, "description", None) or "",
                        "input_schema": schema if isinstance(schema, dict) else {},
                        "qualified_name": tool_qualified_name(cfg, t.name),
                    }
                )
            return tools

    return await _with_timeout(_run(), timeout)


async def call_tool_on_server(
    cfg: MCPServerConfig,
    tool_name: str,
    arguments: dict[str, Any] | None = None,
    *,
    timeout: float = DEFAULT_TIMEOUT_S,
) -> dict[str, Any]:
    arguments = arguments or {}

    async def _run() -> dict[str, Any]:
        async with open_session(cfg) as session:
            result = await session.call_tool(tool_name, arguments=arguments)
            texts: list[str] = []
            for block in getattr(result, "content", None) or []:
                text = getattr(block, "text", None)
                if text is not None:
                    texts.append(str(text))
                else:
                    texts.append(str(block))
            is_error = bool(getattr(result, "isError", None) or getattr(result, "is_error", None))
            structured = getattr(result, "structuredContent", None) or getattr(
                result, "structured_content", None
            )
            return {
                "ok": not is_error,
                "is_error": is_error,
                "text": "\n".join(texts),
                "content": texts,
                "structured": structured,
            }

    return await _with_timeout(_run(), timeout)


async def test_server(cfg: MCPServerConfig) -> dict[str, Any]:
    try:
        tools = await list_tools_for_server(cfg)
        return {
            "ok": True,
            "server_id": cfg.id,
            "server_name": cfg.name,
            "transport": cfg.transport,
            "tools": [{"name": t["name"], "description": t["description"]} for t in tools],
            "tool_names": [t["name"] for t in tools],
            "count": len(tools),
        }
    except Exception as e:  # noqa: BLE001
        return {
            "ok": False,
            "server_id": cfg.id,
            "server_name": cfg.name,
            "transport": cfg.transport,
            "error": str(e),
            "tools": [],
            "tool_names": [],
            "count": 0,
        }


def load_servers_from_pg(user_id: str, *, enabled_only: bool = True) -> list[MCPServerConfig]:
    user_id = (user_id or "").strip()
    if not user_id:
        return []
    try:
        import psycopg
    except ImportError:
        return []
    url = database_url() or DEFAULT_DATABASE_URL
    sql = """
        SELECT id, name, transport, command, args_json, url, env_json, enabled
        FROM mcp_servers WHERE user_id = %s
    """
    if enabled_only:
        sql += " AND enabled = TRUE"
    sql += " ORDER BY created_at ASC"
    out: list[MCPServerConfig] = []
    with psycopg.connect(url) as conn:
        with conn.cursor() as cur:
            cur.execute(sql, (user_id,))
            for row in cur.fetchall():
                args_raw = row[4] or "[]"
                env_raw = row[6] or "{}"
                try:
                    args = json.loads(args_raw) if isinstance(args_raw, str) else (args_raw or [])
                except json.JSONDecodeError:
                    args = []
                try:
                    env = json.loads(env_raw) if isinstance(env_raw, str) else (env_raw or {})
                except json.JSONDecodeError:
                    env = {}
                out.append(
                    MCPServerConfig(
                        id=str(row[0]),
                        name=str(row[1] or ""),
                        transport=str(row[2] or "stdio"),
                        command=str(row[3] or ""),
                        args=[str(a) for a in (args or [])],
                        url=str(row[5] or ""),
                        env={str(k): str(v) for k, v in (env or {}).items()},
                        enabled=bool(row[7]),
                    )
                )
    return out


async def list_tools_for_servers(
    servers: list[MCPServerConfig],
) -> list[dict[str, Any]]:
    all_tools: list[dict[str, Any]] = []
    for cfg in servers:
        try:
            tools = await list_tools_for_server(cfg)
            for t in tools:
                t["server_id"] = cfg.id
                t["server_name"] = cfg.name
                all_tools.append(t)
        except Exception as e:  # noqa: BLE001
            all_tools.append(
                {
                    "name": "",
                    "description": "",
                    "error": str(e),
                    "server_id": cfg.id,
                    "server_name": cfg.name,
                    "qualified_name": "",
                    "input_schema": {},
                }
            )
    return all_tools


def openai_tools_from_mcp(tools: list[dict[str, Any]]) -> list[dict[str, Any]]:
    """Convert MCP tool metas to OpenAI function tool defs."""
    out: list[dict[str, Any]] = []
    for t in tools:
        qn = t.get("qualified_name") or ""
        if not qn or t.get("error"):
            continue
        schema = t.get("input_schema") or {"type": "object", "properties": {}}
        if not isinstance(schema, dict):
            schema = {"type": "object", "properties": {}}
        desc = t.get("description") or t.get("name") or qn
        srv = t.get("server_name") or t.get("server_id") or ""
        if srv:
            desc = f"[MCP:{srv}] {desc}"
        out.append(
            {
                "type": "function",
                "function": {
                    "name": qn,
                    "description": desc[:400],
                    "parameters": schema,
                },
            }
        )
    return out


async def resolve_and_call(
    *,
    servers: list[MCPServerConfig],
    qualified_or_tool: str,
    server_id: str | None = None,
    arguments: dict[str, Any] | None = None,
) -> dict[str, Any]:
    """Call a tool by qualified name (mcp__sid__tool) or by server_id + tool name."""
    arguments = arguments or {}
    tool_name = qualified_or_tool
    target: MCPServerConfig | None = None

    parsed = parse_qualified_tool(qualified_or_tool)
    if parsed:
        sid_key, tool_name = parsed
        for s in servers:
            if sanitize_token(s.id[:8]) == sid_key or sanitize_token(s.id) == sid_key:
                target = s
                break
            if sanitize_token(s.name) == sid_key:
                target = s
                break
    elif server_id:
        for s in servers:
            if s.id == server_id:
                target = s
                break

    if target is None and len(servers) == 1 and not parsed:
        target = servers[0]

    if target is None:
        return {"ok": False, "error": "server not found for tool", "tool": qualified_or_tool}

    try:
        return await call_tool_on_server(target, tool_name, arguments)
    except Exception as e:  # noqa: BLE001
        return {"ok": False, "error": str(e), "tool": tool_name, "server_id": target.id}


def example_echo_command() -> tuple[str, list[str]]:
    """Suggest command/args for the bundled echo MCP server."""
    runtime_root = Path(__file__).resolve().parents[1]
    script = runtime_root / "examples" / "mcp_echo_server.py"
    venv_py = runtime_root / ".venv" / "bin" / "python"
    py = str(venv_py) if venv_py.exists() else "python3"
    return py, [str(script)]


def example_help_text() -> str:
    cmd, args = example_echo_command()
    return (
        "示例 echo MCP（stdio）：\n"
        f"  command: {cmd}\n"
        f"  args: {json.dumps(args, ensure_ascii=False)}\n"
        "或在 Web 设置 → MCP 新增后点「测试连接」。"
    )
