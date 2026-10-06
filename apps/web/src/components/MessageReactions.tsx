import { useState } from "react";
import { REACTION_EMOJIS, type ReactionSummary } from "../api";

type Props = {
  /** Server may send null for messages without reactions. */
  reactions?: ReactionSummary[] | null;
  /** When false, hide interactive controls (local/temp messages). */
  interactive?: boolean;
  onToggle?: (emoji: string) => void | Promise<void>;
};

export function MessageReactions({ reactions, interactive = true, onToggle }: Props) {
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const visible = (reactions ?? []).filter((r) => r.count > 0);

  const handleToggle = async (emoji: string) => {
    if (!interactive || !onToggle || busy) return;
    setBusy(true);
    try {
      await onToggle(emoji);
      setOpen(false);
    } finally {
      setBusy(false);
    }
  };

  if (!interactive && visible.length === 0) return null;

  return (
    <div className={`msg-reactions${open ? " open" : ""}`}>
      {visible.length > 0 ? (
        <div className="msg-reaction-chips" role="list">
          {visible.map((r) => (
            <button
              key={r.emoji}
              type="button"
              className={`msg-reaction-chip${r.me ? " me" : ""}`}
              role="listitem"
              disabled={!interactive || busy}
              title={r.me ? "取消反应" : "添加反应"}
              onClick={() => void handleToggle(r.emoji)}
            >
              <span className="msg-reaction-emoji">{r.emoji}</span>
              <span className="msg-reaction-count">{r.count}</span>
            </button>
          ))}
        </div>
      ) : null}
      {interactive ? (
        <div className="msg-reaction-add-wrap">
          <button
            type="button"
            className="msg-reaction-add"
            title="添加反应"
            aria-expanded={open}
            disabled={busy}
            onClick={() => setOpen((v) => !v)}
          >
            +
          </button>
          {open ? (
            <div className="msg-reaction-picker" role="menu">
              {REACTION_EMOJIS.map((emoji) => (
                <button
                  key={emoji}
                  type="button"
                  className="msg-reaction-pick"
                  role="menuitem"
                  disabled={busy}
                  onClick={() => void handleToggle(emoji)}
                >
                  {emoji}
                </button>
              ))}
            </div>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}
