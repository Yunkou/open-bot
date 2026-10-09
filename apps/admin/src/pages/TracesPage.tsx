import { useCallback, useEffect, useRef, useState } from "react";
import { Button, Drawer, Empty, List, Space, Tag, Typography, message } from "antd";
import { PageContainer, ProTable, type ActionType, type ProColumns } from "@ant-design/pro-components";
import {
  adminGetTrace,
  adminListMemoryRecalls,
  adminListTraces,
  adminTracesStatus,
  type AdminMemoryRecall,
  type AdminMemoryRecallItem,
  type AdminTrace,
} from "../api";
import { LIST_PAGINATION } from "../pagination";

const { Paragraph, Text, Link, Title } = Typography;

const SOURCE_LABEL: Record<string, string> = {
  explicit: "显式",
  mem0: "Mem0",
};

function pickClosestRecall(recalls: AdminMemoryRecall[], timestamp?: string): AdminMemoryRecall | null {
  if (!recalls.length) return null;
  if (!timestamp) return recalls[0];
  const target = Date.parse(timestamp);
  if (Number.isNaN(target)) return recalls[0];
  let best = recalls[0];
  let bestDist = Number.POSITIVE_INFINITY;
  for (const r of recalls) {
    const t = Date.parse(r.created_at || "");
    if (Number.isNaN(t)) continue;
    const dist = Math.abs(t - target);
    if (dist < bestDist) {
      best = r;
      bestDist = dist;
    }
  }
  return best;
}

async function fetchRecallForTrace(trace: AdminTrace): Promise<{
  recall: AdminMemoryRecall | null;
  matchedBy: "langfuse_trace_id" | "conversation_id" | null;
}> {
  const traceId = (trace.id || "").trim();
  if (traceId) {
    const byTrace = await adminListMemoryRecalls({ langfuse_trace_id: traceId, limit: 5 });
    const hit = (byTrace.recalls || [])[0];
    if (hit) return { recall: hit, matchedBy: "langfuse_trace_id" };
  }
  const convId = (trace.sessionId || "").trim();
  if (convId) {
    const byConv = await adminListMemoryRecalls({ conversation_id: convId, limit: 20 });
    const hit = pickClosestRecall(byConv.recalls || [], trace.timestamp);
    if (hit) return { recall: hit, matchedBy: "conversation_id" };
  }
  return { recall: null, matchedBy: null };
}

function RecallItems({ items }: { items: AdminMemoryRecallItem[] }) {
  if (!items.length) {
    return <Text type="secondary">本轮未注入记忆片段</Text>;
  }
  return (
    <List
      size="small"
      bordered
      dataSource={items}
      renderItem={(item, idx) => (
        <List.Item key={`${item.memory_id || item.source}-${idx}`}>
          <Space direction="vertical" size={2} style={{ width: "100%" }}>
            <Space wrap size={4}>
              <Tag color={item.source === "mem0" ? "purple" : "blue"}>
                {SOURCE_LABEL[item.source] || item.source || "—"}
              </Tag>
              {item.scope ? <Tag>{item.scope}</Tag> : null}
              {item.tier ? <Tag>{item.tier}</Tag> : null}
              {item.score != null ? <Text type="secondary">score {item.score.toFixed(2)}</Text> : null}
            </Space>
            <Text style={{ whiteSpace: "pre-wrap" }}>{item.snippet || item.content || "—"}</Text>
          </Space>
        </List.Item>
      )}
    />
  );
}

