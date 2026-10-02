import { useCallback, useEffect, useRef, useState } from "react";
import { Button, Drawer, Empty, Space, Typography, message } from "antd";
import { PageContainer, ProTable, type ActionType, type ProColumns } from "@ant-design/pro-components";
import {
  adminGetTrace,
  adminListTraces,
  adminTracesStatus,
  type AdminTrace,
} from "../api";

const { Paragraph, Text, Link } = Typography;

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
    setDetail(null);
    try {
      const data = await adminGetTrace(id);
      setDetail({
        trace: data.trace,
        observations: data.observations,
        reason: data.reason,
      });
    } catch (err) {
      message.error(err instanceof Error ? err.message : String(err));
      setDetail({ trace: null, reason: err instanceof Error ? err.message : String(err) });
    } finally {
      setDetailLoading(false);
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
    { title: "名称", dataIndex: "name", ellipsis: true },
    {
      title: "用户 ID",
      dataIndex: "userId",
      width: 160,
      ellipsis: true,
      copyable: true,
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
            数据来自本地 Langfuse（经管理 API 代理）。详细分析请使用「在 Langfuse 打开」。每轮注入提示词的记忆片段见「记忆与压缩 → 本轮召回」（不依赖 Langfuse）。
            {status?.reason ? `（${status.reason}）` : ""}
          </Paragraph>
          <ProTable<AdminTrace>
            headerTitle="最近调用"
            actionRef={actionRef}
            rowKey="id"
            search={false}
            options={{ reload: true }}
            pagination={{ pageSize: 20 }}
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
        width={640}
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
              <Text type="secondary">用户</Text>
              <div>{detail.trace.userId || "—"}</div>
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
