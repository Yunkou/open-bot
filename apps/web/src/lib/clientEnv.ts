/**
 * Client environment envelope (shared shape with Go / Python RunRequest.client).
 * Detects browser / Tauri / Capacitor across web + desktop + mobile shells.
 */

export type ClientPlatform = "web" | "macos" | "windows" | "linux" | "ios" | "android";
export type ClientApp = "browser" | "tauri" | "capacitor";

export type ClientCapabilities = {
  host_tools: boolean;
  workspace_tools: boolean;
};

export type ClientContext = {
  platform: ClientPlatform;
  app: ClientApp;
  os: string;
  arch: string;
  app_version: string;
  locale: string;
  machine_id?: string;
  capabilities: ClientCapabilities;
};

declare global {
  interface Window {
    __TAURI__?: unknown;
    __TAURI_INTERNALS__?: unknown;
    Capacitor?: {
      isNativePlatform?: () => boolean;
      getPlatform?: () => string;
    };
  }
}

const APP_VERSION = "0.1.0";

function hasTauri(): boolean {
  if (typeof window === "undefined") return false;
  if (window.__TAURI__ != null || window.__TAURI_INTERNALS__ != null) return true;
  const ua = navigator.userAgent || "";
  return /tauri/i.test(ua);
}

function hasCapacitor(): boolean {
  if (typeof window === "undefined") return false;
  try {
    return Boolean(window.Capacitor?.isNativePlatform?.());
  } catch {
    return false;
  }
}

function detectOsArch(): { os: string; arch: string; platformFromOs: ClientPlatform | null } {
  const nav = typeof navigator !== "undefined" ? navigator : null;
  const ua = (nav?.userAgent || "").toLowerCase();
  const platform = (nav?.platform || "").toLowerCase();
  // Chromium UA-CH when available
  const uaData = (nav as Navigator & { userAgentData?: { platform?: string } })?.userAgentData;
  const chPlatform = (uaData?.platform || "").toLowerCase();

  let os = "";
  let platformFromOs: ClientPlatform | null = null;

  if (/iphone|ipad|ipod/.test(ua) || platform === "iphone" || platform === "ipad") {
    os = "ios";
    platformFromOs = "ios";
  } else if (/android/.test(ua)) {
    os = "android";
    platformFromOs = "android";
  } else if (
    /mac/.test(platform) ||
    /mac os|macintosh/.test(ua) ||
    chPlatform === "macos"
  ) {
    os = "darwin";
    platformFromOs = "macos";
  } else if (/win/.test(platform) || /windows/.test(ua) || chPlatform === "windows") {
    os = "win32";
    platformFromOs = "windows";
  } else if (/linux/.test(platform) || /linux/.test(ua) || chPlatform === "linux") {
    os = "linux";
    platformFromOs = "linux";
  } else {
    os = platform || chPlatform || "unknown";
  }

  let arch = "";
  if (/aarch64|arm64/.test(ua) || /arm64/.test(platform)) arch = "arm64";
  else if (/x86_64|win64|wow64|amd64/.test(ua) || /x86_64/.test(platform)) arch = "x64";
  else if (/i[3-6]86|win32/.test(ua)) arch = "x86";

  return { os, arch, platformFromOs };
}

function capacitorPlatform(): ClientPlatform | null {
  try {
    const p = (window.Capacitor?.getPlatform?.() || "").toLowerCase();
    if (p === "ios") return "ios";
    if (p === "android") return "android";
    if (p === "web") return "web";
  } catch {
    /* ignore */
  }
  return null;
}

/** Detect once per page load; safe to call often (cheap). */
export function detectClientContext(): ClientContext {
  const { os, arch, platformFromOs } = detectOsArch();
  const locale =
    (typeof navigator !== "undefined" && (navigator.language || navigator.languages?.[0])) ||
    "zh-CN";

  if (hasTauri()) {
    return {
      platform: platformFromOs && platformFromOs !== "web" ? platformFromOs : "macos",
      app: "tauri",
      os: os || "darwin",
      arch: arch || "arm64",
      app_version: APP_VERSION,
      locale,
      capabilities: { host_tools: true, workspace_tools: true },
    };
  }

  if (hasCapacitor()) {
    const capPlat = capacitorPlatform();
    const platform: ClientPlatform =
      capPlat && capPlat !== "web"
        ? capPlat
        : platformFromOs === "ios" || platformFromOs === "android"
          ? platformFromOs
          : "android";
    return {
      platform,
      app: "capacitor",
      os: platform === "ios" ? "ios" : platform === "android" ? "android" : os,
      arch: arch || "",
      app_version: APP_VERSION,
      locale,
      capabilities: { host_tools: false, workspace_tools: true },
    };
  }

  return {
    platform: "web",
    app: "browser",
    os: os || "unknown",
    arch,
    app_version: APP_VERSION,
    locale,
    capabilities: { host_tools: false, workspace_tools: true },
  };
}

const MACHINE_KEY_STORAGE = "openbot_machine_key";
const MACHINE_ID_STORAGE = "openbot_machine_id";

/** Stable per-install id for desktop/mobile registration (not for plain browsers). */
export function getOrCreateMachineKey(): string {
  try {
    const existing = localStorage.getItem(MACHINE_KEY_STORAGE);
    if (existing && existing.trim()) return existing.trim();
    const key =
      typeof crypto !== "undefined" && "randomUUID" in crypto
        ? crypto.randomUUID()
        : `mk-${Date.now()}-${Math.random().toString(36).slice(2)}`;
    localStorage.setItem(MACHINE_KEY_STORAGE, key);
    return key;
  } catch {
    return `mk-ephemeral-${Date.now()}`;
  }
}

export function getStoredMachineId(): string | null {
  try {
    return localStorage.getItem(MACHINE_ID_STORAGE);
  } catch {
    return null;
  }
}

export function setStoredMachineId(id: string): void {
  try {
    localStorage.setItem(MACHINE_ID_STORAGE, id);
  } catch {
    /* ignore */
  }
}

export function clearStoredMachineId(): void {
  try {
    localStorage.removeItem(MACHINE_ID_STORAGE);
  } catch {
    /* ignore */
  }
}

/** Whether this client should register as a host machine (desktop/mobile shells). */
export function shouldRegisterAsHost(client: ClientContext = detectClientContext()): boolean {
  return client.app === "tauri" || client.app === "capacitor";
}

export function defaultMachineLabel(client: ClientContext): string {
  switch (client.platform) {
    case "macos":
      return "我的 Mac";
    case "windows":
      return "我的 Windows 电脑";
    case "linux":
      return "我的 Linux 电脑";
    case "ios":
      return "我的 iOS 设备";
    case "android":
      return "我的 Android 设备";
    default:
      return "本机";
  }
}
