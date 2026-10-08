# @open-bot/mobile-rn

Open Bot 的 React Native 移动端。**独立于** `apps/mobile`（那是复用 `apps/web/dist` 的 Capacitor 壳，两条路线并存）。

技术栈：**Expo SDK 57 · React Native 0.86 · Expo Router · HeroUI Native（Uniwind）**

## 为什么是这套

| 决策 | 原因 |
|------|------|
| `expo/fetch` 而非全局 `fetch` | RN 内置 fetch 是 XHR polyfill，拿不到 `res.body`，SSE 会退化成一次性返回。`expo/fetch` 提供真正的 ReadableStream，Web 端 `readSSEStream` 的逻辑可以原样移植。 |
| `expo-secure-store` 而非 `localStorage` | JWT 存 Keychain / Keystore，不进明文存储。 |
| 保持 pnpm 隔离安装 | Expo SDK 55+ 在 monorepo 里自动启用 `autolinkingModuleResolution`，Metro 默认跟随 symlink，**不需要**把仓库降级成 `nodeLinker: hoisted`。 |
| `heroui-native` 而非 `@heroui/react` | 两者包名、样式引擎（Uniwind vs Tailwind）、颜色格式（HSL vs oklch）都不同，Web 端的写法不能直接搬。 |

## 前置条件

- Node ≥ 20.19.4、pnpm ≥ 10
- 后端已启动：`make dev-runtime` + `make dev-api`（默认 `http://127.0.0.1:18080`）
- **iOS 模拟器需要完整 Xcode**（本机当前只有 CommandLineTools，`xcrun simctl` 不可用）。装不了 Xcode 时用真机 + Expo Go。

## 启动

```bash
cp .env.example .env      # 真机必须改成电脑局域网 IP
pnpm start                # 模拟器直接用默认地址
pnpm start --clear        # 改过 .env 后必须 -c 重启才会重新注入
```

`EXPO_PUBLIC_OPENBOT_API_BASE` 的取值：

| 环境 | 值 |
|------|-----|
| iOS 模拟器 / web | `http://127.0.0.1:18080`（默认） |
| Android 模拟器 | `http://10.0.2.2:18080`（默认已处理） |
| 真机 | `http://<电脑局域网IP>:18080`，用 `ipconfig getifaddr en0` 查 |

## HeroUI Pro

本项目用的是开源版 `heroui-native`。Pro 版是**独立付费包** `heroui-native-pro`，需要授权：

```bash
npx heroui-pro@latest login     # 浏览器里用 GitHub 账号授权，必须本人在终端操作
npx heroui-pro@latest install   # 拉取产物 + 装 peer deps + 配置包管理器
```

装完还需要在 `src/global.css` 里补两行：

```css
@import 'heroui-native-pro/styles';
@source '../node_modules/heroui-native-pro/lib';
```

`pnpm-workspace.yaml` 已经把 `heroui-pro` / `heroui-native-pro` 加进 `onlyBuiltDependencies`，否则 pnpm 10 会拦掉它们的 postinstall，产物下不下来。CI 用 `HEROUI_AUTH_TOKEN` 环境变量代替交互登录。

## 目录

```
src/
├── api/
│   ├── config.ts   API 地址（平台默认值 + EXPO_PUBLIC_* 覆盖）
│   ├── http.ts     expo/fetch 封装，统一鉴权与错误
│   ├── sse.ts      SSE 解析，逐行移植自 apps/web/src/api.ts
│   ├── session.ts  SecureStore 存取 JWT
│   ├── index.ts    业务端点
│   └── types.ts    与 Web 端对齐的契约类型
├── app/            Expo Router 路由（root 在 src/app）
│   ├── _layout.tsx  Provider 装配
│   ├── index.tsx    按登录态重定向
│   ├── login.tsx    登录 / 注册
│   └── chats/
│       ├── index.tsx  助手列表 + 新建助手
│       └── [id].tsx   聊天页（SSE 流式 + 停止 + 重连续跑）
└── providers/
    └── session.tsx 登录态 Context
```

## 已知缺口

- **Markdown 渲染未做**：消息气泡目前是纯文本。Web 端用 `react-markdown`，RN 端需要另选方案（`react-native-markdown-display` / Expo DOM Components / WebView），待评估后再定。
- **只做了首期范围**：登录、助手列表、聊天。LLM 连接、Skills、MCP、Routines 等设置页未做。
- `app.json` 里 `NSAppTransportSecurity.NSAllowsArbitraryLoads: true` 是为本地开发放开的，**上架前必须移除**。
- Metro `watchFolders` 指向仓库根，会监听整个仓库；装 `watchman`（`brew install watchman`）能明显减轻负担。