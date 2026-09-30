---
name: host-ssh
description: Connect to remote hosts headlessly over SSH/SFTP from a connected desktop app. Use for ssh, remote files, servers, Host aliases in ~/.ssh/config, and remote commands. Do not open a system terminal.
---

# Host SSH（无界面）

应用在**已连接的电脑桌面端**内嵌 SSH 客户端拨号，自动使用该机 `~/.ssh/config`、默认私钥、目录里其它私钥，以及 ssh-agent。不要用 `host_shell` / 系统终端跑 `ssh`、`scp`、`sftp`。

本包**没有**可执行的拨号脚本：Agent Skills 的 `scripts/` 在本产品里也不会替代 `host_ssh_*`。流程与排错见包内附属文件（需要时再 `load_skill` 并带 `path`）：

- `references/playbook.md` — 列目录 / 读写 / 远程命令步骤
- `references/errors.md` — `tried` / `agent` / 主机密钥错误怎么读

## 何时 load

用户提到：ssh、远程主机、服务器上的文件/命令、`user@host`、`~/.ssh/config` 里的 Host 别名。

## 平台

| 端 | 能力 |
|----|------|
| macOS / Windows / Linux **桌面应用** | 可拨号；用该机密钥与 config |
| Web / Android / iOS | **不拨号**；可对话、点确认。执行须落到一台 `list_machines` 里 **connected** 的电脑 |

手机或网页发起时：选已连接电脑的 `machine_id`（用户点名则用那台；否则用最常用工作设备）。没有已连接电脑就说明要先打开桌面应用。

## 工具

先完成本 skill（及必要时上述 references），再调用：

- `host_ssh_ls` / `host_ssh_read`：列目录、读文本
- `host_ssh_write` / `host_ssh_delete` / `host_ssh_exec`：写入、删除、远程命令（对话确认；任意已登录端可点允许/拒绝，仍在目标电脑执行）

参数：

- `host`：IP、域名，或 `~/.ssh/config` 的 Host 别名；也可 `user@host`
- `user` / `port`：可省略，按 config 与默认
- `path` / `command` / `content`：按工具要求

## 主机密钥

- 新主机：写/删/exec 的确认卡片会带指纹；允许后写入 `known_hosts`
- 与 `known_hosts` 不一致：连接断开，按错误说明处理，不要改用系统 `ssh`

## 失败时

只根据工具 JSON 解释（详见 `references/errors.md`）：

- `tried`：已尝试的私钥与结果（`missing` / `encrypted` / `rejected` / `unreadable`）
- `agent`：`empty` / `unavailable` / `tried_n` 等
- `encrypted`：私钥有口令，当前不支持弹口令；可建议无口令钥或用户自行 `ssh-add`

不要编造「权限 600 / 我先不走 agent 再测」之类旁白。客户端已自动找钥并试连。

## 禁止

- `host_shell` + `terminal=true` 打开系统 SSH
- 在沙箱/`sandbox_*` 里假装连用户的远程机
- 在仅手机/网页且无已连接电脑时硬调 `host_ssh_*`
