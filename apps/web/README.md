# apps/web

Phase 1：Vite + React + TypeScript 深色聊天 UI。

```bash
pnpm --dir apps/web install
pnpm --dir apps/web dev
```

默认请求 `http://127.0.0.1:18080`，可用 `VITE_API_BASE` 覆盖。

## UI 基础设施

- Tailwind CSS v4（`@tailwindcss/vite`）+ 现有 `src/styles.css`（未整页重写）
- shadcn/ui 风格（Radix）：`components.json`、`src/components/ui/`、`src/lib/utils.ts`（`cn`）
- 确认框：`ConfirmProvider` / `useConfirm()`（AlertDialog）
- Toast：sonner（根节点 `<Toaster />`）
- 路径别名：`@/` → `src/`
- 设置页 / HtmlPreview / SecretPrompt 等自定义 Modal 尚未迁到 Dialog，后续再做
