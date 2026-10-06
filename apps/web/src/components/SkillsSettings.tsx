import { useCallback, useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { ChevronDown, Ellipsis, Plus, Search, Upload } from "lucide-react";
import {
  createSkill,
  deleteSkill,
  exportSkillZip,
  listSkills,
  setSkillEnabled,
  uploadSkillPackage,
  type Skill,
  type SkillPackage,
} from "../api";
import { replaceFrontmatterName } from "../lib/skillPackage";
import { useConfirm } from "./ConfirmProvider";
import { SkillEditor } from "./SkillEditor";
import { SettingsCard, SettingsEmpty, SettingsHint, SettingsPage } from "./SettingsLayout";

type Props = {
  /** Skill to open directly (e.g. from「当前 Bot」page name link). */
  openName?: string | null;
  onOpenNameConsumed?: () => void;
  /** Editor dirty state → settings shell guards close / tab switch. */
  onDirtyChange?: (dirty: boolean) => void;
  /** Editor open → settings dialog widens. */
  onEditingChange?: (editing: boolean) => void;
  /** After any mutation (App refreshes composer skill options). */
  onChanged?: () => void;
};

const SKILL_NAME_RE = /^[a-z0-9]+(-[a-z0-9]+)*$/;

function fmtTime(iso?: string): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "—";
  const p = (n: number) => String(n).padStart(2, "0");
  return `${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}`;
}

