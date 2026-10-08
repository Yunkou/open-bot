import { Avatar, Button, Input, Spinner, Typography, useThemeColor } from "heroui-native";
import { useLocalSearchParams, useRouter } from "expo-router";
import type { JSX } from "react";
import { useCallback, useEffect, useRef, useState } from "react";
import {
  KeyboardAvoidingView,
  Platform,
  Pressable,
  ScrollView,
  View,
} from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import * as api from "@/api";
import type { Message } from "@/api/types";

export default function ChatScreen(): JSX.Element {
  const router = useRouter();
  const insets = useSafeAreaInsets();
  const muted = useThemeColor("muted");
  const { id } = useLocalSearchParams<{ id: string }>();
  const conversationId = String(id ?? "");

  const [messages, setMessages] = useState<Message[]>([]);
  const [draft, setDraft] = useState("");
  const [streaming, setStreaming] = useState("");
  const [sending, setSending] = useState(false);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const abortRef = useRef<AbortController | null>(null);
  const scrollRef = useRef<ScrollView>(null);

  const fetchMessages = useCallback(async (): Promise<{ runActive: boolean }> => {
    const res = await api.listMessagesWithStatus(conversationId);
    setMessages(res.messages);
    setLoading(false);
    return { runActive: Boolean(res.run_active) };
  }, [conversationId]);

  const reload = useCallback(async () => {
    const { runActive } = await fetchMessages();
    // 服务端还有 run 在跑（App 被杀掉后重开的情况）→ 重新挂回流
    if (!runActive) return;

    const ctrl = new AbortController();
    void api
      .subscribeConversationEvents(
        conversationId,
        {
          onToken: (t) => setStreaming((prev) => prev + t),
          onDone: () => {
            setStreaming("");
            setSending(false);
            void fetchMessages();
          },
          onError: (m) => setError(m),
        },
        ctrl.signal,
      )
      .catch((err: unknown) => {
        if (!(err instanceof Error && err.name === "AbortError")) {
          setError(err instanceof Error ? err.message : "订阅中断");
        }
      });
  }, [conversationId, fetchMessages]);

  // 切换会话时重新拉取。setState 都在 reload() 的 await 之后发生。
  useEffect(() => {
    if (!conversationId) return;
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setLoading(true);
    reload().catch((err: unknown) =>
      setError(err instanceof Error ? err.message : "加载失败"),
    );
    return () => {
      abortRef.current?.abort();
    };
  }, [conversationId, reload]);

  // 流式输出时贴着底部
  useEffect(() => {
    if (streaming) scrollRef.current?.scrollToEnd({ animated: true });
  }, [streaming]);

  async function send(): Promise<void> {
    const content = draft.trim();
    if (!content || sending || !conversationId) return;

    setDraft("");
    setError(null);
    setSending(true);
    setStreaming("");

    // 先本地插入用户消息，拿到即时的视觉反馈
    const optimistic: Message = {
      id: `local-${Date.now()}`,
      role: "user",
      content,
      created_at: new Date().toISOString(),
    };
    setMessages((prev) => [...prev, optimistic]);
    scrollRef.current?.scrollToEnd({ animated: true });

    const ctrl = new AbortController();
    abortRef.current = ctrl;

    try {
      await api.sendMessageStream(
        conversationId,
        content,
        {
          onToken: (t) => setStreaming((prev) => prev + t),
          onError: (m) => setError(m),
          onDone: () => {
            setStreaming("");
            setSending(false);
            // 以服务端落库结果为准，避免本地拼的消息缺 id / 时间戳
            void reload().catch(() => undefined);
          },
        },
        ctrl.signal,
      );
    } catch (err) {
      if (err instanceof Error && err.name === "AbortError") {
        // 用户主动停止：服务端 run 仍在跑，重新拉一次拿最终结果
        setStreaming("");
        setSending(false);
        void reload().catch(() => undefined);
      } else {
        setError(err instanceof Error ? err.message : "发送失败");
        setSending(false);
      }
    } finally {
      abortRef.current = null;
    }
  }

  async function stop(): Promise<void> {
    abortRef.current?.abort();
    await api.cancelConversationRun(conversationId).catch(() => undefined);
  }

  const visible = streaming
    ? [...messages, { id: "__streaming__", role: "assistant", content: streaming }]
    : messages;

  return (
    <KeyboardAvoidingView
      className="flex-1 bg-background"
      behavior={Platform.OS === "ios" ? "padding" : undefined}
      keyboardVerticalOffset={insets.top}
    >
      {/* 自绘顶部栏：留出状态栏高度 */}
      <View
        className="flex-row items-center gap-3 border-b border-separator px-4 py-3"
        style={{ paddingTop: insets.top + 12 }}
      >
        <Pressable onPress={() => router.back()} hitSlop={12}>
          <Typography.Paragraph className="text-accent">‹ 返回</Typography.Paragraph>
        </Pressable>
      </View>

      <ScrollView
        ref={scrollRef}
        className="flex-1"
        contentContainerClassName="gap-4 px-4 py-4"
        onContentSizeChange={() => scrollRef.current?.scrollToEnd({ animated: false })}
      >
        {loading ? (
          <View className="items-center py-16">
            <Spinner color={muted} />
          </View>
        ) : null}

        {visible.map((m) => (
          <MessageBubble key={m.id} message={m} streaming={m.id === "__streaming__"} />
        ))}

        {error ? (
          <View className="rounded-2xl border border-danger px-4 py-3">
            <Typography.Paragraph className="text-danger">{error}</Typography.Paragraph>
          </View>
        ) : null}
      </ScrollView>

      {/* 输入区 */}
      <View
        className="flex-row items-end gap-2 border-t border-separator px-4 pt-3"
        style={{ paddingBottom: insets.bottom + 12 }}
      >
        <Input
          className="flex-1"
          containerClassName="flex-1"
          value={draft}
          onChangeText={setDraft}
          placeholder="发消息…"
          multiline
          editable={!sending}
          onSubmitEditing={() => void send()}
        />
        {sending ? (
          <Button variant="danger-soft" size="sm" onPress={() => void stop()}>
            <Button.Label>停止</Button.Label>
          </Button>
        ) : (
          <Button size="sm" isDisabled={!draft.trim()} onPress={() => void send()}>
            <Button.Label>发送</Button.Label>
          </Button>
        )}
      </View>
    </KeyboardAvoidingView>
  );
}

function MessageBubble({ message, streaming }: { message: Message; streaming: boolean }): JSX.Element {
  const isUser = message.role === "user";

  return (
    <View className={`flex-row gap-2 ${isUser ? "justify-end" : "justify-start"}`}>
      {!isUser ? (
        <Avatar size="sm">
          <Avatar.Fallback>
            <Typography.Paragraph type="body-xs">AI</Typography.Paragraph>
          </Avatar.Fallback>
        </Avatar>
      ) : null}

      <View
        className={`max-w-[82%] gap-1 rounded-2xl px-4 py-3 ${
          isUser ? "bg-accent" : "bg-surface-secondary"
        }`}
      >
        <Typography.Paragraph
          selectable
          className={isUser ? "text-accent-foreground" : "text-foreground"}
        >
          {message.content}
          {streaming ? " ▌" : ""}
        </Typography.Paragraph>
      </View>
    </View>
  );
}