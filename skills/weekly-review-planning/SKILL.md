---
name: weekly-review-planning
description: Weekly reset: review commitments, stalled work and plan next week (memory_* for continuity; optional weekly routine). Use for 周复盘 / 下周计划.
---

# Weekly Review and Planning

> 改编自 [Hermes Agent](https://github.com/NousResearch/hermes-agent) `skills/productivity/weekly-review-planning`（MIT，Nous Research）。工具名已换成 open-bot 自己的。

## open-bot 运行方式

- 主要是写作/分析流程，不需要特定工具。用户给的是本机文件时用 `host_read`（先 `list_machines`）；是网页时用 `http_fetch`。
- 需要跨会话延续（偏好、上次结论、未完成事项）用 `memory_recall` / `memory_write`；需要定期执行用 `create_routine`。
- 生成较长的交付物（文档/表格）时，可写到 `/workspace/out/`（`sandbox_write`），回复里给出路径作为附件。


Run a bounded weekly reset across the user's chosen systems. This is a concrete recurring task, not a generic productivity methodology. To make it recurring, create an open-bot routine, e.g. `create_routine(name="weekly review", schedule_cron="0 17 * * 5", prompt="load_skill weekly-review-planning，做本周复盘与下周计划")`. Use `memory_recall` for last week's plan and `memory_write` (tier `log`) to save this week's outcome.

## When to Use

- "Run my weekly review."
- "What did I commit to and what is slipping?"
- "Plan next week from my calendar, tasks, and notes."
- "Find stale projects and waiting items."
- A routine run (`[routine]` prompt) fires for a scheduled weekly review.

Don't use for: daily briefs (see the `google-workspace` daily-brief reference) or single-inbox triage (`email-inbox-triage`).

## Procedure

### 1. Set systems and window

Confirm timezone, review period, planning horizon, authoritative task/project store, calendars, inboxes, and allowed writes. Default to recommendations/drafts, not mutations. Done when source-of-truth conflicts have a declared winner.

### 2. Review calendar evidence

Load `google-workspace` or the relevant calendar connector. Inspect the completed week for meetings and commitments, then the next 1-2 weeks for deadlines, travel, preparation, and capacity. Capture follow-ups implied by past events and conflicts ahead. Done when both retrospective and horizon are covered.

### 3. Clear capture inboxes

Review the task inbox, notes (`obsidian`, `notion`), flagged email (`email-inbox-triage` owns thread-level triage), and other declared capture points. Convert each item to next action, project, waiting, scheduled, someday, reference, archive, or delete proposal. Do not mutate until scope is approved. Done when remaining unprocessed items are counted and stated.

### 4. Reconcile active projects

For each project identify desired outcome, next action, owner, deadline, blocker, last meaningful activity, and source link. Flag projects with no next action, missed dates, duplicate records, or contradictory status. Done when every active project is actionable or explicitly paused.

### 5. Review waiting and commitments

Find promises made by the user and items owed by others. Propose follow-ups with dates and channels. Do not infer that silence means completion. Done when each waiting item has an owner and next review/follow-up date.

### 6. Build a capacity-aware plan

Estimate fixed calendar load and select a small set of weekly outcomes plus near-term next actions. Rank by consequence, deadline, dependency, and effort; do not fill every free hour. Done when the plan fits actual capacity and names deferred work.

### 7. Apply approved updates

Update tasks/projects, create calendar holds, archive processed items, and draft follow-ups only as approved. Read every changed record back from the provider. Done when verified writes match the review summary.

## Output Shape

1. Wins and completed commitments
2. Overdue or at risk
3. Waiting/follow-ups
4. Stalled or ambiguous projects
5. Next week's outcomes and calendar constraints
6. Proposed updates awaiting approval
7. Coverage gaps

## Pitfalls

- Planning from tasks without calendar capacity.
- Carrying every unfinished item forward as high priority.
- Marking projects active with no next action.
- Silently deleting or rescheduling personal commitments.
- Treating silence from others as completion.

## Verification

- [ ] Both the completed week and the planning horizon were covered, or gaps are stated.
- [ ] Every stalled/waiting flag traces to a specific record, event, or thread.
- [ ] No task, event, or note was mutated without approval; approved writes were read back.
- [ ] The plan names what was deferred, not just what was chosen.
