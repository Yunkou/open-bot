# Port Notes — baoyu-infographic

Ported from [JimLiu/baoyu-skills](https://github.com/JimLiu/baoyu-skills) v1.56.1 via Hermes Agent (MIT), then adapted for open-bot.

## Changes from upstream

Only `SKILL.md` was modified. All 45 reference files are verbatim copies.

### SKILL.md adaptations

| Change | Upstream | open-bot |
|--------|----------|--------|
| Trigger | `/baoyu-infographic` slash command | Natural language skill matching |
| User config | EXTEND.md file (project/user/XDG paths) | Removed — not part of open-bot infra |
| User prompts | `AskUserQuestion` (batched) | 直接在对话里提问（一次最多 5 个） |
| Image generation | baoyu-imagine (Bun/TypeScript) | HTML/SVG 渲染 + Playwright 截图；或用户配置的生图 MCP；或只交付提示词 |
| Platform support | Linux/macOS/Windows/WSL/PowerShell | Linux/macOS only |
| File operations | Bash commands | `sandbox_write` / `sandbox_read`（产物放 `/workspace/out/`） |

### What was preserved

- All layout definitions (21 files)
- All style definitions (21 files)
- Core reference files (analysis-framework, base-prompt, structured-content-template)
- Recommended combinations table
- Keyword shortcuts table
- Core principles and workflow structure
- Author, version, homepage attribution

## Syncing with upstream

To pull upstream updates:
```bash
# Compare versions
curl -sL https://raw.githubusercontent.com/JimLiu/baoyu-skills/main/skills/baoyu-infographic/SKILL.md | head -5
# Look for version: line

# Diff reference files
diff <(curl -sL https://raw.githubusercontent.com/.../references/layouts/bento-grid.md) references/layouts/bento-grid.md
```

Reference files can be overwritten directly (they're unchanged from upstream). SKILL.md must be manually merged since it contains open-bot-specific adaptations.
