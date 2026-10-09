---
name: teams-meeting-transcripts
description: 拉取 Microsoft Teams 会议转录（Microsoft Graph API，host_shell + curl）并生成纪要/行动项，可用 routine 定时轮询新会议。Use for Teams 会议纪要 / transcript / action items.
---

# Teams Meeting Transcripts

> 改编自 [Hermes Agent](https://github.com/NousResearch/hermes-agent) `skills/productivity/teams-meeting-pipeline`（MIT，Nous Research + Teknium）。原技能依赖 `hermes teams-pipeline` 命令、Teams 网关和 Graph webhook；这里改成直接调 Microsoft Graph REST API，用 `create_routine` 轮询代替 webhook 订阅。

## open-bot 运行方式

- **在哪执行**：用户已连接的电脑。先 `list_machines` 选机，再 `load_skill host-shell`；命令用 `host_shell`（一次一条，stdout 会截断）。
- **凭据**：`host_shell` 不会注入 open-bot 保存的密钥。需要用户自己在 shell 配置（如 `~/.zshrc`）里 export `MSGRAPH_TENANT_ID` / `MSGRAPH_CLIENT_ID` / `MSGRAPH_CLIENT_SECRET`。不要把密钥值打印到对话里，也不要让用户把密钥贴到聊天中。
- **纪要格式**：拿到转录后 `load_skill meeting-action-items`，按它的格式出纪要和行动项。
- **确认**：往 Teams 频道/聊天发消息、创建日程或任务之前，先把内容给用户确认。

## Prerequisites（一次性）

1. Azure AD（Entra）应用注册，**Application** 权限并已管理员同意：
   - `OnlineMeetings.Read.All`、`OnlineMeetingTranscript.Read.All`
   - （按 join URL 查会议时）`User.Read.All` 用于把组织者邮箱换成 user id
2. Teams 管理员为该应用配置 **application access policy**（`New-CsApplicationAccessPolicy` + `Grant-CsApplicationAccessPolicy`），否则应用权限读不到用户的会议 —— 这是最常见的 403 原因。
3. 会议开启了转录（Transcription）。没转录就没有 transcript 可拉。

检查变量是否就绪（只看是否存在，不输出值）：

```bash
for v in MSGRAPH_TENANT_ID MSGRAPH_CLIENT_ID MSGRAPH_CLIENT_SECRET; do [ -n "${!v}" ] && echo "$v ok" || echo "$v MISSING"; done
```

> 若 `host_shell` 的环境里读不到用户在 `~/.zshrc` 中设置的变量，在命令前加 `source ~/.zshrc >/dev/null 2>&1;`。

## 取 token（client credentials）

每次需要时重新取，存在临时文件里，不回显：

```bash
curl -s -X POST "https://login.microsoftonline.com/$MSGRAPH_TENANT_ID/oauth2/v2.0/token" \
  -d "client_id=$MSGRAPH_CLIENT_ID" -d "client_secret=$MSGRAPH_CLIENT_SECRET" \
  -d "scope=https://graph.microsoft.com/.default" -d "grant_type=client_credentials" \
  | python3 -c 'import sys,json;d=json.load(sys.stdin);open("/tmp/.msgraph_token","w").write(d.get("access_token",""));print("token ok" if d.get("access_token") else d)'
chmod 600 /tmp/.msgraph_token
```

后续请求统一用：`-H "Authorization: Bearer $(cat /tmp/.msgraph_token)"`。用完可 `rm -f /tmp/.msgraph_token`。

## 常用操作

### 1. 找到会议

需要组织者的 user id（邮箱可换）：

```bash
curl -s -H "Authorization: Bearer $(cat /tmp/.msgraph_token)" \
  "https://graph.microsoft.com/v1.0/users/organizer@contoso.com?\$select=id,displayName"
```

按 join URL 查会议（URL 要完整，含参数）：

```bash
curl -s -G -H "Authorization: Bearer $(cat /tmp/.msgraph_token)" \
  "https://graph.microsoft.com/v1.0/users/<ORGANIZER_ID>/onlineMeetings" \
  --data-urlencode "\$filter=JoinWebUrl eq '<JOIN_WEB_URL>'"
```

`/meet/` 开头的短链接必须用组织者 id 查；如果查不到，让用户从会议详情里复制完整的 join 链接。

### 2. 列出并下载转录

```bash
# 列出某会议的转录
curl -s -H "Authorization: Bearer $(cat /tmp/.msgraph_token)" \
  "https://graph.microsoft.com/v1.0/users/<ORGANIZER_ID>/onlineMeetings/<MEETING_ID>/transcripts"

# 下载 VTT 文本
curl -s -H "Authorization: Bearer $(cat /tmp/.msgraph_token)" \
  "https://graph.microsoft.com/v1.0/users/<ORGANIZER_ID>/onlineMeetings/<MEETING_ID>/transcripts/<TRANSCRIPT_ID>/content?\$format=text/vtt" \
  -o /tmp/teams-<MEETING_ID>.vtt && wc -c /tmp/teams-<MEETING_ID>.vtt
```

VTT 很长时先压成「说话人: 内容」纯文本再读：

```bash
python3 - <<'PY'
import re,sys
t=open('/tmp/teams-<MEETING_ID>.vtt',encoding='utf-8').read()
out=[]
for m in re.finditer(r'<v ([^>]+)>(.*?)</v>',t,re.S):
    s,x=m.group(1).strip(),' '.join(m.group(2).split())
    if out and out[-1][0]==s: out[-1][1]+=' '+x
    else: out.append([s,x])
open('/tmp/teams-<MEETING_ID>.txt','w').write('\n'.join(f'{s}: {x}' for s,x in out))
print(len(out),'turns')
PY
```

然后用 `host_read` 分段读 `/tmp/teams-<MEETING_ID>.txt`，按 `meeting-action-items` 输出纪要。

### 3. 列出组织者最近所有转录（不知道会议 id 时）

```bash
curl -s -G -H "Authorization: Bearer $(cat /tmp/.msgraph_token)" \
  "https://graph.microsoft.com/v1.0/users/<ORGANIZER_ID>/onlineMeetings/getAllTranscripts(meetingOrganizerUserId='<ORGANIZER_ID>')" \
  --data-urlencode "\$filter=createdDateTime ge 2026-01-01T00:00:00Z"
```

（`getAllTranscripts` 在部分租户只在 beta 端点可用；v1.0 报 400 时把 `v1.0` 换成 `beta` 重试。）

## 定时自动出纪要（代替 webhook）

open-bot 没有可供 Graph 回调的公网 webhook，改用定时轮询：

1. 和用户确认组织者账号、检查频率（如工作日每 2 小时）和纪要发到哪里（默认就在本对话）。
2. `create_routine`：
   - `name`: `teams-transcripts-<organizer>`
   - `schedule_cron`: 如 `0 10-20/2 * * 1-5`，`timezone` 默认 Asia/Shanghai
   - `quiet_unchanged: true`（没有新会议就不打扰）
   - `prompt`: 「load_skill teams-meeting-transcripts；取 token；用 getAllTranscripts 查 <ORGANIZER_ID> 在上次检查之后新增的转录；对每个新转录生成纪要（meeting-action-items 格式）；用 memory_write（tier=log，tag `teams-transcripts:<organizer>`）记录已处理的 transcript id；先用 memory_recall 跳过已处理的。」
3. 已处理列表用 `memory_recall` 查 `teams-transcripts:<organizer>`，避免重复出纪要。

## 发回 Teams（可选，需确认）

用户若要把纪要贴回 Teams 频道，最简单的是频道的 **Incoming Webhook / Workflows webhook URL**（让用户自己 export 成 `TEAMS_WEBHOOK_URL`）：

```bash
python3 -c 'import json,sys;print(json.dumps({"text":open("/tmp/teams-summary.md").read()}))' \
  | curl -s -X POST -H "Content-Type: application/json" -d @- "$TEAMS_WEBHOOK_URL"
```

发之前把纪要全文给用户看并得到明确同意。

## Pitfalls

- **转录还没生成**：会议结束后通常要几分钟。列表为空就等 2–5 分钟再查，routine 下一轮会自动补上。
- **401/403**：token 能拿到但调用失败，多半是权限加了却没重新「Grant admin consent」，或缺 application access policy。
- **token 过期**：约 1 小时。长任务中途 401 就重新取 token。
- **大会议**：VTT 可能上 MB；先压成纯文本，再分段读，不要整段塞进上下文。
- **隐私**：转录含他人发言。只按用户要求的范围总结，不要把原文转发到别处。
