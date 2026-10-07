import { useEffect, useMemo, useRef, useState } from "react";
import {
  Alert,
  Descriptions,
  Drawer,
  Input,
  Modal,
  Segmented,
  Select,
  Space,
  Table,
  Tabs,
  Tag,
  Typography,
  message,
  type TableColumnsType,
} from "antd";
import { PageContainer, ProTable, type ActionType, type ProColumns } from "@ant-design/pro-components";
import {
  adminGetCompactConfig,
  adminGetMemoryRecall,
  adminListAutoMemories,
  adminListBots,
  adminListChannels,
  adminListCompactions,
  adminListMemories,
  adminListMemoryRecalls,
  adminListUsers,
  type AdminAutoMemory,
  type AdminBot,
  type AdminChannel,
  type AdminCompaction,
  type AdminMemory,
  type AdminMemoryRecall,
  type AdminMemoryRecallItem,
  type AdminUser,
  type MemoryQuery,
} from "../api";
import { LIST_PAGINATION } from "../pagination";

const { Paragraph, Text } = Typography;

type Scope = "user" | "bot" | "channel" | "agent_pair";
type CompactScope = "bot" | "channel" | "agent_pair";

const SCOPE_OPTIONS = [
  { label: "用户", value: "user" },
  { label: "Bot", value: "bot" },
  { label: "群组", value: "channel" },
  { label: "Bot 之间", value: "agent_pair" },
];

const COMPACT_OPTIONS = [
  { label: "用户-Bot", value: "bot" },
  { label: "群组", value: "channel" },
  { label: "Bot 之间", value: "agent_pair" },
];

const TIER_LABEL: Record<string, string> = {
  profile: "profile",
  log: "log",
  note: "note",
};

const SOURCE_LABEL: Record<string, string> = {
  explicit: "显式",
  mem0: "Mem0",
};

function preview(text: string, n = 80) {
  const s = (text || "").replace(/\s+/g, " ").trim();
  return s.length > n ? `${s.slice(0, n)}…` : s || "—";
}

