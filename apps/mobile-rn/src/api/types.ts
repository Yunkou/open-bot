/**
 * 与 `apps/web/src/api.ts` 对齐的契约类型。
 * 后端是 Go 的 `/v1/*`，这里是移动端的独立副本（不跨包共享，避免 Metro 解析 pnpm workspace 外的 TS 源码）。
 */

export type User = {
  id: string;
  username: string;
  email?: string;
  org_id?: string;
  role?: "platform_admin" | "org_admin" | "member" | string;
  created_at?: string;
};

export type Agent = {
  id: string;
  name: string;
  description: string;
  system_prompt?: string;
  is_builtin?: boolean;
  computer_mode?: "team" | "private" | string;
  user_id?: string;
  created_at?: string;
  updated_at?: string;
  /** 助手级主会话 id */
  conversation_id?: string;
  /** 侧边栏展示的最后一条消息摘要 */
  last_message?: string;
  conversation_updated_at?: string;
};

export type AgentInput = {
  name: string;
  description: string;
  system_prompt: string;
};

export type Message = {
  id: string;
  role: "user" | "assistant" | string;
  content: string;
  agent_id?: string;
  created_at?: string;
};

export type Conversation = {
  id: string;
  agent_id: string;
  title: string;
  channel_id?: string;
  created_at: string;
  updated_at?: string;
  messages?: Message[];
};

export type ListMessagesResult = {
  messages: Message[];
  run_active?: boolean;
};

export type StatusEvent = {
  phase?: string;
  label?: string;
  tool?: string;
  [key: string]: unknown;
};

export type StreamHandlers = {
  onToken: (text: string) => void;
  onMeta?: (data: Record<string, unknown>) => void;
  onStatus?: (data: StatusEvent) => void;
  onError?: (message: string) => void;
  onDone?: () => void;
  /** 多助手群聊：新 bot 开始流式输出 */
  onAgentStart?: (info: {
    agent_id: string;
    agent_name?: string;
    index?: number;
    total?: number;
  }) => void;
};