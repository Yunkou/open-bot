---
name: grill-me
description: Relentless interview to stress-test a plan or design. Use when the user wants to be grilled, sharpen a decision, or says grill me / 拷问 / 追问方案.
---

# Grill Me

改编自 [mattpocock/skills](https://github.com/mattpocock/skills) 的 `grilling`（MIT）。上游 `grill-me` 只是一行「Call the Skill tool with grilling」；open-bot **没有**独立 `grilling` 包，也没有 `Skill` 工具——本文件即完整拷问流程。需要边拷问边写术语/ADR 时改用 `grill-with-docs`（或本流程结束后 `load_skill domain-modeling`）。

Interview the user relentlessly until you reach a shared understanding. Map this as a **design tree**: every decision branches into the decisions that hang off it.

Work the tree in **rounds**. The **frontier** is every decision whose prerequisites are already settled: the questions you can ask _now_ without guessing at answers you haven't heard yet. Ask the whole frontier in one round: number each question and give your recommended answer. Then wait for the user's answers before the next round.

Format a round like so:

```
❓ **Q1** - **<question title>**: <question body, might be multiple paragraphs, including multiple choices>

➡️ <your recommended answer>

---

❓ **Q2** - **<question title>**: <question body, might be multiple paragraphs, including multiple choices>

➡️ <your recommended answer>
```

Word each question so "yes" accepts your recommended answer.

Each round the user answers reshapes the tree: settled decisions push the frontier outward and unblock questions that depended on them. Recompute the frontier and ask the next round. A question whose answer depends on another question still open in this round belongs to a _later_ round, not this one.

Finding _facts_ is your job, never the user's. When a frontier question needs a fact from the environment (repo, filesystem, tools), look it up yourself with open-bot tools (`host_shell` / `host_read` / `sandbox_*` / `http_fetch` 等，按场景选)；不要把能查到的事实丢回给用户。Don't block the whole round on slow lookups: only questions downstream of an unsettled fact wait; ask the rest of the frontier now. The _decisions_ are the user's: put each to them and wait. Never answer decisions for them.

The session is done when the frontier is empty: every branch of the design tree visited, nothing left silently assumed. **Do not act on the plan** until the user confirms you have reached a shared understanding.

## open-bot notes

- 触发后直接按本文件执行；不要再 `load_skill grilling`（不存在）。
- 用户说「一次只问一个」时，改为每轮只问 frontier 中的一题。
- 其他技能若要发起拷问：`load_skill grill-me`（不要写「Call the Skill tool」）。
