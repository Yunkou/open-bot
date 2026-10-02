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
  /** IANA timezone from user settings (optional). */
  timezone?: string;
  machine_id?: string;
  /** Display name of the current device (OS name or user rename). */
  machine_label?: string;
  capabilities: ClientCapabilities;
};

declare global {
  interface Window {
    __TAURI__?: unknown;
    __TAURI_INTERNALS__?: unknown;
    Capacitor?: {
      isNativePlatform?: () => boolean;
      getPlatform?: () => string;
      Plugins?: {
        Device?: {
          getInfo?: () => Promise<{ name?: string; model?: string }>;
        };
      };
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

function platformFallbackLabel(client: ClientContext): string {
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

function browserName(): string {
  const ua = (typeof navigator !== "undefined" ? navigator.userAgent : "") || "";
  if (/edg\//i.test(ua)) return "Edge";
  if (/chrome|crios/i.test(ua) && !/edg\//i.test(ua)) return "Chrome";
  if (/firefox|fxios/i.test(ua)) return "Firefox";
  if (/safari/i.test(ua) && !/chrome|crios|android/i.test(ua)) return "Safari";
  return "浏览器";
}

function osDisplayName(client: ClientContext): string {
  switch (client.platform) {
    case "macos":
      return "macOS";
    case "windows":
      return "Windows";
    case "linux":
      return "Linux";
    case "ios":
      return "iOS";
    case "android":
      return "Android";
    default:
      if (client.os === "darwin") return "macOS";
      if (client.os === "win32") return "Windows";
      return client.os || "未知系统";
  }
}

/** Sync fallback label (generic). Prefer resolveDefaultMachineLabel for real OS names. */
export function defaultMachineLabel(client: ClientContext, deviceName?: string): string {
  const name = (deviceName || "").trim();
  if (name) return name.slice(0, 64);
  if (client.app === "browser" || client.platform === "web") {
    return `${browserName()} · ${osDisplayName(client)}`.slice(0, 64);
  }
  return platformFallbackLabel(client);
}

type TauriInvoke = (cmd: string, args?: Record<string, unknown>) => Promise<unknown>;

function tauriInvoke(): TauriInvoke | null {
  if (typeof window === "undefined") return null;
  const internals = (window as Window & { __TAURI_INTERNALS__?: { invoke?: TauriInvoke } })
    .__TAURI_INTERNALS__;
  return internals?.invoke ?? null;
}

/**
 * Best-effort real device / computer name:
 * - Tauri: OS computer name (macOS ComputerName / hostname / …)
 * - Capacitor: Device plugin name when available
 * - Browser: "Chrome · macOS" style fallback
 */
export async function resolveDeviceDisplayName(
  client: ClientContext = detectClientContext(),
): Promise<string> {
  if (client.app === "tauri") {
    const invoke = tauriInvoke();
    if (invoke) {
      try {
        const raw = await invoke("host_device_name");
        const name = typeof raw === "string" ? raw.trim() : "";
        if (name) return name.slice(0, 64);
      } catch {
        /* fall through */
      }
    }
  }

  if (client.app === "capacitor") {
    try {
      const mod = await import("@capacitor/device");
      const info = await mod.Device.getInfo();
      const name = (info?.name || info?.model || "").trim();
      if (name) return name.slice(0, 64);
    } catch {
      try {
        const info = await window.Capacitor?.Plugins?.Device?.getInfo?.();
        const name = (info?.name || info?.model || "").trim();
        if (name) return name.slice(0, 64);
      } catch {
        /* fall through */
      }
    }
  }

  return defaultMachineLabel(client);
}

/** Resolve the label to send on first machine register. */
export async function resolveDefaultMachineLabel(
  client: ClientContext = detectClientContext(),
): Promise<string> {
  return resolveDeviceDisplayName(client);
}
