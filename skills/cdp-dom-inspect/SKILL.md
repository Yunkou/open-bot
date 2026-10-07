---
name: cdp-dom-inspect
description: Read the live DOM/CSS/console of a running Chromium/Electron page over the Chrome DevTools Protocol (host_shell + a small Node script). Use to verify UI changes factually.
---

# 用 CDP 读取运行中页面的 DOM / CSS

> 改编自 [Hermes Agent](https://github.com/NousResearch/hermes-agent) `skills/software-development/inspecting-hermes-desktop-dom`（MIT，Nous Research）。原版只针对 Hermes 桌面端；这里改成通用的 Chromium / Electron 页面检查，并附带 `scripts/cdp_eval.mjs`。

在用户电脑上开发前端时，直接读**正在渲染的页面**——计算样式、几何尺寸、哪条 CSS 规则胜出、元素是否渲染、console 输出——而不是从 `.tsx` 猜。

**它不替代肉眼。** CDP 回答事实问题（「padding 计算值是多少」「这个元素渲染了吗」「哪个选择器命中」）；「好不好看」交给用户。

## 何时用

- 确认 UI 改动在运行中的页面里真的生效
- 「为什么这个元素还是 X？」——先找到胜出的规则再改代码
- 给即将修改的组件找稳定选择器
- 读用户说有但复制不出来的 console 报错

不适用：性能剖析 / 堆分析（用 `node-inspect-debugger`）；「看起来对不对」。

## 执行方式

全部在用户电脑上：`list_machines` → `load_skill host-shell` → `host_shell`。先把脚本放上去：`load_skill(name="cdp-dom-inspect", path="scripts/cdp_eval.mjs")` → `host_write` 到 `~/.open-bot/skills/cdp-dom-inspect/cdp_eval.mjs`（需要 Node ≥ 22）。

## 端口

先检查有没有 CDP 端口（默认 9222）：

```bash
curl -s --max-time 3 http://127.0.0.1:9222/json/version
```

空 = 没有端口。不要悄悄去猜别的端口。

- **Chrome / Chromium**：需以 `--remote-debugging-port=<port>` 启动。**不要重启用户正在用的浏览器**——起一个隔离实例：
  ```bash
  nohup "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" --remote-debugging-port=9333 \
    --user-data-dir=/tmp/open-bot-scratch/cdp-profile http://localhost:5173 > /tmp/cdp-chrome.log 2>&1 &
  ```
- **Electron 应用**：开发模式下加 `--remote-debugging-port=9333`（或 `ELECTRON_ENABLE_LOGGING=1` 看日志），同样用单独的 `--user-data-dir` 避开单实例锁。
- **Tauri / Safari / WKWebView**（例如 open-bot 桌面端在 macOS 上）：**没有 CDP**。改为在浏览器里打开同一个前端（如 `apps/web` 的 dev server）用上面的隔离 Chrome 检查，或请用户开 Safari Web Inspector。

刚启动的进程要一两秒端口才可用：轮询 `curl …/json/version`，看到 `DevTools listening on ws://…` 日志即证明已绑定。

## 读取 DOM

```bash
S=~/.open-bot/skills/cdp-dom-inspect/cdp_eval.mjs
node $S --port 9333 --list
node $S --port 9333 --match localhost:5173 "document.querySelectorAll('[data-testid]').length"
```

**只投影出小 JSON**，不要 dump `outerHTML`：

```bash
node $S --port 9333 --match 5173 "JSON.stringify({
  radius: getComputedStyle(document.documentElement).getPropertyValue('--radius').trim(),
  composer: !!document.querySelector('textarea')
})"
```

## 最擅长的问题：哪条规则胜出？

```js
const el = document.querySelector('.message a')
JSON.stringify({
  ownClasses: el.className,
  weight: getComputedStyle(el).fontWeight,
  parents: (() => { const out = []; let n = el; while ((n = n.parentElement) && out.length < 6) out.push(n.className); return out })()
})
```

元素自身没有 class 时，值是**继承**来的——逐个改调用点没用，要找祖先规则。插件样式（如 `@tailwindcss/typography` 的 `prose a { font-weight: 500 }`）常常压过工具类；在共享 class 上覆盖，而不是每个使用处。

## Console

```bash
node $S --port 9333 --match 5173 "JSON.stringify(performance.getEntriesByType('resource').filter(e=>e.responseStatus>=400).map(e=>e.name).slice(0,20))"
```

历史 console 日志不能事后读取；需要时在页面里先挂钩 `console.error` 再复现，或用 `dogfood` 的 Playwright 探针收集。

## 常见坑

- **不要为了「腾出端口」杀掉用户的 dev server 或应用。**
- **轮询，不要只探一次。**
- **用 `--match` 选对 target**，否则可能连到 devtools、弹窗或别的标签页。
- 隔离实例用完要结束：`pkill -f cdp-profile`（会走确认卡）。

## 验证

- 回答里给出读到的具体值（选择器、计算样式、计数），并说明来自哪个页面 / 端口。
