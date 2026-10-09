import { Suspense, lazy, useCallback, useEffect, useMemo, useRef, useState, type DragEvent, type KeyboardEvent as ReactKeyboardEvent } from "react";
import {
  ArrowLeft,
  ChevronDown,
  ChevronRight,
  Ellipsis,
  Eye,
  File as FileIcon,
  FileCode2,
  FilePlus,
  FileText,
  Folder,
  FolderOpen,
  FolderPlus,
  Pencil,
  Plus,
  Save,
  SquareTerminal,
  Star,
  Trash2,
  X,
} from "lucide-react";
import { getSkillPackage, saveSkillPackage, type SkillPackage } from "../api";
import {
  SKILL_ENTRY,
  buildTree,
  checkSkillFrontmatter,
  createFile,
  createFolder,
  deletePath,
  dirname,
  fileKind,
  isFileDirty,
  isPackageDirty,
  isStructureDirty,
  loadPackage,
  markSaved,
  moveInto,
  renamePath,
  serializePackage,
  stripFrontmatter,
  type OpResult,
  type PkgState,
  type TreeNode,
} from "../lib/skillPackage";
import { MarkdownMessage } from "./MarkdownMessage";
import { useConfirm } from "./ConfirmProvider";

// CodeMirror 6 (md / py / js / ts / json / yaml / sh highlighting) is code-split.
const CodeEditor = lazy(() => import("./CodeEditor"));

type Props = {
  name: string;
  onBack: () => void;
  /** Reports any unsaved change (buffers or tree structure) to the settings shell. */
  onDirtyChange?: (dirty: boolean) => void;
  /** Called after a successful save (list refresh). */
  onSaved?: (pkg: SkillPackage) => void;
  /** Built-in only: copy to a new custom skill. */
  onCopyAsCustom?: (pkg: SkillPackage) => void;
};

type Pending =
  | { kind: "new-file"; dir: string }
  | { kind: "new-folder"; dir: string }
  | { kind: "rename"; path: string };

type Menu = { x: number; y: number; path: string; isFolder: boolean };

type CloseChoice = "save" | "discard" | "cancel";

function NodeIcon({ node, open }: { node: TreeNode; open?: boolean }) {
  if (node.kind === "folder") return open ? <FolderOpen size={15} /> : <Folder size={15} />;
  if (node.path === SKILL_ENTRY) return <Star size={15} className="skill-tree-star" />;
  switch (fileKind(node.path)) {
    case "md":
      return <FileText size={15} />;
    case "py":
    case "code":
      return <FileCode2 size={15} />;
    case "sh":
      return <SquareTerminal size={15} />;
    default:
      return <FileIcon size={15} />;
  }
}

