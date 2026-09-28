# End-to-end 测试（用户端 + 管理端）

Playwright 套件位于仓库根目录 [`e2e/`](../e2e/)，覆盖 **Web 用户端**（`:5173`）与 **Admin 管理端**（`:5174`），默认打本地 API（`:18080`）。

## 前置条件

本地栈需已启动（见 [本地启动.md](./本地启动.md)）：

| 服务 | 地址 | 备注 |
|------|------|------|
| Postgres | `127.0.0.1:5432` | `make compose-postgres` |
| Agent runtime | `http://127.0.0.1:8001` | `make dev-runtime` |
| Go API | `http://127.0.0.1:18080` | `make dev-api`（需 bootstrap admin） |
| Web | `http://127.0.0.1:5173` | `make dev-web` |
| Admin | `http://127.0.0.1:5174` | `make dev-admin` |

API 需配置 `BOOTSTRAP_ADMIN_USERNAME` / `BOOTSTRAP_ADMIN_PASSWORD`（见根目录 `.env.example`），管理端用例依赖该平台管理员。

## 两种 LLM 模式

### 1) Mocked LLM（默认，推荐）

`E2E_MOCK_LLM=1`（默认）时，global setup 会拉起 `e2e/mock-llm-server.mjs`（`:18099`），并在聊天相关用例里把测试用户的默认 LLM 连接指向该 mock。

- 确定性流式回复（含 `[short]` / `[long]` / `[slow]` 标记）
- 适合 stop / interrupt / resume / 多 bot 切换等稳定性要求高的场景

手动启动：

```bash
pnpm --dir e2e mock-llm
# → http://127.0.0.1:18099/v1
```

### 2) Full stack（真实模型）

```bash
E2E_MOCK_LLM=0 make e2e
```

此时不启 mock；聊天走用户已有 LLM / 环境 `OPENAI_*`。真实 vLLM 可能较慢或不稳定，超时可能需要调大。

## 环境变量

复制示例：

```bash
cp e2e/.env.e2e.example e2e/.env.e2e
# 填写 E2E_ADMIN_*（或依赖仓库根 .env 的 BOOTSTRAP_ADMIN_*，playwright.config 会映射）
```

| 变量 | 说明 |
|------|------|
| `E2E_WEB_URL` | 默认 `http://127.0.0.1:5173` |
| `E2E_ADMIN_URL` | 默认 `http://127.0.0.1:5174` |
| `E2E_API_URL` | 默认 `http://127.0.0.1:18080` |
| `E2E_ADMIN_USERNAME` / `E2E_ADMIN_PASSWORD` | 管理端登录（勿提交） |
| `E2E_USER_USERNAME` / `E2E_USER_PASSWORD` | 可选固定聊天用户；默认每次注册新用户 |
| `E2E_MOCK_LLM` | `1`（默认）启用 mock；`0` 关 |
| `E2E_MOCK_LLM_URL` | 默认 `http://127.0.0.1:18099/v1` |
| `E2E_MOCK_LLM_TOKEN_DELAY_MS` | mock 流式每 token 延迟 |

`e2e/.env.e2e` 已被 gitignore，不要把真实密码提交进仓库。

## 安装与运行

```bash
# 一次性：依赖 + Chromium
make e2e-install

# 跑全部（web + admin）
make e2e

# 仅用户端 / 仅管理端
make e2e-web
make e2e-admin

# 等价 pnpm
pnpm e2e
pnpm --dir e2e e2e:web
pnpm --dir e2e e2e:admin
pnpm --dir e2e e2e:ui      # Playwright UI
```

报告：`e2e/playwright-report/`（`pnpm --dir e2e e2e:report`）。

## 覆盖场景

### 用户端 (`apps/web`)

| 场景 | 文件 |
|------|------|
| 注册 / 登录 / 退出；错误密码 | `tests/web/auth.spec.ts` |
| 发消息 + 助手回复（mock） | `tests/web/chat.spec.ts` |
| 停止生成 | `tests/web/chat.spec.ts` |
| 流式中 interrupt-and-send（保留 partial） | `tests/web/chat.spec.ts` |
| `run_active` 刷新后 resume | `tests/web/chat.spec.ts` |
| 切换 bot 不打断另一路 run（侧栏 busy） | `tests/web/chat.spec.ts` |
| UI 创建 Bot；创建群聊；搜索过滤 | `tests/web/bots-channels.spec.ts` |
| 设置各 Tab smoke；附件 chip | `tests/web/settings.spec.ts` |

### 管理端 (`apps/admin`)

| 场景 | 文件 |
|------|------|
| Bootstrap admin 登录；member 拒绝；退出 | `tests/admin/auth.spec.ts` |
| 用户创建 smoke；Bot 创建 smoke | `tests/admin/users-bots.spec.ts` |
| users/bots/traces/members/llm/usage/flags/audit 导航 | `tests/admin/pages-smoke.spec.ts` |
| Traces 列表或「未启用」空态；Langfuse 链接形态（若有） | `tests/admin/traces.spec.ts` |

## 刻意未覆盖 / 缺口

- **HTML 预览**：依赖助手产出 workspace HTML / 工具调用，不稳定，未做强制断言
- **Casdoor OIDC**：可选组件，默认本地密码流
- **沙箱电脑 / 真实 MCP / Routines 触发**：仅设置页 smoke，不做端到端执行
- **真实 vLLM 工具调用**：mock 模式关闭 tools；full-stack 模式依赖环境

## 目录

```
e2e/
  playwright.config.ts
  mock-llm-server.mjs
  .env.e2e.example
  helpers/          # env、API、auth、global setup
  tests/web/
  tests/admin/
  fixtures/
```
