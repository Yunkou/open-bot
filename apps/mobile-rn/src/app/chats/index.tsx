import { Avatar, Button, Card, Input, Label, Spinner, TextField, Typography, useThemeColor } from "heroui-native";
import { useRouter } from "expo-router";
import type { JSX } from "react";
import { useCallback, useEffect, useState } from "react";
import { FlatList, Modal, Pressable, RefreshControl, View } from "react-native";
import { useSafeAreaInsets } from "react-native-safe-area-context";

import * as api from "@/api";
import { API_BASE } from "@/api/config";
import type { Agent } from "@/api/types";
import { useSession } from "@/providers/session";

const AVATAR_COLORS = ["accent", "success", "warning", "danger"] as const;

function initials(name: string): string {
  const trimmed = name.trim();
  if (!trimmed) return "?";
  // 中文取首字，英文取首字母
  return /[a-zA-Z]/.test(trimmed[0]) ? trimmed.slice(0, 2).toUpperCase() : trimmed.slice(0, 1);
}

export default function ChatsScreen(): JSX.Element {
  const router = useRouter();
  const insets = useSafeAreaInsets();
  const muted = useThemeColor("muted");
  const { user, signOut } = useSession();

  const [agents, setAgents] = useState<Agent[]>([]);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [openingId, setOpeningId] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      setError(null);
      setAgents(await api.listAgents());
    } catch (err) {
      setError(err instanceof Error ? err.message : "加载失败");
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  }, []);

  // 挂载即拉取：标准的「effect 订阅外部数据源」用法，
  // setState 全部发生在 load() 的 await 之后，不在 effect body 里同步触发。
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void load();
  }, [load]);

  async function openAgent(agent: Agent): Promise<void> {
    if (openingId) return;
    setOpeningId(agent.id);
    try {
      const conversation = await api.openPrimaryConversation(agent.id);
      router.push(`/chats/${conversation.id}`);
    } catch (err) {
      setError(err instanceof Error ? err.message : "打开会话失败");
    } finally {
      setOpeningId(null);
    }
  }

  return (
    <View className="flex-1 bg-background" style={{ paddingTop: insets.top }}>
      <FlatList
        data={agents}
        keyExtractor={(item) => item.id}
        contentContainerClassName="gap-3 px-4 pb-10"
        refreshControl={
          <RefreshControl
            refreshing={refreshing}
            onRefresh={() => {
              setRefreshing(true);
              void load();
            }}
          />
        }
        ListHeaderComponent={
          <View className="gap-4 py-4">
            <View className="flex-row items-center justify-between">
              <View className="gap-1">
                <Typography.Heading type="h2">助手</Typography.Heading>
                <Typography.Paragraph color="muted">
                  {user ? `${user.username} · ${API_BASE}` : ""}
                </Typography.Paragraph>
              </View>
              <View className="flex-row gap-2">
                <NewAgentButton onCreated={load} />
                <Button variant="ghost" size="sm" onPress={() => void signOut()}>
                  <Button.Label>退出</Button.Label>
                </Button>
              </View>
            </View>
            {error ? (
              <Card className="border-danger">
                <Card.Body>
                  <Typography.Paragraph className="text-danger">{error}</Typography.Paragraph>
                </Card.Body>
              </Card>
            ) : null}
          </View>
        }
        ListEmptyComponent={
          loading ? (
            <View className="items-center py-16">
              <Spinner color={muted} />
            </View>
          ) : (
            <View className="items-center gap-2 py-16">
              <Typography.Paragraph color="muted">
                还没有助手，点右上角「+」创建一个
              </Typography.Paragraph>
            </View>
          )
        }
        renderItem={({ item, index }) => (
          <Pressable onPress={() => void openAgent(item)} disabled={openingId !== null}>
            <Card>
              <Card.Body className="flex-row items-center gap-3">
                <Avatar color={AVATAR_COLORS[index % AVATAR_COLORS.length]}>
                  <Avatar.Fallback>
                    <Typography.Paragraph weight="medium">
                      {initials(item.name)}
                    </Typography.Paragraph>
                  </Avatar.Fallback>
                </Avatar>
                <View className="flex-1 gap-0.5">
                  <Typography.Paragraph weight="medium">{item.name}</Typography.Paragraph>
                  <Typography.Paragraph truncate type="body-sm" color="muted">
                    {item.last_message || item.description || "还没有对话"}
                  </Typography.Paragraph>
                </View>
                {openingId === item.id ? <Spinner size="sm" color={muted} /> : null}
              </Card.Body>
            </Card>
          </Pressable>
        )}
      />
    </View>
  );
}

function NewAgentButton({ onCreated }: { onCreated: () => Promise<void> }): JSX.Element {
  const muted = useThemeColor("muted");
  const [open, setOpen] = useState(false);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [systemPrompt, setSystemPrompt] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function submit(): Promise<void> {
    if (!name.trim()) {
      setError("请填写名称");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      await api.createAgent({
        name: name.trim(),
        description: description.trim(),
        system_prompt: systemPrompt.trim() || "你是一个乐于助人的 AI 助手，默认使用中文回答。",
      });
      setOpen(false);
      setName("");
      setDescription("");
      setSystemPrompt("");
      await onCreated();
    } catch (err) {
      setError(err instanceof Error ? err.message : "创建失败");
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <Button size="sm" onPress={() => setOpen(true)}>
        <Button.Label>＋</Button.Label>
      </Button>

      <Modal visible={open} animationType="slide" transparent onRequestClose={() => setOpen(false)}>
        <Pressable className="flex-1 justify-end bg-black/40" onPress={() => setOpen(false)}>
          <Pressable
            className="gap-4 rounded-t-3xl bg-background p-5"
            style={{ paddingBottom: insetsSafeBottom() }}
            onPress={(e) => e.stopPropagation()}
          >
            <Typography.Heading type="h3">新建助手</Typography.Heading>

            <TextField isInvalid={Boolean(error)}>
              <Label>
                <Label.Text>名称</Label.Text>
              </Label>
              <Input value={name} onChangeText={setName} placeholder="写作助手" />
            </TextField>

            <TextField>
              <Label>
                <Label.Text>简介</Label.Text>
              </Label>
              <Input value={description} onChangeText={setDescription} placeholder="擅长润色与结构化" />
            </TextField>

            <TextField>
              <Label>
                <Label.Text>系统提示词</Label.Text>
              </Label>
              <Input
                value={systemPrompt}
                onChangeText={setSystemPrompt}
                placeholder="你是写作助手，默认中文回答。"
                multiline
              />
            </TextField>

            {error ? (
              <Typography.Paragraph className="text-danger">{error}</Typography.Paragraph>
            ) : null}

            <View className="flex-row gap-3">
              <Button variant="secondary" className="flex-1" onPress={() => setOpen(false)}>
                <Button.Label>取消</Button.Label>
              </Button>
              <Button className="flex-1" isDisabled={busy} onPress={() => void submit()}>
                {busy ? <Spinner size="sm" color={muted} /> : null}
                <Button.Label>创建</Button.Label>
              </Button>
            </View>
          </Pressable>
        </Pressable>
      </Modal>
    </>
  );
}

function insetsSafeBottom(): number {
  // Modal 在 root layout 之外，拿不到 useSafeAreaInsets，这里退化为固定间距
  return 28;
}