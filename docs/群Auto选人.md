# 群 Auto 选人（轻量设计）

日期：2026-10-11 · 状态：三策略已落地；decision **可返回多人子集**（opaque `all` + `count` noul + top-N probabilities；**谁会/who can** 能力普查 → 有能力成员全选，不误成全员或 K=1）；**生效模式仅跟 Admin 组织功能开关**（`feature_flags_json.auto_pick`）；客户端不配置群级 auto_pick  
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
| 无人 `@` 且全员意图（「都介绍下自己」/ roast 等） | **decision**（产品路径）：prompt 分类 → `choice=all`；**heuristic**：无短语短路（弱，仅专长/角色） | **见 §4.1** |
| Auto 失败 / 超时 / 无把握 | decision → **heuristic**；heuristic 无命中 → **lead** | **fail-open 链** |

K 默认 **1**（org 级约定；专长上限 3）。`count` noul / 多路高概率可将 decision 有效 K 抬到 `min(成员数, 8)`（含全员）。Lead 始终可作为兜底，不因 Auto 静默丢回复。

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
        auto_pick=heuristic  → AutoPick(K) A 档（专长/角色；无 all-hands 短语短路）
        auto_pick=decision   → 始终 SystemOne choice（含 opaque `all` + mN）；
                               prompt 分类 all-hands/who-can/specialty；
                               choice=all / 全员映射 → 全员 + LeadFallback；失败 → heuristic
        heuristic 无命中      → [lead]（非 forced；lead 提示「你是兜底，请回答」）
