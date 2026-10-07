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
          getId?: () => Promise<{ identifier?: string }>;
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
      capabilities: { host_tools: false, workspace_tools: true }, // rule A: phone never hosts
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

export type ClientDeviceType = "desktop" | "mobile" | "browser";

/** Product device_type for register / list UI (rule A: phone = login-only). */
export function resolveClientDeviceType(
  client: ClientContext = detectClientContext(),
): ClientDeviceType {
  if (client.app === "capacitor") return "mobile";
  if (client.app === "tauri") return "desktop";
  if (client.platform === "ios" || client.platform === "android") return "mobile";
  return "browser";
}

function readStoredMachineKey(): string | null {
  try {
    const existing = localStorage.getItem(MACHINE_KEY_STORAGE);
    if (existing && existing.trim()) return existing.trim();
  } catch {
    /* ignore */
  }
  return null;
}

function writeStoredMachineKey(key: string): void {
  try {
    localStorage.setItem(MACHINE_KEY_STORAGE, key);
  } catch {
    /* ignore */
  }
}

function newLocalMachineKey(): string {
  return typeof crypto !== "undefined" && "randomUUID" in crypto
    ? crypto.randomUUID()
    : `mk-${Date.now()}-${Math.random().toString(36).slice(2)}`;
}

/**
 * Sync localStorage UUID (desktop / browser fallback).
 * Prefer `resolveMachineKey` so Capacitor can use native ANDROID_ID / identifierForVendor.
 */
export function getOrCreateMachineKey(): string {
  const existing = readStoredMachineKey();
  if (existing) return existing;
  const key = newLocalMachineKey();
  writeStoredMachineKey(key);
  return key;
}

/**
 * Stable machine_key priority:
 * 1. Capacitor: Device.getId().identifier (ANDROID_ID / identifierForVendor)
 * 2. Tauri: invoke("host_machine_id") when desktop ships it (IOPlatformUUID / MachineGuid /etc/machine-id)
 * 3. Fallback: localStorage UUID (openbot_machine_key)
 * Migration: native id wins when available and is written back to localStorage so upsert merges;
 * if native unavailable, keep prior localStorage key.
 *
 * Desktop Tauri exposes host_machine_id (IOPlatformUUID / MachineGuid / machine-id).
 * If invoke fails (old build), falls back to localStorage UUID.
 */
export async function resolveMachineKey(
  client: ClientContext = detectClientContext(),
): Promise<string> {
  const existing = readStoredMachineKey();

  if (client.app === "capacitor") {
    const nativeId = await tryCapacitorDeviceIdentifier();
    if (nativeId) {
      writeStoredMachineKey(nativeId);
      return nativeId;
    }
  } else if (client.app === "tauri") {
    const nativeId = await tryTauriMachineId();
    if (nativeId) {
      writeStoredMachineKey(nativeId);
      return nativeId;
    }
  }

  if (existing) return existing;
  const key = newLocalMachineKey();
  writeStoredMachineKey(key);
  return key;
}

async function tryCapacitorDeviceIdentifier(): Promise<string | null> {
  try {
    const mod = await import("@capacitor/device");
    const id = await mod.Device.getId();
    const identifier = (id?.identifier || "").trim();
    if (identifier) return identifier;
  } catch {
    /* fall through to bridge / none */
  }
  try {
    const id = await window.Capacitor?.Plugins?.Device?.getId?.();
    const identifier = (id?.identifier || "").trim();
    if (identifier) return identifier;
  } catch {
    /* ignore */
  }
  return null;
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

/**
 * Register + heartbeat for Tauri/Capacitor shells (presence listing).
 * Host exec WebSocket stays gated on app === "tauri" separately.
 * Capacitor keeps host_tools:false (login-only / rule A).
 */
export function shouldRegisterAsHost(client: ClientContext = detectClientContext()): boolean {
  return client.app === "tauri" || client.app === "capacitor";
}

/** Machine row is phone / Capacitor → login-only, never prefer-host. */
export function isLoginOnlyMachine(m: {
  device_type?: string | null;
  platform?: string | null;
  app?: string | null;
}): boolean {
  const dt = (m.device_type || "").toLowerCase();
  if (dt === "mobile") return true;
  if (dt === "desktop" || dt === "browser") return false;
  const plat = (m.platform || "").toLowerCase();
  if (plat === "ios" || plat === "android") return true;
  if ((m.app || "").toLowerCase() === "capacitor") return true;
  return false;
}

/** Prefer-computer / 优先电脑 dropdown: desktop hosts only. */
export function preferHostMachines<T extends {
  device_type?: string | null;
  platform?: string | null;
  app?: string | null;
}>(machines: T[]): T[] {
  return machines.filter((m) => !isLoginOnlyMachine(m));
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

/** Best-effort: returns null if command missing (current desktop builds). */
async function tryTauriMachineId(): Promise<string | null> {
  const invoke = tauriInvoke();
  if (!invoke) return null;
  for (const cmd of ["host_machine_id", "host_machine_key"] as const) {
    try {
      const raw = await invoke(cmd);
      const id = typeof raw === "string" ? raw.trim() : "";
      if (id) return id.slice(0, 128);
    } catch {
      /* command not registered yet */
    }
  }
  return null;
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
