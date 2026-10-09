# 独立验收结论（Verification Verdict）

> 改编自 [Hermes Agent](https://github.com/NousResearch/hermes-agent) `skills/devops/sdlc-review`（MIT，Jakub Wolniewicz + Nous Research）。原技能依赖 Hermes 的 Kanban 看板，这里去掉看板，只保留「独立验收 → 三选一结论」的方法，作为 `code-review` 的参考资料。
>
> 用法：`load_skill(name="code-review", path="references/verification-verdict.md")`。适用于：验收别人（另一个 bot、编码 CLI、同事、PR 作者）交付的工作，需要给出 **通过 / 要求修改 / 上报用户** 的明确结论。

## When to Use

- 用户让你「验收 / 检查一下 X 做完没有 / 这个 PR 能不能合」；
- `codex` / `claude-code` / `opencode` 或 `send_to_agent` 的另一个 bot 声称完成了任务；
- 需要独立结论，而不是接手实现。

## Inputs

- 原始需求与验收标准（聊天记录、issue、PR 描述；用户给的为准）；
- 交付者的完成说明（视为「待验证的声明」，不是证据）；
- 实际交付物：diff、分支、文件、产出文档。

代码在用户电脑上：`host_shell`（`git diff` / `git log` / 跑测试）+ `host_read`；PR：`load_skill github` 用 `gh pr diff` / `gh pr checks`；在沙箱里：`sandbox_shell` / `sandbox_read`。

## Quick Reference

| Verdict | When | Final action |
|---|---|---|
| 通过 Approve | 验收标准逐条有证据、检查通过 | 在对话里给出结论 + 已核对的检查项 |
| 要求修改 Request changes | 存在可修正的具体缺陷 | 列出编号的修改清单（位置、复现、为何违背需求、最低修复标准）；交回实现方 |
| 上报 Escalate | 需要用户决策或外部前置条件 | 说明卡在哪个决定、继续所需的最少信息 |

若结论要写到 GitHub（PR review approve / request changes / 评论），先把内容给用户确认，再用 `gh pr review` 提交。

## Review Lenses

每一轮换一个视角，避免重复上一轮已发现的问题。轮次 = 之前「要求修改」的次数 + 1。

| Round | Lens | How to apply it |
|---|---|---|
| 1 | Artifact | 先冷读 diff / 交付物，再看交付者说明；逐一追查两者不一致之处。 |
| 2 | Execution | 实际 checkout 并运行：构建、测试、亲手走一遍声明的行为（`host_shell`）。 |
| 3+ | Contract | 重读**原始**需求和验收标准逐条审计；确认之前每一轮要求的修改都已落地。 |

需要多人/多视角时（如同时问另一个 bot），给每个审阅者不同的视角说明（只看 diff / 全上下文 / checkout 并运行），而不是同一份说明。

## Procedure

### 1. Orient

整理：原始需求和验收标准、最新完成说明、改动文件 / commit、测试证据、之前轮次的意见。

### 2. Compare requested vs delivered

把每条验收标准对应到具体实现或产出证据，记录遗漏、语义改变和无关改动。

For code work:

1. 用 `host_read` 和 `host_shell` + `rg` 查看改动路径及其调用方。
2. 用 `host_shell` 看 diff，跑项目已有的针对性测试、lint、类型检查或构建。
3. 走一遍报告的失败路径，并至少走一条正常路径。
4. 检查错误处理、边界、并发、数据保留、安全边界和相关的跨平台行为。
5. 确认测试断言的是行为，而不是快照源码或常量。

For non-code work:

1. 看完整交付物，而不只是摘要。
2. 检查正确性、完整性、格式和来源。
3. 影响结论的 URL / 外部事实用 `http_fetch` 或搜索 MCP 核实。

### 3. Choose exactly one verdict

见 Quick Reference。通过时写明实际跑过的检查和不阻塞的已知限制；要求修改时每条都要可复现、可执行。

### 4. Preserve role separation

验收时不要顺手改实现。把修改清单交回实现方，下一轮再独立验证；如果用户明确让你自己修，切换到 `implement` / `coding-edit` 并在结论里说明角色已切换。

## Pitfalls

- **Rubber-stamping:** 完成说明不是独立证据。
- **Reviewer implementation:** 验收者改代码会模糊责任、削弱复审。
- **Vague findings:** 「还需要改改」无法执行。
- **Style-only blocking:** 行为和仓库规范都满足时，不因偏好类细节要求修改。
- **Skipping prior rounds:** 复审要同时确认上轮修改已落地、之前通过的行为没坏。
- **Completing without evidence:** 每个「通过」都要列出实际检查过的项目。

## Verification

提交结论前确认：

- [ ] 读过原始需求和最新完成说明
- [ ] 每条验收标准都对应到证据
- [ ] 看过实际交付物
- [ ] 跑过相关检查，或写明无法运行的原因
- [ ] 复审时重测了上轮要求的修改
- [ ] 考虑了无关回归和范围变化
- [ ] 只给出一个结论
- [ ] 结论里的证据具体且不含密钥
- [ ] 验收者没有改实现文件
