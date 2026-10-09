---
name: product-price-monitor
description: Watch product, flight or listing prices and alert on a target, using http_fetch + memory_* + create_routine. Use for 降价提醒 / 价格监控.
---

# Product Price Monitor

> 改编自 [Hermes Agent](https://github.com/NousResearch/hermes-agent) `skills/productivity/product-price-monitor`（MIT，Nous Research）。工具名已换成 open-bot 自己的。

## open-bot 运行方式

- **联网**：读网页/API 用 `http_fetch`（只读 GET，公网）。需要 API Key 的请求：用 `request_secret` 让用户保存密钥，再用 `secret_http`（服务端注入鉴权，明文不回到对话）；`secret_http` 不支持自定义请求头时，改在用户电脑上 `host_shell` + `curl`，密钥放环境变量，不要贴进对话。
- **搜索**：open-bot 没有内置搜索工具。若工具列表里有搜索类 MCP（`mcp__*`）就用它；否则 `http_fetch` `https://html.duckduckgo.com/html/?q=<URL 编码关键词>` 取结果页，再逐条 `http_fetch` 原文。
- **算/跑脚本**：需要 Python/curl/jq 时用 `sandbox_shell`（先 `sandbox_ensure`）；产物放 `/workspace/out/`。
- **持续性**：需要跨会话记住的状态用 `memory_write` / `memory_recall`；定时任务用 `create_routine`（5 字段 cron + prompt），查看/暂停/删除用 `list_routines` / `pause_routine` / `delete_routine`。


Monitor a concrete purchasable item and alert on a normalized all-in price or availability condition. Handle variants, taxes, fees, currencies, stock, cancellation terms, and duplicate alerts explicitly. Setup runs once in the foreground; the recurring check runs as an open-bot routine (`create_routine`).

## When to Use

- "Alert me when this laptop drops below $1,000."
- "Watch these flights for a fare under $500."
- "Tell me when this hotel has a refundable room."
- "Track ticket/listing availability."
- A routine run (`[routine]` prompt) fires for an existing price watch (steps 4-6).

Don't use for: one-off "what does this cost right now" lookups (use 搜索（搜索 MCP 或 `http_fetch` DuckDuckGo HTML）/`http_fetch` directly).

## Procedure — Setup (foreground, once)

### 1. Define the exact item

Record source URL/provider, product/listing ID where available, variant, quantity, location, dates, travelers/guests, membership/login assumptions, condition, seller, and acceptable substitutes. Done when two variants cannot be confused.

### 2. Define the alert condition

Specify currency, all-in vs pre-tax price, maximum price, availability/stock rule, shipping, refundability, cabin/room/ticket class, cooldown, and notification destination. Done when synthetic examples have deterministic alert decisions.

### 3. Establish a live baseline, then schedule

Fetch a bounded live result with `http_fetch` (or a Playwright script via `sandbox_shell` when the page needs JS / interaction — see `dogfood`) and record retrieval time, source price, fees/taxes, availability, and terms. Do not schedule until one foreground fetch works. Save the watch contract (item, condition, baseline observation) as described below, then create the routine.

**状态保存（open-bot）**：例行任务在服务端触发，不一定连着用户电脑，所以不要依赖本机状态文件。把 watch 合约和每次观测存进记忆：`memory_write(tier="note", content="[price-watch:<watch-slug>] <JSON：合约 / last_good / last_alert_fingerprint / last_cutoff>")`；每次 tick 先 `memory_recall(query="price-watch:<watch-slug>", tier="note")` 取最新一条。需要完整历史时，也可以把 JSON 写到内部环境 `/workspace/state/price-watch/<watch-slug>.json`（`sandbox_write`），但以记忆里的最新一条为准。

```
create_routine(
  name="price-watch <watch-slug>",
  schedule_cron="0 */6 * * *",
  timezone="Asia/Shanghai",
  prompt="load_skill product-price-monitor，按 Tick 步骤检查 price-watch:<watch-slug>（memory_recall 取合约与上次观测），只在满足条件时提醒。",
  quiet_unchanged=true)
```

Pick a cadence that respects rate limits and site terms. The routine is bound to the current conversation by default, so alerts arrive here. Tell the user how to stop it (`list_routines` / `pause_routine` / `delete_routine`). Done when the baseline matches the exact item contract and the routine exists.

## Procedure — Tick (each scheduled run)

### 4. Fetch and normalize

Re-fetch the source. Convert currency only with a timestamped rate and retain the source currency. Separate base price, mandatory fees, shipping/taxes, total, and availability. Exclude volatile page metadata. A failed fetch means unknown state: report or skip, but never overwrite the last good observation with an error page. Done when the observation is comparable to the baseline or explicitly marked failed.

### 5. Compare and suppress duplicates

Alert on threshold entry, qualifying availability, material lower price, or recovery as requested. Store the last good observation and last alert fingerprint with `memory_write` (tag `price-watch:<watch-slug>`). Replaying the same offer must send no second alert; respect the cooldown. Done when the alert decision is deterministic against stored state.

### 6. Deliver or stay silent

When a condition is met, the alert includes: exact item/variant, observed all-in price and source currency, availability/terms, threshold, retrieval timestamp, source link, and important uncertainty. Never claim inventory is reserved. When nothing qualifies, stay silent — no "still watching" noise unless a periodic all-clear was requested. Done when the stored state (memory) reflects this run.

## Pitfalls

- Comparing a base fare with an all-in threshold.
- Alerting on the wrong size, seller, cabin, dates, or room terms.
- Overwriting a last-known-good value with an error page.
- Polling aggressively enough to trigger blocking or violate site terms.
- Scheduling before a single foreground fetch has succeeded.

## Verification

- [ ] The watch contract pins the item so two variants cannot be confused.
- [ ] One foreground fetch succeeded before any routine was created.
- [ ] Alert decisions replay deterministically from the stored state; duplicates suppressed.
- [ ] Failed fetches never replaced last-known-good state.
- [ ] Alerts carry all-in price, source currency, timestamp, and source link.
