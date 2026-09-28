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

const config: CapacitorConfig = {
  appId: "com.openbot.mobile",
  appName: "Open Bot",
  webDir: "../web/dist",
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
