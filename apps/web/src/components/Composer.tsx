import { ChangeEvent, FormEvent, KeyboardEvent, useMemo, useRef, useState } from "react";
import type { Agent } from "../api";
import { AgentAvatar } from "./AgentAvatar";

export type PendingFile = {
  key: string;
  file: File;
};

type Props = {
  value: string;
  onChange: (value: string) => void;
  onSubmit: (e: FormEvent) => void;
  /** Cancel the in-flight run (Stop). */
  onStop?: () => void;
  disabled?: boolean;
  sending?: boolean;
  agentName?: string;
  files?: PendingFile[];
  onFilesChange?: (files: PendingFile[]) => void;
  /** When set, typing @ shows member autocomplete (group chat). */
  mentionMembers?: Agent[];
};

function formatSize(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MB`;
}

type MentionState = {
  start: number; // index of '@'
  query: string;
};

function detectMention(value: string, caret: number): MentionState | null {
  const before = value.slice(0, caret);
  const at = before.lastIndexOf("@");
  if (at < 0) return null;
  if (at > 0) {
    const prev = before[at - 1];
    if (prev && !/\s/.test(prev)) return null;
  }
  const query = before.slice(at + 1);
  if (/\s/.test(query)) return null;
  return { start: at, query };
}

export function Composer({
  value,
  onChange,
  onSubmit,
  onStop,
  disabled,
  sending,
  agentName,
  files = [],
  onFilesChange,
  mentionMembers,
}: Props) {
  const fileRef = useRef<HTMLInputElement>(null);
  const taRef = useRef<HTMLTextAreaElement>(null);
  const [mention, setMention] = useState<MentionState | null>(null);
  const [mentionIndex, setMentionIndex] = useState(0);

  const placeholder = mentionMembers?.length
    ? `给群聊发消息，@ 点名助手`
    : `给 ${agentName || "助手"} 发消息`;
  const canSend = Boolean(value.trim() || files.length > 0);

  const mentionOptions = useMemo(() => {
    if (!mention || !mentionMembers?.length) return [];
    const q = mention.query.trim().toLowerCase();
    return mentionMembers.filter((a) => {
      if (!q) return true;
      return (
        a.name.toLowerCase().includes(q) ||
        a.id.toLowerCase().includes(q) ||
        (a.description || "").toLowerCase().includes(q)
      );
    }).slice(0, 8);
  }, [mention, mentionMembers]);

  const applyMention = (agent: Agent) => {
    if (!mention) return;
    const ta = taRef.current;
    const caret = ta?.selectionStart ?? value.length;
    const before = value.slice(0, mention.start);
    const after = value.slice(caret);
    const insert = `@${agent.name} `;
    const next = before + insert + after;
    onChange(next);
    setMention(null);
    setMentionIndex(0);
    requestAnimationFrame(() => {
      const pos = before.length + insert.length;
      ta?.focus();
      ta?.setSelectionRange(pos, pos);
    });
  };

  const onKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (mention && mentionOptions.length > 0) {
      if (e.key === "ArrowDown") {
        e.preventDefault();
        setMentionIndex((i) => (i + 1) % mentionOptions.length);
        return;
      }
      if (e.key === "ArrowUp") {
        e.preventDefault();
        setMentionIndex((i) => (i - 1 + mentionOptions.length) % mentionOptions.length);
        return;
      }
      if (e.key === "Enter" || e.key === "Tab") {
        e.preventDefault();
        applyMention(mentionOptions[mentionIndex] ?? mentionOptions[0]);
        return;
      }
      if (e.key === "Escape") {
        e.preventDefault();
        setMention(null);
        return;
      }
    }
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      if (sending) {
        // Busy: empty Enter = Stop; with content = keep-partial-next-turn interrupt-and-send (parent).
        if (canSend) {
          void onSubmit(e);
        } else {
          onStop?.();
        }
        return;
      }
      if (canSend) {
        void onSubmit(e);
      }
    }
  };

  const onPickFiles = (e: ChangeEvent<HTMLInputElement>) => {
    const list = e.target.files;
    if (!list || !onFilesChange) return;
    const next = [...files];
    for (const f of Array.from(list)) {
      next.push({ key: `${Date.now()}-${Math.random().toString(36).slice(2)}-${f.name}`, file: f });
    }
    onFilesChange(next);
    e.target.value = "";
  };

  const removeFile = (key: string) => {
    onFilesChange?.(files.filter((f) => f.key !== key));
  };

  const onTextChange = (next: string) => {
    onChange(next);
    const caret = taRef.current?.selectionStart ?? next.length;
    if (mentionMembers?.length) {
      const m = detectMention(next, caret);
      setMention(m);
      setMentionIndex(0);
    } else {
      setMention(null);
    }
  };

  return (
    <form className="composer" onSubmit={onSubmit}>
      {files.length > 0 ? (
        <div className="composer-chips">
          {files.map((f) => (
            <div key={f.key} className="composer-chip" title={f.file.name}>
              <span className="composer-chip-name">{f.file.name}</span>
              <span className="composer-chip-size">{formatSize(f.file.size)}</span>
              <button
                type="button"
                className="composer-chip-remove"
                title="移除"
                disabled={sending}
                onClick={() => removeFile(f.key)}
              >
                ×
              </button>
            </div>
          ))}
        </div>
      ) : null}
      <div className="composer-bar">
        <input
          ref={fileRef}
          type="file"
          multiple
          className="composer-file-input"
          onChange={onPickFiles}
          disabled={disabled || sending}
          aria-hidden
          tabIndex={-1}
        />
        <button
          type="button"
          className="composer-attach"
          title="添加附件"
          disabled={disabled || sending || !onFilesChange}
          onClick={() => fileRef.current?.click()}
        >
          +
        </button>
        <div className="composer-input-wrap">
          {mention && mentionOptions.length > 0 ? (
            <div className="mention-popup" role="listbox" aria-label="提及助手">
              {mentionOptions.map((a, i) => (
                <button
                  key={a.id}
                  type="button"
                  role="option"
                  aria-selected={i === mentionIndex}
                  className={`mention-option${i === mentionIndex ? " active" : ""}`}
                  onMouseDown={(ev) => {
                    ev.preventDefault();
                    applyMention(a);
                  }}
                >
                  <AgentAvatar id={a.id} name={a.name} size={24} />
                  <span className="mention-option-name">{a.name}</span>
                  <span className="mention-option-id">@{a.id}</span>
                </button>
              ))}
            </div>
          ) : null}
          <textarea
            ref={taRef}
            value={value}
            onChange={(e) => onTextChange(e.target.value)}
            onClick={() => {
              if (!mentionMembers?.length) return;
              const caret = taRef.current?.selectionStart ?? value.length;
              setMention(detectMention(value, caret));
            }}
            onKeyUp={() => {
              if (!mentionMembers?.length) return;
              const caret = taRef.current?.selectionStart ?? value.length;
              setMention(detectMention(value, caret));
            }}
            placeholder={placeholder}
            rows={1}
            onKeyDown={onKeyDown}
            disabled={disabled}
          />
        </div>
        {sending ? (
          <button
            type="button"
            className="composer-send composer-stop"
            title="停止生成"
            aria-label="停止生成"
            onClick={(ev) => {
              ev.preventDefault();
              onStop?.();
            }}
          >
            <span className="composer-stop-icon" aria-hidden />
          </button>
        ) : (
          <button
            type="submit"
            className="composer-send"
            disabled={!canSend}
            title="发送"
          >
            ↑
          </button>
        )}
      </div>
    </form>
  );
}
