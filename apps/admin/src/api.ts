const API_BASE = (import.meta.env.VITE_API_BASE || "http://127.0.0.1:18080").replace(
  /\/$/,
  "",
);

export const WEB_URL = (import.meta.env.VITE_WEB_URL || "http://127.0.0.1:5173").replace(
  /\/$/,
  "",
);

const TOKEN_KEY = "openbot_admin_token";
const USER_KEY = "openbot_admin_user";

export type User = {
  id: string;
  username: string;
  email?: string;
  org_id?: string;
  role?: "platform_admin" | "org_admin" | "member" | string;
  created_at?: string;
};

export type AdminMember = {
  id: string;
  username: string;
  email?: string;
  role: string;
  org_id?: string;
  created_at?: string;
};

export type AdminInvite = {
  id: string;
  org_id: string;
  username_or_email: string;
  role: string;
  status: string;
  created_at?: string;
};

export type OrgLLMSettings = {
  org_id: string;
  llm_name: string;
  llm_base_url: string;
  llm_model: string;
  llm_enable_tools: boolean;
  llm_context_window?: number | null;
  api_key_set: boolean;
  api_key_hint?: string;
  feature_flags_json?: string;
  updated_at?: string;
};

export type OrgUsage = {
  org_id: string;
  member_count: number;
  conversation_count: number;
  message_count: number;
  agent_count: number;
};

export type AuditLog = {
  id: string;
  org_id: string;
  actor_user_id: string;
  actor_username?: string;
  action: string;
  target_type: string;
  target_id: string;
  meta_json: string;
  created_at: string;
};

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

export function getStoredUser(): User | null {
  const raw = localStorage.getItem(USER_KEY);
  if (!raw) return null;
  try {
    return JSON.parse(raw) as User;
  } catch {
    return null;
  }
}

export function setSession(token: string, user: User) {
  localStorage.setItem(TOKEN_KEY, token);
  localStorage.setItem(USER_KEY, JSON.stringify(user));
}

export function clearSession() {
  localStorage.removeItem(TOKEN_KEY);
  localStorage.removeItem(USER_KEY);
}

function authHeaders(extra?: HeadersInit): HeadersInit {
  const token = getToken();
  return {
    ...(extra || {}),
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
  };
}

async function readError(res: Response): Promise<string> {
  const text = await res.text().catch(() => "");
  try {
    const j = JSON.parse(text) as { error?: string; message?: string };
    return j.error || j.message || text || `HTTP ${res.status}`;
  } catch {
    return text || `HTTP ${res.status}`;
  }
}

