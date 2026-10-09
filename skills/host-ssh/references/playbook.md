# Host SSH 操作手册（渐进加载）

在已 `load_skill host-ssh` 后，需要细节时再 `load_skill` 并带 `path: references/playbook.md`。

**拨号不靠本包脚本。** 真正连接由已连接电脑桌面端的 `host_ssh_*` 完成；不要用 `host_shell` 跑系统 `ssh`/`scp`。

## 常见流程

### 1. 列远程目录

1. 确认发起端：若当前是 Web/手机，先 `list_machines`，选 `connected` 的电脑 `machine_id`。
2. `host_ssh_ls`：`host`（IP / 域名 / `~/.ssh/config` Host 别名，或 `user@host`），可选 `path`（默认家目录）。

### 2. 读远程文本

`host_ssh_read`：`host` + `path`。大文件先 `ls` 确认路径。

### 3. 写 / 删 / 远程命令

`host_ssh_write` / `host_ssh_delete` / `host_ssh_exec` 会出确认卡片（任意已登录端可点允许）。

- 新主机：卡片带主机密钥指纹；允许后写入该电脑 `known_hosts`
- `host_ssh_exec` 的 `command` 是**远程** shell 一行命令，不是本机命令

### 4. 参数习惯

| 字段 | 说明 |
|------|------|
| `host` | 必填；别名优先走用户 config |
| `user` / `port` | 可省略 |
| `machine_id` | 多台已连接电脑时指定执行机 |

## 不要做

- `host_shell` + `ssh` / `terminal=true` 开系统终端
- 沙箱里假装连用户的机
- 无已连接电脑时硬调 `host_ssh_*`
