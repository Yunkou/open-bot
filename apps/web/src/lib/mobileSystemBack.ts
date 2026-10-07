/**
 * Mobile system back / gesture stack (Android Capacitor backButton; iOS if Cap fires).
 * Same order as UI ← — see bot-mobile-back-gesture-v1.md.
 *
 * Priority:
 *  1. Any sheet / modal / bottom confirm → close first
 *  2. Settings subpage → hub → then close settings
 *  3. Chat → list
 *  4. List root → first back: toast + hold; second within ~2s → exitApp
 *
 * Child overlays (e.g. MessageActionSheet) opt in via OPENBOT_SYSTEM_BACK_EVENT:
 * call preventDefault() on the CustomEvent and dismiss themselves.
 */

import { detectClientContext } from "./clientEnv";

/** Document event: cancelable; preventDefault = overlay handled the back. */
export const OPENBOT_SYSTEM_BACK_EVENT = "openbot:system-back";

export const LIST_ROOT_EXIT_WINDOW_MS = 2000;

/** Toast copy — locked design allows either; button/backButton-centric default. */
export const EXIT_TOAST_COPY = "再按一次退出";
/** Gesture-nav flavored alternate (product may swap). */
export const EXIT_TOAST_COPY_SWIPE = "再滑一次退出";

export type SystemBackOverlayState = {
  /** ConfirmProvider dismiss — returns true if a confirm was open. */
  dismissConfirm?: () => boolean;
  secretPromptOpen?: boolean;
  dismissSecretPrompt?: () => void;
  feedbackOpen?: boolean;
  dismissFeedback?: () => void;
  trainOpen?: boolean;
  dismissTrain?: () => void;
  avatarSettingsOpen?: boolean;
  dismissAvatarSettings?: () => void;
  createSheetOpen?: boolean;
  dismissCreateSheet?: () => void;
  newChatOpen?: boolean;
  dismissNewChat?: () => void;
  convSheetOpen?: boolean;
  dismissConvSheet?: () => void;
  showSettings?: boolean;
  settingsShellHub?: boolean;
  setSettingsShellHub?: (hub: boolean) => void;
  closeSettings?: () => void;
  mobileView?: "list" | "chat";
  setMobileView?: (v: "list" | "chat") => void;
  /** Narrow / shell list+chat split active */
  isNarrowLayout?: boolean;
};

export type SystemBackResult =
  | { handled: true; action: string }
  | { handled: false; action: "exit-ready" };

/**
 * Pure stack walk — mirrors UI ←. Returns whether back was consumed.
 * Caller exits app only when result.action === "exit-ready".
 */
export function handleSystemBack(
  state: SystemBackOverlayState,
  opts?: {
    /** ms since last list-root back; null/undefined = no prior */
    lastListRootBackAt?: number | null;
    now?: number;
    exitWindowMs?: number;
    /** Called on first list-root back (show toast). */
    onExitPrompt?: () => void;
    /** Dispatch child-overlay event; return true if a listener prevented default. */
    notifyChildOverlays?: () => boolean;
  },
): SystemBackResult {
  const now = opts?.now ?? Date.now();
  const windowMs = opts?.exitWindowMs ?? LIST_ROOT_EXIT_WINDOW_MS;

  // --- Priority 1: sheets / modals / confirms (top-most first) ---
  if (opts?.notifyChildOverlays?.()) {
    return { handled: true, action: "child-overlay" };
  }
  if (state.dismissConfirm?.()) {
    return { handled: true, action: "confirm" };
  }
  if (state.secretPromptOpen) {
    state.dismissSecretPrompt?.();
    return { handled: true, action: "secret-prompt" };
  }
  if (state.feedbackOpen) {
    state.dismissFeedback?.();
    return { handled: true, action: "feedback" };
  }
  if (state.trainOpen) {
    state.dismissTrain?.();
    return { handled: true, action: "train" };
  }
  if (state.avatarSettingsOpen) {
    state.dismissAvatarSettings?.();
    return { handled: true, action: "bot-avatar-settings" };
  }
  if (state.createSheetOpen) {
    state.dismissCreateSheet?.();
    return { handled: true, action: "create-sheet" };
  }
  if (state.newChatOpen) {
    state.dismissNewChat?.();
    return { handled: true, action: "new-chat" };
  }
  if (state.convSheetOpen) {
    state.dismissConvSheet?.();
    return { handled: true, action: "conv-sheet" };
  }

  // --- Priority 2: settings subpage → hub → close ---
  if (state.showSettings) {
    if (state.settingsShellHub === false) {
      state.setSettingsShellHub?.(true);
      return { handled: true, action: "settings-to-hub" };
    }
    state.closeSettings?.();
    return { handled: true, action: "settings-close" };
  }

  // --- Priority 3: chat → list ---
  if (state.mobileView === "chat") {
    state.setMobileView?.("list");
    return { handled: true, action: "chat-to-list" };
  }

  // --- Priority 4: list root double-back ---
  const last = opts?.lastListRootBackAt;
  if (last != null && now - last <= windowMs) {
    return { handled: false, action: "exit-ready" };
  }
  opts?.onExitPrompt?.();
  return { handled: true, action: "list-root-prompt" };
}

/** Dispatch cancelable event; true if a child overlay consumed it. */
export function notifySystemBackOverlays(): boolean {
  if (typeof document === "undefined") return false;
  const ev = new CustomEvent(OPENBOT_SYSTEM_BACK_EVENT, {
    cancelable: true,
    bubbles: true,
  });
  document.dispatchEvent(ev);
  return ev.defaultPrevented;
}

export type InstallBackButtonOptions = {
  /** Return true if the back was handled (do not exit). */
  onBack: () => boolean | SystemBackResult;
  /** Called when stack says exit-ready. Default: App.exitApp(). */
  exitApp?: () => void | Promise<void>;
};

/**
 * Dynamic-import `@capacitor/app` backButton listener (matches clientEnv Device import).
 * No-op on web / when plugin missing — build still works.
 * Registering the listener disables Cap default exit; we call exitApp only on double-back.
 */
export async function installCapacitorBackButton(
  opts: InstallBackButtonOptions,
): Promise<() => void> {
  const client = detectClientContext();
  if (client.app !== "capacitor") {
    return () => {};
  }

  try {
    const mod = await import("@capacitor/app");
    const App = mod.App;
    if (!App?.addListener) return () => {};

    const handle = await App.addListener("backButton", (_ev) => {
      // Cap 7: listening disables default exit. No preventDefault on the event object.
      // canGoBack reflects WebView history — we own the product stack instead of history.back().
      const raw = opts.onBack();
      const handled =
        typeof raw === "boolean" ? raw : raw.handled;
      if (handled) return;
      const exit = opts.exitApp ?? (() => void App.exitApp());
      void Promise.resolve(exit()).catch(() => {
        /* ignore */
      });
    });

    return () => {
      void handle.remove();
    };
  } catch {
    /* plugin missing in web / unsynced native — silent */
    return () => {};
  }
}
