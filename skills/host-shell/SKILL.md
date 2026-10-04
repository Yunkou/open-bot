---
name: host-shell
description: Run local commands on a connected computer via host_shell. Use for any on-machine shell work (inspect, build, process, open apps) with short output; revise on error. Not for ssh (use host-ssh) or file-summary recipes (use host-file-query).
---

# 本机 Shell

在**已连接的电脑**上用 `host_shell` 跑本地命令。输出保持短；失败就换一条，不要重复同一条命令。

## 何时 load

用户要在本机执行命令、打开软件、读写/删除/移动文件，或你需要自行拼命令时。

- 文件「最大 / 按扩展名 / 最新」等摘要：优先 `load_skill host-file-query`
- 远程 SSH / 远程文件：`load_skill host-ssh`（不要用 `host_shell` 跑 `ssh`/`scp`/`sftp`）

## 基本流程

1. 需要选机时先 `list_machines`，再传 `machine_id`（用户点名用对应且 connected 的；没点名用最常用工作设备）。
2. 本机命令：`host_shell`（自己写；stdout 可能被截断）。
3. 浅层浏览已知小目录：`host_ls`（不要拿它做「最大文件」汇总后再自己排序）。
4. 打开软件：`host_open`。
5. 删除：`host_delete`（多个文件传 `paths` 一次完成）。用户说再试一次且上一轮是删除时，再次调用 `host_delete`。
6. 写入/移动：`host_write` / `host_move`。

## 输出与失败

- 打印短摘要，不要整树 dump。
- 工具结果若在**等待**：尚未执行，不要声称已跑完或让用户「去电脑上确认」代替系统确认卡。
- **denied**：用户拒绝，停下来说明。
- 硬危险命令可能直接失败且没有允许卡。
- 不要编造文件名、大小或成功结果；没有已连接电脑时先 `list_machines` 并说明需打开桌面应用。

## 确认与安全（摘要）

覆盖、删除、移动、主目录外写入、看起来会改动或有风险的 `host_shell`（以及 `terminal`）、远程写入/删除/执行：立刻调用对应工具；由系统 Auto-review / 确认卡处理，你不能自己批准或拒绝。「始终允许」且自动审核开着时，除硬拒绝和用户规则「先询问」外会直接执行。操作仍在目标电脑执行。

## 禁止

- 用 `host_shell` 跑系统 `ssh` / `scp` / `sftp`
- `terminal=true` 仅用于打开可见本机终端，不要用来拨 SSH
- 用 `sandbox_*` 或记忆冒充本机 Downloads/桌面
- 对用户提及 sandbox / Docker / 容器 / 内部路径
