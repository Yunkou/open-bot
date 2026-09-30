import { useCallback, useMemo, useRef, useState } from "react";
import { Button, Input, Modal, Popconfirm, Space, Switch, Typography, Upload, message } from "antd";
import {
  ModalForm,
  PageContainer,
  ProFormSwitch,
  ProFormText,
  ProFormTextArea,
  ProTable,
  type ActionType,
  type ProColumns,
} from "@ant-design/pro-components";
import {
  adminDeleteSkill,
  adminDeleteSkillFile,
  adminExportSkillZip,
  adminGetSkill,
  adminImportSkillZip,
  adminListSkills,
  adminPatchSkill,
  adminUpsertSkill,
  adminUpsertSkillFile,
  type AdminSkill,
  type AdminSkillFile,
} from "../api";

const { Paragraph, Text } = Typography;

function skillMdBody(files: AdminSkillFile[] | undefined, fallback?: string): string {
  const skill = files?.find((f) => f.path === "SKILL.md");
  return skill?.content ?? fallback ?? "";
}

export default function SkillsPage() {
  const actionRef = useRef<ActionType>(null);
  const [editOpen, setEditOpen] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [editing, setEditing] = useState<AdminSkill | null>(null);
  const [activePath, setActivePath] = useState("SKILL.md");
  const [draftContent, setDraftContent] = useState("");
  const [savingFile, setSavingFile] = useState(false);
  const [addFileOpen, setAddFileOpen] = useState(false);
  const [importing, setImporting] = useState(false);

  const reload = useCallback(() => {
    void actionRef.current?.reload();
  }, []);

  const openEdit = useCallback(async (name: string) => {
    const full = await adminGetSkill(name);
    setEditing(full);
    setActivePath("SKILL.md");
    setDraftContent(skillMdBody(full.files, full.body_markdown));
    setEditOpen(true);
  }, []);

  const files = useMemo(() => {
    const list = [...(editing?.files || [])];
    if (!list.some((f) => f.path === "SKILL.md") && editing?.body_markdown) {
      list.unshift({ path: "SKILL.md", content: editing.body_markdown });
    }
    return list.sort((a, b) => a.path.localeCompare(b.path));
  }, [editing]);

  const selectFile = useCallback(
    (path: string) => {
      const f = files.find((x) => x.path === path);
      setActivePath(path);
      setDraftContent(f?.content ?? "");
    },
    [files],
  );

  const saveCurrentFile = useCallback(async () => {
    if (!editing) return;
    setSavingFile(true);
    try {
      const updated = await adminUpsertSkillFile(editing.name, activePath, draftContent);
      setEditing(updated);
      message.success(`已保存 ${activePath}`);
      reload();
    } catch (err) {
      message.error(err instanceof Error ? err.message : String(err));
    } finally {
      setSavingFile(false);
    }
  }, [activePath, draftContent, editing, reload]);

  const onExport = useCallback(async (name: string) => {
    try {
      await adminExportSkillZip(name);
      message.success(`已导出 ${name}.zip`);
    } catch (err) {
      message.error(err instanceof Error ? err.message : String(err));
    }
  }, []);

  const columns: ProColumns<AdminSkill>[] = [
    { title: "名称", dataIndex: "name", copyable: true, width: 160 },
    {
      title: "说明",
      dataIndex: "description",
      ellipsis: true,
    },
    {
      title: "启用",
      dataIndex: "enabled",
      width: 90,
      render: (_, row) => (
        <Switch
          checked={row.enabled}
          onChange={async (checked) => {
            try {
              await adminPatchSkill(row.name, { enabled: checked });
              message.success(checked ? "已启用" : "已下架");
              reload();
            } catch (err) {
              message.error(err instanceof Error ? err.message : String(err));
            }
          }}
        />
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
    {
      title: "操作",
      valueType: "option",
      width: 200,
      render: (_, row) => [
        <a
          key="edit"
          onClick={() => {
            void openEdit(row.name).catch((err) =>
              message.error(err instanceof Error ? err.message : String(err)),
            );
          }}
        >
          编辑包
        </a>,
        <a key="export" onClick={() => void onExport(row.name)}>
          导出 zip
        </a>,
        <Popconfirm
          key="del"
          title="确认删除该平台技能？用户侧目录会随之消失（已关闭的开关也会失效）。"
          okText="删除"
          okButtonProps={{ danger: true }}
          onConfirm={async () => {
            try {
              await adminDeleteSkill(row.name);
              message.success("已删除");
              reload();
            } catch (err) {
              message.error(err instanceof Error ? err.message : String(err));
            }
          }}
        >
          <a style={{ color: "#ff4d4f" }}>删除</a>
        </Popconfirm>,
      ],
    },
  ];

  return (
    <PageContainer title="Skills 管理">
      <Paragraph type="secondary">
        技能是<strong>目录包</strong>：必有 <code>SKILL.md</code>，还可含{" "}
        <code>references/</code>、<code>scripts/</code> 等文本文件。支持导入 / 导出{" "}
        <code>.zip</code>（导入会校验路径与体积，同名则覆盖包内容）。运行时{" "}
        <code>load_skill</code> 返回正文与文件列表。启动时会把仓库 <code>skills/</code>{" "}
        里尚不存在的技能 seed 进库。用户默认全部可用，可在「用户管理」里按人关闭。
      </Paragraph>
      <ProTable<AdminSkill>
        headerTitle="平台 Skills"
        actionRef={actionRef}
        rowKey="name"
        search={false}
        options={{ reload: true }}
        pagination={{ pageSize: 20 }}
        request={async () => {
          const data = await adminListSkills(true);
          return { data: data.skills || [], success: true };
        }}
        columns={columns}
        toolBarRender={() => [
          <Upload
            key="import"
            accept=".zip,application/zip"
            showUploadList={false}
            beforeUpload={(file) => {
              setImporting(true);
              void adminImportSkillZip(file)
                .then((sk) => {
                  const n = sk.files?.length;
                  message.success(
                    n != null ? `已导入「${sk.name}」（${n} 个文件）` : `已导入「${sk.name}」`,
                  );
                  reload();
                })
                .catch((err) => message.error(err instanceof Error ? err.message : String(err)))
                .finally(() => setImporting(false));
              return false;
            }}
          >
            <Button loading={importing}>导入 zip</Button>
          </Upload>,
          <Button
            key="create"
            type="primary"
            onClick={() => {
              setCreateOpen(true);
            }}
          >
            新建
          </Button>,
        ]}
      />

      <ModalForm
        title="新建 Skill"
        open={createOpen}
        modalProps={{ destroyOnClose: true, onCancel: () => setCreateOpen(false) }}
        onFinish={async (values) => {
          try {
            await adminUpsertSkill({
              name: String(values.name || "").trim(),
              description: String(values.description || "").trim(),
              body_markdown: String(values.body_markdown || ""),
              enabled: values.enabled !== false,
            });
            message.success("已创建（可再编辑包内附属文件）");
            setCreateOpen(false);
            reload();
            return true;
          } catch (err) {
            message.error(err instanceof Error ? err.message : String(err));
            return false;
          }
        }}
      >
        <ProFormText
          name="name"
          label="名称"
          placeholder="host-ssh"
          rules={[
            { required: true, message: "必填" },
            {
              pattern: /^[a-z0-9]+(-[a-z0-9]+)*$/,
              message: "小写字母、数字与连字符",
            },
          ]}
        />
        <ProFormText
          name="description"
          label="描述"
          rules={[{ required: true, message: "必填（路由与目录用）" }]}
        />
        <ProFormTextArea
          name="body_markdown"
          label="SKILL.md 正文（可不含 frontmatter）"
          fieldProps={{ rows: 14, style: { fontFamily: "ui-monospace, Menlo, monospace" } }}
        />
        <ProFormSwitch name="enabled" label="启用" initialValue={true} />
      </ModalForm>

      <Modal
        title={editing ? `编辑包 · ${editing.name}` : "编辑 Skill"}
        open={editOpen}
        width={920}
        destroyOnClose
        onCancel={() => {
          setEditOpen(false);
          setEditing(null);
        }}
        footer={[
          <Button key="export" onClick={() => editing && void onExport(editing.name)}>
            导出 zip
          </Button>,
          <Button
            key="close"
            onClick={() => {
              setEditOpen(false);
              setEditing(null);
            }}
          >
            关闭
          </Button>,
          <Button key="save" type="primary" loading={savingFile} onClick={() => void saveCurrentFile()}>
            保存当前文件
          </Button>,
        ]}
      >
        {editing ? (
          <div style={{ display: "flex", gap: 12, minHeight: 420 }}>
            <div
              style={{
                width: 220,
                flexShrink: 0,
                borderRight: "1px solid rgba(0,0,0,0.06)",
                paddingRight: 8,
              }}
            >
              <Space direction="vertical" style={{ width: "100%" }} size={4}>
                <Text type="secondary" style={{ fontSize: 12 }}>
                  包内文件
                </Text>
                {files.map((f) => (
                  <Button
                    key={f.path}
                    type={f.path === activePath ? "primary" : "text"}
                    block
                    style={{ textAlign: "left", height: "auto", whiteSpace: "normal" }}
                    onClick={() => selectFile(f.path)}
                  >
                    {f.path}
                  </Button>
                ))}
                <Button type="dashed" block onClick={() => setAddFileOpen(true)}>
                  添加文件
                </Button>
                {activePath !== "SKILL.md" ? (
                  <Popconfirm
                    title={`删除 ${activePath}？`}
                    okText="删除"
                    okButtonProps={{ danger: true }}
                    onConfirm={async () => {
                      try {
                        await adminDeleteSkillFile(editing.name, activePath);
                        const full = await adminGetSkill(editing.name);
                        setEditing(full);
                        setActivePath("SKILL.md");
                        setDraftContent(skillMdBody(full.files, full.body_markdown));
                        message.success("已删除");
                        reload();
                      } catch (err) {
                        message.error(err instanceof Error ? err.message : String(err));
                      }
                    }}
                  >
                    <Button danger block>
                      删除当前文件
                    </Button>
                  </Popconfirm>
                ) : null}
                <Switch
                  checked={editing.enabled}
                  checkedChildren="启用"
                  unCheckedChildren="下架"
                  onChange={async (checked) => {
                    try {
                      const updated = await adminPatchSkill(editing.name, { enabled: checked });
                      setEditing({ ...editing, enabled: updated.enabled });
                      message.success(checked ? "已启用" : "已下架");
                      reload();
                    } catch (err) {
                      message.error(err instanceof Error ? err.message : String(err));
                    }
                  }}
                />
              </Space>
            </div>
            <div style={{ flex: 1, minWidth: 0 }}>
              <Text strong>{activePath}</Text>
              <Input.TextArea
                value={draftContent}
                onChange={(e) => setDraftContent(e.target.value)}
                rows={18}
                disabled={savingFile}
                style={{
                  marginTop: 8,
                  fontFamily: "ui-monospace, Menlo, monospace",
                  fontSize: 13,
                }}
              />
            </div>
          </div>
        ) : null}
      </Modal>

      <ModalForm
        title="添加包内文件"
        open={addFileOpen}
        modalProps={{ destroyOnClose: true, onCancel: () => setAddFileOpen(false) }}
        onFinish={async (values) => {
          if (!editing) return false;
          const path = String(values.path || "").trim();
          try {
            const updated = await adminUpsertSkillFile(
              editing.name,
              path,
              String(values.content || ""),
            );
            setEditing(updated);
            setActivePath(path);
            setDraftContent(String(values.content || ""));
            setAddFileOpen(false);
            message.success("已添加");
            reload();
            return true;
          } catch (err) {
            message.error(err instanceof Error ? err.message : String(err));
            return false;
          }
        }}
      >
        <ProFormText
          name="path"
          label="相对路径"
          placeholder="references/examples.md"
          extra="如 references/foo.md、scripts/helper.sh"
          rules={[
            { required: true, message: "必填" },
            {
              pattern: /^(?!SKILL\.md$)[A-Za-z0-9._-]+(?:\/[A-Za-z0-9._-]+)*$/,
              message: "包内相对路径，如 references/a.md",
            },
          ]}
        />
        <ProFormTextArea
          name="content"
          label="内容"
          fieldProps={{ rows: 10, style: { fontFamily: "ui-monospace, Menlo, monospace" } }}
        />
      </ModalForm>
    </PageContainer>
  );
}
