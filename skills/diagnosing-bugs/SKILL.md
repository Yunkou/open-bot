---
name: diagnosing-bugs
description: Diagnosis loop for hard bugs and performance regressions. Use when the user says diagnose/debug, or reports something broken, throwing, failing, or slow.
---

# Diagnosing Bugs

改编自 [mattpocock/skills](https://github.com/mattpocock/skills)（MIT）。在 open-bot 里用本机 `host_*` / 临时 `sandbox_*` 跑复现命令；远程机先 `load_skill host-ssh`。

纪律：难 bug 不要跳阶段；探索代码时若有 `CONTEXT.md` / ADR 先读。

## 脱敏

展示命令与输出前，把密钥改成 `<REDACTED>`。能用环境变量就不要把凭证写进对话。

## Phase 1：先建反馈环

**这才是本 skill。** 有一个对「这个 bug」能红的 pass/fail 信号，后面的二分、假设、埋点才有意义。

大致按此顺序构造：

1. 落到 bug 的失败测试（单测 / 集成 / e2e）
2. 对运行中服务的 curl / HTTP 脚本
3. 带夹具的 CLI，对比 stdout
4. 无头浏览器脚本（Playwright 等）
5. 回放捕获的 trace / payload
6. 临时 harness（最小子集 + 一次调用）
7. 属性 / fuzz 循环
8. 可 `git bisect run` 的自动化
9. 新旧版本 / 配置差分
10. 需要人点的场景：用包内 `scripts/hitl-loop.template.sh` 结构化人机循环

收紧环：更快、断言对准症状、更确定性。30 秒不稳定的环几乎等于没有。

做不到环时：明确说试过什么，向用户要环境 / 脱敏产物 / 临时埋点权限。**没有红环，不要进入假设阶段。**

完成标准：已跑过至少一次、可复述的一条命令——能对准用户症状变红、尽量确定性、尽量秒级、代理可无人值守跑。

## Phase 2：复现 + 最小化

跑环到红；确认是用户描述的失败模式；再一次去掉非必要输入/步骤，每次重跑环。

## Phase 3：假设

先列出 **3–5 条可证伪** 假设再测（「若 X 是因，则改 Y 会…」）。展示给用户再测；用户不在也按你的排序继续。

## Phase 4：探测

一次改一个变量。优先调试器 / REPL，其次对准假设边界的日志；日志打唯一前缀如 `[DEBUG-a4f2]` 便于收尾。性能问题先量基线再二分。

## Phase 5：修复 + 回归

有正确 seam 时：先把最小复现写成失败测试 → 再修 → 再跑 Phase 1 原场景。没有正确 seam 本身就是发现，记下来。

## Phase 6：收尾

- [ ] 原环不再复现
- [ ] 回归测试通过（或无 seam 已说明）
- [ ] 去掉所有 `[DEBUG-…]`
- [ ] 扔掉临时 harness
- [ ] 在回复/提交说明里写清正确假设

## open-bot 执行提示

- 本机：`list_machines` → `host_shell` / `host_read` / `host_write`
- 临时脚本：`sandbox_shell` / `sandbox_write`（对用户只谈结果）
- 禁止未跑环就声称已定位 / 已修好
