#!/usr/bin/env python3
"""Minimal MCP stdio server for open-bot demos.

Tools:
  - echo(message): returns the same message
  - add(a, b): returns a+b as text

Run (from repo root, with agent-runtime venv):
  services/agent-runtime/.venv/bin/python services/agent-runtime/examples/mcp_echo_server.py

Or register in Web 设置 → MCP:
  transport=stdio
  command=<abs path to that python>
  args=[<abs path to this script>]
"""

from __future__ import annotations

from mcp.server.fastmcp import FastMCP

mcp = FastMCP("open-bot-echo")


@mcp.tool()
def echo(message: str) -> str:
    """Echo back the given message."""
    return message


@mcp.tool()
def add(a: float, b: float) -> str:
    """Add two numbers and return the sum as text."""
    return str(a + b)


if __name__ == "__main__":
    mcp.run(transport="stdio")
