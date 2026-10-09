/** Formal palette — locked with open bot / UI UE design (unchanged in v2). */
export const AVATAR_COLOR_PALETTE = [
  "#e85d4c",
  "#2a9d8f",
  "#f4a261",
  "#e76f51",
  "#457b9d",
  "#9b5de5",
  "#00bbf9",
  "#f15bb5",
  "#00f5d4",
  "#fee440",
  "#06d6a0",
  "#118ab2",
] as const;

/** Formal shapes — avatar v2 organic silhouettes. */
export const AVATAR_SHAPES = [
  "cloud",
  "bean",
  "drop",
  "soft-hex",
  "petal",
  "puff",
] as const;

export type AvatarShape = (typeof AVATAR_SHAPES)[number];
export type AvatarColor = (typeof AVATAR_COLOR_PALETTE)[number] | string;

/** Old CSS-shape ids → v2 silhouette ids (read-path compat). */
export const LEGACY_SHAPE_MAP: Record<string, AvatarShape> = {
  circle: "cloud",
  rounded: "puff",
  squircle: "bean",
  hex: "soft-hex",
  diamond: "drop",
  "soft-square": "petal",
};

export const BOT_PRESENCE_STATUSES = [
  "idle",
  "thinking",
  "working",
  "awaiting_approval",
  "error",
] as const;

export type BotPresenceStatus = "idle" | "thinking" | "working" | "awaiting_approval" | "error";

/**
 * Normalize any wire/UI status to one of the five faces.
 * Empty / missing → idle; known five pass through; anything else → working
 * (product rule v2.1: unknown status shows as working, never throws).
 */
export function normalizePresenceStatus(status?: string | null): BotPresenceStatus {
  const s = (status ?? "").trim().toLowerCase();
  if (!s) return "idle";
  return (BOT_PRESENCE_STATUSES as readonly string[]).includes(s)
    ? (s as BotPresenceStatus)
    : "working";
}


const AVATAR_COLORS = AVATAR_COLOR_PALETTE;

export function hashString(input: string): number {
  let h = 0;
  for (let i = 0; i < input.length; i++) {
    h = (h * 31 + input.charCodeAt(i)) | 0;
  }
  return Math.abs(h);
}

export function avatarColor(seed: string): string {
  const idx = hashString(seed || "?") % AVATAR_COLORS.length;
  return AVATAR_COLORS[idx];
}

export function avatarShape(seed: string): AvatarShape {
  const idx = hashString(`shape:${seed || "?"}`) % AVATAR_SHAPES.length;
  return AVATAR_SHAPES[idx];
}

export function resolveAvatarColor(seed: string, override?: string | null): string {
  if (override && /^#[0-9a-fA-F]{6}$/.test(override.trim())) return override.trim();
  return avatarColor(seed);
}

export function resolveAvatarShape(seed: string, override?: string | null): AvatarShape {
  const s = (override || "").trim();
  if ((AVATAR_SHAPES as readonly string[]).includes(s)) return s as AvatarShape;
  if (s && LEGACY_SHAPE_MAP[s]) return LEGACY_SHAPE_MAP[s];
  return avatarShape(seed);
}

/** Initial letter(s) — kept for non-avatar uses; AgentAvatar must not show letters. */
export function avatarInitials(name: string): string {
  const t = (name || "?").trim();
  if (!t) return "?";
  const parts = t.split(/[\s_-]+/).filter(Boolean);
  if (parts.length >= 2) {
    return (parts[0][0] + parts[1][0]).toUpperCase();
  }
  return t.slice(0, 2).toUpperCase();
}

const LOCAL_AVATAR_KEY = "openbot_avatar_prefs";

export type LocalAvatarPref = { shape?: string; color?: string };

export function loadLocalAvatarPrefs(): Record<string, LocalAvatarPref> {
  try {
    const raw = localStorage.getItem(LOCAL_AVATAR_KEY);
    if (!raw) return {};
    return JSON.parse(raw) as Record<string, LocalAvatarPref>;
  } catch {
    return {};
  }
}

export function saveLocalAvatarPref(agentId: string, pref: LocalAvatarPref) {
  const all = loadLocalAvatarPrefs();
  all[agentId] = { ...all[agentId], ...pref };
  localStorage.setItem(LOCAL_AVATAR_KEY, JSON.stringify(all));
}

/** Pick shape+color for a new Bot, avoiding pairs already used by sibling agents. */
export function pickDefaultAvatar(
  seed: string,
  usedPairs: Iterable<string> = [],
): { shape: AvatarShape; color: string } {
  const used = new Set(usedPairs);
  const baseShape = avatarShape(seed);
  const baseColor = avatarColor(seed);
  const pair0 = `${baseShape}|${baseColor}`;
  if (!used.has(pair0)) return { shape: baseShape, color: baseColor };
  for (let i = 0; i < AVATAR_COLOR_PALETTE.length * AVATAR_SHAPES.length; i++) {
    const shape = AVATAR_SHAPES[(hashString(seed) + i) % AVATAR_SHAPES.length];
    const color = AVATAR_COLOR_PALETTE[(hashString(seed) + i * 3) % AVATAR_COLOR_PALETTE.length];
    const pair = `${shape}|${color}`;
    if (!used.has(pair)) return { shape, color };
  }
  return { shape: baseShape, color: baseColor };
}
