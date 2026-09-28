# agent-runtime

Python FastAPI runtime（端口 `8001`）。

模块：`app/skills.py`、`app/memory.py`、`app/compact.py`、`app/llm.py`、`app/main.py`。

环境变量（根目录 `.env` 或进程环境）：`OPENAI_API_KEY`、`OPENAI_BASE_URL`、`OPENAI_MODEL`。

## MCP

- 依赖：`mcp>=1.9,<2`（见 requirements.txt）
- 示例 stdio server：`examples/mcp_echo_server.py`
- 端点：`POST /v1/mcp/test`、`/v1/mcp/list-tools`、`/v1/mcp/call-tool`；`GET /v1/mcp/example`
- 超时：`MCP_TIMEOUT_SECONDS`（默认 25）