export default function TracesPage() {
  const actionRef = useRef<ActionType>(null);
  const [status, setStatus] = useState<{
    enabled: boolean;
    ready?: boolean;
    public_ui_url?: string;
    project_id?: string;
    reason?: string;
  } | null>(null);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [detail, setDetail] = useState<{
    trace: AdminTrace | null;
    observations?: unknown[];
    reason?: string;
  } | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [recall, setRecall] = useState<AdminMemoryRecall | null>(null);
  const [recallMatchedBy, setRecallMatchedBy] = useState<"langfuse_trace_id" | "conversation_id" | null>(
    null,
  );
  const [recallLoading, setRecallLoading] = useState(false);

  const refreshStatus = useCallback(async () => {
    try {
      const s = await adminTracesStatus();
      setStatus(s);
    } catch (err) {
      setStatus({
        enabled: false,
        reason: err instanceof Error ? err.message : String(err),
      });
    }
  }, []);

  useEffect(() => {
    void refreshStatus();
  }, [refreshStatus]);

  const openDetail = async (id: string) => {
    setDrawerOpen(true);
    setDetailLoading(true);
    setRecallLoading(true);
    setDetail(null);
    setRecall(null);
    setRecallMatchedBy(null);
    try {
      const data = await adminGetTrace(id);
      setDetail({
        trace: data.trace,
        observations: data.observations,
        reason: data.reason,
      });
      if (data.trace) {
        try {
          const { recall: hit, matchedBy } = await fetchRecallForTrace(data.trace);
          setRecall(hit);
          setRecallMatchedBy(matchedBy);
        } catch (err) {
          message.warning(err instanceof Error ? err.message : String(err));
        }
      }
    } catch (err) {
      message.error(err instanceof Error ? err.message : String(err));
      setDetail({ trace: null, reason: err instanceof Error ? err.message : String(err) });
    } finally {
      setDetailLoading(false);
      setRecallLoading(false);
    }
  };

  const columns: ProColumns<AdminTrace>[] = [
    {
      title: "时间",
      dataIndex: "timestamp",
      valueType: "dateTime",
      width: 180,
      defaultSortOrder: "descend",
      sorter: (a, b) => String(a.timestamp || "").localeCompare(String(b.timestamp || "")),
    },
    { title: "名称", dataIndex: "name", ellipsis: true, width: 140 },
    {
      title: "Bot",
      dataIndex: "agentName",
      width: 140,
      ellipsis: true,
      render: (_, row) => row.agentName || row.agentId || "—",
    },
    {
      title: "用户",
      dataIndex: "userName",
      width: 140,
      ellipsis: true,
      render: (_, row) =>
        row.userName ? (
          <span>
            {row.userName}
            {row.userId ? (
              <Text type="secondary" style={{ marginLeft: 6, fontSize: 12 }}>
                {row.userId.slice(0, 8)}…
              </Text>
            ) : null}
          </span>
        ) : (
          row.userId || "—"
        ),
    },
    {
      title: "会话",
      dataIndex: "sessionId",
      width: 140,
      ellipsis: true,
      copyable: true,
      render: (_, row) => row.sessionId || "—",
    },
    {
      title: "耗时 (s)",
      dataIndex: "latency",
      width: 100,
      render: (_, row) =>
        row.latency == null ? "—" : typeof row.latency === "number" ? row.latency.toFixed(2) : row.latency,
    },
    {
      title: "Trace ID",
      dataIndex: "id",
      copyable: true,
      ellipsis: true,
      width: 220,
    },
    {
      title: "操作",
      valueType: "option",
      width: 100,
      render: (_, row) => [
        <a key="view" onClick={() => void openDetail(row.id)}>
          详情
        </a>,
      ],
    },
  ];

  const uiURL = status?.public_ui_url || "";

  return (
    <PageContainer
      title="调用追踪"
      extra={
        uiURL ? (
          <Button href={uiURL} target="_blank" rel="noreferrer">
            在 Langfuse 打开
          </Button>
        ) : null
      }
    >
      {status && !status.enabled ? (
        <Empty
          description={
            <Space direction="vertical" size={4}>
              <Text>追踪未启用或不可用</Text>
              <Paragraph type="secondary" style={{ maxWidth: 560, marginBottom: 0 }}>
                {status.reason ||
                  "请在 .env 配置 LANGFUSE_PUBLIC_KEY / LANGFUSE_SECRET_KEY / LANGFUSE_BASE_URL，并执行 make compose-langfuse 启动本地 Langfuse。"}
              </Paragraph>
            </Space>
          }
        />
      ) : (
        <>
          <Paragraph type="secondary">
            数据来自本地 Langfuse（经管理 API 代理）。打开详情可看「本轮召回」（按 Langfuse trace id /
            会话匹配 `memory_recalls`）。全量检索仍在「记忆与压缩 → 本轮召回」。
            {status?.reason ? `（${status.reason}）` : ""}
          </Paragraph>
          <ProTable<AdminTrace>
            headerTitle="最近调用"
            actionRef={actionRef}
            rowKey="id"
            search={false}
            options={{ reload: true }}
            pagination={{ ...LIST_PAGINATION }}
            columns={columns}
            request={async (params) => {
              try {
                const data = await adminListTraces({
                  limit: params.pageSize || 20,
                  page: params.current || 1,
                });
                if (!data.enabled) {
                  setStatus((prev) => ({
                    enabled: false,
                    public_ui_url: data.public_ui_url || prev?.public_ui_url,
                    project_id: data.project_id || prev?.project_id,
                    reason: data.reason,
                  }));
                  return { data: [], success: true };
                }
                if (data.public_ui_url || data.project_id) {
                  setStatus((prev) => ({
                    enabled: true,
                    ready: prev?.ready,
                    public_ui_url: data.public_ui_url || prev?.public_ui_url,
                    project_id: data.project_id || prev?.project_id,
                    reason: "",
                  }));
                }
                return {
                  data: data.traces || [],
                  success: true,
                };
              } catch (err) {
                message.error(err instanceof Error ? err.message : String(err));
                return { data: [], success: false };
              }
            }}
          />
        </>
      )}

      <Drawer
        title="Trace 详情"
        width={720}
        open={drawerOpen}
        onClose={() => setDrawerOpen(false)}
        destroyOnClose
        extra={(() => {
          const traceURL =
            detail?.trace?.langfuse_url ||
            (uiURL && status?.project_id && detail?.trace?.id
              ? `${uiURL.replace(/\/$/, "")}/project/${encodeURIComponent(status.project_id)}/traces/${encodeURIComponent(detail.trace.id)}`
              : "");
          return traceURL ? (
            <Link href={traceURL} target="_blank" rel="noreferrer">
              在 Langfuse 打开
            </Link>
          ) : null;
        })()}
      >
        {detailLoading ? (
          <Text type="secondary">加载中…</Text>
        ) : detail?.trace ? (
          <Space direction="vertical" style={{ width: "100%" }} size="middle">
            <div>
              <Text type="secondary">ID</Text>
              <div>
                <Text copyable>{detail.trace.id}</Text>
              </div>
            </div>
            <div>
              <Text type="secondary">名称</Text>
              <div>{detail.trace.name || "—"}</div>
            </div>
            <div>
              <Text type="secondary">Bot</Text>
              <div>
                {detail.trace.agentName || "—"}
                {detail.trace.agentId ? (
                  <Text type="secondary" style={{ marginLeft: 8 }} copyable={{ text: detail.trace.agentId }}>
                    {detail.trace.agentId.slice(0, 8)}…
                  </Text>
                ) : null}
              </div>
            </div>
            <div>
              <Text type="secondary">用户</Text>
              <div>
                {detail.trace.userName || "—"}
                {detail.trace.userId ? (
                  <Text type="secondary" style={{ marginLeft: 8 }} copyable={{ text: detail.trace.userId }}>
                    {detail.trace.userId.slice(0, 8)}…
                  </Text>
                ) : null}
              </div>
            </div>
            <div>
              <Text type="secondary">会话（conversation_id）</Text>
              <div>
                {detail.trace.sessionId ? (
                  <Text copyable>{detail.trace.sessionId}</Text>
                ) : (
                  "—"
                )}
              </div>
            </div>
            <div>
              <Text type="secondary">时间</Text>
              <div>{detail.trace.timestamp || "—"}</div>
            </div>
            <div>
              <Text type="secondary">耗时</Text>
              <div>{detail.trace.latency ?? "—"}</div>
            </div>

            <div>
              <Title level={5} style={{ marginTop: 8, marginBottom: 8 }}>
                本轮召回
              </Title>
              {recallLoading ? (
                <Text type="secondary">加载召回…</Text>
              ) : recall ? (
                <Space direction="vertical" style={{ width: "100%" }} size="small">
                  <Text type="secondary">
                    显式 {recall.explicit_count ?? 0} / Mem0 {recall.mem0_count ?? 0}
                    {recall.scene ? ` · 场景 ${recall.scene}` : ""}
                    {recallMatchedBy === "langfuse_trace_id"
                      ? " · 按 Langfuse trace 匹配"
                      : recallMatchedBy === "conversation_id"
                        ? " · 按会话就近匹配"
                        : ""}
                    {recall.run_id ? (
                      <>
                        {" "}
                        · run <Text code copyable={{ text: recall.run_id }}>{recall.run_id.slice(0, 8)}…</Text>
                      </>
                    ) : null}
                  </Text>
                  <RecallItems items={recall.items || []} />
                </Space>
              ) : (
                <Empty
                  image={Empty.PRESENTED_IMAGE_SIMPLE}
                  description="未找到对应本轮召回（可能早于 ID 落库，或本轮未注入记忆）"
                />
              )}
            </div>

            <div>
              <Text type="secondary">Observations（原始 JSON）</Text>
              <pre
                style={{
                  background: "#f5f5f5",
                  padding: 12,
                  borderRadius: 6,
                  maxHeight: 420,
                  overflow: "auto",
                  fontSize: 12,
                }}
              >
                {JSON.stringify(detail.observations || [], null, 2)}
              </pre>
            </div>
          </Space>
        ) : (
          <Empty description={detail?.reason || "无数据"} />
        )}
      </Drawer>
    </PageContainer>
  );
}