```

**客户端 `agent_ids`：** no-`@` 时**不要**附带全员 id。旧 web 在无 `@` 时把全体成员塞进 `agent_ids`，服务端曾把「任意非空 agent_ids」当成 explicit 而跳过 AutoPick（表现：问「谁会写代码」22+33 都答）。现约定：仅 `@` 子集 / UI 点名写入；全员 dump 在服务端也视为非 explicit（兼容旧客户端）。并行多 bot 时，客户端 `message_saved` 必须按 `agent_id` stamp，避免同 bot 双泡。

## 4. 轻量选人（A / B）

输入：用户本轮文本、成员名 + 短角色（已有 `groupParticipationRoleHint`）、可选最近 1～2 条摘要。  
输出：有序 `agent_id[]`（长度 ≤ K）+ `confidence`。

| 档 | 做法 | 成本 | 状态 |
|---|---|---|---|
| **A. 启发式** | 关键词 / 角色描述子串命中；否则 lead | 无额外 LLM | **已实现** |
| **B. 决策模型** | 一次 SystemOne **choice**（opaque `all` + `m0..` → agent_id[]）；温度低、短超时 | 1 次小调用 | **已实现** |
| **C. 嵌入** | 成员角色向量 vs 用户句（后置） | 中 | 非本轮 |

### B 档细节

- **State**：`User message:` + 可选 `Recent context:`（synopsis；**优先近期 user 话**，跳过本轮刚落库的 user；长 bot SVG/HTML 不占槽，向前看以保留专长追问上下文，如「用代码画个笑脸」→「再来个哭脸」）
- **Question**：`pick` choice + `count` noul；`criteria` 含 opaque **`all`**（全员应发言）+ `m_i = "显示名 — 角色短描述"`（**不用**原始 agent_id / 裸名作 key）
- **指令**：可选一人专长、若干互补专长、或全员；集体介绍 / 轮流 → 选 `all`；多专长时主选 + `count` + `probabilities` 补多人子集；**SPECIALTY follow-up / anaphora**（「再来个/再画…」接在专长任务后）→ 保持原专长 mN，**禁止** `choice=all`（靠 decide prompt，非 keyword specialty 纠偏）
- **映射**：`choice=all` → 频道顺序展开全部 `agent_id`（**who-can 例外**：按 mN 概率 + `count` 取有能力子集，不展开全员）；否则仅接受 `mN`；按 probability 降序、≥ θ（who-can 用相对阈值）取 top-N；`count` noul→1..N 定人数；无 count / who-can 时可由多路高概率推断多人（不必 `all`）
- **有效 K**：专长用 `auto_pick_k`（软默认，上限 3）；`count` / 多路高概率可将有效 K 抬到 `min(成员数, 8)`（例如 2 of 3）；**无**短语抬 K
- **置信度**：`max(confidence, probs[choice])`；who-can 从 `all` 重映射时再考虑 `probs[all]` / 所选 mN 合计，避免低 `confidence` 字段误 fail-open；再否则 0.55
- **调用路径**：API `planSendTargets` → runtime `POST /v1/decision/decide` → factory（Jev / Laya / Unsloth SystemOne）
- **超时**：约 20s（Laya 常见 5–12s；8s 曾与 cf23 7.3s 完成竞态 → client timeout → lead-only）

**Fail-open 规则（硬）：**

- decision：解析失败、超时、空列表、`confidence < θ`、决策层 off、runtime 不可达 → **heuristic**；若 heuristic 无专长命中 / 仅 lead → **全员 + LeadFallback**（禁止误变成 lead-only；Langfuse cf23 「大家好」）
- heuristic：无命中 / 低分 → `[lead]`；lead 不在群成员里 → 退回**全员候选**
- Auto 选人**不得**替代 `@` 强制集合

## 4.1 全员意图（all-hands）

无人 `@` 时，用户话术要求**每位成员发言**（自我介绍 / 轮流 / 面向全场吐槽）：

| 模式 | 行为 |
|---|---|
| **decision**（**产品路径**） | **调用** SystemOne；criteria 含 `all`；**prompt 始终**教模型分类 ALL-HANDS vs WHO-CAN vs SPECIALTY（示例含「都介绍下自己」「在座的各位都是辣鸡」「各位都是…」）。模型应 `choice=all`、count=全员、高概率质量压在 `all`。**无** `isAllHandsIntent` 短语门控；薄结构安全见下。 |
| **heuristic** | **无** all-hands 短语短路（弱路径，仅专长/角色匹配）。全员介绍 / roast 请用 decision 模式。 |

**Prompt 驱动（替代短语列表）**：`buildAutoPickDecisionPayload` **始终**写入 ALL-HANDS / WHO-CAN / SPECIALTY 分类指令与 count 规则——不再靠 `Contains` 短语列表切换 framing。已删除 `isAllHandsIntent` 及 fixed/collective/action 词表；已移除 heuristic `*->all_hands` 短语短路。

**薄结构安全（非 all-hands 短语表）**：

- 模型 `choice=all` 或映射结果覆盖全员 → 每位选出成员标 `LeadFallback`（禁止 candidacy `[PASS]`）+ `AllowSecondFailOpen=false` + `fallback=all`。
- **信任全员 count**：非 who-can、非 specialty（`userTextHitsRoleSynonym`）且 `count ≥ 成员数` 时，即使 primary 落在单个 mN（underpick），也展开全员 + LeadFallback。
- decision 解析失败 / 超时 / 低置信 → **heuristic**；无专长命中或仅 lead → **全员 + LeadFallback**（避免慢 SystemOne 把全员意图塌成单 lead；Langfuse cf23 「大家好」）。
- `choice=all` 时若 `confidence` 字段被稀释（~0.1）但 probs/`all` 或映射有效，抬置信度避免误 fail-open。
- decide 客户端超时约 **20s**（原 8s 曾与 ~7.3s Laya 完成竞态）。

否则会出现「文章+代码都 PASS → 全员沉默」——14:25 CST「大家都介绍下自己」（Langfuse peek `f24b88df`，runs `9bd33f4a` / `aeb5cffa`）。

**日志：**

- `auto_pick=decision ids=[…] conf=… fallback=all`（`choice=all` / 全员映射）
- `auto_pick=decision ids=[…] conf=… fallback=`（多人但非全员）
- `auto_pick=decision->heuristic …`（decision 失败 → 普通 heuristic）

**历史漏检（已用 prompt 方案取代短语扩表）**：2026-10-12 15:30 CST Langfuse `53cd5422` / `7abb3a17`，「在座的各位都是辣鸡」曾因短语漏检走 specialty → `choice=m0`。现靠 decide prompt 中的 roast/address-everyone 示例约束模型输出 `choice=all`，不再维护 roast 关键词列表。

`@everyone` / `@all` 仍走强制全员（不可 PASS），与本路径无关。


## 4.2 能力普查（who-can / 「都谁会 X」）

无人 `@` 时，若用户在问**谁具备某能力**（「都谁会查资料」「谁会写代码」「who can research」「both」）：

| 模式 | 行为 |
|---|---|
| **decision** | **完全信任决策模型** choice+count+probs（**无** keyword / role-match 纠偏）。Prompt 要求：按 criteria 里各 mN 的角色标签做能力普查——写代码→仅 coder；查资料→全部 research；**禁止**在仅子集能做时选 `all` 或把 count 抬到全员。映射：`choice=all` + who-can → 按 mN 概率 + `count` 取有能力子集（不字面展开全员）；`choice=mN` → 主选 + 相对高概率同伴。置信度取 `max(confidence, probs[choice], probs[all], …)`，避免低 `confidence` 字段误 fail-open |
| **heuristic** | 同能力关键词命中的成员**全部**入选（抬高有效 K），不只 soft `auto_pick_k=1`（decision 失败 fail-open 时仍用） |

**Prompt 驱动（who-can）**：`buildAutoPickDecisionPayload` 在 who-can 时强化 instructions / count noul / state / `criteria[all]`——明确「匹配 criteria 角色，Recent context 同伴自介不能覆盖角色」；硬例「谁会写代码」→仅 coding mN + noul≈0（禁止 `all`/全员 count），「谁会查资料」→多 research。已移除 `capableIDsFromRoleMatch` / `who_can_role_match` 覆盖 SystemOne 结果的路径。

**映射硬化（who-can，已批准的 all→capable 重映射）**：`choice=all` + `count≥成员数`（SystemOne 把 noul 抬到全员，如 Langfuse `21ed840c` / 12:18 CST「谁会写代码」）时**不**信任该 count，改为按 mN 相对概率推断有能力子集（与 count 省略/undershoot 同一路径）。不是 keyword role-match，也不是 all-hands underpick→全员展开。

**PASS / 二次 fail-open（who-can，结构安全）**：模型选出有能力子集后，对这些 id 标 `LeadFallback`（参与提示禁止 `[PASS]`），且 `AllowSecondFailOpen=false`。否则会出现「只选中 33 → 33 PASS → 二次 fail-open 把 22/44 拉进来」——11:41 CST「谁会写代码」即此（Langfuse `a1f143e3` / obs `30796cdf`）。这是结构安全，不是 keyword 纠偏。

**PASS / 二次 fail-open（specialty，结构安全）**：专长问（`userTextHitsRoleSynonym`，如「写个js的代码」「帮我查下…」）选出有能力成员后同样 `LeadFallback` + `AllowSecondFailOpen=false`。否则会出现「两个 coder 都 PASS → 二次 fail-open 把文档助手拉进来」——13:45 CST「写个js的代码…」即此（Langfuse `a32fde27` / obs `5808f45a`）。heuristic 与 decision 成功路径均适用。

**专长追问准确度（prompt + synopsis，非 keyword 纠偏）**：短回指「再来个哭脸」「再画一个」等 anaphora 在 Recent context 有先前专长 user 话（如「用代码画个笑脸」）时应保持 coding（或原专长）mN，不得因无「代码」字样塌成 `choice=all`。靠 decide prompt 的 SPECIALTY FOLLOW-UP 示例 + synopsis 优先 user；**不**把「再来个/哭脸」扩进 `userTextHitsRoleSynonym`，也**不**对 `choice=all` 做 specialty 拒收→fail-open-all。薄安全：若 synopsis 仍含专长同义词、primary 为 mN 且 `probs[all]` 偏低，则**跳过** full-roster count 展开；若模型仍 `choice=all` 且当前句是短 anaphora（再来个/再画…）且 synopsis 含专长同义词，则 fail-open heuristic（synopsis 混合打分选专长 bot），并禁止 decision→heuristic 单人结果再被抬成全员。

**SVG/HTML 专长（channel 333 / 2026-10-12 16:56–16:59 CST）**：「用svg给我写一个笑脸图标」「要html」「来个html页面…」SystemOne 常 `choice=all` / 高 noul 或 mN+全员 count（Langfuse `6aa69f96` / `7a1d93b8` / `22e8cc7a`），all-hands 置信度抬升会让文章+代码双答。coding 同义词补 `svg`/`html`/`css`/… 后走已有 specialty mismatch → heuristic（仅代码）；synopsis 含 SVG 时跳过 count 展开。prompt 增 SVG/HTML→coding 与「要html」追问示例。仍非新建 all 拒收陷阱。

**短语检测**（`isWhoCanIntent`）：`都谁会` / `谁会` / `谁能` / `哪些人会` / `who can` / `which of you can` / `both can` 等。与全员介绍互斥场景（由 decide prompt 分类，非短语互斥）：前者要**有能力子集**，后者要**每位发言**。

**日志：** `auto_pick=decision ids=[…] conf=… choice=… count=… probs=map[…] fallback=`

---

## 5. 与 PASS / 全员候选并存

- 选中的非 forced 候选：**保留** `[PASS]` 提示与「不落泡」逻辑。  
- 若 Autopick 的唯一候选也 PASS → 二次 fail-open：补跑 lead（或全员），避免「直接无返回」。**例外**：who-can / specialty / **all-hands** 关闭二次 fail-open，并对候选标 `LeadFallback` 强制作答（见 §4.1 / §4.2）。  
- 开关：仅 org `feature_flags_json.auto_pick` 三态；默认 **heuristic**。

## 6. 文案 / 可观测

- 内部日志：`auto_pick=decision ids=… conf=… fallback=`（含 `fallback=all`）；失败时 `auto_pick=decision->heuristic …`（无短语 `*->all_hands`）  
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
2b. 「都介绍下自己」无人 `@` + **heuristic** → **无**短语全员短路（弱路径）。  
2c. 「都介绍下自己」无人 `@` + **decision** → **调用** SystemOne；prompt 分类 ALL-HANDS → 模型 `choice=all` → 全员 + LeadFallback；日志 `auto_pick=decision … fallback=all`；失败则 `decision->heuristic`。  
3. 「帮我写个函数」无人 `@` + heuristic → 优先角色匹配的 1 人；其 PASS → lead 补答。  
4. heuristic 无命中 → 仅 lead 答，不出现空回合。  
5. org `auto_pick=off` → 与今日全员+PASS 一致。  
6. org `auto_pick=decision` + 决策层可用 → SystemOne choice 选人；日志 `auto_pick=decision`。  
7. org `auto_pick=decision` 但决策 off / 超时 / 低置信 → `decision->heuristic`；有专长命中则用专长子集，否则 **全员 + LeadFallback**（**不是**仅 lead）。  
8. 历史 bool `true/false` 在 org flags / channel 存储可读为 heuristic / off；channel 显式值**不**覆盖 org。  
9. Admin 功能开关可选三策略中文标签，写回 `feature_flags_json.auto_pick`。  
10. channel `settings.auto_pick=off` 且 org=`decision` → 仍按 decision 规划。

## 10. 本地验证（Unsloth / Jev）

1. Admin → **决策模型**：选 Unsloth（默认 `http://localhost:8888`，模型 `laya`）或 Jev（需 API key）；点「检测模型」。  
2. Admin → **功能开关**：群 Auto 选人 = **决策模型**（无需改 channel）。  
3. 群聊发无人 `@` 的专长问题（如「帮我写个函数」），看 API 日志：`auto_pick=decision ids=[…] conf=…`。  
4. 关掉决策层或停 Unsloth，再发同样消息 → 应见 `decision->heuristic` 且仍有专长匹配（非空回合）。  
5. 单测：`go test ./internal/httpserver/ -run AutoPick` 与 `./internal/db/ -run EffectiveAutoPick`（mocked decide，不连外网）。
