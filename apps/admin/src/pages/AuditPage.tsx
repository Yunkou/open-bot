import { useRef } from "react";
import { Typography, message } from "antd";
import { PageContainer, ProTable, type ActionType, type ProColumns } from "@ant-design/pro-components";
import { adminListAuditLogs, type AuditLog } from "../api";

const { Paragraph } = Typography;

export default function AuditPage() {
  const actionRef = useRef<ActionType>(null);

  const columns: ProColumns<AuditLog>[] = [
    {
      title: "时间",
      dataIndex: "created_at",
      valueType: "dateTime",
      width: 180,
      defaultSortOrder: "descend",
      sorter: (a, b) => String(a.created_at || "").localeCompare(String(b.created_at || "")),
    },
    {
      title: "操作者",
      dataIndex: "actor_username",
      render: (_, row) => row.actor_username || row.actor_user_id || "—",
      width: 140,
    },
    { title: "动作", dataIndex: "action", width: 180, copyable: true },
    {
      title: "目标",
      dataIndex: "target_type",
      width: 160,
      render: (_, row) =>
        `${row.target_type || ""}${row.target_id ? `:${row.target_id.slice(0, 8)}` : ""}`,
    },
    {
      title: "详情",
      dataIndex: "meta_json",
      ellipsis: true,
      copyable: true,
    },
  ];

  return (
    <PageContainer title="审计日志">
      <Paragraph type="secondary">
        最近操作：邀请、角色变更、组织模型、功能开关、登录（可选）。
      </Paragraph>
      <ProTable<AuditLog>
        headerTitle="审计记录"
        actionRef={actionRef}
        rowKey="id"
        search={false}
        options={{ reload: true }}
        pagination={{ pageSize: 50 }}
        columns={columns}
        request={async () => {
          try {
            const data = await adminListAuditLogs(150);
            return { data: data.logs || [], success: true };
          } catch (err) {
            message.error(err instanceof Error ? err.message : String(err));
            return { data: [], success: false };
          }
        }}
      />
    </PageContainer>
  );
}
