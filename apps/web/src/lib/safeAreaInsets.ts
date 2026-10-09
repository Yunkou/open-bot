/**
 * Android Capacitor safe-area floors (chat immersive + settings padded chrome).
 *
 * Spec: bot-chat-immersive-overlay-v1.md — messages full-bleed under overlay chrome;
 * interactive topbar/composer (and settings hub) still need real insets when
 * env(safe-area-inset-*)=0 (Cap7 + targetSdk 35 edge-to-edge, no StatusBar plugin).
 *
 * Do NOT pair with capacitor.config android.adjustMarginsForEdgeToEdge="auto":
 * that insets the whole WebView and fights immersive full-height chat.
 *
 * Writes --sat / --sab on :root. styles.css:
 *   --safe-top:    max(env(safe-area-inset-top), var(--sat))
 *   --safe-bottom: max(env(safe-area-inset-bottom), var(--sab))
 *
 * Floors (Capacitor only): top 32px · bottom 20px
 *   - 32 ≈ typical 24dp status bar + breathing room (cutouts often 28–44)
 *   - 20 ≈ gesture/nav hint (16–24); 3-button nav may need APK verify
 */

import { detectClientContext } from "./clientEnv";

/** Capacitor Android / iOS fallback when env() and plugins yield 0. */
export const CAPACITOR_SAFE_TOP_FLOOR_PX = 32;
export const CAPACITOR_SAFE_BOTTOM_FLOOR_PX = 20;

export type SafeAreaInsets = { top: number; bottom: number; source: string };


function isCapacitorNative(): boolean {
  if (typeof window === "undefined") return false;
  try {
    if (window.Capacitor?.isNativePlatform?.()) return true;
  } catch {
    /* ignore */
  }
  return detectClientContext().app === "capacitor";
}

/** Read computed env(safe-area-inset-*) via a probe (may still be 0 on Android). */
function readCssEnvInsets(): { top: number; bottom: number } {
  if (typeof document === "undefined") return { top: 0, bottom: 0 };
  const el = document.createElement("div");
  el.style.cssText =
    "position:fixed;visibility:hidden;pointer-events:none;" +
    "padding-top:env(safe-area-inset-top,0px);" +
    "padding-bottom:env(safe-area-inset-bottom,0px);";
  document.documentElement.appendChild(el);
  const cs = getComputedStyle(el);
  const top = px(cs.paddingTop);
  const bottom = px(cs.paddingBottom);
  el.remove();
  return { top, bottom };
}

function px(v: string): number {
  const n = parseFloat(v || "0");
  return Number.isFinite(n) ? Math.max(0, Math.round(n)) : 0;
}

/** Best-effort native SafeArea / community plugin insets. */
async function tryPluginInsets(): Promise<{ top: number; bottom: number } | null> {
  if (typeof window === "undefined") return null;

  // capacitor-plugin-safe-area / @capacitor-community/safe-area style bridge
  try {
    const raw = await window.Capacitor?.Plugins?.SafeArea?.getSafeAreaInsets?.();
    if (raw) {
      const top = Math.round(raw.insets?.top ?? raw.top ?? 0);
      const bottom = Math.round(raw.insets?.bottom ?? raw.bottom ?? 0);
      if (top > 0 || bottom > 0) return { top, bottom };
    }
  } catch {
    /* plugin missing */
  }

  // Dynamic import variants (present only when APK bundles the plugin)
  for (const spec of [
    "@capacitor-community/safe-area",
    "capacitor-plugin-safe-area",
  ] as const) {
    try {
      const mod = await import(/* @vite-ignore */ spec);
      const api = mod.SafeArea ?? mod.SafeAreaController ?? mod.default;
      const fn = api?.getSafeAreaInsets ?? api?.getInsets;
      if (typeof fn === "function") {
        const raw = await fn.call(api);
        const top = Math.round(raw?.insets?.top ?? raw?.top ?? 0);
        const bottom = Math.round(raw?.insets?.bottom ?? raw?.bottom ?? 0);
        if (top > 0 || bottom > 0) return { top, bottom };
      }
    } catch {
      /* not in web-only pack */
    }
  }

  return null;
}

/**
 * Heuristic: when the layout viewport is shorter than the screen by a status-bar-sized
 * gap, treat the gap as top inset. Often useless when overlaysWebView makes
 * innerHeight === screen.height (the common Android Capacitor case).
 */
function heuristicInsets(): { top: number; bottom: number } {
  if (typeof window === "undefined" || typeof screen === "undefined") {
    return { top: 0, bottom: 0 };
  }
  const screenH = screen.height || 0;
  const innerH = window.innerHeight || 0;
  const vvH = window.visualViewport?.height ?? innerH;
  const gap = Math.max(0, screenH - Math.round(vvH));
  // Only trust a status-bar-sized gap (20–56); larger gaps are usually browser chrome / keyboard
  if (gap >= 20 && gap <= 56) {
    return { top: gap, bottom: 0 };
  }
  return { top: 0, bottom: 0 };
}

export function applySafeAreaCssVars(top: number, bottom: number): void {
  if (typeof document === "undefined") return;
  const root = document.documentElement;
  root.style.setProperty("--sat", `${Math.max(0, Math.round(top))}px`);
  root.style.setProperty("--sab", `${Math.max(0, Math.round(bottom))}px`);
}

export async function resolveSafeAreaInsets(): Promise<SafeAreaInsets> {
  const css = readCssEnvInsets();
  const plugin = await tryPluginInsets();
  const heur = heuristicInsets();
  const native = isCapacitorNative();

  let top = Math.max(css.top, plugin?.top ?? 0, heur.top);
  let bottom = Math.max(css.bottom, plugin?.bottom ?? 0, heur.bottom);
  let source = "css-env";

  if (plugin && (plugin.top > 0 || plugin.bottom > 0)) source = "plugin";
  else if (heur.top > 0 && heur.top >= css.top) source = "heuristic";

  if (native) {
    document.documentElement.classList.add("capacitor-native");
    if (top < CAPACITOR_SAFE_TOP_FLOOR_PX) {
      top = CAPACITOR_SAFE_TOP_FLOOR_PX;
      source = source === "css-env" && css.top === 0 ? "capacitor-floor" : `${source}+floor`;
    }
    if (bottom < CAPACITOR_SAFE_BOTTOM_FLOOR_PX) {
      bottom = CAPACITOR_SAFE_BOTTOM_FLOOR_PX;
      if (!source.includes("floor")) source = `${source}+floor`;
    }
  }

  return { top, bottom, source };
}

/**
 * Install inset CSS vars and keep them in sync on rotate / resize.
 * Returns cleanup. Safe to call in browser (no-op floors) and Capacitor.
 *
 * Does NOT call StatusBar.setOverlaysWebView(false): edge-to-edge Android expects
 * overlay + padding. Plugin may be absent in web-only packs anyway.
 */
export async function installSafeAreaInsets(): Promise<() => void> {
  if (typeof document === "undefined") return () => {};

  const sync = async () => {
    const insets = await resolveSafeAreaInsets();
    applySafeAreaCssVars(insets.top, insets.bottom);
    try {
      document.documentElement.dataset.safeAreaSource = insets.source;
    } catch {
      /* ignore */
    }
  };

  await sync();

  const onChange = () => {
    void sync();
  };
  window.addEventListener("resize", onChange);
  window.addEventListener("orientationchange", onChange);
  window.visualViewport?.addEventListener("resize", onChange);

  return () => {
    window.removeEventListener("resize", onChange);
    window.removeEventListener("orientationchange", onChange);
    window.visualViewport?.removeEventListener("resize", onChange);
  };
}
