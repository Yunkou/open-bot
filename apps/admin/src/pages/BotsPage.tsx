import { useCallback, useEffect, useRef, useState } from "react";
import { Button, Popconfirm, Typography, message } from "antd";
import {
  ModalForm,
  PageContainer,
  ProFormSelect,
  ProFormText,
  ProFormTextArea,
  ProTable,
  type ActionType,
  type ProColumns,
} from "@ant-design/pro-components";
import {
  adminCreateBot,
  adminDeleteBot,
  adminListBots,
  adminListUsers,
  adminPatchBot,
  type AdminBot,
  type AdminUser,
} from "../api";

const { Paragraph } = Typography;

const MODE_OPTIONS = [
  { label: "团队模式", value: "team" },
  { label: "私人模式", value: "private" },
];

export default function BotsPage() {
  const actionRef = useRef<ActionType>(null);
  const [users, setUsers] = useState<AdminUser[]>([]);
  const [editOpen, setEditOpen] = useState(false);
  const [editing, setEditing] = useState<AdminBot | null>(null);

  const loadUsers = useCallback(async () => {
    try {
      const data = await adminListUsers();
      setUsers(data.users || []);
    } catch {
      /* ignore for owner select */
    }
  }, []);

  useEffect(() => {
    void loadUsers();
  }, [loadUsers]);

  const reload = useCallback(() => {
    void actionRef.current?.reload();
  }, []);

  const columns: ProColumns<AdminBot>[] = [
    { title: "名称", dataIndex: "name", copyable: true },
    {
      title: "所有者",
      dataIndex: "owner_username",
      width: 140,
      render: (_, row) => row.owner_username || row.user_id,
    },
    {
      title: "描述",
      dataIndex: "description",
      ellipsis: true,
      render: (_, row) => row.description || "—",
    },
    {
      title: "电脑模式",
      dataIndex: "computer_mode",
      width: 110,
      valueEnum: {
        team: { text: "团队" },
        private: { text: "私人" },
      },
    },
    {
      title: "更新时间",
      dataIndex: "updated_at",
      valueType: "dateTime",
      width: 180,
    },
    {
      title: "操作",
      valueType: "option",
      width: 140,
      render: (_, row) => [
        <a
          key="edit"
          onClick={() => {
            setEditing(row);
            setEditOpen(true);
          }}
        >
          编辑
        </a>,
        row.is_builtin ? (
          <span key="del" style={{ color: "#999" }}>
            删除
          </span>
        ) : (
          <Popconfirm
            key="del"
            title="确认删除该 Bot？"
            okText="删除"
            okButtonProps={{ danger: true }}
            onConfirm={async () => {
              try {
                await adminDeleteBot(row.id);
                message.success("已删除");
                reload();
              } catch (err) {
                message.error(err instanceof Error ? err.message : String(err));
              }
            }}
          >
            <a style={{ color: "#ff4d4f" }}>删除</a>
          </Popconfirm>
        ),
      ],
    },
  ];

  return (
    <PageContainer title="Bot 管理">
      <Paragraph type="secondary">
        管理本组织成员名下的助手（Bot）：创建、编辑系统提示与电脑模式、删除。
      </Paragraph>
      <ProTable<AdminBot>
        headerTitle="Bot 列表"
        actionRef={actionRef}
        rowKey="id"
        search={false}
        options={{ reload: true }}
        pagination={{ pageSize: 20 }}
        toolBarRender={() => [
          <ModalForm
            key="create"
            title="创建 Bot"
            trigger={<Button type="primary">新建 Bot</Button>}
            modalProps={{ destroyOnClose: true }}
            initialValues={{ computer_mode: "team" }}
            onOpenChange={(open) => {
              if (open) void loadUsers();
            }}
            onFinish={async (values) => {
              try {
                await adminCreateBot({
                  user_id: String(values.user_id),
                  name: String(values.name).trim(),
                  description: values.description ? String(values.description) : "",
                  system_prompt: values.system_prompt ? String(values.system_prompt) : "",
                  computer_mode: String(values.computer_mode || "team"),
                });
                message.success("已创建");
                reload();
                return true;
              } catch (err) {
                message.error(err instanceof Error ? err.message : String(err));
                return false;
              }
            }}
          >
            <ProFormSelect
              name="user_id"
              label="所属用户"
              options={users.map((u) => ({
                label: `${u.username}（${u.role}）`,
                value: u.id,
              }))}
              rules={[{ required: true, message: "请选择所属用户" }]}
              showSearch
            />
            <ProFormText name="name" label="名称" rules={[{ required: true, message: "请输入名称" }]} />
            <ProFormTextArea name="description" label="描述" fieldProps={{ rows: 2 }} />
            <ProFormTextArea name="system_prompt" label="系统提示" fieldProps={{ rows: 4 }} />
            <ProFormSelect name="computer_mode" label="电脑模式" options={MODE_OPTIONS} />
          </ModalForm>,
        ]}
        columns={columns}
        request={async () => {
          try {
            const data = await adminListBots();
            return { data: data.bots || [], success: true };
          } catch (err) {
            message.error(err instanceof Error ? err.message : String(err));
            return { data: [], success: false };
          }
        }}
      />

      <ModalForm
        title="编辑 Bot"
        open={editOpen}
        onOpenChange={(open) => {
          setEditOpen(open);
          if (!open) setEditing(null);
        }}
        modalProps={{ destroyOnClose: true }}
        initialValues={{
          name: editing?.name,
          description: editing?.description || "",
          system_prompt: editing?.system_prompt || "",
          computer_mode: editing?.computer_mode || "team",
        }}
        onFinish={async (values) => {
          if (!editing) return false;
          try {
            await adminPatchBot(editing.id, {
              name: String(values.name).trim(),
              description: values.description ? String(values.description) : "",
              system_prompt: values.system_prompt ? String(values.system_prompt) : "",
              computer_mode: String(values.computer_mode || "team"),
            });
            message.success("已更新");
            reload();
            return true;
          } catch (err) {
            message.error(err instanceof Error ? err.message : String(err));
            return false;
          }
        }}
      >
        <ProFormText name="name" label="名称" rules={[{ required: true }]} />
        <ProFormTextArea name="description" label="描述" fieldProps={{ rows: 2 }} />
        <ProFormTextArea name="system_prompt" label="系统提示" fieldProps={{ rows: 4 }} />
        <ProFormSelect name="computer_mode" label="电脑模式" options={MODE_OPTIONS} />
      </ModalForm>
    </PageContainer>
  );
}
