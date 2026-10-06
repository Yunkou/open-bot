import { FormEvent, useEffect, useState } from "react";
import type { Agent } from "../api";
import {
  AVATAR_COLOR_PALETTE,
  AVATAR_SHAPES,
  resolveAvatarColor,
  resolveAvatarShape,
  type AvatarShape,
} from "./avatarColor";
import { AgentAvatar } from "./AgentAvatar";

type Props = {
  agent: Agent | null;
  open: boolean;
  onClose: () => void;
  onSave: (agentId: string, patch: { avatar_shape: string; avatar_color: string }) => Promise<void>;
};

export function BotAvatarSettings({ agent, open, onClose, onSave }: Props) {
  const [shape, setShape] = useState<AvatarShape>("rounded");
  const [color, setColor] = useState("#457b9d");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  useEffect(() => {
    if (!agent || !open) return;
    const seed = agent.id || agent.name;
    setShape(resolveAvatarShape(seed, agent.avatar_shape));
    setColor(resolveAvatarColor(seed, agent.avatar_color));
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
              status="idle"
            />
            <div className="muted small">预览（工作态动效在侧边栏/顶栏实时显示）</div>
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
