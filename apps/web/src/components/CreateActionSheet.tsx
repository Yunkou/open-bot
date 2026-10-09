import { useEffect } from "react";

type Props = {
  open: boolean;
  onClose: () => void;
  onCreateBot: () => void;
  onCreateGroup: () => void;
};

/**
 * Touch / layout-narrow 「+」 Action Sheet (C-1).
 * Only 新建 Bot / 新建群聊 — machine picker stays in create flow, not here.
 */
export function CreateActionSheet({ open, onClose, onCreateBot, onCreateGroup }: Props) {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.removeEventListener("keydown", onKey);
      document.body.style.overflow = prev;
    };
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div className="action-sheet-root" role="presentation">
      <button type="button" className="action-sheet-mask" aria-label="关闭" onClick={onClose} />
      <div className="action-sheet create-action-sheet" role="dialog" aria-modal="true" aria-label="新建">
        <div className="action-sheet-handle" aria-hidden />
        <button
          type="button"
          className="action-sheet-row action-sheet-row-icon"
          onClick={() => {
            onClose();
            onCreateBot();
          }}
        >
          <span className="action-sheet-row-leading" aria-hidden>
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M12 5v14M5 12h14" />
            </svg>
          </span>
          新建 Bot
        </button>
        <button
          type="button"
          className="action-sheet-row action-sheet-row-icon"
          onClick={() => {
            onClose();
            onCreateGroup();
          }}
        >
          <span className="action-sheet-row-leading" aria-hidden>
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
              <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
              <circle cx="9" cy="7" r="4" />
              <path d="M23 21v-2a4 4 0 0 0-3-3.87M16 3.13a4 4 0 0 1 0 7.75" />
            </svg>
          </span>
          新建群聊
        </button>
        <div className="action-sheet-sep" />
        <button type="button" className="action-sheet-row action-sheet-cancel" onClick={onClose}>
          取消
        </button>
      </div>
    </div>
  );
}
