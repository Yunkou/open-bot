---
name: dev-assist
description: Router for local coding work on a connected computer. Use when writing code, fixing bugs, reviewing diffs, or working in a repo; then load diagnosing-bugs, tdd, implement, or code-review as needed.
---

# Dev Assist（写代码入口）

本包是写代码岗的**入口说明**。具体纪律来自改编的 mattpocock skills（MIT）：

| 场景 | 再 load |
|------|---------|
| 难 bug / 变慢 / 报错 | `diagnosing-bugs` |
| 先测后写 / 红绿 | `tdd`（可再读 `tests.md` / `mocking.md`） |
| 按规格落地功能 | `implement` |
| 审 diff / PR / 相对定点 | `code-review` |
| 远程 SSH | `host-ssh`（不要用本包冒充） |

## 本机 vs 临时环境

| 意图 | 工具 |
|------|------|
| 用户仓库 / 本机文件 | `list_machines` → `host_*` |
| 临时脚本与可预览产物 | `sandbox_*`（对用户只谈结果） |

无 connected 电脑：说明先开桌面应用；禁止编造目录或测试输出。

## 最短路径

1. 澄清目标与项目路径。
2. 选上表 skill 并 `load_skill`。
3. 小步改、用工具验证，再回复。
