---
name: implement
description: Implement a piece of work from a spec or tickets with small verified steps. Use when the user asks to implement a feature, finish tickets, or build from a written plan.
---

# Implement

改编自 [mattpocock/skills](https://github.com/mattpocock/skills)（MIT），按 open-bot 单会话代理收窄。

按用户给出的规格 / ticket 实现。

1. 先对齐范围与验收标准；路径不清就问。
2. 能测的部分优先 `load_skill tdd`，在约定 seam 上红绿推进。
3. 本机改代码：`list_machines` → `host_ls` / `host_read` / `host_write` / `host_shell`。
4. 定期跑类型检查与单测；结束前跑一轮相关测试套件（或说明为何不能跑）。
5. 完成后 `load_skill code-review`，对照规格与仓库惯例自审。
6. 用户要求提交时再 `host_shell` 走 git；不要擅自 force push / 改历史。

不要：未读规格就大面积改；空口「做完了」却没跑验证；用 sandbox 冒充用户仓库。
