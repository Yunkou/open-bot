#!/usr/bin/env node
/**
 * Deterministic OpenAI-compatible mock for e2e.
 * Endpoints: GET /healthz, POST /v1/chat/completions (stream + non-stream).
 *
 * Query/body hints:
 *   - messages[*].content containing "[slow]" or env TOKEN_DELAY_MS → slow stream
 *   - content containing "[long]" → long reply (good for stop/interrupt)
 *   - content containing "[short]" → one-line reply
 */
import http from "node:http";

const PORT = Number(process.env.E2E_MOCK_LLM_PORT || process.env.PORT || 18099);
const DEFAULT_DELAY = Number(process.env.E2E_MOCK_LLM_TOKEN_DELAY_MS || 40);

function json(res, status, body) {
  const data = JSON.stringify(body);
  res.writeHead(status, {
    "Content-Type": "application/json",
    "Access-Control-Allow-Origin": "*",
    "Access-Control-Allow-Headers": "*",
    "Access-Control-Allow-Methods": "GET,POST,OPTIONS",
  });
  res.end(data);
}

function cors(res) {
  res.writeHead(204, {
    "Access-Control-Allow-Origin": "*",
    "Access-Control-Allow-Headers": "*",
    "Access-Control-Allow-Methods": "GET,POST,OPTIONS",
  });
  res.end();
}

function lastUserText(body) {
  const msgs = Array.isArray(body?.messages) ? body.messages : [];
  for (let i = msgs.length - 1; i >= 0; i--) {
    const m = msgs[i];
    if (m?.role === "user") {
      if (typeof m.content === "string") return m.content;
      if (Array.isArray(m.content)) {
        return m.content.map((p) => (typeof p === "string" ? p : p?.text || "")).join("");
      }
    }
  }
  return "";
}

function buildReply(userText) {
  const t = userText || "";
  if (t.includes("[short]")) return `E2E_MOCK_OK short: ${t.replace("[short]", "").trim() || "hi"}`;
  if (t.includes("[long]")) {
    const base = "E2E_MOCK_LONG ";
    return base + "α".repeat(120) + ` | echo=${t.slice(0, 80)}`;
  }
  return `E2E_MOCK_OK: ${t.slice(0, 200) || "(empty)"}`;
}

function delayFor(userText) {
  if (userText.includes("[fast]")) return 5;
  if (userText.includes("[slow]") || userText.includes("[long]")) {
    return Math.max(DEFAULT_DELAY, 50);
  }
  return DEFAULT_DELAY;
}

function sleep(ms, signal) {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) {
      reject(new Error("aborted"));
      return;
    }
    const t = setTimeout(resolve, ms);
    signal?.addEventListener("abort", () => {
      clearTimeout(t);
      reject(new Error("aborted"));
    });
  });
}

async function streamCompletion(req, res, body) {
  const userText = lastUserText(body);
  const reply = buildReply(userText);
  const delay = delayFor(userText);
  const ac = new AbortController();
  req.on("close", () => ac.abort());

  res.writeHead(200, {
    "Content-Type": "text/event-stream; charset=utf-8",
    "Cache-Control": "no-cache",
    Connection: "keep-alive",
    "Access-Control-Allow-Origin": "*",
  });

  const id = `chatcmpl-e2e-${Date.now()}`;
  const model = body?.model || "e2e-mock";

  const writeChunk = (delta) => {
    const payload = {
      id,
      object: "chat.completion.chunk",
      created: Math.floor(Date.now() / 1000),
      model,
      choices: [{ index: 0, delta, finish_reason: null }],
    };
    res.write(`data: ${JSON.stringify(payload)}\n\n`);
  };

  writeChunk({ role: "assistant" });
  try {
    for (const ch of reply) {
      await sleep(delay, ac.signal);
      writeChunk({ content: ch });
    }
    const done = {
      id,
      object: "chat.completion.chunk",
      created: Math.floor(Date.now() / 1000),
      model,
      choices: [{ index: 0, delta: {}, finish_reason: "stop" }],
    };
    res.write(`data: ${JSON.stringify(done)}\n\n`);
    res.write("data: [DONE]\n\n");
    res.end();
  } catch {
    try {
      res.end();
    } catch {
      /* ignore */
    }
  }
}

function nonStreamCompletion(res, body) {
  const userText = lastUserText(body);
  const reply = buildReply(userText);
  json(res, 200, {
    id: `chatcmpl-e2e-${Date.now()}`,
    object: "chat.completion",
    created: Math.floor(Date.now() / 1000),
    model: body?.model || "e2e-mock",
    choices: [
      {
        index: 0,
        message: { role: "assistant", content: reply },
        finish_reason: "stop",
      },
    ],
    usage: { prompt_tokens: 8, completion_tokens: reply.length, total_tokens: 8 + reply.length },
  });
}

const server = http.createServer(async (req, res) => {
  const url = new URL(req.url || "/", `http://127.0.0.1:${PORT}`);
  if (req.method === "OPTIONS") {
    cors(res);
    return;
  }
  if (req.method === "GET" && (url.pathname === "/healthz" || url.pathname === "/v1/healthz")) {
    json(res, 200, { ok: true, service: "e2e-mock-llm", port: PORT });
    return;
  }
  if (req.method === "GET" && (url.pathname === "/v1/models" || url.pathname === "/models")) {
    json(res, 200, {
      object: "list",
      data: [{ id: "e2e-mock", object: "model", owned_by: "open-bot-e2e" }],
    });
    return;
  }
  if (req.method === "POST" && (url.pathname === "/v1/chat/completions" || url.pathname === "/chat/completions")) {
    const chunks = [];
    for await (const c of req) chunks.push(c);
    let body = {};
    try {
      body = JSON.parse(Buffer.concat(chunks).toString("utf8") || "{}");
    } catch {
      json(res, 400, { error: { message: "invalid json" } });
      return;
    }
    if (body.stream) {
      await streamCompletion(req, res, body);
    } else {
      nonStreamCompletion(res, body);
    }
    return;
  }
  json(res, 404, { error: { message: `not found: ${url.pathname}` } });
});

server.listen(PORT, "127.0.0.1", () => {
  console.log(`[e2e-mock-llm] listening on http://127.0.0.1:${PORT}/v1`);
});
