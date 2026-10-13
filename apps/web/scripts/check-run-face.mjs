/**
 * Smoke-check RunStatus face attribution during group decision vs DM.
 * Run: node apps/web/scripts/check-run-face.mjs
 *
 * Mirrors apps/web/src/lib/runFace.ts (keep in sync).
 */
function resolveRunFaceAgentId(faceAgentId, opts) {
  const face = (faceAgentId || "").trim();
  if (face) return face;
  if (opts.isGroup) return null;
  const active = (opts.activeAgentId || "").trim();
  return active || null;
}

function isGroupDecisionPending(opts) {
  return Boolean(opts.isGroup && !(opts.faceAgentId || "").trim() && opts.runLive);
}

function assert(cond, msg) {
  if (!cond) throw new Error(msg);
}

// Group decision window: no face yet → neutral (not list-focus).
assert(
  resolveRunFaceAgentId("", { isGroup: true, activeAgentId: "lead-or-random" }) === null,
  "group pre-start must not use activeAgent",
);
assert(
  isGroupDecisionPending({ isGroup: true, faceAgentId: "", runLive: true }) === true,
  "group live + empty face = decision pending",
);

// After agent_start: use answering bot.
assert(
  resolveRunFaceAgentId("bot-coder", { isGroup: true, activeAgentId: "lead" }) === "bot-coder",
  "group post-start uses faceAgentId",
);
assert(
  isGroupDecisionPending({ isGroup: true, faceAgentId: "bot-coder", runLive: true }) === false,
  "face set clears decision pending",
);

// DM: fall back to focus bot.
assert(
  resolveRunFaceAgentId("", { isGroup: false, activeAgentId: "dm-bot" }) === "dm-bot",
  "DM falls back to activeAgent",
);
assert(
  isGroupDecisionPending({ isGroup: false, faceAgentId: "", runLive: true }) === false,
  "DM is never group decision pending",
);

// Idle group: not pending.
assert(
  isGroupDecisionPending({ isGroup: true, faceAgentId: "", runLive: false }) === false,
  "idle group is not decision pending",
);

console.log("check-run-face: ok");
