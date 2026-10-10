# 群 Auto 选人（轻量设计）

日期：2026-10-10 · 状态：三策略已落地（off / heuristic / decision）；**生效模式仅跟 Admin 组织功能开关**（`feature_flags_json.auto_pick`）；客户端不配置群级 auto_pick  
对照现状：`resolveGroupSendTargets` — 有 `@` → 强制集合；无人 `@` → 按 org `auto_pick` 模式选人。

---

## 1. 问题

无人 `@` 时全员并行候选 + PASS，成本高、延迟大，且易出现「全员敷衍 / 全员沉默再 fail」。  
需要一层**轻量选人**，在保留现有 PASS / 全员候选能力的前提下，默认少跑几路。

## 2. 目标行为

| 用户输入 | 选人 | 与现状关系 |
|---|---|---|
| `@某人` / 多 `@` | 仅被点名者（forced，不可 PASS） | **不变** |
| `@everyone` / `@all` | 全员 forced | **不变** |
| **无人 `@`** | 按 org `auto_pick` 模式（见 §3） | **新增** |
| Auto 失败 / 超时 / 无把握 | decision → **heuristic**；heuristic 无命中 → **lead**（`conv.AgentID`） | **fail-open 链** |

K 默认 **1**（org 级默认；上限 3）。Lead 始终可作为兜底，不因 Auto 静默丢回复。

## 3. 三策略 `auto_pick`（仅组织功能开关）

**唯一生效配置：** org `feature_flags_json.auto_pick`（Admin → **功能开关**）。

| 值 | UE 文案 | 行为 |
|---|---|---|
| **`off`** | 全员自决 | 全员候选（现状：各 bot 可 PASS） |
| **`heuristic`** | 智能匹配 | A 档：关键词 / 角色匹配选 ≤K 人；**默认** |
| **`decision`** | 决策模型 | B 档：组织决策模型（Jev / Laya / Unsloth SystemOne）choice 选人 |

**默认：`heuristic`。** 缺省 / omit / 非法 → `heuristic`。

**迁移（历史 bool）：** `true` / omit → `heuristic`；`false` → `off`。读写时归一为字符串枚举。

**产品决策（2026-10-10）：** 客户端**不**配置群级 auto_pick；群设置 UI / switch pack 中与 auto_pick 模式相关的项可取消。`EffectiveAutoPickMode` / `planSendTargets` **忽略** channel `settings.auto_pick`。PATCH `/v1/channels/{id}` 仍可写入该字段（back-compat），但不影响选人。

`auto_pick_k`：默认 1（org 级约定）；频道若历史写过 K 仍可读，非客户端必配。`show_reply_candidates`：可选调试字段，默认关。

```
resolveGroupSendTargets / planSendTargets
  ├─ @ / @everyone / explicit → 现状（forced）
  └─ no @ → org feature_flags.auto_pick（忽略 channel）
        auto_pick=off        → 全员候选（现状）
        auto_pick=heuristic  → AutoPick(K) A 档
        auto_pick=decision   → SystemOne choice → ≤K ids；失败 → heuristic
        heuristic 无命中      → [lead]（非 forced；lead 提示「你是兜底，请回答」）
```

**客户端 `agent_ids`：** no-`@` 时**不要**附带全员 id。旧 web 在无 `@` 时把全体成员塞进 `agent_ids`，服务端曾把「任意非空 agent_ids」当成 explicit 而跳过 AutoPick（表现：问「谁会写代码」22+33 都答）。现约定：仅 `@` 子集 / UI 点名写入；全员 dump 在服务端也视为非 explicit（兼容旧客户端）。并行多 bot 时，客户端 `message_saved` 必须按 `agent_id` stamp，避免同 bot 双泡。

## 4. 轻量选人（A / B）

输入：用户本轮文本、成员名 + 短角色（已有 `groupParticipationRoleHint`）、可选最近 1～2 条摘要。  
输出：有序 `agent_id[]`（长度 ≤ K）+ `confidence`。

| 档 | 做法 | 成本 | 状态 |
|---|---|---|---|
| **A. 启发式** | 关键词 / 角色描述子串命中；否则 lead | 无额外 LLM | **已实现** |
| **B. 决策模型** | 一次 SystemOne **choice**（opaque `m0..` → agent_id）；温度低、短超时 | 1 次小调用 | **已实现** |
| **C. 嵌入** | 成员角色向量 vs 用户句（后置） | 中 | 非本轮 |

### B 档细节