export function SkillEditor({ name, onBack, onDirtyChange, onSaved, onCopyAsCustom }: Props) {
  const confirm = useConfirm();
  const [pkg, setPkg] = useState<SkillPackage | null>(null);
  const [st, setSt] = useState<PkgState | null>(null);
  const [loadErr, setLoadErr] = useState("");
  const [tabs, setTabs] = useState<string[]>([]);
  const [active, setActive] = useState<string>("");
  const [expanded, setExpanded] = useState<Set<string>>(new Set([""]));
  const [pending, setPending] = useState<Pending | null>(null);
  const [pendingValue, setPendingValue] = useState("");
  const [pendingErr, setPendingErr] = useState("");
  const [menu, setMenu] = useState<Menu | null>(null);
  const [preview, setPreview] = useState(false);
  const [saving, setSaving] = useState(false);
  const [msg, setMsg] = useState("");
  const [saveIssues, setSaveIssues] = useState<{ line: number; message: string }[]>([]);
  const [dragOver, setDragOver] = useState<string | null>(null);
  const [closeAsk, setCloseAsk] = useState<{ path: string; resolve: (c: CloseChoice) => void } | null>(null);
  const dragSrc = useRef<string | null>(null);
  const gutterRef = useRef<HTMLDivElement>(null);
  const textRef = useRef<HTMLTextAreaElement>(null);

  const readOnly = Boolean(pkg?.read_only);
  const dirty = st ? isPackageDirty(st) : false;

  useEffect(() => {
    let cancelled = false;
    setLoadErr("");
    void getSkillPackage(name)
      .then((p) => {
        if (cancelled) return;
        const s = loadPackage(p.files);
        setPkg(p);
        setSt(s);
        const first = SKILL_ENTRY in s.files ? SKILL_ENTRY : Object.keys(s.files)[0] || "";
        setTabs(first ? [first] : []);
        setActive(first);
        setExpanded(new Set(["", ...Object.keys(s.files).map(dirname)]));
      })
      .catch((err) => {
        if (!cancelled) setLoadErr(err instanceof Error ? err.message : String(err));
      });
    return () => {
      cancelled = true;
    };
  }, [name]);

  useEffect(() => {
    onDirtyChange?.(dirty);
  }, [dirty, onDirtyChange]);

  useEffect(() => () => onDirtyChange?.(false), [onDirtyChange]);

  useEffect(() => {
    if (!dirty) return;
    const h = (e: BeforeUnloadEvent) => {
      e.preventDefault();
      e.returnValue = "";
    };
    window.addEventListener("beforeunload", h);
    return () => window.removeEventListener("beforeunload", h);
  }, [dirty]);

  const tree = useMemo(() => (st ? buildTree(st) : []), [st]);
  const activeFile = st && active ? st.files[active] : undefined;
  const isMd = active ? fileKind(active) === "md" : false;

  const fmInfo = useMemo(() => {
    if (!st || !(SKILL_ENTRY in st.files)) return null;
    return checkSkillFrontmatter(st.files[SKILL_ENTRY].content, pkg?.custom ? name : undefined);
  }, [st, pkg?.custom, name]);

  const errorLines = useMemo(() => {
    const set = new Set<number>();
    if (active === SKILL_ENTRY) for (const i of saveIssues) set.add(i.line);
    return set;
  }, [saveIssues, active]);
  const errorLineList = useMemo(() => [...errorLines], [errorLines]);

  const apply = (r: OpResult): boolean => {
    if (r.error !== undefined) {
      setMsg(r.error);
      return false;
    }
    const next = r.state;
    if (r.renamed && Object.keys(r.renamed).length) {
      const map = r.renamed;
      setTabs((ts) => ts.map((t) => map[t] ?? t));
      setActive((a) => map[a] ?? a);
    }
    setSt(next);
    setMsg("");
    return true;
  };

  const openFile = (path: string) => {
    if (!st || !(path in st.files)) return;
    setTabs((ts) => (ts.includes(path) ? ts : [...ts, path]));
    setActive(path);
    if (fileKind(path) !== "md") setPreview(false);
  };

  const doSave = useCallback(async (): Promise<boolean> => {
    if (!st || !pkg || readOnly) return false;
    const entry = st.files[SKILL_ENTRY];
    if (!entry) {
      setMsg("缺少入口文件 SKILL.md");
      return false;
    }
    const info = checkSkillFrontmatter(entry.content, name);
    if (info.issues.length) {
      setSaveIssues(info.issues);
      setTabs((ts) => (ts.includes(SKILL_ENTRY) ? ts : [SKILL_ENTRY, ...ts]));
      setActive(SKILL_ENTRY);
      setPreview(false);
      setMsg("");
      return false;
    }
    setSaveIssues([]);
    setSaving(true);
    setMsg("");
    try {
      const saved = await saveSkillPackage(name, serializePackage(st));
      setPkg(saved);
      setSt((cur) => (cur ? markSaved(cur) : cur));
      setMsg("已保存");
      onSaved?.(saved);
      return true;
    } catch (err) {
      setMsg(err instanceof Error ? err.message : String(err));
      return false;
    } finally {
      setSaving(false);
    }
  }, [st, pkg, readOnly, name, onSaved]);

  // Ctrl/Cmd+S → 保存全部 (package is written back as a whole).
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "s") {
        e.preventDefault();
        if (!readOnly && !saving) void doSave();
      }
    };
    window.addEventListener("keydown", h);
    return () => window.removeEventListener("keydown", h);
  }, [doSave, readOnly, saving]);

  // Live-clear the error bar once the frontmatter is fixed.
  useEffect(() => {
    if (saveIssues.length && fmInfo && fmInfo.issues.length === 0) setSaveIssues([]);
  }, [fmInfo, saveIssues.length]);

  useEffect(() => {
    if (!menu) return;
    // Capture phase: the settings dialog stops click propagation.
    const onDown = (e: MouseEvent) => {
      if (!(e.target as HTMLElement)?.closest?.(".skill-ctx-menu")) setMenu(null);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setMenu(null);
    };
    window.addEventListener("mousedown", onDown, true);
    window.addEventListener("keydown", onKey);
    return () => {
      window.removeEventListener("mousedown", onDown, true);
      window.removeEventListener("keydown", onKey);
    };
  }, [menu]);

  const askClose = (path: string) =>
    new Promise<CloseChoice>((resolve) => setCloseAsk({ path, resolve }));

  const closeTab = async (path: string) => {
    if (!st) return;
    const f = st.files[path];
    if (!readOnly && isFileDirty(f)) {
      const choice = await askClose(path);
      setCloseAsk(null);
      if (choice === "cancel") return;
      if (choice === "save") {
        const ok = await doSave();
        if (!ok) return;
      } else if (f) {
        setSt((cur) =>
          cur ? { ...cur, files: { ...cur.files, [path]: { ...cur.files[path], content: cur.files[path].saved ?? "" } } } : cur,
        );
      }
    }
    setTabs((ts) => {
      const idx = ts.indexOf(path);
      const next = ts.filter((t) => t !== path);
      if (active === path) setActive(next[Math.min(idx, next.length - 1)] || "");
      return next;
    });
  };

  const goBack = async () => {
    if (dirty && !readOnly) {
      const ok = await confirm({
        title: "有未保存的更改",
        description: "返回列表将丢失未保存的修改。",
        confirmLabel: "不保存并返回",
        cancelLabel: "继续编辑",
        danger: true,
      });
      if (!ok) return;
    }
    onBack();
  };

  const startPending = (p: Pending) => {
    if (readOnly) return;
    setPending(p);
    setPendingErr("");
    if (p.kind === "rename") {
      setPendingValue(p.path.split("/").pop() || "");
    } else {
      setPendingValue("");
      const dir = p.dir;
      setExpanded((e) => new Set([...e, dir]));
    }
  };

  const commitPending = () => {
    if (!pending || !st) return;
    let r: OpResult;
    if (pending.kind === "new-file") r = createFile(st, pending.dir, pendingValue);
    else if (pending.kind === "new-folder") r = createFolder(st, pending.dir, pendingValue);
    else r = renamePath(st, pending.path, pendingValue);
    if (r.error !== undefined) {
      setPendingErr(r.error);
      return;
    }
    apply(r);
    if (pending.kind === "new-file") {
      const p = pending.dir ? `${pending.dir}/${pendingValue.trim()}` : pendingValue.trim();
      setTabs((ts) => [...ts, p]);
      setActive(p);
    } else if (pending.kind === "new-folder") {
      const p = pending.dir ? `${pending.dir}/${pendingValue.trim()}` : pendingValue.trim();
      setExpanded((e) => new Set([...e, p]));
    }
    setPending(null);
  };

  const onPendingKey = (e: ReactKeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") {
      e.preventDefault();
      commitPending();
    } else if (e.key === "Escape") {
      e.preventDefault();
      setPending(null);
    }
  };

  const removePath = async (path: string, isFolder: boolean) => {
    if (!st || readOnly || path === SKILL_ENTRY) return;
    const ok = await confirm({
      title: `删除${isFolder ? "文件夹" : "文件"}「${path}」？`,
      description: isFolder ? "文件夹内的所有文件都会删除；保存后生效。" : "保存后生效。",
      confirmLabel: "删除",
      danger: true,
    });
    if (!ok) return;
    const r = deletePath(st, path);
    if (apply(r)) {
      setTabs((ts) => ts.filter((t) => !(t === path || t.startsWith(path + "/"))));
      if (active === path || active.startsWith(path + "/")) setActive(SKILL_ENTRY);
    }
  };

  // ---- drag & drop (into folders / root; SKILL.md stays put) ----
  const onDragStart = (e: DragEvent, path: string) => {
    if (readOnly || path === SKILL_ENTRY) {
      e.preventDefault();
      return;
    }
    dragSrc.current = path;
    e.dataTransfer.effectAllowed = "move";
    e.dataTransfer.setData("text/plain", path);
  };
  const dropTargetDir = (node: TreeNode | null) => (node === null ? "" : node.kind === "folder" ? node.path : dirname(node.path));
  const onDragOver = (e: DragEvent, node: TreeNode | null) => {
    const src = dragSrc.current;
    if (!src) return;
    const target = dropTargetDir(node);
    if (target === src || target.startsWith(src + "/")) return;
    e.preventDefault();
    e.stopPropagation();
    e.dataTransfer.dropEffect = "move";
    setDragOver(target);
  };
  const onDrop = (e: DragEvent, node: TreeNode | null) => {
    e.preventDefault();
    e.stopPropagation();
    const src = dragSrc.current;
    dragSrc.current = null;
    setDragOver(null);
    if (!src || !st) return;
    const target = dropTargetDir(node);
    if (apply(moveInto(st, src, target))) setExpanded((x) => new Set([...x, target]));
  };

  const renderPendingRow = (depth: number, kind: "file" | "folder") => (
    <div className="skill-tree-row pending" style={{ paddingLeft: 8 + depth * 14 }}>
      {kind === "folder" ? <Folder size={15} /> : <FileIcon size={15} />}
      <input
        autoFocus
        value={pendingValue}
        placeholder={kind === "folder" ? "文件夹名" : "文件名，如 tips.md"}
        onChange={(e) => {
          setPendingValue(e.target.value);
          setPendingErr("");
        }}
        onKeyDown={onPendingKey}
        onBlur={() => setPending(null)}
      />
      {pendingErr ? <div className="skill-tree-pending-err">{pendingErr}</div> : null}
    </div>
  );

  const renderNodes = (nodes: TreeNode[], depth: number, dir: string) => (
    <>
      {pending && pending.kind !== "rename" && pending.dir === dir
        ? renderPendingRow(depth, pending.kind === "new-folder" ? "folder" : "file")
        : null}
      {nodes.map((n) => {
        const isOpen = n.kind === "folder" && expanded.has(n.path);
        const renaming = pending?.kind === "rename" && pending.path === n.path;
        const isEntry = n.path === SKILL_ENTRY;
        const fileDirty = n.kind === "file" && st ? isFileDirty(st.files[n.path]) : false;
        return (
          <div key={n.path}>
            {renaming ? (
              <div className="skill-tree-row pending" style={{ paddingLeft: 8 + depth * 14 }}>
                <NodeIcon node={n} open={isOpen} />
                <input
                  autoFocus
                  value={pendingValue}
                  onChange={(e) => {
                    setPendingValue(e.target.value);
                    setPendingErr("");
                  }}
                  onKeyDown={onPendingKey}
                  onBlur={() => setPending(null)}
                />
                {pendingErr ? <div className="skill-tree-pending-err">{pendingErr}</div> : null}
              </div>
            ) : (
              <div
                className={[
                  "skill-tree-row",
                  n.kind === "file" && active === n.path ? "active" : "",
                  n.kind === "folder" && dragOver === n.path ? "drop" : "",
                ].join(" ")}
                style={{ paddingLeft: 8 + depth * 14 }}
                draggable={!readOnly && !isEntry}
                onDragStart={(e) => onDragStart(e, n.path)}
                onDragOver={(e) => onDragOver(e, n)}
                onDragLeave={() => setDragOver(null)}
                onDrop={(e) => onDrop(e, n)}
                onDragEnd={() => {
                  dragSrc.current = null;
                  setDragOver(null);
                }}
                onClick={() => {
                  if (n.kind === "folder") {
                    setExpanded((e) => {
                      const next = new Set(e);
                      if (next.has(n.path)) next.delete(n.path);
                      else next.add(n.path);
                      return next;
                    });
                  } else openFile(n.path);
                }}
                onContextMenu={(e) => {
                  e.preventDefault();
                  setMenu({ x: e.clientX, y: e.clientY, path: n.path, isFolder: n.kind === "folder" });
                }}
                title={n.path}
              >
                <span className="skill-tree-caret">
                  {n.kind === "folder" ? isOpen ? <ChevronDown size={13} /> : <ChevronRight size={13} /> : null}
                </span>
                <NodeIcon node={n} open={isOpen} />
                <span className="skill-tree-name">{n.name}</span>
                {isEntry ? <span className="skill-entry-pill">入口</span> : null}
                {fileDirty ? <span className="skill-dirty-dot" aria-label="未保存" /> : null}
                {!readOnly ? (
                  <span className="skill-tree-actions">
                    {n.kind === "folder" ? (
                      <button
                        type="button"
                        title="在此新建文件"
                        onClick={(e) => {
                          e.stopPropagation();
                          startPending({ kind: "new-file", dir: n.path });
                        }}
                      >
                        <Plus size={13} />
                      </button>
                    ) : null}
                    <button
                      type="button"
                      title="更多"
                      onClick={(e) => {
                        e.stopPropagation();
                        const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
                        setMenu({ x: r.left, y: r.bottom + 2, path: n.path, isFolder: n.kind === "folder" });
                      }}
                    >
                      <Ellipsis size={13} />
                    </button>
                  </span>
                ) : null}
              </div>
            )}
            {n.kind === "folder" && isOpen ? renderNodes(n.children, depth + 1, n.path) : null}
          </div>
        );
      })}
    </>
  );

  if (loadErr) {
    return (
      <div className="skill-editor">
        <div className="skill-editor-head">
          <button type="button" className="ghost skill-back" onClick={onBack}>
            <ArrowLeft size={16} /> 返回
          </button>
        </div>
        <div className="skill-editor-error">无法打开技能「{name}」：{loadErr}</div>
      </div>
    );
  }
  if (!st || !pkg) return <div className="skill-editor-loading">加载中…</div>;

  const lineCount = activeFile ? activeFile.content.split("\n").length : 0;
  /** Plain textarea: shown while CodeMirror chunk loads (and as a no-JS-chunk fallback). */
  const plainEditor = activeFile ? (
              <div className="skill-code">
                <div className="skill-gutter" ref={gutterRef} aria-hidden>
                  {Array.from({ length: lineCount }, (_, i) => (
                    <div key={i} className={errorLines.has(i + 1) ? "err" : ""}>
                      {i + 1}
                    </div>
                  ))}
                </div>
                <textarea
                  ref={textRef}
                  className={`skill-textarea kind-${fileKind(active)}`}
                  value={activeFile.content}
                  readOnly={readOnly}
                  spellCheck={false}
                  wrap="off"
                  onScroll={(e) => {
                    if (gutterRef.current) gutterRef.current.scrollTop = e.currentTarget.scrollTop;
                  }}
                  onKeyDown={(e) => {
                    if (e.key === "Tab" && !readOnly) {
                      e.preventDefault();
                      const el = e.currentTarget;
                      const { selectionStart: a, selectionEnd: b, value } = el;
                      const next = value.slice(0, a) + "  " + value.slice(b);
                      const path = active;
                      setSt((cur) => (cur ? { ...cur, files: { ...cur.files, [path]: { ...cur.files[path], content: next } } } : cur));
                      requestAnimationFrame(() => {
                        if (textRef.current) textRef.current.selectionStart = textRef.current.selectionEnd = a + 2;
                      });
                    }
                  }}
                  onChange={(e) => {
                    const v = e.target.value;
                    const path = active;
                    setSt((cur) => (cur ? { ...cur, files: { ...cur.files, [path]: { ...cur.files[path], content: v } } } : cur));
                  }}
                />
              </div>
  ) : null;
  const menuIsEntry = menu?.path === SKILL_ENTRY;

  return (
    <div className="skill-editor">
      <div className="skill-editor-head">
        <button type="button" className="ghost skill-back" onClick={() => void goBack()} title="返回技能列表">
          <ArrowLeft size={16} />
        </button>
        <div className="skill-editor-title">
          <span>{pkg.name}</span>
          <span className={`skill-source-pill ${pkg.source}`}>{pkg.source === "builtin" ? "内置" : "自建"}</span>
          {dirty && !readOnly ? <span className="skill-editor-dirty">未保存{isStructureDirty(st) ? "（含结构变更）" : ""}</span> : null}
        </div>
        <div className="skill-editor-actions">
          {isMd ? (
            <div className="skill-seg" role="group" aria-label="编辑或预览">
              <button type="button" className={!preview ? "on" : ""} onClick={() => setPreview(false)}>
                <Pencil size={13} /> 编辑
              </button>
              <button type="button" className={preview ? "on" : ""} onClick={() => setPreview(true)}>
                <Eye size={13} /> 预览
              </button>
            </div>
          ) : null}
          {!readOnly ? (
            <button
              type="button"
              className="primary"
              disabled={saving || !dirty}
              title="保存全部（Ctrl/⌘+S）"
              onClick={() => void doSave()}
            >
              <Save size={14} /> {saving ? "保存中…" : "保存全部"}
            </button>
          ) : null}
        </div>
      </div>
      {readOnly ? (
        <div className="skill-readonly-banner">
          内置技能，可复制为自建后编辑。
          {onCopyAsCustom ? (
            <button type="button" className="ghost" onClick={() => onCopyAsCustom(pkg)}>
              复制为自建
            </button>
          ) : null}
        </div>
      ) : null}
      {saveIssues.length ? (
        <div className="skill-fm-errors" role="alert">
          <strong>SKILL.md frontmatter 有误，已拦截保存：</strong>
          <ul>
            {saveIssues.map((i, k) => (
              <li key={k}>
                第 {i.line} 行：{i.message}
              </li>
            ))}
          </ul>
        </div>
      ) : null}
      {msg ? <div className="skill-editor-msg">{msg}</div> : null}
      <div className="skill-editor-body">
        <aside
          className={`skill-tree ${dragOver === "" ? "drop" : ""}`}
          onDragOver={(e) => onDragOver(e, null)}
          onDrop={(e) => onDrop(e, null)}
          onContextMenu={(e) => {
            if (e.target === e.currentTarget) {
              e.preventDefault();
              setMenu({ x: e.clientX, y: e.clientY, path: "", isFolder: true });
            }
          }}
        >
          <div className="skill-tree-root">
            <button
              type="button"
              className="skill-tree-root-name"
              onClick={() =>
                setExpanded((e) => {
                  const next = new Set(e);
                  if (next.has("")) next.delete("");
                  else next.add("");
                  return next;
                })
              }
            >
              {expanded.has("") ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
              <span>{pkg.name}</span>
            </button>
            {!readOnly ? (
              <span className="skill-tree-root-actions">
                <button type="button" title="新建文件" onClick={() => startPending({ kind: "new-file", dir: "" })}>
                  <FilePlus size={14} />
                </button>
                <button type="button" title="新建文件夹" onClick={() => startPending({ kind: "new-folder", dir: "" })}>
                  <FolderPlus size={14} />
                </button>
              </span>
            ) : null}
          </div>
          {expanded.has("") ? renderNodes(tree, 1, "") : null}
        </aside>
        <section className="skill-pane">
          <div className="skill-tabs" role="tablist">
            {tabs.map((t) => {
              const f = st.files[t];
              const d = !readOnly && isFileDirty(f);
              return (
                <div
                  key={t}
                  role="tab"
                  aria-selected={t === active}
                  className={`skill-tab ${t === active ? "active" : ""}`}
                  onClick={() => openFile(t)}
                  onAuxClick={(e) => {
                    if (e.button === 1) void closeTab(t);
                  }}
                  title={t}
                >
                  <span>{t.split("/").pop()}</span>
                  {d ? <span className="skill-dirty-dot" aria-label="未保存" /> : null}
                  <button
                    type="button"
                    className="skill-tab-x"
                    aria-label="关闭标签"
                    onClick={(e) => {
                      e.stopPropagation();
                      void closeTab(t);
                    }}
                  >
                    <X size={12} />
                  </button>
                </div>
              );
            })}
          </div>
          {activeFile ? (
            preview && isMd ? (
              <div className="skill-preview">
                {active === SKILL_ENTRY && fmInfo ? (
                  <div className="skill-preview-fm">
                    <div>
                      <b>name</b> {fmInfo.name || "—"}
                    </div>
                    <div>
                      <b>description</b> {fmInfo.description || "—"}
                    </div>
                  </div>
                ) : null}
                <MarkdownMessage content={active === SKILL_ENTRY ? stripFrontmatter(activeFile.content) : activeFile.content} />
              </div>
            ) : (
              <Suspense fallback={plainEditor}>
                <CodeEditor
                  path={active}
                  value={activeFile.content}
                  readOnly={readOnly}
                  errorLines={errorLineList}
                  onChange={(v) => {
                    const path = active;
                    setSt((cur) => (cur && cur.files[path] ? { ...cur, files: { ...cur.files, [path]: { ...cur.files[path], content: v } } } : cur));
                  }}
                />
              </Suspense>
            )
          ) : (
            <div className="skill-pane-empty">从左侧选择文件</div>
          )}
        </section>
      </div>

      {menu ? (
        <div className="skill-ctx-menu" style={{ left: menu.x, top: menu.y }} onClick={(e) => e.stopPropagation()}>
          {!menu.isFolder ? (
            <button type="button" onClick={() => (openFile(menu.path), setMenu(null))}>
              <FileText size={13} /> 在编辑器打开
            </button>
          ) : null}
          <button
            type="button"
            disabled={readOnly}
            onClick={() => {
              startPending({ kind: "new-file", dir: menu.isFolder ? menu.path : dirname(menu.path) });
              setMenu(null);
            }}
          >
            <FilePlus size={13} /> 新建文件
          </button>
          <button
            type="button"
            disabled={readOnly}
            onClick={() => {
              startPending({ kind: "new-folder", dir: menu.isFolder ? menu.path : dirname(menu.path) });
              setMenu(null);
            }}
          >
            <FolderPlus size={13} /> 新建文件夹
          </button>
          {menu.path ? (
            <>
              <button
                type="button"
                disabled={readOnly || menuIsEntry}
                title={menuIsEntry ? "入口文件不可改名" : undefined}
                onClick={() => {
                  startPending({ kind: "rename", path: menu.path });
                  setMenu(null);
                }}
              >
                <Pencil size={13} /> 重命名
              </button>
              <button
                type="button"
                className="danger"
                disabled={readOnly || menuIsEntry}
                title={menuIsEntry ? "入口文件不可删" : undefined}
                onClick={() => {
                  const m = menu;
                  setMenu(null);
                  void removePath(m.path, m.isFolder);
                }}
              >
                <Trash2 size={13} /> 删除
              </button>
            </>
          ) : null}
        </div>
      ) : null}

      {closeAsk ? (
        <div className="skill-choice-backdrop" onClick={() => closeAsk.resolve("cancel")}>
          <div className="skill-choice" role="alertdialog" aria-modal onClick={(e) => e.stopPropagation()}>
            <h4>「{closeAsk.path}」有未保存的更改</h4>
            <p>关闭前要保存吗？（保存会写回整个技能包）</p>
            <div className="skill-choice-actions">
              <button type="button" className="ghost" onClick={() => closeAsk.resolve("cancel")}>
                取消
              </button>
              <button type="button" className="ghost danger" onClick={() => closeAsk.resolve("discard")}>
                不保存
              </button>
              <button type="button" className="primary" onClick={() => closeAsk.resolve("save")}>
                保存
              </button>
            </div>
          </div>
        </div>
      ) : null}
    </div>
  );
}
