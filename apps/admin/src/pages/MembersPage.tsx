import { useCallback, useRef } from "react";
import { message, Select, Typography } from "antd";
import {
  PageContainer,
  ProForm,
  ProFormSelect,
  ProFormText,
  ProTable,
  type ActionType,
  type ProColumns,
} from "@ant-design/pro-components";
import {
  adminInviteMember,
  adminListMembers,
  adminPatchMember,
  type AdminInvite,
  type AdminMember,
} from "../api";
import { useAuth } from "../auth/AuthContext";

const { Paragraph } = Typography;

const ROLE_OPTIONS_BASE = [
  { label: "成员（终端用户 / 聊天）", value: "member" },
  { label: "组织管理员（管理员账号）", value: "org_admin" },
  { label: "平台管理员（管理员账号）", value: "platform_admin" },
];

export default function MembersPage() {
  const { user, refreshMe } = useAuth();
  const membersAction = useRef<ActionType>(null);
  const invitesAction = useRef<ActionType>(null);
  const inviteRoleOptions =
    user?.role === "platform_admin"
      ? ROLE_OPTIONS_BASE
      : ROLE_OPTIONS_BASE.filter((o) => o.value !== "platform_admin");

  const reloadAll = useCallback(() => {
    void membersAction.current?.reload();
    void invitesAction.current?.reload();
  }, []);

  const memberColumns: ProColumns<AdminMember>[] = [
    { title: "用户名", dataIndex: "username", copyable: true },
    {
      title: "邮箱",
      dataIndex: "email",
      render: (_, row) => row.email || "—",
    },
    {
      title: "角色",
      dataIndex: "role",
      width: 280,
      render: (_, row) => (
        <Select
          style={{ width: "100%" }}
          value={row.role}
          disabled={row.id === user?.id}
          options={ROLE_OPTIONS_BASE}
          onChange={(role) => {
            void (async () => {
              try {
                await adminPatchMember(row.id, role);
                message.success("角色已更新");
                reloadAll();
                await refreshMe();
              } catch (err) {
                message.error(err instanceof Error ? err.message : String(err));
              }
            })();
          }}
        />
      ),
    },
    {
      title: "创建时间",
      dataIndex: "created_at",
      valueType: "dateTime",
      width: 180,
      defaultSortOrder: "descend",
      sorter: (a, b) => String(a.created_at || "").localeCompare(String(b.created_at || "")),
    },
  ];

  const inviteColumns: ProColumns<AdminInvite>[] = [
    { title: "用户名或邮箱", dataIndex: "username_or_email" },
    {
      title: "角色",
      dataIndex: "role",
      valueEnum: {
        member: { text: "成员" },
        org_admin: { text: "组织管理员" },
        platform_admin: { text: "平台管理员" },
      },
    },
    { title: "状态", dataIndex: "status", width: 100 },
    {
      title: "创建时间",
      dataIndex: "created_at",
      valueType: "dateTime",
      width: 180,
      defaultSortOrder: "descend",
      sorter: (a, b) => String(a.created_at || "").localeCompare(String(b.created_at || "")),
    },
  ];

  return (
    <PageContainer title="成员与角色">
      <ProForm
        layout="inline"
        submitter={{ searchConfig: { submitText: "邀请" } }}
        initialValues={{ role: "member" }}
        onFinish={async (values) => {
          try {
            const res = await adminInviteMember(
              String(values.username).trim(),
              String(values.role || "member"),
            );
            message.success(res.joined ? "已加入组织" : "已创建邀请");
            reloadAll();
            return true;
          } catch (err) {
            message.error(err instanceof Error ? err.message : String(err));
            return false;
          }
        }}
        style={{ marginBottom: 8 }}
      >
        <ProFormText
          name="username"
          placeholder="用户名或邮箱"
          rules={[{ required: true, message: "请输入用户名或邮箱" }]}
          width="md"
        />
        <ProFormSelect
          name="role"
          width="md"
          options={inviteRoleOptions}
          rules={[{ required: true }]}
        />
      </ProForm>
      <Paragraph type="secondary" style={{ marginBottom: 16 }}>
        默认邀请「成员」= 聊天端用户。邀请「组织/平台管理员」= 运营账号（可登管理端）。
        已注册用户会立即加入；未注册用户保留邀请，在用户端注册/登录后自动入组。
      </Paragraph>

      <ProTable<AdminMember>
        headerTitle="成员列表"
        actionRef={membersAction}
        rowKey="id"
        search={false}
        options={{ reload: true }}
        pagination={{ pageSize: 20 }}
        columns={memberColumns}
        request={async () => {
          const data = await adminListMembers();
          return { data: data.members || [], success: true };
        }}
      />

      <ProTable<AdminInvite>
        headerTitle="待接受邀请"
        actionRef={invitesAction}
        rowKey="id"
        search={false}
        options={{ reload: true }}
        pagination={false}
        columns={inviteColumns}
        request={async () => {
          const data = await adminListMembers();
          return { data: data.invites || [], success: true };
        }}
        style={{ marginTop: 16 }}
      />
    </PageContainer>
  );
}