- **State**：`User message:` + 可选 `Recent context:`（synopsis）
- **Question**：单个 `pick` choice；`criteria[m_i] = "显示名 — 角色短描述"`（**不用**原始 agent_id / 裸名作 key，避免模型回显错乱）
- **映射**：仅接受 criteria 内的 opaque key；`probabilities` 按降序补齐至 K（且 ≥ θ）
- **置信度**：优先 `confidence`，否则所选 key 的 probability，再否则 0.55
- **调用路径**：API `planSendTargets` → runtime `POST /v1/decision/decide` → factory（Jev / Laya / Unsloth SystemOne）
- **超时**：约 8s

**Fail-open 规则（硬）：**

- decision：解析失败、超时、空列表、`confidence < θ`、决策层 off、runtime 不可达 → **heuristic**（禁止误变成 lead-only）
- heuristic：无命中 / 低分 → `[lead]`；lead 不在群成员里 → 退回**全员候选**
- Auto 选人**不得**替代 `@` 强制集合

## 5. 与 PASS / 全员候选并存

- 选中的非 forced 候选：**保留** `[PASS]` 提示与「不落泡」逻辑。  
- 若 Autopick 的唯一候选也 PASS → 二次 fail-open：补跑 lead（或全员），避免「直接无返回」。  
- 开关：仅 org `feature_flags_json.auto_pick` 三态；默认 **heuristic**。

## 6. 文案 / 可观测

- 内部日志：`auto_pick=decision ids=… conf=… fallback=`；失败时 `auto_pick=decision->heuristic ids=… conf=… fallback=lead|all`  
- 用户侧不展示选人过程；可选调试：`show_reply_candidates`（默认关；无客户端群设置 UI 要求）。
- **永不**在用户气泡泄漏 Auto / PASS 等内部词。

## 7. 配置字段（Admin / API）

| 字段 | 位置 | 类型 | 说明 |
|---|---|---|---|
| `feature_flags_json.auto_pick` | **org（生效）** | `"off"\|"heuristic"\|"decision"` | Admin → 功能开关；**规划唯一来源** |
| `settings.auto_pick` | channel | 同上 | **不生效**；PATCH 可写（back-compat） |
| `settings.auto_pick_k` | channel | number | 可选；0→默认 1；上限 3；无客户端必配 |
| `settings.show_reply_candidates` | channel | bool | 调试「本轮将由谁回答」；默认 false |
| PATCH body | `/v1/channels/{id}` | 同上字段 | 仍接受；mode 写入不影响 EffectiveAutoPickMode |
| 决策提供方 | org decision_* | jev / laya / unsloth / off | Admin → 决策模型；decision 模式依赖此项 |

### 给产品

客户端群设置 switch pack **不必**做 auto_pick 模式开关（可取消 / 不排期）。运营改 Admin「功能开关」即可。

## 8. 非目标

- 不做多轮辩论 / 主持人投票  
- 不改 DM  
- 不在本设计落地 Team pack / 跨群路由  
- 不做嵌入 C 档  
- 不做客户端群级 auto_pick 配置 UI

## 9. 验收

1. 「@码农 修 bug」→ 仅码农，无 Auto。  
2. 「大家介绍自己」`@everyone` → 全员 forced。  
3. 「帮我写个函数」无人 `@` + heuristic → 优先角色匹配的 1 人；其 PASS → lead 补答。  
4. heuristic 无命中 → 仅 lead 答，不出现空回合。  
5. org `auto_pick=off` → 与今日全员+PASS 一致。  
6. org `auto_pick=decision` + 决策层可用 → SystemOne choice 选人；日志 `auto_pick=decision`。  
7. org `auto_pick=decision` 但决策 off / 超时 / 低置信 → 行为同 heuristic，日志 `decision->heuristic`；**不是**仅 lead。  
8. 历史 bool `true/false` 在 org flags / channel 存储可读为 heuristic / off；channel 显式值**不**覆盖 org。  
9. Admin 功能开关可选三策略中文标签，写回 `feature_flags_json.auto_pick`。  
10. channel `settings.auto_pick=off` 且 org=`decision` → 仍按 decision 规划。

## 10. 本地验证（Unsloth / Jev）

1. Admin → **决策模型**：选 Unsloth（默认 `http://localhost:8888`，模型 `laya`）或 Jev（需 API key）；点「检测模型」。  
2. Admin → **功能开关**：群 Auto 选人 = **决策模型**（无需改 channel）。  
3. 群聊发无人 `@` 的专长问题（如「帮我写个函数」），看 API 日志：`auto_pick=decision ids=[…] conf=…`。  
4. 关掉决策层或停 Unsloth，再发同样消息 → 应见 `decision->heuristic` 且仍有专长匹配（非空回合）。  
5. 单测：`go test ./internal/httpserver/ -run AutoPick` 与 `./internal/db/ -run EffectiveAutoPick`（mocked decide，不连外网）。
