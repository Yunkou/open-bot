/** Deterministic avatar palette (OpenClaw-like red / green / orange / blue / magenta / teal). */
const AVATAR_COLORS = [
  "#e85d4c", // red
  "#2a9d8f", // teal
  "#f4a261", // orange
  "#e76f51", // coral
  "#457b9d", // blue
  "#9b5de5", // purple
  "#00bbf9", // sky
  "#f15bb5", // magenta
  "#00f5d4", // mint
  "#fee440", // yellow (dark text needed)
] as const;

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

/** Initial letter(s) for avatar square. */
export function avatarInitials(name: string): string {
  const t = (name || "?").trim();
  if (!t) return "?";
  const parts = t.split(/[\s_-]+/).filter(Boolean);
  if (parts.length >= 2) {
    return (parts[0][0] + parts[1][0]).toUpperCase();
  }
  return t.slice(0, 2).toUpperCase();
}
