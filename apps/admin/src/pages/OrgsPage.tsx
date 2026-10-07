import { useEffect, useState } from "react";
import { Button, Form, Input, Modal, Table, message, Space, Select } from "antd";
import { PageContainer } from "@ant-design/pro-components";
import {
  adminCreatePlatformOrg,
  adminListPlatformOrgs,
  getAdminScopeOrgId,
  setAdminScopeOrgId,
  type PlatformOrgSummary,
} from "../api";
import { useAuth } from "../auth/AuthContext";
import { LIST_PAGINATION } from "../pagination";

export default function OrgsPage() {
  const { user } = useAuth();
  const [orgs, setOrgs] = useState<PlatformOrgSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [days, setDays] = useState(30);
  const [open, setOpen] = useState(false);
  const [scope, setScope] = useState<string | null>(getAdminScopeOrgId());
  const isPlatform = user?.role === "platform_admin";

  const reload = () => {
    if (!isPlatform) return;
    setLoading(true);
    void adminListPlatformOrgs(days)
      .then((d) => setOrgs(d.orgs || []))
      .catch((err) => message.error(err instanceof Error ? err.message : String(err)))
      .finally(() => setLoading(false));
  };

  useEffect(() => {
    reload();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [days, isPlatform]);

  if (!isPlatform) {
    return (
      <PageContainer title="组织（平台）">
        <p>仅平台管理员可查看跨租户组织列表与切换作用域。</p>
      </PageContainer>
    );
  }

  return (
    <PageContainer
      title="组织（平台）"
      extra={
        <Space>
          <Select
            value={days}
            style={{ width: 120 }}
            onChange={setDays}
            options={[
              { value: 7, label: "近 7 天" },
              { value: 30, label: "近 30 天" },
              { value: 90, label: "近 90 天" },
            ]}
          />
          <Button type="primary" onClick={() => setOpen(true)}>
            新建组织
          </Button>
        </Space>
      }
    >
      <Space style={{ marginBottom: 16 }}>
        <span>当前查看作用域：</span>
        <Select
          allowClear
          placeholder="本账号所属组织（默认）"
          style={{ minWidth: 240 }}
          value={scope || undefined}
          onChange={(v) => {
            const next = v || null;
            setScope(next);
            setAdminScopeOrgId(next);
            message.success(next ? "已切换组织作用域（用量等只读页生效）" : "已恢复默认组织作用域");
          }}
          options={orgs.map((o) => ({ value: o.org_id, label: `${o.name} (${o.slug})` }))}
        />
      </Space>
      <Table
        rowKey="org_id"
        loading={loading}
        dataSource={orgs}
        pagination={{ ...LIST_PAGINATION }}
        columns={[
          { title: "名称", dataIndex: "name" },
          { title: "Slug", dataIndex: "slug" },
          { title: "成员", dataIndex: "member_count" },
          { title: "会话", dataIndex: "conversation_count" },
          { title: "消息", dataIndex: "message_count" },
          { title: "Bot", dataIndex: "agent_count" },
          { title: "Runs", dataIndex: "run_count" },
          { title: "Tokens", dataIndex: "total_tokens" },
          {
            title: "操作",
            render: (_, row) => (
              <Button
                type="link"
                onClick={() => {
                  setScope(row.org_id);
                  setAdminScopeOrgId(row.org_id);
                  message.success(`已切换到 ${row.name}`);
                }}
              >
                设为作用域
              </Button>
            ),
          },
        ]}
      />
      <Modal
        title="新建组织"
        open={open}
        onCancel={() => setOpen(false)}
        footer={null}
        destroyOnClose
      >
        <Form
          layout="vertical"
          onFinish={async (vals) => {
            try {
              await adminCreatePlatformOrg(vals.slug, vals.name);
              message.success("已创建");
              setOpen(false);
              reload();
            } catch (err) {
              message.error(err instanceof Error ? err.message : String(err));
            }
          }}
        >
          <Form.Item name="slug" label="Slug" rules={[{ required: true }]}>
            <Input placeholder="acme" />
          </Form.Item>
          <Form.Item name="name" label="名称" rules={[{ required: true }]}>
            <Input placeholder="Acme 公司" />
          </Form.Item>
          <Button type="primary" htmlType="submit" block>
            创建
          </Button>
        </Form>
      </Modal>
    </PageContainer>
  );
}