export async function login(username: string, password: string): Promise<{ token: string; user: User }> {
  const res = await fetch(`${API_BASE}/v1/admin/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password }),
  });
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}

export async function fetchMe(): Promise<User> {
  const res = await fetch(`${API_BASE}/v1/me`, { headers: authHeaders() });
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}

export async function fetchOIDCConfig(): Promise<{
  enabled: boolean;
  endpoint?: string;
  client_id?: string;
  redirect_uri?: string;
}> {
  const res = await fetch(`${API_BASE}/v1/auth/oidc/config`);
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}

export async function startOIDCLogin(): Promise<{ authorize_url: string; state: string }> {
  const res = await fetch(`${API_BASE}/v1/auth/oidc/start?redirect=0`, {
    headers: { Accept: "application/json" },
    credentials: "include",
  });
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}

export async function exchangeOIDCCode(
  code: string,
  state: string,
): Promise<{ token: string; user: User }> {
  const res = await fetch(`${API_BASE}/v1/admin/auth/oidc/exchange`, {
    method: "POST",
    headers: { "Content-Type": "application/json", "X-Client": "admin" },
    credentials: "include",
    body: JSON.stringify({ code, state }),
  });
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}

export async function adminGetOrg(): Promise<{ org: { id: string; slug: string; name: string }; me?: User }> {
  const res = await fetch(`${API_BASE}/v1/admin/org`, { headers: authHeaders() });
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}

export async function adminListMembers(): Promise<{ members: AdminMember[]; invites: AdminInvite[] }> {
  const res = await fetch(`${API_BASE}/v1/admin/members`, { headers: authHeaders() });
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}

export async function adminInviteMember(
  username: string,
  role: string,
): Promise<{ joined: boolean; user?: User; invite?: AdminInvite; role?: string }> {
  const res = await fetch(`${API_BASE}/v1/admin/members/invite`, {
    method: "POST",
    headers: authHeaders({ "Content-Type": "application/json" }),
    body: JSON.stringify({ username, role }),
  });
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}

export async function adminPatchMember(id: string, role: string): Promise<{ user: User }> {
  const res = await fetch(`${API_BASE}/v1/admin/members/${id}`, {
    method: "PATCH",
    headers: authHeaders({ "Content-Type": "application/json" }),
    body: JSON.stringify({ role }),
  });
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}

export async function adminGetOrgLLM(): Promise<OrgLLMSettings> {
  const res = await fetch(`${API_BASE}/v1/admin/org/llm`, { headers: authHeaders() });
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}

export async function adminPutOrgLLM(body: {
  name: string;
  base_url: string;
  api_key?: string;
  model: string;
  enable_tools: boolean;
  context_window?: number | null;
}): Promise<OrgLLMSettings> {
  const res = await fetch(`${API_BASE}/v1/admin/org/llm`, {
    method: "PUT",
    headers: authHeaders({ "Content-Type": "application/json" }),
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}

export async function adminGetUsage(): Promise<OrgUsage> {
  const res = await fetch(`${API_BASE}/v1/admin/usage`, { headers: authHeaders() });
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}

export async function adminGetFeatureFlags(): Promise<{ feature_flags_json: string }> {
  const res = await fetch(`${API_BASE}/v1/admin/feature-flags`, { headers: authHeaders() });
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}

export async function adminPutFeatureFlags(feature_flags_json: string): Promise<{ feature_flags_json: string }> {
  const res = await fetch(`${API_BASE}/v1/admin/feature-flags`, {
    method: "PUT",
    headers: authHeaders({ "Content-Type": "application/json" }),
    body: JSON.stringify({ feature_flags_json }),
  });
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}

export async function adminListAuditLogs(limit = 100): Promise<{ logs: AuditLog[] }> {
  const res = await fetch(`${API_BASE}/v1/admin/audit-logs?limit=${limit}`, {
    headers: authHeaders(),
  });
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}


export type AdminUser = {
  id: string;
  username: string;
  email?: string;
  role: string;
  org_id?: string;
  created_at?: string;
};

export type AdminBot = {
  id: string;
  user_id: string;
  owner_username?: string;
  name: string;
  description?: string;
  system_prompt?: string;
  is_builtin?: boolean;
  computer_mode?: string;
  created_at?: string;
  updated_at?: string;
};

export type AdminTrace = {
  id: string;
  name?: string;
  userId?: string;
  sessionId?: string;
  timestamp?: string;
  latency?: number | null;
  observationId?: string;
  type?: string;
  level?: string;
  projectId?: string;
  langfuse_url?: string;
};

export async function adminListUsers(): Promise<{ users: AdminUser[] }> {
  const res = await fetch(`${API_BASE}/v1/admin/users`, { headers: authHeaders() });
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}

export async function adminCreateUser(body: {
  username: string;
  password: string;
  email?: string;
  role?: string;
}): Promise<{ user: User }> {
  const res = await fetch(`${API_BASE}/v1/admin/users`, {
    method: "POST",
    headers: authHeaders({ "Content-Type": "application/json" }),
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}

export async function adminPatchUser(
  id: string,
  body: { email?: string; role?: string; password?: string },
): Promise<{ user: User }> {
  const res = await fetch(`${API_BASE}/v1/admin/users/${id}`, {
    method: "PATCH",
    headers: authHeaders({ "Content-Type": "application/json" }),
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}

export async function adminDeleteUser(id: string): Promise<void> {
  const res = await fetch(`${API_BASE}/v1/admin/users/${id}`, {
    method: "DELETE",
    headers: authHeaders(),
  });
  if (!res.ok) throw new Error(await readError(res));
}

export async function adminListBots(): Promise<{ bots: AdminBot[] }> {
  const res = await fetch(`${API_BASE}/v1/admin/bots`, { headers: authHeaders() });
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}

export async function adminCreateBot(body: {
  user_id: string;
  name: string;
  description?: string;
  system_prompt?: string;
  computer_mode?: string;
}): Promise<{ bot: AdminBot }> {
  const res = await fetch(`${API_BASE}/v1/admin/bots`, {
    method: "POST",
    headers: authHeaders({ "Content-Type": "application/json" }),
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}

export async function adminPatchBot(
  id: string,
  body: {
    name?: string;
    description?: string;
    system_prompt?: string;
    computer_mode?: string;
  },
): Promise<{ bot: AdminBot }> {
  const res = await fetch(`${API_BASE}/v1/admin/bots/${id}`, {
    method: "PATCH",
    headers: authHeaders({ "Content-Type": "application/json" }),
    body: JSON.stringify(body),
  });
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}

export async function adminDeleteBot(id: string): Promise<void> {
  const res = await fetch(`${API_BASE}/v1/admin/bots/${id}`, {
    method: "DELETE",
    headers: authHeaders(),
  });
  if (!res.ok) throw new Error(await readError(res));
}

export async function adminTracesStatus(): Promise<{
  enabled: boolean;
  ready?: boolean;
  public_ui_url?: string;
  base_url?: string;
  project_id?: string;
  reason?: string;
}> {
  const res = await fetch(`${API_BASE}/v1/admin/traces/status`, { headers: authHeaders() });
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}

export async function adminListTraces(params?: {
  limit?: number;
  page?: number;
  cursor?: string;
}): Promise<{
  enabled: boolean;
  reason?: string;
  traces: AdminTrace[];
  public_ui_url?: string;
  project_id?: string;
  meta?: { cursor?: string };
}> {
  const q = new URLSearchParams();
  if (params?.limit) q.set("limit", String(params.limit));
  if (params?.page) q.set("page", String(params.page));
  if (params?.cursor) q.set("cursor", params.cursor);
  const qs = q.toString();
  const res = await fetch(`${API_BASE}/v1/admin/traces${qs ? `?${qs}` : ""}`, {
    headers: authHeaders(),
  });
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}

export async function adminGetTrace(id: string): Promise<{
  enabled: boolean;
  reason?: string;
  trace: AdminTrace | null;
  observations?: unknown[];
  public_ui_url?: string;
  project_id?: string;
}> {
  const res = await fetch(`${API_BASE}/v1/admin/traces/${encodeURIComponent(id)}`, {
    headers: authHeaders(),
  });
  if (!res.ok) throw new Error(await readError(res));
  return res.json();
}
