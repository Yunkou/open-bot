---
name: tdd
description: Test-driven development with red-green loop. Use when building features or fixing bugs test-first, or when the user mentions red-green-refactor or integration tests.
---

# Test-Driven Development

改编自 [mattpocock/skills](https://github.com/mattpocock/skills)（MIT）。细节示例见同包 `tests.md`、`mocking.md`（需要时 `load_skill tdd` 并带 `path`）。

TDD 是红 → 绿循环。每轮都要对照：好测试长什么样、测在哪条 seam、反模式、循环规则。

探索仓库时若有 `CONTEXT.md` / ADR，测试命名与领域用语对齐。

## 好测试

通过公共接口验证行为，不耦合实现。好测试读起来像规格：「用户可用有效购物车结账」。

## Seam：测在哪里

**Seam** 是观察行为的公共边界。只在与用户预先约定的 seam 上写测试；未确认的 seam 不写。

先问：「公共接口是什么？我们要测哪些 seam？」

## 反模式

- **实现耦合**：mock 内部协作者、测私有方法、抄侧信道断言；一重构就红但行为没变。
- **同义反复**：断言用与实现相同的方式重算期望；期望须来自独立事实（字面量、演算例子、规格）。
- **横向切片**：先写完全部测试再实现。改为纵向切片：一条测试 → 一点实现 → 再下一条。

## 循环规则

- **先红后绿。** 只写刚好够过的实现，不预支下一刀。
- **一次一条。** 一个 seam、一条测试、一点实现。
- **重构不在本循环。** 交给 `code-review` / 评审阶段，不塞进红绿之间。

## open-bot

- 本机跑测：`host_shell`（如 `pnpm test`、`go test`、`pytest`）
- 生成/改测试文件：`host_read` / `host_write`；危险写入等确认卡
- 没有已连接电脑时说明限制，不要编造测试结果
