---
name: gif-search
description: Search and download GIFs from Tenor (http_fetch for search; host_shell/sandbox_shell curl to download). Use for 找个 GIF / 表情包.
---

# GIF Search (Tenor API)

> 改编自 [Hermes Agent](https://github.com/NousResearch/hermes-agent) `skills/media/gif-search`（MIT，Nous Research）。工具名已换成 open-bot 自己的。

## open-bot 运行方式

- **联网**：读网页/API 用 `http_fetch`（只读 GET，公网）。需要 API Key 的请求：用 `request_secret` 让用户保存密钥，再用 `secret_http`（服务端注入鉴权，明文不回到对话）；`secret_http` 不支持自定义请求头时，改在用户电脑上 `host_shell` + `curl`，密钥放环境变量，不要贴进对话。
- **搜索**：open-bot 没有内置搜索工具。若工具列表里有搜索类 MCP（`mcp__*`）就用它；否则 `http_fetch` `https://html.duckduckgo.com/html/?q=<URL 编码关键词>` 取结果页，再逐条 `http_fetch` 原文。
- **算/跑脚本**：需要 Python/curl/jq 时用 `sandbox_shell`（先 `sandbox_ensure`）；产物放 `/workspace/out/`。
- **持续性**：需要跨会话记住的状态用 `memory_write` / `memory_recall`；定时任务用 `create_routine`（5 字段 cron + prompt），查看/暂停/删除用 `list_routines` / `pause_routine` / `delete_routine`。


Search and download GIFs directly via the Tenor API using curl. No extra tools needed.

## When to use

Useful for finding reaction GIFs, creating visual content, and sending GIFs in chat.

## Setup

Set your Tenor API key in your environment (export it in `~/.zshrc`, or store it with `request_secret` and call the API via `secret_http`):

```bash
TENOR_API_KEY=your_key_here
```

Get a free API key at https://developers.google.com/tenor/guides/quickstart — the Google Cloud Console Tenor API key is free and has generous rate limits.

## Prerequisites

- `curl` and `jq` (both standard on macOS/Linux)
- `TENOR_API_KEY` environment variable

## Search for GIFs

```bash
# Search and get GIF URLs
curl -s "https://tenor.googleapis.com/v2/search?q=thumbs+up&limit=5&key=${TENOR_API_KEY}" | jq -r '.results[].media_formats.gif.url'

# Get smaller/preview versions
curl -s "https://tenor.googleapis.com/v2/search?q=nice+work&limit=3&key=${TENOR_API_KEY}" | jq -r '.results[].media_formats.tinygif.url'
```

## Download a GIF

```bash
# Search and download the top result
URL=$(curl -s "https://tenor.googleapis.com/v2/search?q=celebration&limit=1&key=${TENOR_API_KEY}" | jq -r '.results[0].media_formats.gif.url')
curl -sL "$URL" -o celebration.gif
```

## Get Full Metadata

```bash
curl -s "https://tenor.googleapis.com/v2/search?q=cat&limit=3&key=${TENOR_API_KEY}" | jq '.results[] | {title: .title, url: .media_formats.gif.url, preview: .media_formats.tinygif.url, dimensions: .media_formats.gif.dims}'
```

## API Parameters

| Parameter | Description |
|-----------|-------------|
| `q` | Search query (URL-encode spaces as `+`) |
| `limit` | Max results (1-50, default 20) |
| `key` | API key (from `$TENOR_API_KEY` env var) |
| `media_filter` | Filter formats: `gif`, `tinygif`, `mp4`, `tinymp4`, `webm` |
| `contentfilter` | Safety: `off`, `low`, `medium`, `high` |
| `locale` | Language: `en_US`, `es`, `fr`, etc. |

## Available Media Formats

Each result has multiple formats under `.media_formats`:

| Format | Use case |
|--------|----------|
| `gif` | Full quality GIF |
| `tinygif` | Small preview GIF |
| `mp4` | Video version (smaller file size) |
| `tinymp4` | Small preview video |
| `webm` | WebM video |
| `nanogif` | Tiny thumbnail |

## Notes

- URL-encode the query: spaces as `+`, special chars as `%XX`
- For sending in chat, `tinygif` URLs are lighter weight
- GIF URLs can be used directly in markdown: `![alt](url)`
