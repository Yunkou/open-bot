/**
 * Smoke-check chronological insert used by mergeIncomingMessage.
 * Run: node apps/web/scripts/check-merge-order.mjs
 */
function insertKeepingStreamLast(prev, msg) {
  let streamAt = prev.length;
  while (streamAt > 0 && prev[streamAt - 1].streaming && prev[streamAt - 1].role === "assistant") {
    streamAt -= 1;
  }
  const sealed = prev.slice(0, streamAt);
  const trailing = prev.slice(streamAt);
  const incomingAt = msg.created_at ? Date.parse(msg.created_at) : Number.NaN;
  let insertAt = sealed.length;
  if (!Number.isNaN(incomingAt)) {
    for (let i = 0; i < sealed.length; i++) {
      const raw = sealed[i].created_at;
      const t = raw ? Date.parse(String(raw)) : Number.NaN;
      if (!Number.isNaN(t) && t > incomingAt) {
        insertAt = i;
        break;
      }
    }
  }
  const next = sealed.slice();
  next.splice(insertAt, 0, { ...msg, streaming: false });
  return trailing.length ? next.concat(trailing) : next;
}

function assert(cond, msg) {
  if (!cond) throw new Error(msg);
}

const t0 = "2026-10-09T12:00:00.000Z";
const t1 = "2026-10-09T12:00:01.000Z";
const t2 = "2026-10-09T12:00:02.000Z";

// Mid ack while final still streaming → ack before stream placeholder.
let list = [
  { id: "u1", role: "user", content: "hi", created_at: t0 },
  { id: "local-stream", role: "assistant", content: "", streaming: true },
];
list = insertKeepingStreamLast(list, {
  id: "ack",
  role: "assistant",
  content: "先做一步",
  created_at: t1,
});
assert(list.map((m) => m.id).join(",") === "u1,ack,local-stream", `got ${list.map((m) => m.id)}`);

// Late mid after final sealed → still before final by created_at.
list = [
  { id: "u1", role: "user", content: "hi", created_at: t0 },
  { id: "final", role: "assistant", content: "结论", created_at: t2 },
];
list = insertKeepingStreamLast(list, {
  id: "ack",
  role: "assistant",
  content: "先做一步",
  created_at: t1,
});
assert(list.map((m) => m.id).join(",") === "u1,ack,final", `got ${list.map((m) => m.id)}`);

console.log("check-merge-order: ok");

// Parallel group: never fuzzy-adopt across different agent_ids; preserve agent_id on same-id merge.
function mergeIncomingMessage(prev, msg) {
  if (!msg?.id) return prev;
  if (prev.some((m) => m.id === msg.id)) {
    return prev.map((m) => {
      if (m.id !== msg.id) return m;
      const merged = { ...m, ...msg, streaming: false };
      const keep = (m.agent_id || "").trim();
      const next = (msg.agent_id || "").trim();
      if (keep && !next) merged.agent_id = keep;
      return merged;
    });
  }
  const incomingAgent = (msg.agent_id || "").trim();
  if (msg.role === "assistant" && incomingAgent) {
    for (let i = prev.length - 1; i >= 0; i--) {
      const m = prev[i];
      if (m.role !== "assistant") continue;
      const local = Boolean(m.streaming) || String(m.id).startsWith("local-");
      if (!local) continue;
      if ((m.agent_id || "").trim() === incomingAgent) {
        return prev.map((item, j) => (j === i ? { ...item, ...msg, streaming: false } : item));
      }
    }
  }
  for (let i = prev.length - 1; i >= 0; i--) {
    const m = prev[i];
    if (m.role !== msg.role) continue;
    if (m.streaming && m.role === "assistant" && !(m.content ?? "").trim()) continue;
    const local = Boolean(m.streaming) || String(m.id).startsWith("local-");
    if (!local) break;
    const localAgent = (m.agent_id || "").trim();
    if (incomingAgent && localAgent && localAgent !== incomingAgent) continue;
    if (incomingAgent && localAgent && localAgent === incomingAgent) {
      return prev.map((item, j) => (j === i ? { ...item, ...msg, streaming: false } : item));
    }
    const same =
      m.content === msg.content ||
      (m.content === "" && !m.streaming) ||
      (m.content !== "" && msg.content.startsWith(m.content));
    if (!same) {
      if (incomingAgent && !localAgent) continue;
      break;
    }
    return prev.map((item, j) => (j === i ? { ...item, ...msg, streaming: false } : item));
  }
  return [...prev, { ...msg, streaming: false }];
}

// Same-id merge must keep agent_id when incoming omits it (avatar stamp).
let avatars = [
  { id: "m1", role: "assistant", content: "hi", agent_id: "bot-a", streaming: true },
];
avatars = mergeIncomingMessage(avatars, { id: "m1", role: "assistant", content: "hi there" });
assert(avatars[0].agent_id === "bot-a", `agent_id wiped: ${avatars[0].agent_id}`);

// Must not adopt bot-a bubble for bot-b projection.
avatars = [
  { id: "local-a", role: "assistant", content: "partial", agent_id: "bot-a", streaming: true },
  { id: "local-b", role: "assistant", content: "", agent_id: "bot-b", streaming: true },
];
avatars = mergeIncomingMessage(avatars, {
  id: "srv-b",
  role: "assistant",
  content: "from b",
  agent_id: "bot-b",
});
assert(avatars.some((m) => m.id === "local-a" && m.agent_id === "bot-a"), "bot-a bubble lost");
assert(avatars.some((m) => m.agent_id === "bot-b" && m.content === "from b"), "bot-b not adopted");
assert(!avatars.some((m) => m.id === "local-a" && m.agent_id === "bot-b"), "avatar swap onto bot-a");

console.log("check-merge-order: agent_id stamp ok");
