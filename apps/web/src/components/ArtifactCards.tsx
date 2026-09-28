import { useMemo, useState } from "react";
import { readSandboxFile } from "../api";
import { HtmlPreviewModal, friendlyOpenError, looksLikeHtml, normalizeWorkspacePath, previewTitleFromPath } from "./HtmlPreviewModal";
import { MarkdownMessage } from "./MarkdownMessage";

/** Friendly label for artifact cards (hide internal /workspace prefix). */
export function artifactDisplayName(path: string): string {
  return previewTitleFromPath(path);
}

/** Extract internal workspace paths and JSON tool blobs from assistant text. */
export function splitResultOriented(content: string): {
  display: string;
  artifacts: { path: string; kind: string }[];
  collapsedJson: string[];
} {
  const artifacts: { path: string; kind: string }[] = [];
  const collapsedJson: string[] = [];
  let display = content || "";

  const pathRe = /(?:sandbox:(?:\/\/)?)?(\/workspace\/[^\s`"'<>]+)/gi;
  let m: RegExpExecArray | null;
  while ((m = pathRe.exec(content || "")) !== null) {
    const path = normalizeWorkspacePath(m[1].replace(/[.,;:!?)]+$/, ""));
    if (!artifacts.some((a) => a.path === path)) {
      artifacts.push({ path, kind: path.split(".").pop() || "file" });
    }
  }

  display = display.replace(/```(?:json)?\s*([\s\S]*?)```/gi, (full, body: string) => {
    const trimmed = body.trim();
    if (!trimmed.startsWith("{") && !trimmed.startsWith("[")) return full;
    try {
      const parsed = JSON.parse(trimmed) as Record<string, unknown>;
      if (parsed && typeof parsed === "object") {
        if (typeof parsed.path === "string" && (parsed.ok === true || typeof parsed.content === "string")) {
          const path = normalizeWorkspacePath(String(parsed.path));
          if (!artifacts.some((a) => a.path === path)) {
            artifacts.push({ path, kind: "write" });
          }
        }
        collapsedJson.push(trimmed.length > 400 ? `${trimmed.slice(0, 400)}…` : trimmed);
        return "\n\n";
      }
    } catch {
      /* keep */
    }
    return full;
  });

  // Hide bare /workspace/ prefixes in chat body; leave sandbox:… hrefs intact for click handlers.
  display = display.replace(/(sandbox:(?:\/\/)?)?\/workspace\//gi, (_full, proto?: string) =>
    proto ? `${proto}/workspace/` : "",
  );

  return { display, artifacts, collapsedJson };
}

export function ArtifactCards({
  artifacts,
  collapsedJson,
  agentId,
}: {
  artifacts: { path: string; kind: string }[];
  collapsedJson: string[];
  agentId?: string;
}) {
  const [preview, setPreview] = useState<{
    open: boolean;
    title: string;
    html: string | null;
    loading: boolean;
    error: string | null;
  }>({ open: false, title: "", html: null, loading: false, error: null });
  const [openJson, setOpenJson] = useState(false);

  const openPath = async (path: string) => {
    const wp = normalizeWorkspacePath(path);
    setPreview({ open: true, title: previewTitleFromPath(wp), html: null, loading: true, error: null });
    try {
      const res = await readSandboxFile(wp, agentId ? { agent_id: agentId } : undefined);
      const content = res.content || "";
      if (looksLikeHtml(content, wp)) {
        setPreview({ open: true, title: previewTitleFromPath(wp), html: content, loading: false, error: null });
      } else {
        setPreview({
          open: true,
          title: previewTitleFromPath(wp),
          html: `<pre style="white-space:pre-wrap;font:12px/1.4 ui-monospace,monospace;padding:12px">${escapeHtml(content)}</pre>`,
          loading: false,
          error: null,
        });
      }
    } catch (e) {
      setPreview({
        open: true,
        title: previewTitleFromPath(wp),
        html: null,
        loading: false,
        error: friendlyOpenError(e),
      });
    }
  };

  if (!artifacts.length && !collapsedJson.length) return null;

  return (
    <div className="artifact-row">
      {artifacts.map((a) => (
        <button key={a.path} type="button" className="artifact-card" onClick={() => void openPath(a.path)}>
          <span className="artifact-kind">{a.kind}</span>
          <span className="artifact-path">{artifactDisplayName(a.path)}</span>
          <span className="artifact-action">打开文件</span>
        </button>
      ))}
      {collapsedJson.length > 0 ? (
        <button type="button" className="tool-chip" onClick={() => setOpenJson((v) => !v)}>
          {openJson ? "收起工具详情" : `工具结果 ×${collapsedJson.length}`}
        </button>
      ) : null}
      {openJson
        ? collapsedJson.map((j, i) => (
            <pre key={i} className="tool-json-collapsed">
              {j}
            </pre>
          ))
        : null}
      <HtmlPreviewModal
        open={preview.open}
        title={preview.title}
        html={preview.html}
        loading={preview.loading}
        error={preview.error}
        onClose={() => setPreview((p) => ({ ...p, open: false }))}
      />
    </div>
  );
}

function escapeHtml(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

export function ResultOrientedMessage({
  content,
  streaming,
  agentId,
}: {
  content: string;
  streaming?: boolean;
  agentId?: string;
}) {
  const { display, artifacts, collapsedJson } = useMemo(() => splitResultOriented(content), [content]);
  return (
    <>
      <MarkdownMessage content={display.trim()} streaming={streaming} agentId={agentId} />
      {!streaming ? (
        <ArtifactCards artifacts={artifacts} collapsedJson={collapsedJson} agentId={agentId} />
      ) : null}
    </>
  );
}
