# Open Bot Desktop（Tauri 2）

macOS arm64 桌面壳，**前端直接复用** `apps/web`（不复制 UI）。

## 依赖

- Rust（rustup）+ Xcode CLT
- Node / pnpm（workspace 已含 `@tauri-apps/cli`）
- 本地已启动：Postgres、`make dev-api`、`make dev-runtime`

## 开发

```bash
# 终端 1–2（仓库根）
make compose-postgres
make dev-runtime
make dev-api

# 终端 3
make dev-desktop
# 或：pnpm --dir apps/desktop tauri dev
```

- 窗口标题：Open Bot；默认约 1200×800；深色背景 `#0f0f12`
- 开发时 Tauri 加载 `http://127.0.0.1:5173`（由 `beforeDevCommand` 启动 web vite）
- API：默认 `http://127.0.0.1:18080`（web 的 `VITE_API_BASE`）；可在仓库根 `.env` 覆盖

## 构建

```bash
make check-desktop          # cargo check
pnpm --dir apps/desktop tauri build --debug
# 或 release：
make build-desktop
```

产物在 `apps/desktop/src-tauri/target/`（debug/release）。本轮不做代码签名/公证/自动更新。

## 结构说明

| 路径 | 作用 |
|------|------|
| `src-tauri/` | Rust + Tauri 2 壳 |
| `tauri.conf.json` → `frontendDist` | `../../web/dist` |
| `devUrl` | `http://127.0.0.1:5173` |

## 验收记录（本机 macOS arm64）

- `cargo check`：通过
- `pnpm --dir apps/desktop tauri build --debug`：通过，产物 `src-tauri/target/debug/bundle/macos/Open Bot.app`
- 二进制可启动（进程存活）；需本机已起 api（默认 `:18080`）才能登录
- bundle 目标暂为 `app`（未默认打 dmg；dmg 在部分环境会因 Finder/权限失败）