export default function MemoryPage() {
  const memoryRef = useRef<ActionType>(null);
  const autoRef = useRef<ActionType>(null);
  const compactRef = useRef<ActionType>(null);
  const recallRef = useRef<ActionType>(null);

  const [scope, setScope] = useState<Scope>("user");
  const [compactScope, setCompactScope] = useState<CompactScope>("bot");
  const [userId, setUserId] = useState<string>();
  const [agentId, setAgentId] = useState<string>();
  const [peerAgentId, setPeerAgentId] = useState<string>();
  const [channelId, setChannelId] = useState<string>();
  const [conversationId, setConversationId] = useState<string>();
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [bots, setBots] = useState<AdminBot[]>([]);
  const [channels, setChannels] = useState<AdminChannel[]>([]);
  const [drawer, setDrawer] = useState<{ title: string; body: string } | null>(null);
  const [recallDetail, setRecallDetail] = useState<AdminMemoryRecall | null>(null);
  const [recallDetailLoading, setRecallDetailLoading] = useState(false);
  const [autoReason, setAutoReason] = useState("");
  const [compactNote, setCompactNote] = useState("");
  const [compactConfig, setCompactConfig] = useState<Record<string, unknown> | null>(null);

  useEffect(() => {
    void (async () => {
      try {
        const [u, b, c] = await Promise.all([adminListUsers(), adminListBots(), adminListChannels()]);
        setUsers(u.users || []);
        setBots(b.bots || []);
        setChannels(c.channels || []);
      } catch (err) {
        message.error(err instanceof Error ? err.message : String(err));
      }
    })();
  }, []);

  useEffect(() => {
    memoryRef.current?.reload();
    autoRef.current?.reload();
  }, [scope, userId, agentId, peerAgentId, channelId]);

  useEffect(() => {
    compactRef.current?.reload();
  }, [compactScope, userId, agentId, channelId]);

  useEffect(() => {
    recallRef.current?.reload();
  }, [userId, agentId, conversationId]);

  const userBots = useMemo(
    () => (userId ? bots.filter((b) => b.user_id === userId) : bots),
    [bots, userId],
  );
  const userChannels = useMemo(
    () => (userId ? channels.filter((c) => c.user_id === userId) : channels),
    [channels, userId],
  );

  const memoryQuery = useMemo<MemoryQuery>(() => {
    const q: MemoryQuery = { scope, limit: 100, user_id: userId };
    if (scope === "bot" || scope === "agent_pair") q.agent_id = agentId;
    if (scope === "channel") q.channel_id = channelId;
    if (scope === "agent_pair") q.peer_agent_id = peerAgentId;
    return q;
  }, [scope, userId, agentId, channelId, peerAgentId]);

  const openRecallDetail = async (id: string) => {
    setRecallDetailLoading(true);
    setRecallDetail(null);
    try {
      const data = await adminGetMemoryRecall(id);
      setRecallDetail(data.recall);
    } catch (err) {
      message.error(err instanceof Error ? err.message : String(err));
    } finally {
      setRecallDetailLoading(false);
    }
  };

  const memoryColumns: ProColumns<AdminMemory>[] = [
    { title: "用户", dataIndex: "username", width: 120, render: (_, row) => row.username || row.user_id },
    { title: "层级", dataIndex: "tier", width: 90, render: (_, row) => TIER_LABEL[row.tier || ""] || row.tier || "—" },
    {
      title: "归属",
      width: 180,
      ellipsis: true,
      render: (_, row) => row.agent_name || row.channel_name || row.peer_agent_name || "—",
    },
    {
      title: "内容",
      dataIndex: "content",
      ellipsis: true,
      render: (_, row) => (
        <a onClick={() => setDrawer({ title: "显式记忆", body: row.content })}>{preview(row.content)}</a>
      ),
    },
    {
      title: "更新时间",
      dataIndex: "updated_at",
      valueType: "dateTime",
      width: 180,
      defaultSortOrder: "descend",
      sorter: (a, b) => String(a.updated_at || "").localeCompare(String(b.updated_at || "")),
    },
  ];

  const autoColumns: ProColumns<AdminAutoMemory>[] = [
    { title: "作用域", dataIndex: "scope", width: 120 },
    {
      title: "内容",
      dataIndex: "content",
      ellipsis: true,
      render: (_, row) => (
        <a onClick={() => setDrawer({ title: "自动记忆", body: row.content })}>{preview(row.content)}</a>
      ),
    },
    {
      title: "更新时间",
      dataIndex: "updated_at",
      valueType: "dateTime",
      width: 180,
      defaultSortOrder: "descend",
      sorter: (a, b) => String(a.updated_at || a.created_at || "").localeCompare(String(b.updated_at || b.created_at || "")),
    },
  ];

  const compactColumns: ProColumns<AdminCompaction>[] = [
    { title: "用户", dataIndex: "username", width: 120, render: (_, row) => row.username || row.user_id || "—" },
    { title: "Bot", dataIndex: "agent_name", width: 140, render: (_, row) => row.agent_name || row.agent_id || "—" },
    { title: "群组", dataIndex: "channel_name", width: 140, render: (_, row) => row.channel_name || "—" },
    { title: "会话", dataIndex: "title", width: 160, ellipsis: true },
    {
      title: "摘要",
      dataIndex: "content",
      ellipsis: true,
      render: (_, row) => (
        <a onClick={() => setDrawer({ title: row.title || "压缩摘要", body: row.content })}>{preview(row.content)}</a>
      ),
    },
    {
      title: "时间",
      dataIndex: "created_at",
      valueType: "dateTime",
      width: 180,
      defaultSortOrder: "descend",
      sorter: (a, b) => String(a.created_at || "").localeCompare(String(b.created_at || "")),
    },
  ];

  const recallColumns: ProColumns<AdminMemoryRecall>[] = [
    {
      title: "时间",
      dataIndex: "created_at",
      valueType: "dateTime",
      width: 180,
      defaultSortOrder: "descend",
      sorter: (a, b) => String(a.created_at || "").localeCompare(String(b.created_at || "")),
    },
    { title: "用户", dataIndex: "username", width: 120, render: (_, row) => row.username || row.user_id || "—" },
    { title: "Bot", dataIndex: "agent_name", width: 140, render: (_, row) => row.agent_name || row.agent_id || "—" },
    {
      title: "会话",
      dataIndex: "conversation_id",
      width: 180,
      ellipsis: true,
      copyable: true,
    },
    {
      title: "显式/Mem0",
      width: 110,
      render: (_, row) => `${row.explicit_count ?? 0} / ${row.mem0_count ?? 0}`,
    },
    {
      title: "注入条数",
      dataIndex: "item_count",
      width: 90,
    },
    {
      title: "场景",
      dataIndex: "scene",
      width: 100,
      render: (_, row) => row.scene || "—",
    },
    {
      title: "操作",
      valueType: "option",
      width: 100,
      render: (_, row) => [
        <a key="detail" onClick={() => void openRecallDetail(row.id)}>
          本轮召回
        </a>,
      ],
    },
  ];

  const recallItemColumns: TableColumnsType<AdminMemoryRecallItem> = [
    {
      title: "来源",
      dataIndex: "source",
      width: 90,
      render: (_, row) => (
        <Tag color={row.source === "mem0" ? "purple" : "blue"}>{SOURCE_LABEL[row.source] || row.source}</Tag>
      ),
    },
    { title: "作用域", dataIndex: "scope", width: 100, render: (_, row) => row.scope || "—" },
    { title: "层级", dataIndex: "tier", width: 90, render: (_, row) => row.tier || "—" },
    {
      title: "内容",
      dataIndex: "content",
      ellipsis: true,
      render: (_, row) => (
        <a
          onClick={() =>
            setDrawer({
              title: row.source === "mem0" ? "Mem0 召回" : "显式召回",
              body: row.snippet || row.content || "",
            })
          }
        >
          {preview(row.snippet || row.content || "")}
        </a>
      ),
    },
  ];

  return (
    <PageContainer title="记忆与压缩">
      <Paragraph type="secondary">
        一次对话先用本会话近期消息和压缩摘要，再按场景取长期记忆：私聊是 Bot 然后用户；群聊是群组、当前 Bot、然后用户；Bot 之间是这一对、当前 Bot、然后用户。历史记忆没有 Bot 或群信息，已归入用户档。Bot 之间要有显式写入才会出现。
        「本轮召回」为全量库存/检索；按聊天轮次看注入片段请优先用「调用追踪」详情里的本轮召回（不依赖 Langfuse 即可落库）。
      </Paragraph>
      <Space wrap style={{ marginBottom: 16 }}>
        <Segmented
          options={SCOPE_OPTIONS}
          value={scope}
          onChange={(v) => {
            setScope(v as Scope);
            setAgentId(undefined);
            setPeerAgentId(undefined);
            setChannelId(undefined);
          }}
        />
        <Select
          allowClear
          showSearch
          optionFilterProp="label"
          placeholder="用户"
          style={{ width: 180 }}
          value={userId}
          onChange={(v) => {
            setUserId(v);
            setAgentId(undefined);
            setPeerAgentId(undefined);
            setChannelId(undefined);
          }}
          options={users.map((u) => ({ value: u.id, label: u.username }))}
        />
        {(scope === "bot" || scope === "agent_pair") && (
          <Select
            allowClear
            showSearch
            optionFilterProp="label"
            placeholder={scope === "agent_pair" ? "Bot A" : "Bot"}
            style={{ width: 180 }}
            value={agentId}
            onChange={setAgentId}
            options={userBots.map((b) => ({
              value: b.id,
              label: `${b.name}${b.owner_username ? `（${b.owner_username}）` : ""}`,
            }))}
          />
        )}
        {scope === "agent_pair" && (
          <Select
            allowClear
            showSearch
            optionFilterProp="label"
            placeholder="Bot B"
            style={{ width: 180 }}
            value={peerAgentId}
            onChange={setPeerAgentId}
            options={userBots
              .filter((b) => b.id !== agentId)
              .map((b) => ({
                value: b.id,
                label: `${b.name}${b.owner_username ? `（${b.owner_username}）` : ""}`,
              }))}
          />
        )}
        {scope === "channel" && (
          <Select
            allowClear
            showSearch
            optionFilterProp="label"
            placeholder="群组"
            style={{ width: 200 }}
            value={channelId}
            onChange={setChannelId}
            options={userChannels.map((c) => ({
              value: c.id,
              label: `${c.name}${c.username ? `（${c.username}）` : ""}`,
            }))}
          />
        )}
      </Space>
      <Tabs
        items={[
          {
            key: "explicit",
            label: "显式记忆",
            children: (
              <ProTable<AdminMemory>
                headerTitle="显式记忆"
                actionRef={memoryRef}
                rowKey="id"
                search={false}
                options={{ reload: true }}
                pagination={{ ...LIST_PAGINATION }}
                columns={memoryColumns}
                request={async () => {
                  try {
                    const data = await adminListMemories(memoryQuery);
                    return { data: data.memories || [], success: true };
                  } catch (err) {
                    message.error(err instanceof Error ? err.message : String(err));
                    return { data: [], success: false };
                  }
                }}
              />
            ),
          },
          {
            key: "auto",
            label: "自动记忆",
            children: (
              <>
                {!userId && (
                  <Alert
                    type="info"
                    showIcon
                    style={{ marginBottom: 12 }}
                    message="自动记忆按用户存放。请先选择用户。"
                  />
                )}
                {autoReason && (
                  <Alert type="warning" showIcon style={{ marginBottom: 12 }} message={autoReason} />
                )}
                <ProTable<AdminAutoMemory>
                  headerTitle="Mem0 自动记忆"
                  actionRef={autoRef}
                  rowKey={(row) => row.id || row.content}
                  search={false}
                  options={{ reload: true }}
                  pagination={{ ...LIST_PAGINATION }}
                  columns={autoColumns}
                  request={async () => {
                    if (!userId) {
                      setAutoReason("");
                      return { data: [], success: true };
                    }
                    try {
                      const data = await adminListAutoMemories(memoryQuery);
                      setAutoReason(data.reason || (data.enabled ? "" : "自动记忆未启用"));
                      return { data: data.memories || [], success: true };
                    } catch (err) {
                      message.error(err instanceof Error ? err.message : String(err));
                      return { data: [], success: false };
                    }
                  }}
                />
              </>
            ),
          },
          {
            key: "recall",
            label: "本轮召回",
            children: (
              <>
                <Space wrap style={{ marginBottom: 12 }}>
                  <Input
                    allowClear
                    placeholder="会话 ID"
                    style={{ width: 280 }}
                    value={conversationId}
                    onChange={(e) => setConversationId(e.target.value.trim() || undefined)}
                  />
                  <Select
                    allowClear
                    showSearch
                    optionFilterProp="label"
                    placeholder="按 Bot 过滤"
                    style={{ width: 200 }}
                    value={agentId}
                    onChange={setAgentId}
                    options={userBots.map((b) => ({
                      value: b.id,
                      label: `${b.name}${b.owner_username ? `（${b.owner_username}）` : ""}`,
                    }))}
                  />
                  <Text type="secondary">按用户 / 会话 / Bot 查看每次运行注入提示词的记忆片段。</Text>
                </Space>
                <ProTable<AdminMemoryRecall>
                  headerTitle="本轮召回"
                  actionRef={recallRef}
                  rowKey="id"
                  search={false}
                  options={{ reload: true }}
                  pagination={{ ...LIST_PAGINATION }}
                  columns={recallColumns}
                  request={async () => {
                    try {
                      const data = await adminListMemoryRecalls({
                        user_id: userId,
                        agent_id: agentId,
                        conversation_id: conversationId,
                        limit: 100,
                      });
                      return { data: data.recalls || [], success: true };
                    } catch (err) {
                      message.error(err instanceof Error ? err.message : String(err));
                      return { data: [], success: false };
                    }
                  }}
                />
              </>
            ),
          },
          {
            key: "compact",
            label: "压缩",
            children: (
              <>
                <Space wrap style={{ marginBottom: 12 }}>
                  <Segmented
                    options={COMPACT_OPTIONS}
                    value={compactScope}
                    onChange={(v) => setCompactScope(v as CompactScope)}
                  />
                  <Text type="secondary">压缩摘要挂在会话上，不按用户档单独存储。</Text>
                </Space>
                {compactNote && (
                  <Alert type="info" showIcon style={{ marginBottom: 12 }} message={compactNote} />
                )}
                <CompactConfig config={compactConfig} onLoad={setCompactConfig} />
                <ProTable<AdminCompaction>
                  headerTitle="压缩摘要"
                  actionRef={compactRef}
                  rowKey="id"
                  search={false}
                  options={{ reload: true }}
                  pagination={{ ...LIST_PAGINATION }}
                  columns={compactColumns}
                  request={async () => {
                    try {
                      const data = await adminListCompactions({
                        scope: compactScope,
                        user_id: userId,
                        agent_id: compactScope === "bot" ? agentId : undefined,
                        channel_id: compactScope === "channel" ? channelId : undefined,
                        limit: 100,
                      });
                      setCompactNote(data.note || "");
                      return { data: data.compactions || [], success: true };
                    } catch (err) {
                      message.error(err instanceof Error ? err.message : String(err));
                      return { data: [], success: false };
                    }
                  }}
                />
              </>
            ),
          },
        ]}
      />
      {/* Content preview: Modal (not nested Drawer) so it stacks above recall Drawer / ProLayout */}
      <Modal
        title={drawer?.title}
        open={!!drawer}
        width={640}
        footer={null}
        destroyOnClose
        zIndex={1200}
        getContainer={() => document.body}
        onCancel={() => setDrawer(null)}
      >
        <Paragraph style={{ whiteSpace: "pre-wrap", marginBottom: 0 }}>{drawer?.body}</Paragraph>
      </Modal>
      <Drawer
        title="本轮召回详情"
        open={!!recallDetail || recallDetailLoading}
        width={720}
        destroyOnClose
        zIndex={1100}
        getContainer={() => document.body}
        onClose={() => {
          setRecallDetail(null);
          setRecallDetailLoading(false);
          setDrawer(null);
        }}
      >
        {recallDetailLoading && <Text type="secondary">加载中…</Text>}
        {recallDetail && (
          <>
            <Descriptions size="small" column={2} style={{ marginBottom: 16 }}>
              <Descriptions.Item label="时间">{recallDetail.created_at || "—"}</Descriptions.Item>
              <Descriptions.Item label="用户">
                {recallDetail.username || recallDetail.user_id || "—"}
              </Descriptions.Item>
              <Descriptions.Item label="Bot">
                {recallDetail.agent_name || recallDetail.agent_id || "—"}
              </Descriptions.Item>
              <Descriptions.Item label="场景">{recallDetail.scene || "—"}</Descriptions.Item>
              <Descriptions.Item label="会话">
                <Text copyable={!!recallDetail.conversation_id}>
                  {recallDetail.conversation_id || "—"}
                </Text>
              </Descriptions.Item>
              <Descriptions.Item label="消息">
                <Text copyable={!!recallDetail.message_id}>{recallDetail.message_id || "—"}</Text>
              </Descriptions.Item>
              <Descriptions.Item label="显式召回数">{recallDetail.explicit_count ?? 0}</Descriptions.Item>
              <Descriptions.Item label="Mem0 召回数">{recallDetail.mem0_count ?? 0}</Descriptions.Item>
            </Descriptions>
            <Table<AdminMemoryRecallItem>
              size="small"
              rowKey={(_, i) => String(i)}
              pagination={false}
              dataSource={recallDetail.items || []}
              columns={recallItemColumns}
              locale={{ emptyText: "本轮未注入记忆片段" }}
            />
          </>
        )}
      </Drawer>
    </PageContainer>
  );
}

