import { FormEvent, useEffect, useState } from "react";
import type { Agent } from "../api";
import {
  AVATAR_COLOR_PALETTE,
  AVATAR_SHAPES,
  resolveAvatarColor,
  resolveAvatarShape,
  type AvatarShape,
  type BotPresenceStatus,
} from "./avatarColor";
import { AgentAvatar } from "./AgentAvatar";

type Props = {
  agent: Agent | null;
  open: boolean;
  onClose: () => void;
  onSave: (agentId: string, patch: { avatar_shape: string; avatar_color: string }) => Promise<void>;
};

const PREVIEW_STATUSES: { id: BotPresenceStatus; label: string }[] = [
  { id: "idle", label: "idle" },
  { id: "working", label: "working" },
  { id: "awaiting_approval", label: "awaiting" },
  { id: "error", label: "error" },
];

export function BotAvatarSettings({ agent, open, onClose, onSave }: Props) {
  const [shape, setShape] = useState<AvatarShape>("cloud");
  const [color, setColor] = useState("#457b9d");
  const [previewStatus, setPreviewStatus] = useState<BotPresenceStatus>("idle");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  useEffect(() => {
    if (!agent || !open) return;
    const seed = agent.id || agent.name;
    setShape(resolveAvatarShape(seed, agent.avatar_shape));
    setColor(resolveAvatarColor(seed, agent.avatar_color));
    setPreviewStatus("idle");
    setErr("");
  }, [agent, open]);

  if (!open || !agent) return null;

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setErr("");
    try {
      await onSave(agent.id, { avatar_shape: shape, avatar_color: color });
      onClose();
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : String(ex));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal bot-avatar-modal" onClick={(e) => e.stopPropagation()}>
        <div className="modal-head">
          <h3>Bot 形象 · {agent.name}</h3>
          <button type="button" className="ghost" onClick={onClose}>
            关闭
          </button>
        </div>
        <form className="bot-avatar-form" onSubmit={(e) => void submit(e)}>
          <div className="bot-avatar-preview">
            <AgentAvatar
              id={agent.id}
              name={agent.name}
              size={64}
              shape={shape}
              color={color}
              status={previewStatus}
            />
            <div className="bot-avatar-status-toggles" role="group" aria-label="预览表情">
              {PREVIEW_STATUSES.map((s) => (
                <button
                  key={s.id}
                  type="button"
                  className={`bot-avatar-status-opt${previewStatus === s.id ? " active" : ""}`}
                  onClick={() => setPreviewStatus(s.id)}
                >
                  {s.label}
                </button>
              ))}
            </div>
            <div className="muted small">预览四态（仅本地试看，不改真实 presence）</div>
          </div>

          <label className="bot-avatar-label">形状</label>
          <div className="bot-avatar-shape-grid">
            {AVATAR_SHAPES.map((s) => (
              <button
                key={s}
                type="button"
                className={`bot-avatar-shape-opt${shape === s ? " active" : ""}`}
                onClick={() => setShape(s)}
                title={s}
                aria-label={s}
              >
                <AgentAvatar id={agent.id} name={agent.name} size={36} shape={s} color={color} />
              </button>
            ))}
          </div>

          <label className="bot-avatar-label">主色</label>
          <div className="bot-avatar-color-grid">
            {AVATAR_COLOR_PALETTE.map((c) => (
              <button
                key={c}
                type="button"
                className={`bot-avatar-color-opt${color.toLowerCase() === c.toLowerCase() ? " active" : ""}`}
                style={{ background: c }}
                onClick={() => setColor(c)}
                title={c}
                aria-label={c}
              />
            ))}
          </div>

          {err ? <div className="auth-error">{err}</div> : null}
          <div className="llm-actions">
            <button type="submit" className="primary" disabled={busy}>
              {busy ? "保存中…" : "保存"}
            </button>
            <button type="button" onClick={onClose} disabled={busy}>
              取消
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
