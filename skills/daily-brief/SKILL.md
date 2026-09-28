---
name: daily-brief
description: Produce a short daily briefing covering priorities, calendar-style reminders, and follow-ups. Use when the user asks for a morning brief, day plan, standup summary, or 今日简报.
---

# Daily Brief

## When to use
- Morning / end-of-day planning
- Standup or status rollup
- User says 今日简报 / 日程规划 / priorities

## Instructions
1. Ask only for missing essentials (timezone, top goals) if not already in memory/profile.
2. Structure the brief: Focus / Schedule hints / Risks / Follow-ups.
3. Keep under ~200 words unless the user wants detail.
4. Pull relevant `profile` and `note` memories when available.
5. End with one clarifying question if priorities conflict.

## Output template
```
## 今日简报
**焦点**：…
**安排**：…
**风险**：…
**跟进**：…
```
