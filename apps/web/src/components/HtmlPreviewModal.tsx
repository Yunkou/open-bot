import { useEffect } from "react";
import { createPortal } from "react-dom";

type Props = {
  open: boolean;
  title: string;
  html: string | null;
  loading?: boolean;
  error?: string | null;
  onClose: () => void;
};

/** Safe in-app HTML preview: srcDoc + sandbox without allow-same-origin. */
export function HtmlPreviewModal({ open, title, html, loading, error, onClose }: Props) {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);

  if (!open) return null;

  return createPortal(
    <div className="modal-backdrop preview-backdrop" onClick={onClose} role="presentation">
      <div
        className="modal preview-modal"
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-modal="true"
        aria-label={title || "HTML 预览"}
      >
        <div className="modal-head">
          <h3>{title || "HTML 预览"}</h3>
          <button type="button" className="ghost" onClick={onClose}>
            关闭
          </button>
        </div>
        {loading ? <div className="preview-status">加载中…</div> : null}
        {error ? <div className="preview-error">{error}</div> : null}
        {!loading && !error && html != null ? (
          <iframe
            className="preview-iframe"
            title={title || "HTML 预览"}
            sandbox="allow-scripts allow-forms allow-modals"
            referrerPolicy="no-referrer"
            srcDoc={html}
          />
        ) : null}
      </div>
    </div>,
    document.body,
  );
}

export function looksLikeHtml(content: string, hintPathOrLang?: string): boolean {
  const hint = (hintPathOrLang || "").toLowerCase();
  if (/\.(html?|xhtml|svg)$/.test(hint) || hint === "html" || hint === "htm" || hint === "svg") {
    return true;
  }
  const head = content.trimStart().slice(0, 256).toLowerCase();
  return (
    head.startsWith("<!doctype html") ||
    head.startsWith("<html") ||
    head.startsWith("<svg") ||
    (head.startsWith("<") && /<(html|head|body|div|style|meta)\b/.test(head))
  );
}

/** Map any user/tool path onto /workspace/... (never leave bare /tetris.html). */
export function normalizeWorkspacePath(raw: string): string {
  let p = (raw || "").trim();
  if (!p) return "/workspace";
  p = p.replace(/^sandbox:(?:\/\/)?/i, "").trim();
  if (!p) return "/workspace";
  if (!p.startsWith("/")) {
    p = `/workspace/${p}`;
  } else if (p !== "/workspace" && !p.startsWith("/workspace/")) {
    // Absolute but outside workspace root → treat as workspace-relative.
    p = `/workspace${p}`;
  }
  // Collapse duplicate slashes; keep leading /
  p = p.replace(/\/+/g, "/");
  if (p.length > 1 && p.endsWith("/")) p = p.slice(0, -1);
  return p;
}

/** Parse sandbox:/workspace/foo.html → /workspace/foo.html */
export function parseSandboxHref(href: string | undefined | null): string | null {
  if (!href) return null;
  const trimmed = href.trim();
  const m = /^sandbox:(?:\/\/)?(.+)$/i.exec(trimmed);
  if (!m) return null;
  const path = normalizeWorkspacePath(m[1]);
  return path || null;
}

export function previewTitleFromPath(path: string): string {
  const base = path.split("/").filter(Boolean).pop() || path;
  return base;
}

/** Soften API/OS errors so UI never dumps host or /workspace paths. */
export function friendlyOpenError(err: unknown): string {
  const raw = err instanceof Error ? err.message : String(err ?? "");
  const lower = raw.toLowerCase();
  if (
    /no such file|not found|enoent|does not exist|path must stay|is a directory/i.test(lower) ||
    /\/workspace|sandbox|docker|container|data\/sandboxes/i.test(raw)
  ) {
    return "无法打开文件，请确认文件仍存在后重试";
  }
  if (!raw.trim() || raw.length > 160 || /[/\\]/.test(raw)) {
    return "无法打开文件";
  }
  return raw;
}
