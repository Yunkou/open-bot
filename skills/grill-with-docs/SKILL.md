---
name: grill-with-docs
description: Grill a plan while writing GLOSSARY.md and ADRs as terms/decisions settle. Use when designing in a repo and wanting a paper trail, or 边拷问边写术语表.
---

# Grill With Docs

改编自 [mattpocock/skills](https://github.com/mattpocock/skills)（MIT）。上游包体是「Call the Skill tool twice…」；open-bot 改为显式组合两个已入库技能。

## 流程

1. **`load_skill grill-me`**，按其中的 design-tree / frontier / rounds 拷问（本仓没有独立 `grilling`）。
2. **`load_skill domain-modeling`**，在拷问过程中按该技能纪律写文档：
   - 术语一敲定就写入 `GLOSSARY.md`（或 `GLOSSARY-MAP.md` 指向的上下文 glossary）
   - 难逆、需上下文、有真实取舍的决策 → `docs/adr/` 下的 ADR
   - 其余决定只留在对话里，不要塞进 glossary

先读仓库里已有的 `GLOSSARY.md` / ADR，拷问时对照挑战冲突用语。文件按需创建，没有内容就不要空建。

## open-bot

- 写仓库文件用已连接电脑上的 `host_read` / `host_write`（或用户指定的工作区工具）；危险写入走确认卡。
- 不要使用「Skill tool」或依赖未安装的 `grilling` 包名。
- 会话结束、frontier 清空后，等用户确认共享理解再动手实现。
