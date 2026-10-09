import type { CapacitorConfig } from "@capacitor/cli";

/**
 * Mobile shell reuses apps/web build output (no UI source copy).
 *
 * Live reload (simulator / emulator only):
 *   CAP_LIVE_RELOAD=1 make sync-mobile
 *   optional: CAP_SERVER_URL=http://127.0.0.1:5173
 * Physical device: use LAN IP, e.g. CAP_SERVER_URL=http://192.168.x.x:5173
 */
const liveReload = process.env.CAP_LIVE_RELOAD === "1";
const serverUrl =
  process.env.CAP_SERVER_URL?.trim() || "http://127.0.0.1:5173";

// WebView 默认以 https://localhost 为源（androidScheme 默认 https）。
// 真机打 http://<局域网IP> 属于混合内容，会被 WebView 拦成
// net::ERR_BLOCKED_BY_MIXED_CONTENT（表现为连不上），与后端无关。
// 因此 API 为明文时自动放开混合内容。
const apiBase = process.env.VITE_API_BASE?.trim() || "";
const allowMixedContent = apiBase.startsWith("http://");

const config: CapacitorConfig = {
  appId: "com.openbot.mobile",
  appName: "Open Bot",
  webDir: "../web/dist",
  android: {
    allowMixedContent,
  },
  ...(liveReload
    ? {
        server: {
          url: serverUrl,
          cleartext: true,
        },
      }
    : {}),
};

export default config;
