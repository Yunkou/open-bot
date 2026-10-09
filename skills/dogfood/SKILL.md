---
name: dogfood
description: Exploratory QA of a web app with a Playwright script (via host_shell or sandbox_shell): find bugs, capture evidence, write a report. Use for 帮我测一下这个网站 / QA.
---

# Dogfood: Systematic Web Application QA Testing

> 改编自 [Hermes Agent](https://github.com/NousResearch/hermes-agent) `skills/software-development/dogfood`（MIT，Nous Research）。工具名已换成 open-bot 自己的。

## open-bot 运行方式

open-bot 没有内置浏览器工具。本 skill 用包内的 **Playwright 探针脚本** `scripts/qa_probe.py`：打开页面、按 JSON 动作列表点击/填写/按键、收集 console 错误 / 页面异常 / 失败请求 / HTTP 4xx-5xx、列出可见交互元素，并保存整页截图。

- **测公网站点**：在内部执行环境跑：`sandbox_ensure` → `load_skill(name="dogfood", path="scripts/qa_probe.py")` → `sandbox_write` 到 `/workspace/skills/dogfood/qa_probe.py` → `sandbox_shell`：`pip install playwright && python -m playwright install --with-deps chromium`（首次）→ `python3 /workspace/skills/dogfood/qa_probe.py <URL> --out /workspace/out/dogfood`。报告和截图放 `/workspace/out/dogfood/`，回复里写出路径即可作为附件。
- **测用户本机 / 内网应用**（`localhost`、公司内网）：在用户电脑上跑：`list_machines` → `host_write` 脚本到 `~/.open-bot/skills/dogfood/qa_probe.py` → `host_shell` 运行（缺 Playwright 时先征得同意再安装）。
- open-bot 没有看图工具：视觉问题靠截图交给用户确认，以及 DOM / console / 网络信号判断；不要声称「看过截图」。
- 只读探索为主；不要在生产环境提交真实订单、发消息、删除数据。需要登录的站点，请用户提供测试账号（用 `request_secret` 保存，不要贴明文）。

## Overview

Systematic exploratory QA of a web application: navigate, interact, capture evidence, and produce a structured bug report.

## Inputs

1. **Target URL** — entry point
2. **Scope** — features to focus on (or "full site")
3. **Output directory** (optional) — default `/workspace/out/dogfood`（内部环境）或 `./dogfood-output`（用户电脑）

## Workflow

### Phase 1: Plan

1. Create `{output_dir}/screenshots/` and plan `{output_dir}/report.md`.
2. Build a rough sitemap: landing page, navigation (header/footer/sidebar), key flows (sign up, login, search, checkout), forms, edge cases (empty states, error pages, 404s).

### Phase 2: Explore

For each page or feature:

1. **Probe the page**:
   ```bash
   python3 qa_probe.py "https://example.com/page" --out "$OUT"
   ```
   Read `status`, `console`, `page_errors`, `failed_requests`, `http_errors`, `elements`, `screenshot` from the JSON.
2. **Console / page errors are high-value findings** — check them after every navigation and interaction.
3. **Interact** by re-running with an action list (each run starts fresh, so include the steps that lead to the state):
   ```bash
   python3 qa_probe.py "https://example.com/login" --out "$OUT" \
     --actions '[{"fill":"input[name=email]","text":"not-an-email"},{"click":"role=button[name=\"Sign in\"]"},{"wait":1}]'
   ```
   - Buttons / links: `{"click": "text=Pricing"}`
   - Forms: `{"fill": "#q", "text": "test input"}` — try valid, invalid, empty, very long, special characters
   - Keyboard: `{"press": "Tab"}`, `{"press": "Enter", "selector": "#q"}`
   - Scroll: `{"scroll": 2000}`
   - Narrow viewport: `--width 390 --height 844`
4. After each run compare expected vs actual: `final_url`, `title`, errors, failed actions (`"ok": false`).

### Phase 3: Collect Evidence

For every issue record: URL, steps to reproduce (the exact `--actions` JSON), expected, actual, console / network errors, screenshot path. Classify with `references/issue-taxonomy.md` — Severity: Critical / High / Medium / Low; Category: Functional / Visual / Accessibility / Console / UX / Content.

### Phase 4: Categorize

De-duplicate, assign final severity/category, sort Critical → Low, count by severity and category.

### Phase 5: Report

Fill `templates/dogfood-report-template.md`（`load_skill(name="dogfood", path="templates/dogfood-report-template.md")`）and save to `{output_dir}/report.md`. Include: executive summary, per-issue sections (title, severity, category, URL, description, repro steps, expected vs actual, screenshot path, console errors), summary table, testing notes (tested / not tested / blockers). Give the user the report and screenshot paths.

## Tips

- Silent JS errors and failed API calls are among the most valuable findings.
- Test both valid and invalid inputs; test empty submissions and rapid repeated clicks.
- Probe long pages with `{"scroll": …}`; check mobile width once.
- Visual / layout judgement: hand screenshots to the user rather than guessing.

## Verification

- Every issue in the report has repro steps that re-run with the probe and still show the problem (or is marked as intermittent).
- Report saved and its path given to the user.
