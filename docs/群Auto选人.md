# 群 Auto 选人（轻量设计）

日期：2026-10-10 · 状态：A 档已实现（启发式；见 planSendTargets / autoPickHeuristic）· 范围：群聊无人 `@` 时的候选收敛  
对照现状：`resolveGroupSendTargets` — 无人 `@` → **全员候选**；各员可 `[PASS]`；`@` / `@everyone` 强制回复。

---

## 1. 问题

无人 `@` 时全员并行候选 + PASS，成本高、延迟大，且易出现「全员敷衍 / 全员沉默再 fail」。  
需要一层**轻量选人**，在保留现有 PASS / 全员候选能力的前提下，默认少跑几路。

## 2. 目标行为

| 用户输入 | 选人 | 与现状关系 |
|---|---|---|
| `@某人` / 多 `@` | 仅被点名者（forced，不可 PASS） | **不变** |
| `@everyone` / `@all` | 全员 forced | **不变** |
| **无人 `@`** | **Auto 选人** → 1～K 名候选；未入选者不跑 | **新增**；候选仍可 PASS |
| Auto 失败 / 超时 / 无把握 | **fail-open → lead**（`conv.AgentID` / 群主助理）单路 | **新增兜底** |

K 建议默认 **1**（可配置上限 2～3）。Lead 始终可作为兜底，不因 Auto 静默丢回复。

## 3. 轻量选人（不做重编排）

输入：用户本轮文本、成员名 + 短角色（已有 `groupParticipationRoleHint`）、可选最近 1～2 条摘要。  
输出：有序 `agent_id[]`（长度 ≤ K）+ `confidence`。

推荐实现顺序（由轻到重，可只做 A）：

| 档 | 做法 | 成本 |
|---|---|---|
| **A. 启发式** | 关键词 / 角色描述子串命中；否则 lead | 无额外 LLM |
| **B. 小分类器** | 一次短 prompt：`只输出 JSON {ids, confidence}`，温度低、max tokens 小 | 1 次小调用 |
| **C. 嵌入** | 成员角色向量 vs 用户句（后置） | 中 |

**Fail-open 规则（硬）：**

- 解析失败、超时、空列表、`confidence < θ` → `[lead]`
- lead 不在群成员里 → 退回**全员候选**（兼容旧行为，避免无回复）
- Auto 选人**不得**替代 `@` 强制集合

## 4. 与 PASS / 全员候选并存

```
resolveGroupSendTargets
  ├─ @ / @everyone / explicit → 现状（forced）
  └─ no @ →
        AutoPick(K) 成功 → 候选子集（非 forced，可 PASS）
        AutoPick 失败   → [lead]（非 forced；lead 提示「你是兜底，请回答」）
        配置关闭 Auto   → 全员候选（现状）
```

- 选中的非 forced 候选：**保留** `[PASS]` 提示与「不落泡」逻辑。  
- 若 Autopick 的唯一候选也 PASS → 二次 fail-open：补跑 lead（或全员），避免「直接无返回」。  
- 开关：`channel.auto_pick` 或 org flag；默认 **开（A 档）** 便于灰度。

## 5. 文案 / 可观测

- 内部日志：`auto_pick=heuristic|llm ids=… conf=… fallback=lead|all`  
- 用户侧不展示选人过程；可选调试：群设置「高级 · 显示本轮候选」（默认关）。

## 6. 非目标

- 不做多轮辩论 / 主持人投票  
- 不改 DM  
- 不在本设计落地 Team pack / 跨群路由

## 7. 验收（实现时）

1. 「@码农 修 bug」→ 仅码农，无 Auto。  
2. 「大家介绍自己」`@everyone` → 全员 forced。  
3. 「帮我写个函数」无人 `@` → 优先角色匹配的 1 人；其 PASS → lead 补答。  
4. Auto 超时 → 仅 lead 答，不出现空回合。  
5. 关闭 Auto → 与今日全员+PASS 一致。
