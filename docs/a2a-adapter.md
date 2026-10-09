# A2A 网关说明

open-bot 提供 JSON-RPC A2A 网关（适配现有 runtime），支持任务持久化、查询/取消、SSE 流式与 push webhook。

## 端点

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/.well-known/agent-card.json` | Agent Card（`?agent_id=` 可选） |
| POST | `/a2a/v1` | JSON-RPC |

可选鉴权：环境变量 `A2A_TOKEN`；请求头 `X-A2A-Token` 或 `Authorization: Bearer …`。未设则开放（仅本机开发）。

公开基址：`A2A_PUBLIC_BASE`（写入 card 内 url）。

## JSON-RPC 方法

| method | 说明 |
|--------|------|
| `message/send`（别名 `SendMessage`） | 同步跑一轮，落库 Task，返回终态 |
| `message/stream`（别名 `SendStreamingMessage`） | SSE：`status-update` / `artifact-update` |
| `tasks/get`（`GetTask`） | 按 task id 查询 |
| `tasks/cancel`（`CancelTask`） | 非终态则标 `canceled` 并尝试 push |
| `tasks/pushNotificationConfig/set` | 注册 webhook（task 或 agent 级） |
| `tasks/pushNotificationConfig/get` | 读取已注册 push |

任务状态：`submitted` → `working` → `completed` | `failed` | `canceled`（表 `a2a_tasks`）。

Push：终态时 POST JSON `{kind, task, timestamp}` 到注册 URL；可用 `Authorization: Bearer <token>` / `X-A2A-Notification-Token`。

## curl 示例

```bash
# Agent Card
curl -s http://127.0.0.1:18080/.well-known/agent-card.json | jq .

# 同步发送
curl -s http://127.0.0.1:18080/a2a/v1 \
  -H 'Content-Type: application/json' \
  -d '{
    "jsonrpc":"2.0","id":1,
    "method":"message/send",
    "params":{"message":{"parts":[{"kind":"text","text":"你好"}]}}
  }' | jq .

# 查询任务（把 TASK_ID 换成上一步 result.id）
curl -s http://127.0.0.1:18080/a2a/v1 \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tasks/get","params":{"id":"TASK_ID"}}' | jq .

# 取消
curl -s http://127.0.0.1:18080/a2a/v1 \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":3,"method":"tasks/cancel","params":{"id":"TASK_ID"}}' | jq .

# 注册 push（可用 webhook.site 测）
curl -s http://127.0.0.1:18080/a2a/v1 \
  -H 'Content-Type: application/json' \
  -d '{
    "jsonrpc":"2.0","id":4,
    "method":"tasks/pushNotificationConfig/set",
    "params":{"id":"TASK_ID","pushNotificationConfig":{"url":"https://example.com/a2a-hook","token":"secret"}}
  }' | jq .

# 流式（SSE）
curl -N http://127.0.0.1:18080/a2a/v1 \
  -H 'Content-Type: application/json' \
  -H 'Accept: text/event-stream' \
  -d '{
    "jsonrpc":"2.0","id":5,
    "method":"message/stream",
    "params":{"message":{"parts":[{"kind":"text","text":"用一句话介绍你自己"}]}}
  }'
```

设了 `A2A_TOKEN` 时加：`-H "X-A2A-Token: $A2A_TOKEN"`。

系统用户：`__a2a__`（自动确保默认 LLM）。
