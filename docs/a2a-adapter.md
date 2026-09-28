# A2A 适配层说明

open-bot 提供的 A2A 接口是**适配层 / 演示网关**，用于：

- 发布符合 A2A 思路的 Agent Card（`GET /.well-known/agent-card.json`）
- 接受 JSON-RPC `message/send`（及 v1 别名 `SendMessage`），内部落到现有 runtime 出回复

**不是**：

- 完整 A2A 认证 / OAuth / 卡片签名
- 流式 `SendStreamingMessage`、任务订阅、push notification
- 与官方 SDK 的生产级互操作保证

可选环境变量：`A2A_TOKEN`、`A2A_PUBLIC_BASE`。未设 `A2A_TOKEN` 时端点开放（仅适合本机开发）。