export function SkillsSettings({ openName, onOpenNameConsumed, onDirtyChange, onEditingChange, onChanged }: Props) {
  const confirm = useConfirm();
  const [skills, setSkills] = useState<Skill[]>([]);
  const [q, setQ] = useState("");
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState("");
  const [editing, setEditing] = useState<string | null>(null);
  const [rowMenu, setRowMenu] = useState<string | null>(null);
  const [botsPop, setBotsPop] = useState<string | null>(null);
  const [newMenu, setNewMenu] = useState(false);
  const [creating, setCreating] = useState<{ copyFrom?: SkillPackage } | null>(null);
  const [newName, setNewName] = useState("");
  const [newDesc, setNewDesc] = useState("");
  const [newErr, setNewErr] = useState("");
  const zipRef = useRef<HTMLInputElement>(null);
  const folderRef = useRef<HTMLInputElement>(null);

  const refresh = useCallback(async () => {
    try {
      setSkills(await listSkills());
    } catch (err) {
      setMsg(err instanceof Error ? err.message : String(err));
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    if (openName) {
      setEditing(openName);
      onOpenNameConsumed?.();
    }
  }, [openName, onOpenNameConsumed]);

  useEffect(() => {
    onEditingChange?.(Boolean(editing));
  }, [editing, onEditingChange]);
  useEffect(() => () => onEditingChange?.(false), [onEditingChange]);

  useEffect(() => {
    if (!rowMenu && !botsPop && !newMenu) return;
    // Capture phase: the settings dialog stops click propagation.
    const close = (e: MouseEvent) => {
      const t = e.target as HTMLElement;
      if (t?.closest?.(".skill-ctx-menu, .skills-bots-btn, .skills-row-menu button, .skills-new-caret")) return;
      setRowMenu(null);
      setBotsPop(null);
      setNewMenu(false);
    };
    window.addEventListener("mousedown", close, true);
    return () => window.removeEventListener("mousedown", close, true);
  }, [rowMenu, botsPop, newMenu]);

  const filtered = useMemo(() => {
    const t = q.trim().toLowerCase();
    if (!t) return skills;
    return skills.filter((s) => s.name.toLowerCase().includes(t) || (s.description || "").toLowerCase().includes(t));
  }, [skills, q]);

  const changed = async () => {
    await refresh();
    onChanged?.();
  };

  const toggle = async (s: Skill, enabled: boolean) => {
    setBusy(true);
    setMsg("");
    try {
      await setSkillEnabled(s.name, enabled);
      await changed();
    } catch (err) {
      setMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (s: Skill) => {
    const ok = await confirm({
      title: `删除自建技能「${s.name}」？`,
      description: s.bot_count ? `${s.bot_count} 个 Bot 正在使用，删除后将不再可用。` : undefined,
      confirmLabel: "删除",
      danger: true,
    });
    if (!ok) return;
    setBusy(true);
    try {
      await deleteSkill(s.name);
      await changed();
      setMsg(`已删除「${s.name}」`);
    } catch (err) {
      setMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  const submitCreate = async (e: FormEvent) => {
    e.preventDefault();
    const name = newName.trim();
    if (!SKILL_NAME_RE.test(name)) {
      setNewErr("名称仅允许小写字母、数字与 -（如 my-helper）");
      return;
    }
    if (skills.some((s) => s.name === name)) {
      setNewErr(`「${name}」已存在`);
      return;
    }
    setBusy(true);
    setNewErr("");
    try {
      const src = creating?.copyFrom;
      const files = src
        ? src.files.map((f) => (f.path === "SKILL.md" ? { ...f, content: replaceFrontmatterName(f.content, name) } : f))
        : undefined;
      await createSkill({ name, description: newDesc.trim() || undefined, files });
      setCreating(null);
      setNewName("");
      setNewDesc("");
      await changed();
      setEditing(name);
    } catch (err) {
      setNewErr(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  const upload = async (opts: { archive?: File; folderFiles?: File[] }) => {
    setBusy(true);
    setMsg("");
    try {
      const sk = await uploadSkillPackage(opts);
      await changed();
      setMsg(`已上传「${sk.name}」`);
    } catch (err) {
      setMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
      if (zipRef.current) zipRef.current.value = "";
      if (folderRef.current) folderRef.current.value = "";
    }
  };

  if (editing) {
    return (
      <SkillEditor
        key={editing}
        name={editing}
        onBack={() => {
          setEditing(null);
          void refresh();
        }}
        onDirtyChange={onDirtyChange}
        onSaved={() => void changed()}
        onCopyAsCustom={(pkg) => {
          setEditing(null);
          setCreating({ copyFrom: pkg });
          setNewName(SKILL_NAME_RE.test(pkg.name) ? `${pkg.name}-copy` : "");
          setNewDesc(pkg.description);
        }}
      />
    );
  }

  return (
    <SettingsPage>
      <SettingsHint>
        技能是全局共享的能力包（必含入口 SKILL.md）。账号级开关关闭后不注入、不可 load；各 Bot 在「当前 Bot」里勾选启用（与这里取交集）。
      </SettingsHint>
      <div className="skills-toolbar">
        <label className="skills-search">
          <Search size={14} />
          <input placeholder="搜索名称或描述…" value={q} onChange={(e) => setQ(e.target.value)} />
        </label>
        <div className="skills-new">
          <button
            type="button"
            className="primary"
            disabled={busy}
            onClick={() => {
              setCreating({});
              setNewName("");
              setNewDesc("");
              setNewErr("");
            }}
          >
            <Plus size={14} /> 新建
          </button>
          <button
            type="button"
            className="primary skills-new-caret"
            aria-label="更多新建方式"
            disabled={busy}
            onClick={(e) => {
              e.stopPropagation();
              setNewMenu((v) => !v);
            }}
          >
            <ChevronDown size={14} />
          </button>
          {newMenu ? (
            <div className="skill-ctx-menu skills-new-menu" onClick={(e) => e.stopPropagation()}>
              <button type="button" onClick={() => (setNewMenu(false), zipRef.current?.click())}>
                <Upload size={13} /> 上传 .zip
              </button>
              <button type="button" onClick={() => (setNewMenu(false), folderRef.current?.click())}>
                <Upload size={13} /> 上传文件夹
              </button>
            </div>
          ) : null}
          <input
            ref={zipRef}
            type="file"
            hidden
            accept=".zip,application/zip"
            onChange={(e) => {
              const f = e.target.files?.[0];
              if (f) void upload({ archive: f });
            }}
          />
          <input
            ref={folderRef}
            type="file"
            hidden
            multiple
            {...({ webkitdirectory: "", directory: "" } as Record<string, string>)}
            onChange={(e) => {
              const list = Array.from(e.target.files || []);
              if (list.length) void upload({ folderFiles: list });
            }}
          />
        </div>
      </div>

      {creating ? (
        <SettingsCard padded className="skills-create">
          <form className="llm-form" onSubmit={(e) => void submitCreate(e)}>
            <div className="agent-name">{creating.copyFrom ? `复制「${creating.copyFrom.name}」为自建技能` : "新建技能"}</div>
            <label>
              名称（小写字母、数字、-）
              <input autoFocus value={newName} placeholder="如 my-helper" onChange={(e) => (setNewName(e.target.value), setNewErr(""))} />
            </label>
            <label>
              描述（单行，可稍后在 SKILL.md 修改）
              <input value={newDesc} placeholder="何时使用这个技能" onChange={(e) => setNewDesc(e.target.value.replace(/\n/g, " "))} />
            </label>
            {newErr ? <div className="skill-editor-msg">{newErr}</div> : null}
            <div className="llm-actions">
              <button type="submit" className="primary" disabled={busy}>
                创建并编辑
              </button>
              <button type="button" className="ghost" onClick={() => setCreating(null)}>
                取消
              </button>
            </div>
          </form>
        </SettingsCard>
      ) : null}

      <SettingsCard className="skills-table-card">
        {filtered.length === 0 ? (
          <SettingsEmpty>{skills.length ? "没有匹配的技能" : "暂无技能"}</SettingsEmpty>
        ) : (
          <table className="skills-table">
            <thead>
              <tr>
                <th>名称</th>
                <th>描述</th>
                <th>来源</th>
                <th>启用 Bot</th>
                <th>更新时间</th>
                <th>账号启用</th>
                <th aria-label="操作" />
              </tr>
            </thead>
            <tbody>
              {filtered.map((s) => {
                const builtin = s.source ? s.source === "builtin" : !s.custom;
                return (
                  <tr key={s.name} className={s.enabled ? "" : "off"}>
                    <td>
                      <button type="button" className="skills-name-link" onClick={() => setEditing(s.name)}>
                        {s.name}
                      </button>
                    </td>
                    <td className="skills-desc" title={s.description}>
                      {s.description}
                    </td>
                    <td>
                      <span className={`skill-source-pill ${builtin ? "builtin" : "custom"}`}>{builtin ? "内置" : "自建"}</span>
                    </td>
                    <td className="skills-bots">
                      <button
                        type="button"
                        className="skills-bots-btn"
                        disabled={!s.bot_count}
                        onClick={(e) => {
                          e.stopPropagation();
                          setBotsPop((v) => (v === s.name ? null : s.name));
                        }}
                      >
                        {s.bot_count ?? 0}
                      </button>
                      {botsPop === s.name ? (
                        <div className="skill-ctx-menu skills-bots-pop" onClick={(e) => e.stopPropagation()}>
                          {(s.bots ?? []).map((b) => (
                            <div key={b.id} className="skills-bots-item">
                              {b.name}
                            </div>
                          ))}
                        </div>
                      ) : null}
                    </td>
                    <td className="skills-time">{fmtTime(s.updated_at)}</td>
                    <td>
                      <label className="skills-switch" title={s.enabled ? "已启用（点击关闭）" : "已关闭（点击启用）"}>
                        <input type="checkbox" checked={s.enabled} disabled={busy} onChange={(e) => void toggle(s, e.target.checked)} />
                        <span />
                      </label>
                    </td>
                    <td className="skills-row-menu">
                      <button
                        type="button"
                        className="settings-icon-btn"
                        aria-label="更多"
                        onClick={(e) => {
                          e.stopPropagation();
                          setRowMenu((v) => (v === s.name ? null : s.name));
                        }}
                      >
                        <Ellipsis size={15} />
                      </button>
                      {rowMenu === s.name ? (
                        <div className="skill-ctx-menu skills-row-pop" onClick={(e) => e.stopPropagation()}>
                          {builtin ? (
                            <button type="button" onClick={() => (setRowMenu(null), setEditing(s.name))}>
                              查看（只读）
                            </button>
                          ) : (
                            <>
                              <button type="button" onClick={() => (setRowMenu(null), setEditing(s.name))}>
                                编辑
                              </button>
                              <button
                                type="button"
                                onClick={() => {
                                  setRowMenu(null);
                                  void exportSkillZip(s.name).catch((err) => setMsg(err instanceof Error ? err.message : String(err)));
                                }}
                              >
                                导出 zip
                              </button>
                              <button type="button" className="danger" onClick={() => (setRowMenu(null), void remove(s))}>
                                删除
                              </button>
                            </>
                          )}
                        </div>
                      ) : null}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        )}
      </SettingsCard>
      {msg ? <div className="settings-status">{msg}</div> : null}
    </SettingsPage>
  );
}
