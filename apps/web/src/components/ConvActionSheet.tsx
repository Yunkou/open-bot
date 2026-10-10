import { useEffect } from "react";

type Props = {
  open: boolean;
  title?: string;
  onClose: () => void;
  onDelete: () => void;
  /** When set (agent long-press), show non-danger「复制助手」above delete. */
  onClone?: () => void;
};

/**
 * Bottom sheet for conversation / agent / channel row long-press (touch UI).
 * Mirrors MessageActionSheet chrome; agents get 复制助手 + destructive delete.
 */
export function ConvActionSheet({ open, title, onClose, onDelete, onClone }: Props) {
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
      <div className="action-sheet" role="dialog" aria-modal="true" aria-label={title || "会话操作"}>
        <div className="action-sheet-handle" aria-hidden />
        {title ? <div className="action-sheet-title">{title}</div> : null}
        {onClone ? (
          <button
            type="button"
            className="action-sheet-row"
            onClick={() => {
              onClose();
              onClone();
            }}
          >
            复制助手
          </button>
        ) : null}
        <button
          type="button"
          className="action-sheet-row action-sheet-row-danger"
          onClick={() => {
            onClose();
            onDelete();
          }}
        >
          删除会话
        </button>
        <button type="button" className="action-sheet-row action-sheet-cancel" onClick={onClose}>
          取消
        </button>
      </div>
    </div>
  );
}
