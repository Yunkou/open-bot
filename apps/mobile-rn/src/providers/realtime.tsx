import type { JSX, ReactNode } from "react";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { AppState, type AppStateStatus } from "react-native";

import * as api from "@/api";
import type { BotOnlineEvent, BotPresenceEvent, Message, ReactionUpdatedEvent } from "@/api/types";
import { useSession } from "@/providers/session";

/**
 * 全局实时通道（`GET /v1/events/ws`）。
 *
 * 单会话 SSE 只覆盖「我正在看的那个会话」，而 Web 端这条全局 WS 负责另外几件事：
 * - `bot_online` / `bot_presence`：助手在线绿点与五态表情，列表页要实时跟着变
 * - `reaction_updated`：另一台设备给消息点了表情，当前页面同步
 * - `conversation_message` / `task_status`：群聊里别的助手在回消息
 * - `host_activity`：本机正在执行操作时的顶部提示
 *
 * App 切到后台再回来必须重连：手机系统的后台 socket 回收不可靠，
 * 这里靠 AppState 变化显式重连 + 重连后由各页面自己补拉一次列表
 * （与 Web 端 `visibilitychange` 拉取补偿同一思路）。
 */

export type ChatServerEvent =
  | BotOnlineEvent
  | BotPresenceEvent
  | ReactionUpdatedEvent
  | { type: "conversation_message"; message: Message }
  | {
      type: "task_status";
      conversation_id: string;
      agent_id?: string;
      channel_id?: string;
      status: string;
      label?: string;
    }
  | { type: "host_activity"; active: boolean; machine_id?: string; label?: string }
  | { type: string; [key: string]: unknown };

type RealtimeState = {
  connected: boolean;
  /** 助手在线状态，key 为 agent_id */
  online: Record<string, boolean>;
  /** 助手 presence，key 为 agent_id */
  presence: Record<string, string>;
  /** 本机正在执行操作时的提示文案；null = 无 */
  hostActivity: string | null;
  /** 订阅事件；返回退订函数 */
  subscribe: (fn: (evt: ChatServerEvent) => void) => () => void;
};

const RealtimeContext = createContext<RealtimeState | null>(null);

const RECONNECT_MS = 3000;

/** 登出后展示用的空表。模块级常量，保证引用稳定，不会让下游 memo 失效。 */
const EMPTY_MAP_BOOL: Record<string, boolean> = {};
const EMPTY_MAP_STR: Record<string, string> = {};

export function RealtimeProvider({ children }: { children: ReactNode }): JSX.Element {
  const { user } = useSession();
  const userId = user?.id ?? null;

  /**
   * 状态连同它属于哪个用户一起存。
   *
   * 退出登录时不必在 effect 里同步清空 —— 读的时候比对 `userId` 即可：
   * 上一位用户的在线状态天然不可见，下一位登录也不会看到串号数据。
   * 这比「登出就 setState({})」少一次级联渲染，也不用写 eslint-disable。
   */
  const [state, setState] = useState<{
    userId: string | null;
    online: Record<string, boolean>;
    presence: Record<string, string>;
    hostActivity: string | null;
  }>({ userId: null, online: {}, presence: {}, hostActivity: null });

  const [connected, setConnected] = useState(false);

  const online = state.userId === userId ? state.online : EMPTY_MAP_BOOL;
  const presence = state.userId === userId ? state.presence : EMPTY_MAP_STR;
  const hostActivity = state.userId === userId ? state.hostActivity : null;

  const listenersRef = useRef(new Set<(evt: ChatServerEvent) => void>());
  const socketRef = useRef<WebSocket | null>(null);
  const retryRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const generationRef = useRef(0);

  const emit = useCallback((evt: ChatServerEvent) => {
    for (const fn of listenersRef.current) {
      try {
        fn(evt);
      } catch {
        /* 单个订阅者出错不该拖垮整条通道 */
      }
    }
  }, []);

  const subscribe = useCallback((fn: (evt: ChatServerEvent) => void) => {
    listenersRef.current.add(fn);
    return () => {
      listenersRef.current.delete(fn);
    };
  }, []);

  useEffect(() => {
    if (!userId) {
      socketRef.current?.close();
      socketRef.current = null;
      return;
    }
    const scope = userId;

    let disposed = false;
    const generation = ++generationRef.current;

    const clearRetry = (): void => {
      if (retryRef.current) {
        clearTimeout(retryRef.current);
        retryRef.current = null;
      }
    };

    const connect = async (): Promise<void> => {
      if (disposed || generation !== generationRef.current) return;
      let url: string;
      try {
        url = await api.chatEventsWebSocketUrl();
      } catch {
        scheduleRetry();
        return;
      }
      if (disposed || generation !== generationRef.current) return;

      let ws: WebSocket;
      try {
        ws = new WebSocket(url);
      } catch {
        scheduleRetry();
        return;
      }
      socketRef.current = ws;

      ws.onopen = () => {
        if (disposed || generation !== generationRef.current) return;
        clearRetry();
        setConnected(true);
      };

      ws.onmessage = (ev: WebSocketMessageEvent) => {
        if (disposed || generation !== generationRef.current) return;
        let evt: ChatServerEvent;
        try {
          evt = JSON.parse(String(ev.data)) as ChatServerEvent;
        } catch {
          return;
        }
        switch (evt.type) {
          case "bot_online": {
            const e = evt as BotOnlineEvent;
            setState((prev) => ({
              ...prev,
              userId: scope,
              online: { ...(prev.userId === scope ? prev.online : {}), [e.agent_id]: e.online },
            }));
            break;
          }
          case "bot_presence": {
            const e = evt as BotPresenceEvent;
            setState((prev) => ({
              ...prev,
              userId: scope,
              presence: {
                ...(prev.userId === scope ? prev.presence : {}),
                [e.agent_id]: e.status,
              },
            }));
            break;
          }
          case "host_activity": {
            const e = evt as { active: boolean; label?: string };
            setState((prev) => ({
              ...prev,
              userId: scope,
              hostActivity: e.active ? (e.label ?? "正在操作中") : null,
            }));
            break;
          }
          default:
            break;
        }
        emit(evt);
      };

      ws.onerror = () => {
        // onclose 紧随其后，重连逻辑统一放在那里
      };

      ws.onclose = () => {
        if (disposed || generation !== generationRef.current) return;
        setConnected(false);
        scheduleRetry();
      };
    };

    const scheduleRetry = (): void => {
      clearRetry();
      retryRef.current = setTimeout(() => {
        retryRef.current = null;
        void connect();
      }, RECONNECT_MS);
    };

    void connect();

    // 后台 socket 常被系统回收，回前台显式重连一次
    const onAppState = (state: AppStateStatus): void => {
      if (state !== "active") return;
      if (socketRef.current?.readyState === WebSocket.OPEN) return;
      clearRetry();
      void connect();
    };
    const sub = AppState.addEventListener("change", onAppState);

    return () => {
      disposed = true;
      sub.remove();
      clearRetry();
      socketRef.current?.close();
      socketRef.current = null;
    };
  }, [userId, emit]);

  const value = useMemo<RealtimeState>(
    () => ({ connected, online, presence, hostActivity, subscribe }),
    [connected, online, presence, hostActivity, subscribe]
  );

  return <RealtimeContext.Provider value={value}>{children}</RealtimeContext.Provider>;
}

export function useRealtime(): RealtimeState {
  const ctx = useContext(RealtimeContext);
  if (!ctx) throw new Error("useRealtime 必须在 RealtimeProvider 内使用");
  return ctx;
}
