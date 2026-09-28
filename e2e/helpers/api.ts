import { apiURL, mockLLMURL } from "./env";

export type AuthSession = {
  token: string;
  user: { id: string; username: string; role?: string; org_id?: string };
};

async function readError(res: Response): Promise<string> {
  try {
    const j = await res.json();
    return j.error || j.message || JSON.stringify(j);
  } catch {
    return res.statusText || String(res.status);
  }
}

export async function apiRegister(username: string, password: string): Promise<AuthSession> {
  const res = await fetch(`${apiURL()}/v1/auth/register`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password }),
  });
  if (!res.ok) throw new Error(`register failed: ${await readError(res)}`);
  return res.json();
}

export async function apiLogin(username: string, password: string): Promise<AuthSession> {
  const res = await fetch(`${apiURL()}/v1/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password }),
  });
  if (!res.ok) throw new Error(`login failed: ${await readError(res)}`);
  return res.json();
}

export async function apiAdminLogin(username: string, password: string): Promise<AuthSession> {
  const res = await fetch(`${apiURL()}/v1/admin/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password }),
  });
  if (!res.ok) throw new Error(`admin login failed: ${await readError(res)}`);
  return res.json();
}

function authHeaders(token: string, extra?: HeadersInit): HeadersInit {
  return { Authorization: `Bearer ${token}`, ...(extra || {}) };
}

export async function apiCreateAgent(
  token: string,
  input: { name: string; description?: string; system_prompt?: string },
): Promise<{ id: string; name: string }> {
  const res = await fetch(`${apiURL()}/v1/agents`, {
    method: "POST",
    headers: authHeaders(token, { "Content-Type": "application/json" }),
    body: JSON.stringify(input),
  });
  if (!res.ok) throw new Error(`create agent failed: ${await readError(res)}`);
  return res.json();
}

export async function apiListAgents(token: string): Promise<{ id: string; name: string }[]> {
  const res = await fetch(`${apiURL()}/v1/agents`, { headers: authHeaders(token) });
  if (!res.ok) throw new Error(`list agents failed: ${await readError(res)}`);
  const data = await res.json();
  return data.agents ?? data ?? [];
}

export async function apiCreateChannel(
  token: string,
  name: string,
  memberIds: string[],
): Promise<{ id: string; name: string }> {
  const res = await fetch(`${apiURL()}/v1/channels`, {
    method: "POST",
    headers: authHeaders(token, { "Content-Type": "application/json" }),
    body: JSON.stringify({ name, member_ids: memberIds }),
  });
  if (!res.ok) throw new Error(`create channel failed: ${await readError(res)}`);
  const data = await res.json();
  return data.channel ?? data;
}

/** Point the user's default LLM at the e2e mock server for deterministic streaming. */
export async function apiEnsureMockLLM(token: string): Promise<void> {
  const base = mockLLMURL().replace(/\/$/, "");
  const listRes = await fetch(`${apiURL()}/v1/llm-connections`, { headers: authHeaders(token) });
  if (!listRes.ok) throw new Error(`list llm failed: ${await readError(listRes)}`);
  const listData = await listRes.json();
  const connections: { id: string; name: string; base_url: string }[] = listData.connections ?? [];
  const existing = connections.find((c) => (c.base_url || "").replace(/\/$/, "") === base);
  if (existing) {
    await fetch(`${apiURL()}/v1/llm-connections/${existing.id}/default`, {
      method: "POST",
      headers: authHeaders(token),
    });
    return;
  }
  const createRes = await fetch(`${apiURL()}/v1/llm-connections`, {
    method: "POST",
    headers: authHeaders(token, { "Content-Type": "application/json" }),
    body: JSON.stringify({
      name: "e2e-mock",
      base_url: base,
      api_key: "e2e-mock-key",
      model: "e2e-mock",
      enable_tools: false,
      is_default: true,
    }),
  });
  if (!createRes.ok) throw new Error(`create mock llm failed: ${await readError(createRes)}`);
}

export async function apiHealth(): Promise<void> {
  const res = await fetch(`${apiURL()}/healthz`);
  if (!res.ok) throw new Error(`API not healthy at ${apiURL()}/healthz`);
}
