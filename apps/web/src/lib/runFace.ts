/**
 * In-chat RunStatus / robot-heads face attribution.
 *
 * Group AutoPick (especially decision/SystemOne) can take many seconds before
 * any agent_start. During that window faceAgentId is empty — falling back to
 * list-focus activeAgent paints a random member's icon as if they were already
 * answering. Keep a neutral face until a real answering bot is known.
 */

/** Who owns the RunStatus AgentAvatar id, or null for a neutral/generic face. */
export function resolveRunFaceAgentId(
  faceAgentId: string | undefined | null,
  opts: { isGroup: boolean; activeAgentId?: string | null },
): string | null {
  const face = (faceAgentId || "").trim();
  if (face) return face;
  if (opts.isGroup) return null;
  const active = (opts.activeAgentId || "").trim();
  return active || null;
}

/**
 * Group decision / AutoPick window: run is live but no answering bot yet.
 * Drive a generic searching animation (not a member bot's thinking face).
 */
export function isGroupDecisionPending(opts: {
  isGroup: boolean;
  faceAgentId?: string | null;
  runLive: boolean;
}): boolean {
  return Boolean(opts.isGroup && !(opts.faceAgentId || "").trim() && opts.runLive);
}
