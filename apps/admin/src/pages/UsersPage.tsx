import { useCallback, useRef, useState } from "react";
import { Button, Popconfirm, Tabs, Typography, message } from "antd";
import {
  ModalForm,
  PageContainer,
  ProForm,
  ProFormSelect,
  ProFormText,
  ProTable,
  type ActionType,
  type ProColumns,
} from "@ant-design/pro-components";
import {
  adminBatchDeleteUsers,
  adminCreateUser,
  adminDeleteUser,
  adminInviteMember,
  adminListMembers,
  adminListUsers,
  adminPatchUser,
  type AdminInvite,
  type AdminUser,
} from "../api";
import { useAuth } from "../auth/AuthContext";

const { Paragraph } = Typography;

const ROLE_OPTIONS_BASE = [
  { label: "成员（终端用户 / 聊天）", value: "member" },
  { label: "组织管理员", value: "org_admin" },
  { label: "平台管理员", value: "platform_admin" },
];

export default function UsersPage() {
  const { user, refreshMe } = useAuth();
  const usersAction = useRef<ActionType>(null);
  const invitesAction = useRef<ActionType>(null);
  const [editOpen, setEditOpen] = useState(false);
  const [editing, setEditing] = useState<AdminUser | null>(null);
  const [selectedKeys, setSelectedKeys] = useState<string[]>([]);

  const roleOptions =
    user?.role === "platform_admin"
      ? ROLE_OPTIONS_BASE
      : ROLE_OPTIONS_BASE.filter((o) => o.value !== "platform_admin");

  const reload = useCallback(() => {
    void usersAction.current?.reload();
    void invitesAction.current?.reload();
  }, []);

  const columns: ProColumns<AdminUser>[] = [
    { title: "用户名", dataIndex: "username", copyable: true },
    {
      title: "邮箱",
      dataIndex: "email",
      render: (_, row) => row.email || "—",
    },
    {
      title: "角色",
      dataIndex: "role",
      width: 140,
      valueEnum: {
        member: { text: "成员" },
        org_admin: { text: "组织管理员" },
        platform_admin: { text: "平台管理员" },
      },
    },
    {
      title: "创建时间",
      dataIndex: "created_at",
      valueType: "dateTime",
      width: 180,
      defaultSortOrder: "descend",
      sorter: (a, b) => String(a.created_at || "").localeCompare(String(b.created_at || "")),
    },
    {
      title: "操作",
      valueType: "option",
      width: 160,
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
        row.id === user?.id ? (
          <span key="del" style={{ color: "#999" }}>
            删除
          </span>
        ) : (
          <Popconfirm
            key="del"
            title="确认软删除该用户？其 Bot 也会一并软删除，数据仍保留在库中。"
            okText="删除"
            okButtonProps={{ danger: true }}
            onConfirm={async () => {
              try {
                await adminDeleteUser(row.id);
                message.success("已软删除");
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
    <PageContainer title="用户管理">
      <Tabs
        items={[
          {
            key: "users",
            label: "用户列表",
            children: (
              <>
                <Paragraph type="secondary">
                  创建组织内用户、重置密码、调整角色。系统账号不可见/不可删。删除为软删除：用户及其 Bot
                  会从列表消失，行仍留在数据库里。
                </Paragraph>
                <ProTable<AdminUser>
                  headerTitle="用户"
                  actionRef={usersAction}
                  rowKey="id"
                  search={false}
                  options={{ reload: true }}
                  pagination={{ pageSize: 20 }}
                  rowSelection={{
                    selectedRowKeys: selectedKeys,
                    onChange: (keys) => setSelectedKeys(keys as string[]),
                    getCheckboxProps: (row) => ({ disabled: row.id === user?.id }),
                  }}
                  toolBarRender={() => [
                    <Popconfirm
                      key="batch-delete"
                      title={`确认软删除选中的 ${selectedKeys.length} 个用户？其 Bot 也会一并软删除。`}
                      okText="删除"
                      okButtonProps={{ danger: true }}
                      disabled={selectedKeys.length === 0}
                      onConfirm={async () => {
                        try {
                          const res = await adminBatchDeleteUsers(selectedKeys);
                          setSelectedKeys([]);
                          const failed = res.failed || [];
                          const deleted = res.deleted || [];
                          if (failed.length) {
                            const detail = [...new Set(failed.map((f) => f.error))].join("；");
                            if (deleted.length) {
                              message.warning(
                                `已软删除 ${deleted.length} 个用户，${failed.length} 个未删除：${detail}`,
                              );
                            } else {
                              message.error(detail || "删除失败");
                            }
                          } else {
                            message.success(`已软删除 ${deleted.length} 个用户`);
                          }
                          reload();
                        } catch (err) {
                          message.error(err instanceof Error ? err.message : String(err));
                        }
                      }}
                    >
                      <Button danger disabled={selectedKeys.length === 0}>
                        批量删除
                      </Button>
                    </Popconfirm>,
                    <ModalForm
                      key="create"
                      title="创建用户"
                      trigger={<Button type="primary">新建用户</Button>}
                      modalProps={{ destroyOnClose: true }}
                      initialValues={{ role: "member" }}
                      onFinish={async (values) => {
                        try {
                          await adminCreateUser({
                            username: String(values.username).trim(),
                            password: String(values.password),
                            email: values.email ? String(values.email).trim() : undefined,
                            role: String(values.role || "member"),
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
                      <ProFormText
                        name="username"
                        label="用户名"
                        rules={[{ required: true, message: "请输入用户名" }]}
                      />
                      <ProFormText.Password
                        name="password"
                        label="密码"
                        rules={[{ required: true, message: "请输入密码" }]}
                      />
                      <ProFormText name="email" label="邮箱" />
                      <ProFormSelect
                        name="role"
                        label="角色"
                        options={roleOptions}
                        rules={[{ required: true }]}
                      />
                    </ModalForm>,
                  ]}
                  columns={columns}
                  request={async () => {
                    try {
                      const data = await adminListUsers();
                      return { data: data.users || [], success: true };
                    } catch (err) {
                      message.error(err instanceof Error ? err.message : String(err));
                      return { data: [], success: false };
                    }
                  }}
                />
                <ModalForm
                  title="编辑用户"
                  open={editOpen}
                  onOpenChange={(open) => {
                    setEditOpen(open);
                    if (!open) setEditing(null);
                  }}
                  modalProps={{ destroyOnClose: true }}
                  initialValues={{
                    username: editing?.username || "",
                    email: editing?.email || "",
                    role: editing?.role || "member",
                    password: "",
                  }}
                  onFinish={async (values) => {
                    if (!editing) return false;
                    try {
                      const body: { email?: string; role?: string; password?: string } = {
                        email: values.email != null ? String(values.email).trim() : "",
                        role: String(values.role || "member"),
                      };
                      const pw = values.password ? String(values.password) : "";
                      if (pw) body.password = pw;
                      await adminPatchUser(editing.id, body);
                      message.success("已更新");
                      reload();
                      await refreshMe();
                      return true;
                    } catch (err) {
                      message.error(err instanceof Error ? err.message : String(err));
                      return false;
                    }
                  }}
                >
                  <ProFormText name="username" label="用户名" disabled />
                  <ProFormText name="email" label="邮箱" />
                  <ProFormSelect
                    name="role"
                    label="角色"
                    options={roleOptions}
                    rules={[{ required: true }]}
                    disabled={editing?.id === user?.id}
                  />
                  <ProFormText.Password
                    name="password"
                    label="重置密码"
                    placeholder="留空则不修改"
                  />
                </ModalForm>
              </>
            ),
          },
          {
            key: "invites",
            label: "邀请",
            children: (
              <>
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
                      reload();
                      return true;
                    } catch (err) {
                      message.error(err instanceof Error ? err.message : String(err));
                      return false;
                    }
                  }}
                  style={{ marginBottom: 16 }}
                >
                  <ProFormText
                    name="username"
                    placeholder="用户名或邮箱"
                    rules={[{ required: true, message: "请输入用户名或邮箱" }]}
                    width="md"
                  />
                  <ProFormSelect name="role" width="md" options={roleOptions} rules={[{ required: true }]} />
                </ProForm>
                <Paragraph type="secondary" style={{ marginBottom: 16 }}>
                  已注册用户会立即加入；未注册用户保留邀请，在用户端注册/登录后自动入组。完整 CRUD
                  请用「用户列表」页签。
                </Paragraph>
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
                />
              </>
            ),
          },
        ]}
      />
    </PageContainer>
  );
}
