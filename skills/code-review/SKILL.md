---
name: code-review
description: Review changes since a fixed point along Standards and Spec axes. Use when reviewing a branch, PR, WIP diff, or when the user asks to review since a commit/branch.
---

# Code Review（双轴）

改编自 [mattpocock/skills](https://github.com/mattpocock/skills)（MIT）。open-bot 在同一会话内**顺序**做两轴（不依赖并行子代理）。

对比 `HEAD` 与用户给定定点（commit / 分支 / tag / `main` 等）：

- **Standards**：是否符合本仓文档化编码规范
- **Spec**：是否忠实实现来源 issue / 规格

## 流程

### 1. 钉定点

用户没给就问。用 `git diff <定点>...HEAD`（三点）与 `git log <定点>..HEAD --oneline`。先 `git rev-parse`；空 diff 直接停。

本机执行：`host_shell`（需已连接电脑）。

### 2. Spec 来源（按序）

1. 提交信息里的 issue 引用
2. 用户给的路径
3. `docs/`、`specs/`、`.scratch/` 下匹配分支/功能的文件
4. 都没有就问；用户说没有 → Spec 轴报告「无规格」

### 3. Standards 来源

仓库内 `CODING_STANDARDS.md`、`CONTRIBUTING.md` 等。另加 **气味基线**（Fowler《重构》ch.3 启发式，永远是判断题；仓内明文规范优先）：

Mysterious Name / Duplicated Code / Feature Envy / Data Clumps / Primitive Obsession / Repeated Switches / Shotgun Surgery / Divergent Change / Speculative Generality / Message Chains / Middle Man / Refused Bequest。

工具已强制检查的项跳过。

### 4. 两轴报告

先写 `## Standards`，再写 `## Spec`，**不要合并重排**。每条尽量落到文件/hunk；Spec 引用规格原文。合计各轴条数与该轴最严重问题；不跨轴挑「唯一最差」。

无规格时 Spec 轴写明跳过。

## 为何两轴

规范满分但做错需求 → Standards 过、Spec 不过；需求做对但破坏惯例 → 相反。分开报才不会互相掩盖。
