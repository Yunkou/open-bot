import { useEffect, useMemo, useRef, useState, type RefObject } from "react";
import type { Agent } from "../api";
import { AgentAvatar } from "./AgentAvatar";

export type CreateBotInput = {
  name: string;
  description: string;
  system_prompt: string;
};

export type NewChatPopoverProps = {
  open: boolean;
  agents: Agent[];
  anchorRef: RefObject<HTMLElement | null>;
  onClose: () => void;
  onSelectAgent: (agent: Agent) => void | Promise<void>;
  onCreateBot: (input: CreateBotInput) => void | Promise<void>;
  onCreateGroup: (name: string, memberIds: string[]) => void | Promise<void>;
};

type Mode = "list" | "group" | "bot";

export function NewChatPopover({
  open,
  agents,
  anchorRef,
  onClose,
  onSelectAgent,
  onCreateBot,
  onCreateGroup,
}: NewChatPopoverProps) {
  const panelRef = useRef<HTMLDivElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  const [query, setQuery] = useState("");
  const [mode, setMode] = useState<Mode>("list");
  const [groupName, setGroupName] = useState("");
  const [memberIds, setMemberIds] = useState<string[]>([]);
  const [botName, setBotName] = useState("");
  const [botDesc, setBotDesc] = useState("");
  const [botPrompt, setBotPrompt] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [pos, setPos] = useState<{ top: number; left: number }>({ top: 24, left: 300 });

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return agents;
    return agents.filter((a) => {
      const hay = `${a.name} ${a.description || ""} ${a.id}`.toLowerCase();
      return hay.includes(q);
    });
  }, [agents, query]);

  useEffect(() => {
    if (!open) return;
    setQuery("");
    setMode("list");
    setGroupName("");
    setMemberIds([]);
    setBotName("");
    setBotDesc("");
    setBotPrompt("");
    setBusy(false);
    setError("");
    const place = () => {
      const el = anchorRef.current;
      if (!el) return;
      const r = el.getBoundingClientRect();
      const left = Math.min(r.right + 12, window.innerWidth - 380);
      const top = Math.max(12, Math.min(r.top - 4, window.innerHeight - 520));
      setPos({ top, left: Math.max(12, left) });
    };
    place();
    const t = window.setTimeout(() => searchRef.current?.focus(), 30);
    window.addEventListener("resize", place);
    return () => {
      window.clearTimeout(t);
      window.removeEventListener("resize", place);
    };
  }, [open, anchorRef]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        if (mode === "group" || mode === "bot") {
          setMode("list");
          setError("");
        } else {
          onClose();
        }
      }
    };
    const onPointer = (e: MouseEvent) => {
      const t = e.target as Node;
      if (panelRef.current?.contains(t)) return;
      if (anchorRef.current?.contains(t)) return;
      onClose();
    };
    document.addEventListener("keydown", onKey);
    document.addEventListener("mousedown", onPointer);
    return () => {
      document.removeEventListener("keydown", onKey);
      document.removeEventListener("mousedown", onPointer);
    };
  }, [open, onClose, mode, anchorRef]);

  if (!open) return null;

  const toggleMember = (id: string) => {
    setMemberIds((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]));
  };

  const submitGroup = async () => {
    const name = groupName.trim();
    if (!name) {
      setError("请输入群聊名称");
      return;
    }
    if (memberIds.length === 0) {
      setError("请至少选择一名助手");
      return;
    }
    setBusy(true);
    setError("");
    try {
      await onCreateGroup(name, memberIds);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setBusy(false);
    }
  };

  const submitBot = async () => {
    const name = botName.trim();
    if (!name) {
      setError("请输入 Bot 名称");
      return;
    }
    setBusy(true);
    setError("");
    try {
      await onCreateBot({
        name,
        description: botDesc.trim(),
        system_prompt: botPrompt.trim(),
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
      setBusy(false);
    }
  };

  const backToList = () => {
    setMode("list");
    setError("");
  };

  return (
    <div className="new-chat-layer" role="presentation">
      <div
        ref={panelRef}
        className="new-chat-popover"
        style={{ top: pos.top, left: pos.left }}
        role="dialog"
        aria-label="选择助手"
      >
        {mode === "list" ? (
          <>
            <label className="new-chat-search">
              <span className="new-chat-search-prefix">收件人：</span>
              <input
                ref={searchRef}
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="搜索或创建 Bot"
                aria-label="搜索或创建 Bot"
              />
            </label>

            <div className="new-chat-actions">
              <button
                type="button"
                className="new-chat-action"
                onClick={() => {
                  setMode("bot");
                  setError("");
                  setBotName(query.trim());
                }}
              >
                <span className="new-chat-action-icon" aria-hidden>
                  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                    <path d="M12 5v14M5 12h14" />
                  </svg>
                </span>
                <span>创建新 Bot</span>
              </button>
              <button
                type="button"
                className="new-chat-action"
                onClick={() => {
                  setMode("group");
                  setError("");
                  setGroupName(query.trim());
                }}
              >
                <span className="new-chat-action-icon" aria-hidden>
                  <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
                    <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
                    <circle cx="9" cy="7" r="4" />
                    <path d="M23 21v-2a4 4 0 0 0-3-3.87M16 3.13a4 4 0 0 1 0 7.75" />
                  </svg>
                </span>
                <span>创建群聊</span>
              </button>
            </div>

            <div className="new-chat-list" role="listbox" aria-label="助手列表">
              {filtered.length === 0 ? (
                <div className="new-chat-empty muted">无匹配助手</div>
              ) : (
                filtered.map((a) => (
                  <button
                    key={a.id}
                    type="button"
                    className="new-chat-row"
                    role="option"
                    onClick={() => void onSelectAgent(a)}
                  >
                    <AgentAvatar id={a.id} name={a.name} size={36} />
                    <div className="new-chat-row-main">
                      <div className="new-chat-row-name">{a.name}</div>
                      {a.description ? (
                        <div className="new-chat-row-desc">{a.description}</div>
                      ) : null}
                    </div>
                    <span className="new-chat-row-cta">打开对话</span>
                  </button>
                ))
              )}
            </div>
          </>
        ) : mode === "bot" ? (
          <div className="new-chat-group">
            <div className="new-chat-group-head">
              <button type="button" className="ghost new-chat-back" onClick={backToList}>
                ← 返回
              </button>
              <h4>创建新 Bot</h4>
            </div>
            <label className="new-chat-field">
              名称
              <input
                autoFocus
                value={botName}
                onChange={(e) => setBotName(e.target.value)}
                placeholder="Bot 名称（必填）"
              />
            </label>
            <label className="new-chat-field">
              描述（可选）
              <input
                value={botDesc}
                onChange={(e) => setBotDesc(e.target.value)}
                placeholder="简短介绍"
              />
            </label>
            <label className="new-chat-field">
              人设 / System Prompt（可选）
              <textarea
                className="new-chat-textarea"
                rows={4}
                value={botPrompt}
                onChange={(e) => setBotPrompt(e.target.value)}
                placeholder="你是……"
              />
            </label>
            {error ? <div className="new-chat-error">{error}</div> : null}
            <button
              type="button"
              className="primary new-chat-group-submit"
              disabled={busy}
              onClick={() => void submitBot()}
            >
              {busy ? "创建中…" : "创建并开始聊天"}
            </button>
          </div>
        ) : (
          <div className="new-chat-group">
            <div className="new-chat-group-head">
              <button type="button" className="ghost new-chat-back" onClick={backToList}>
                ← 返回
              </button>
              <h4>创建群聊</h4>
            </div>
            <label className="new-chat-field">
              名称
              <input
                autoFocus
                value={groupName}
                onChange={(e) => setGroupName(e.target.value)}
                placeholder="群聊名称"
              />
            </label>
            <div className="new-chat-members-label">选择成员（助手）</div>
            <div className="new-chat-members">
              {agents.map((a) => {
                const checked = memberIds.includes(a.id);
                return (
                  <label key={a.id} className={`new-chat-member ${checked ? "on" : ""}`}>
                    <input
                      type="checkbox"
                      checked={checked}
                      onChange={() => toggleMember(a.id)}
                    />
                    <AgentAvatar id={a.id} name={a.name} size={28} />
                    <span>{a.name}</span>
                  </label>
                );
              })}
            </div>
            {error ? <div className="new-chat-error">{error}</div> : null}
            <button
              type="button"
              className="primary new-chat-group-submit"
              disabled={busy}
              onClick={() => void submitGroup()}
            >
              {busy ? "创建中…" : "创建群聊"}
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
