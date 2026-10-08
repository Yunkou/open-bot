import { API_BASE } from "./config";
import { ApiError, expoFetch, request } from "./http";
import { readSSEStream } from "./sse";
import { getToken } from "./session";
import type {
  Agent,
  AgentInput,
  Conversation,
  ListMessagesResult,
  Message,
  StreamHandlers,
  User,
} from "./types";

export class AuthError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "AuthError";
  }
}

/* ------------------------------------------------------------------ 鉴权 */

export async function login(
  username: string,
  password: string,
): Promise<{ token: string; user: User }> {
  return request("/v1/auth/login", {
    method: "POST",
    auth: false,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password }),
  });
}

export async function register(
  username: string,
  password: string,
): Promise<{ token: string; user: User }> {
  return request("/v1/auth/register", {
    method: "POST",
    auth: false,
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password }),
  });
}

export async function fetchMe(): Promise<User> {
  return request("/v1/me");
}

/* ------------------------------------------------------------------ 助手 */

export async function listAgents(): Promise<Agent[]> {
  const data = await request<{ agents?: Agent[] }>("/v1/agents");
  return data.agents ?? [];
}

export async function createAgent(body: AgentInput): Promise<Agent> {
  return request("/v1/agents", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

/** Grok 式主会话：每个助手一条线程，取不到就建。 */
export async function openPrimaryConversation(agentId: string): Promise<Conversation> {
  const data = await request<{ conversation: Conversation }>(
    `/v1/agents/${encodeURIComponent(agentId)}/conversation`,
  );
  return data.conversation;
}

/* ------------------------------------------------------------------ 会话 */

export async function listConversations(): Promise<Conversation[]> {
  const data = await request<{ conversations?: Conversation[] }>("/v1/conversations");
  return data.conversations ?? [];
}

export async function deleteConversation(id: string): Promise<void> {
  await request(`/v1/conversations/${id}`, { method: "DELETE" });
}

export async function listMessagesWithStatus(conversationId: string): Promise<ListMessagesResult> {
  const data = await request<{ messages?: Message[]; run_active?: boolean }>(
    `/v1/conversations/${conversationId}/messages`,
  );
  return { messages: data.messages ?? [], run_active: Boolean(data.run_active) };
}

export async function getConversationRunStatus(
  conversationId: string,
): Promise<{ active: boolean }> {
  const data = await request<{ active?: boolean }>(`/v1/conversations/${conversationId}/run`);
  return { active: Boolean(data.active) };
}

export async function cancelConversationRun(conversationId: string): Promise<void> {
  // 幂等：无活跃 run 时后端也返回 204，旧版本服务器可能 404
  try {
    await request(`/v1/conversations/${conversationId}/cancel`, { method: "POST" });
  } catch (err) {
    if (err instanceof ApiError && (err.status === 204 || err.status === 404)) return;
    throw err;
  }
}

/* ------------------------------------------------------------------ 发消息（SSE） */

/**
 * 发消息并流式接收。`signal` 用来中止（对应 Web 的 AbortSignal）。
 *
 * 走的是 `POST` + `text/event-stream`，不是原生 EventSource —— 原生 EventSource 不支持
 * 自定义 Authorization header，也不支持请求体。
 */
export async function sendMessageStream(
  conversationId: string,
  content: string,
  handlers: StreamHandlers,
  signal?: AbortSignal,
): Promise<void> {
  const token = await getToken();
  const res = await expoFetch(`${API_BASE}/v1/conversations/${conversationId}/messages`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      Accept: "text/event-stream",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: JSON.stringify({ content }),
    signal,
  });

  if (!res.ok || !res.body) {
    throw new ApiError(res.status, `发送失败（HTTP ${res.status}）`);
  }
  await readSSEStream(res.body, handlers);
}

/** 刷新 / 重连后重新挂回服务端仍在跑的 run；中止信号不会取消服务端任务。 */
export async function subscribeConversationEvents(
  conversationId: string,
  handlers: StreamHandlers,
  signal?: AbortSignal,
): Promise<void> {
  const token = await getToken();
  const res = await expoFetch(`${API_BASE}/v1/conversations/${conversationId}/events`, {
    headers: {
      Accept: "text/event-stream",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    signal,
  });

  if (!res.ok || !res.body) {
    throw new ApiError(res.status, `订阅失败（HTTP ${res.status}）`);
  }
  await readSSEStream(res.body, handlers);
}