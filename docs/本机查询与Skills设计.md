# 本机查询：host_ls vs host_shell vs Skill 脚本（设计草案）

> 状态：**A+B+C+D 已落地**（提示词路由、`skills/host-file-query`、收窄 `host_ls`、只读 `host_shell` allowlist 软放行）。
> 对齐 Cursor Grok Bot（`ListMachines` + 本机 Shell/Read；Skills 可带 `scripts/`）。
> 相关实现：`apps/desktop/.../host_fs.rs`、`host_cmd.rs`；runtime `llm.py` / `client_env.py` / `deferral.py`；`skills/host-file-query/`。

## 现状（代码为准；部分旧文档仍写 stub）

| 工具 | 作用 | 紧凑性 / 限制 |
|------|------|----------------|
| `list_machines` | 已登记电脑 + `connected` | 小 |
| `host_ls` | 浅层列目录 | 默认 **50**（最大 200）；`sort`=`mtime|size|name`；可选 `glob`；响应含 `total`/`truncated` |
| `host_read` | 读文本 | ≤200KB |
| `host_shell` | 本机命令（`sh -c`） | 输出 **≤4000 字**；30s；只读 allowlist **免确认**，其余/terminal **对话确认**；禁 ssh/scp/sftp |
| `host_ssh_*` | 远程（先 `load_skill host-ssh`） | 与上类似 |
| `load_skill` | 加载 `SKILL.md` / `scripts/` **内容** | **不自动执行**脚本；模型再经 `host_shell` / `sandbox_*` 跑 |

Skills 包可含 `scripts/`（例：`diagnosing-bugs/scripts/`），但产品侧没有「skill 脚本直接当工具跑」的通道。

### 核心缺口

1. ~~提示词反模式~~：**已改**——聚合查询走 `host-file-query` + `host_shell`；`host_ls` 仅浅层浏览。
2. ~~`host_ls` 语义过弱~~：**已收窄**——更小默认 limit、可选 sort/glob、标记 truncated。
3. ~~Skill 脚本未产品化~~：**已加** `skills/host-file-query`（`largest-by-size` / `largest-by-ext` / `list-by-ext`）。

## 设计原则（同意用户论点）

- **聚合/筛选/排序 → 在本机算完，只回摘要**（对齐 Grok：`ListMachines` 选机 + Shell）。
- **`host_ls` 收窄为浅层浏览**，不再作为「找最大 / 哪些 mp4」的默认路径。
- **两条推荐路径**（可并存）：
  1. **Skill + 脚本**：常见任务有固定、可审的命令；
  2. **LLM 生成 shell → `host_shell`**：长尾查询，仍返回紧凑 stdout。

## 何时用什么

| 意图 | 推荐 | 避免 |
|------|------|------|
| 有哪些电脑 / 哪台在线 | `list_machines` | — |
| 打开某目录随便看看（条目少） | `host_ls`（可加 limit/sort/glob，见下） | 把整个 Downloads dump 进上下文 |
| 最大文件、按扩展名、前 N、磁盘占用 | **`host_shell`** 或 **skill 脚本** | 依赖 `host_ls` 再在模型侧排序 |
| 读/改单个已知路径 | `host_read` / `host_write`… | 用 ls 猜路径后整目录搬进上下文 |
| SSH 远程 | `load_skill host-ssh` → `host_ssh_*` | `host_shell` 跑 ssh |

## 示例 Skill（草案名 `host-file-query`）

`SKILL.md` 要点：

- 何时 load：用户问「最大的 / 哪些 mp4 / 下载里占空间」等。
- 流程：`list_machines`（若需选机）→ `load_skill` 取脚本 → `host_shell` 执行（或把脚本内容内联为一条只读命令）。
- 禁止：为回答「最大」而多次/整树 `host_ls`。

示例 `scripts/largest-by-ext.sh`（思路，非定稿）：

```bash
#!/usr/bin/env bash
# Usage: largest-by-ext.sh <dir> <ext> [n]
set -euo pipefail
DIR="${1:-$HOME/Downloads}"; EXT="${2:-mp4}"; N="${3:-10}"
find "$DIR" -type f -iname "*.${EXT}" -print0 2>/dev/null \
  | xargs -0 stat -f '%z %N' 2>/dev/null \
  | sort -nr | head -n "$N" \
  | awk '{ printf "%.1fMB\t%s\n", $1/1024/1024, substr($0, index($0,$2)) }'
```

（Windows 另附 PowerShell 变体或 skill 内注明仅 macOS/Linux。）

## 提示词 / 工具描述改动（实现时）

1. **删除/改写**「必须 host_ls」；改为：
   - 聚合查询 → `host_shell` 或相关 skill 脚本，**只汇报摘要行**；
   - `host_ls` → 浅层列举、已知小目录、需要条目元数据且预期条目不多时。
2. `host_ls` description：写明「非排序/筛选/递归汇总工具；大目录请用 host_shell」。
3. `host_shell` description：点名适合 `find`/`du`/`stat` 等只读汇总（只读 allowlist 免确认，见下）。
4. Skills 目录加一条 `host-file-query`（或并入现有 host 相关 skill）。

## 安全 / 确认策略

- **免确认（读）**：`host_ls` / `host_read` / `host_ssh_ls` / `host_ssh_read`；以及本机 `host_shell` 且命令匹配**只读 allowlist**（`ls`/`find`/`du`/`stat`/`md5`/`wc`/`cat`/`head`/`grep`… 的管道组合；无写重定向、无 `$()`/`rm`/`curl|sh`/`find -delete` 等）。实现：`services/api/.../readonly_shell.go` + 客户端 `apps/web/src/lib/hostExec.ts`（deny-by-default）。
- **仍确认**：写/删/移、非 allowlist 的 `host_shell`、`terminal=true` 的 shell、全部 `host_ssh_write|delete|exec`。对话卡任意端可点；本机执行仍在目标机。
- **设置**：桌面端「只读本机命令免确认」可关（localStorage）；关掉后客户端对 shell 仍弹卡（API 对 allowlist 仍可能直接下发执行请求）。
- Skill 脚本（`bash`/`sh script.sh`）**不**在 allowlist 内，仍确认；脚本内容经 `load_skill` 可见。
- 继续截断 stdout（~4k）；`host_ls` 默认 50 + `truncated`/`total`。

## 实现阶段

| 阶段 | 内容 | 风险 |
|------|------|------|
| **A 文档+提示词** | ✅ 改 `client_env` / tool description / deferral；本设计文档 | 低 |
| **B Skill 脚本** | ✅ `host-file-query` + `largest-by-size` / `largest-by-ext` / `list-by-ext` | 低 |
| **C 收窄 host_ls** | ✅ `limit`/`sort`/`glob`；默认 50；响应 `truncated`/`total` | 中（兼容） |
| **D 只读 shell 放行** | ✅ allowlist + 测试；桌面设置项可关 | 中高（安全面） |

不建议新增与 `host_shell` 重复的 `host_exec` 名称；对外统一 `host_shell`，对内已是 `op=shell`。

## 验收场景（实现后）

1. 「下载里最大的 mp4」→ 不出现数百条 `host_ls` entries；工具结果为十余行摘要。
2. 「随便看看桌面有什么」→ 可用 `host_ls`。
3. 无已连接电脑 → 仍如实说明，不编造。
4. `host_shell` 危险命令 → 仍出确认卡。