function CompactConfig({
  config,
  onLoad,
}: {
  config: Record<string, unknown> | null;
  onLoad: (cfg: Record<string, unknown> | null) => void;
}) {
  useEffect(() => {
    if (config) return;
    void (async () => {
      try {
        const data = await adminGetCompactConfig();
        onLoad(data.compact || {});
      } catch {
        onLoad({});
      }
    })();
  }, [config, onLoad]);

  if (!config) return null;
  const pick = (key: string) => {
    const v = config[key];
    return v === undefined || v === null || v === "" ? "—" : String(v);
  };
  return (
    <Descriptions size="small" column={3} style={{ marginBottom: 16 }} title="压缩配置">
      <Descriptions.Item label="消息条数">{pick("max_messages")}</Descriptions.Item>
      <Descriptions.Item label="字符上限">{pick("max_chars")}</Descriptions.Item>
      <Descriptions.Item label="保留最近">{pick("keep_recent")}</Descriptions.Item>
      <Descriptions.Item label="上下文窗口">{pick("context_window")}</Descriptions.Item>
      <Descriptions.Item label="token 预算">{pick("token_budget")}</Descriptions.Item>
      <Descriptions.Item label="预算比例">{pick("budget_ratio")}</Descriptions.Item>
    </Descriptions>
  );
}
