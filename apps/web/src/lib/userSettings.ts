/**
 * Per-user Bot settings (timezone + Auto-review) with a small local cache
 * so host-exec gates can apply rules without awaiting the network each time.
 */

export type AutoReviewRuleAction = "ask_first" | "auto_allow";

export type AutoReviewRule = {
  id: string;
  when: string;
  action: AutoReviewRuleAction;
};

export type UserSettings = {
  user_id?: string;
  timezone: string; // IANA; empty = auto-detect
  auto_review_enabled: boolean;
  auto_review_rules: AutoReviewRule[];
  updated_at?: string;
};

export type MachineExecPolicy = "allow" | "ask" | "deny";

const CACHE_KEY = "openbot_user_settings";

export function defaultUserSettings(): UserSettings {
  return {
    timezone: "",
    auto_review_enabled: true,
    auto_review_rules: [],
  };
}

export function detectBrowserTimezone(): string {
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || "Asia/Shanghai";
  } catch {
    return "Asia/Shanghai";
  }
}

/** Effective IANA timezone: stored preference, else browser auto-detect. */
export function effectiveTimezone(settings: UserSettings | null | undefined): string {
  const stored = (settings?.timezone || "").trim();
  if (stored) return stored;
  return detectBrowserTimezone();
}

export function readCachedUserSettings(): UserSettings {
  try {
    const raw = localStorage.getItem(CACHE_KEY);
    if (!raw) return defaultUserSettings();
    const parsed = JSON.parse(raw) as UserSettings;
    return {
      timezone: typeof parsed.timezone === "string" ? parsed.timezone : "",
      auto_review_enabled: parsed.auto_review_enabled !== false,
      auto_review_rules: Array.isArray(parsed.auto_review_rules)
        ? parsed.auto_review_rules.filter(
            (r) =>
              r &&
              typeof r.when === "string" &&
              (r.action === "ask_first" || r.action === "auto_allow"),
          )
        : [],
      user_id: parsed.user_id,
      updated_at: parsed.updated_at,
    };
  } catch {
    return defaultUserSettings();
  }
}

export function writeCachedUserSettings(settings: UserSettings): void {
  try {
    localStorage.setItem(CACHE_KEY, JSON.stringify(settings));
  } catch {
    /* ignore */
  }
}

export const COMMON_TIMEZONES: { value: string; label: string }[] = [
  { value: "", label: "自动检测" },
  { value: "Asia/Shanghai", label: "Asia/Shanghai（中国）" },
  { value: "Asia/Hong_Kong", label: "Asia/Hong_Kong" },
  { value: "Asia/Tokyo", label: "Asia/Tokyo" },
  { value: "Asia/Singapore", label: "Asia/Singapore" },
  { value: "UTC", label: "UTC" },
  { value: "America/New_York", label: "America/New_York" },
  { value: "America/Los_Angeles", label: "America/Los_Angeles" },
  { value: "Europe/London", label: "Europe/London" },
  { value: "Europe/Paris", label: "Europe/Paris" },
];

export function normalizeMachineExecPolicy(v: string | undefined | null): MachineExecPolicy {
  const s = (v || "").trim().toLowerCase();
  if (s === "ask") return "ask";
  if (s === "deny") return "deny";
  return "allow";
}

export const MACHINE_EXEC_POLICY_OPTIONS: { value: MachineExecPolicy; label: string }[] = [
  { value: "allow", label: "始终允许" },
  { value: "ask", label: "每次询问" },
  { value: "deny", label: "不允许" },
];
