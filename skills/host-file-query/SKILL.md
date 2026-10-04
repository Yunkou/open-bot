---
name: host-file-query
description: Query files on a connected computer with compact summaries (largest by size, by extension such as mp4, newest of a type). Prefer the host_file_query tool when available; otherwise use this skill with host_shell. Use when the user asks for biggest/largest files, which mp4s, Downloads disk use, or top-N by size/type. Prefer this over dumping host_ls.
---

# 本机文件查询（摘要）

在**已连接的电脑**上做聚合/筛选，只回少量摘要行。不要为了「最大 / 哪些 mp4」整目录 `host_ls`。

## 默认路径（优先）

直接调用工具 **`host_file_query`**（只读；经与 `host_shell` 相同的 Auto-review / 确认 / 拒绝路径，不绕过）：

| query | 含义 | 参数 |
|------|------|------|
| `largest` | 目录下按大小前 N | `path`（默认 ~/Downloads）、`limit`（默认 10） |
| `by_ext` | 某扩展名按大小前 N | 另需 `ext`（如 mp4） |
| `newest` | 某扩展名按修改时间新→旧 | 另需 `ext`；`limit` 默认 20 |

需要选机时先 `list_machines`，再传 `machine_id`。

## 何时仍 load 本 skill

定制查询、或模型需要看 `scripts/` 正文时：`load_skill` **不会**自动执行 `scripts/`；取到脚本后用 **`host_shell`** 在目标机跑（是否确认由 Auto-review 看命令行，不看脚本正文）。

### 用 host_shell 跑脚本（补充）

不要用 bash heredoc。命令行里的 `<<`（例如 `bash -s <<'SCRIPT'`）会被 Go Auto-review 判为需确认，不会自动执行。

取到脚本正文后，用 `bash -c` 执行（把 `SCRIPT_BODY` 换成脚本内容；正文里的单引号写成 `'\''`），参数放在 `--` 后面：

```bash
bash -c 'SCRIPT_BODY' -- "$HOME/Downloads" mp4 10
```

也可以不取脚本，直接 `host_shell` 跑等价的只读 `find` / `du` / `stat`（输出保持短，同样不要写 `<<`）。长尾查询才这样；常见「最大 / 按扩展名」用 `host_file_query`。

## 脚本

| 脚本 | 用途 | 参数 |
|------|------|------|
| `scripts/largest-by-size.sh` | 目录下按大小前 N | `[dir] [n]`，默认 `~/Downloads` `10` |
| `scripts/largest-by-ext.sh` | 某扩展名按大小前 N | `[dir] [ext] [n]`，默认 `~/Downloads` `mp4` `10` |
| `scripts/list-by-ext.sh` | 某扩展名按修改时间新→旧 | `[dir] [ext] [n]`，默认 `~/Downloads` `mp4` `20`；mtime 为 epoch 秒 |

平台：macOS / Linux。Windows 请用 `host_shell` 自拟等价 PowerShell，或说明当前脚本仅 Unix。

## 何时仍用 host_ls

仅浅层浏览、预期条目不多的已知目录（如「桌面有什么」）。`host_ls` 默认条数少、可截断，**不是**「找最大文件」工具。

## 禁止

- 为回答「最大 / 哪些 mp4 / 前几名」而多次或整树 `host_ls`
- 用 heredoc（`<<`）把脚本送进 `host_shell`；改用上面的 `bash -c`，或直接 `host_file_query`
- 用记忆或摘要编造文件名与大小
- 在无已连接电脑时硬调本机工具；应如实说明需先打开桌面应用
- 对用户提及内部运行环境路径或架构词

## 安全

`host_file_query` 与 skill 脚本都不绕过 Auto-review：硬拒绝始终有效；命令行含重定向、`rm`、装包等仍确认。内容经 `load_skill` 可见，便于审计。
