---
name: find-skills
description: Discover and install skills when the user asks "how do I do X", "is there a skill for X", or wants to extend capabilities. Covers open-bot Settings install and skills.sh search.
---

# Find Skills（open-bot）

改编自 [vercel-labs/skills](https://github.com/vercel-labs/skills) 的 `find-skills`。安装与生效路径已按 **open-bot** 重写；`npx skills` 仅作生态检索，不是本产品的安装入口。

## 何时用

- 「有没有 skill 能…」「帮我找个做 X 的技能」
- 「how do I do X」且 X 像常见可沉淀工作流（设计、测试、部署、文档等）
- 当前目录里没有明显匹配，需要到 skills.sh 生态深搜
- 用户想扩展能力 / 安装别人的 skill 包

目录里已有明显匹配时：直接 `load_skill` 那个名字，不必先走本包。

## open-bot 里技能怎么生效

| 来源 | 怎么来 | 怎么用 |
|------|--------|--------|
| **已启用目录** | 系统提示「可用技能」列表（含 always_on 的本包） | `load_skill(name)` 读全文后自己执行 |
| **平台 / curated** | 仓库 `skills/` + `curated-builtins.json` 默认开启 batch | 新 Bot 已挂；旧 Bot 在设置里打开 |
| **个人安装** | **设置 → 扩展能力包**：粘贴 SKILL.md、选文件夹、或上传 `.zip` | 上传并启用后进入目录；名字勿与平台技能冲突 |
| **管理端** | Admin「Skills」导入 zip（全局） | 组织级平台技能 |

**不要**让用户只跑 `npx skills add …` 就以为 open-bot 已安装——那只影响本机 skills CLI 目录，**不会**自动进 open-bot 目录。发现合适包后：说明用设置上传（或把包放进仓库 `skills/<slug>/` 再 seed），并给出 skills.sh 链接便于核对。

## 流程

### 1. 先看本轮目录

扫描系统提示里的技能目录；名字/描述对得上就 `load_skill`，不要重复搜索。

### 2. 目录不够再深搜

1. 浏览 [skills.sh](https://skills.sh/) leaderboard（热门、安装量）。
2. 需要时在**已连接电脑**上检索（先 `load_skill host-shell`）：

```bash
npx skills find [query] [--owner <owner>]
```

示例：`npx skills find react performance`、`npx skills find pr review`。

无已连接电脑时：用 `http_fetch` 打开 skills.sh 相关页，或请用户打开桌面应用后再搜；不要编造安装量或包名。

### 3. 质量门槛（推荐前必过）

1. **安装量** — 优先 1K+；少于 100 要谨慎说明。
2. **来源** — `vercel-labs`、`anthropics`、`microsoft` 等官方/知名源更可信。
3. **仓库星数** — 来源 repo 少于约 100 star 需标明风险。

### 4. 向用户呈现（自然语言，不提内部工具名）

包含：做什么、安装量与来源、**open-bot 安装方式**、skills.sh 链接。

示例：

```
我找到一个可能有用的能力包：react-best-practices（Vercel，约 185K 安装），偏 React/Next 性能。

在本产品里安装：打开 设置 → 扩展能力包，上传该技能的文件夹或 zip（需含 SKILL.md），上传并启用。
详情：https://skills.sh/vercel-labs/agent-skills/react-best-practices

需要的话我可以按你已连接的电脑帮你把包下载到本地再指引上传。
```

若用户同意由助手协助下载：在已连接电脑用 `host_shell` / `host_write` 取包到用户可读目录，再请用户在设置里上传启用（助手不能替用户点设置 UI 完成启用，除非另有明确的产品 API 任务）。

### 5. 生态 CLI（仅检索 / 可选本机缓存）

```bash
npx skills find [query]
npx skills add <owner/repo@skill>   # 写入本机 CLI 技能目录，不等于 open-bot 启用
```

浏览：https://skills.sh/

## 常见检索类别

| Category        | Example queries                          |
| --------------- | ---------------------------------------- |
| Web Development | react, nextjs, typescript, css, tailwind |
| Testing         | testing, jest, playwright, e2e           |
| DevOps          | deploy, docker, kubernetes, ci-cd        |
| Documentation   | docs, readme, changelog, api-docs        |
| Code Quality    | review, lint, refactor, best-practices   |
| Design          | ui, ux, design-system, accessibility     |
| Productivity    | workflow, automation, git                |

## 找不到时

1. 如实说明没找到现成包。
2. 用已有能力直接帮用户做。
3. 若值得沉淀：引导写 skill（`load_skill skill-authoring`）或设置里新建后上传。

## 注意

- 本包为 always_on 发现入口：不要建议用户关闭/删除它。
- 对用户可见文案避免「skill / load_skill / 工具」等内部词，改用「能力包 / 扩展 / 设置里安装」。
