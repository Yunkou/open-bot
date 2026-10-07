---
name: coding-edit
description: Disciplined local code edits on a connected computer. Use when changing repo files, fixing a failed edit, or applying patches; re-read before retry and avoid thrashing the same change. Prefer host_shell / host_read / host_write (and sandbox_* for temp work).
---

# 本机改代码（纪律）

在**已连接的电脑**上改用户仓库或本地文件。工具用 open-bot 自己的：`host_read` / `host_write` / `host_shell`（以及临时产物用 `sandbox_*`）。不要假设有单独的「补丁工具」。

## 何时 load

- 用户要改代码、修编译错误、改配置
- 同一处编辑已经失败过一次或多次
- 从 `dev-assist` / `implement` 切入的写码任务

## 流程

1. 选机：`list_machines` → 传 `machine_id`（点名用对应且 connected；否则最常用工作设备）。
2. **先读再改**：`host_read`（或 `host_shell` 看片段）确认当前内容与行上下文。
3. 小步改：优先局部替换；大文件整文件 `host_write` 前必须有本次读取依据。
4. 验证：用 `host_shell` 跑相关测试 / 类型检查 / 构建；把真实输出当依据。
5. 对用户：中文短报——改了什么、怎么验证；不复述整段 diff。

## 失败与重试（硬纪律）

- 编辑失败、测试红、或写入被拒：**先重新读取**相关文件，再改；不要凭上一次记忆里的旧正文继续打补丁。
- **同一处相同改法最多两轮**；第三轮必须换思路（换命令、缩小范围、换文件、先跑测试定位、或 `load_skill diagnosing-bugs` / `tdd`）。
- 不要编造「已改好 / 测试已过」；没有工具输出就说还没跑。
- `denied` / 等待中：按系统结果处理，不要让用户「去电脑上点」代替确认卡。

## 与其它技能

| 场景 | 再 load |
|------|---------|
| 难 bug | `diagnosing-bugs` |
| 先测后写 | `tdd` |
| 按规格落地 | `implement` |
| 审 diff | `code-review` |
| 本机通用命令 | `host-shell` |
| 远程机 | `host-ssh` |

## 禁止

- 用 `sandbox_*` 冒充用户仓库路径
- 对用户提 sandbox / Docker / 容器 / 内部路径
- 无已连接电脑时硬调 `host_*`；应说明先开桌面应用
