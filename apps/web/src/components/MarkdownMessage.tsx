import { isValidElement, useCallback, useState, type ReactNode } from "react";
import ReactMarkdown, { defaultUrlTransform } from "react-markdown";
import remarkGfm from "remark-gfm";
import { readSandboxFile } from "../api";
import {
  HtmlPreviewModal,
  friendlyOpenError,
  looksLikeHtml,
  normalizeWorkspacePath,
  parseSandboxHref,
  previewTitleFromPath,
} from "./HtmlPreviewModal";

type Props = {
  content: string;
  streaming?: boolean;
  agentId?: string;
};

function extractText(node: ReactNode): string {
  if (node == null || typeof node === "boolean") return "";
  if (typeof node === "string" || typeof node === "number") return String(node);
  if (Array.isArray(node)) return node.map(extractText).join("");
  if (isValidElement<{ children?: ReactNode }>(node)) {
    return extractText(node.props.children);
  }
  return "";
}

/** Keep sandbox: links; defaultUrlTransform strips non-http(s)/mailto/irc. */
function urlTransform(url: string): string {
  const trimmed = (url || "").trim();
  if (/^sandbox:/i.test(trimmed)) return trimmed;
  return defaultUrlTransform(url);
}

const PREVIEWABLE_LANGS = new Set(["html", "htm", "svg", "xhtml"]);

type PreviewState = {
  open: boolean;
  title: string;
  html: string | null;
  loading: boolean;
  error: string | null;
};

const PREVIEW_CLOSED: PreviewState = {
  open: false,
  title: "",
  html: null,
  loading: false,
  error: null,
};

function CodeBlock({
  className,
  children,
  onPreview,
}: {
  className?: string;
  children?: ReactNode;
  onPreview: (html: string, title: string) => void;
}) {
  const match = /language-(\w+)/.exec(className || "");
  const lang = match?.[1] ?? "";
  const code = extractText(children).replace(/\n$/, "");
  const [copied, setCopied] = useState(false);
  const canPreview = PREVIEWABLE_LANGS.has(lang.toLowerCase()) || looksLikeHtml(code, lang);

  const onCopy = useCallback(() => {
    void navigator.clipboard.writeText(code).then(() => {
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    });
  }, [code]);

  return (
    <div className="md-codeblock">
      <div className="md-codeblock-head">
        <span className="md-codeblock-lang">{lang || "code"}</span>
        <div className="md-codeblock-actions">
          {canPreview ? (
            <button type="button" className="md-copy" onClick={() => onPreview(code, lang || "HTML")}>
              预览
            </button>
          ) : null}
          <button type="button" className="md-copy" onClick={onCopy}>
            {copied ? "已复制" : "复制"}
          </button>
        </div>
      </div>
      <pre>
        <code className={className}>{children}</code>
      </pre>
    </div>
  );
}

export function MarkdownMessage({ content, streaming, agentId }: Props) {
  const text = content || (streaming ? "…" : "");
  const [preview, setPreview] = useState<PreviewState>(PREVIEW_CLOSED);

  const closePreview = useCallback(() => setPreview(PREVIEW_CLOSED), []);

  const openHtmlPreview = useCallback((html: string, title: string) => {
    setPreview({ open: true, title, html, loading: false, error: null });
  }, []);

  const openSandboxLink = useCallback(async (path: string) => {
    const wp = normalizeWorkspacePath(path);
    setPreview({
      open: true,
      title: previewTitleFromPath(wp),
      html: null,
      loading: true,
      error: null,
    });
    try {
      const { content: fileContent } = await readSandboxFile(wp, agentId ? { agent_id: agentId } : undefined);
      if (!looksLikeHtml(fileContent, wp)) {
        // Non-HTML: wrap plain text so the user still gets an in-app view.
        const escaped = fileContent
          .replace(/&/g, "&amp;")
          .replace(/</g, "&lt;")
          .replace(/>/g, "&gt;");
        const wrapped = `<!doctype html><html><head><meta charset="utf-8"><title>${previewTitleFromPath(wp)}</title>
<style>body{font:13px/1.5 ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;margin:16px;white-space:pre-wrap;word-break:break-word;color:#1a1a1b;background:#fafafa}</style>
</head><body>${escaped}</body></html>`;
        setPreview({
          open: true,
          title: previewTitleFromPath(wp),
          html: wrapped,
          loading: false,
          error: null,
        });
        return;
      }
      setPreview({
        open: true,
        title: previewTitleFromPath(wp),
        html: fileContent,
        loading: false,
        error: null,
      });
    } catch (err) {
      setPreview({
        open: true,
        title: previewTitleFromPath(wp),
        html: null,
        loading: false,
        error: friendlyOpenError(err),
      });
    }
  }, [agentId]);

  return (
    <div className={`md-body${streaming ? " streaming" : ""}`}>
      <ReactMarkdown
        remarkPlugins={[remarkGfm]}
        urlTransform={urlTransform}
        components={{
          a: ({ href, children }) => {
            const sandboxPath = parseSandboxHref(href);
            if (sandboxPath) {
              const labelText = extractText(children).trim();
              const friendly =
                !labelText ||
                /(?:^sandbox:)|\/workspace\//i.test(labelText) ||
                labelText === sandboxPath
                  ? previewTitleFromPath(sandboxPath)
                  : children;
              return (
                <a
                  href={href}
                  className="md-sandbox-link"
                  onClick={(e) => {
                    e.preventDefault();
                    void openSandboxLink(sandboxPath);
                  }}
                >
                  {friendly}
                </a>
              );
            }
            return (
              <a href={href} target="_blank" rel="noreferrer noopener">
                {children}
              </a>
            );
          },
          // Flatten <pre> so our code handler owns the block chrome.
          pre: ({ children }) => <>{children}</>,
          code: ({ className, children }) => {
            const raw = extractText(children);
            const isBlock =
              Boolean(className && /language-/.test(className)) || raw.includes("\n");
            if (!isBlock) {
              return <code className="md-inline-code">{children}</code>;
            }
            return (
              <CodeBlock className={className} onPreview={openHtmlPreview}>
                {children}
              </CodeBlock>
            );
          },
        }}
      >
        {text}
      </ReactMarkdown>
      {streaming ? <span className="caret" /> : null}
      <HtmlPreviewModal
        open={preview.open}
        title={preview.title}
        html={preview.html}
        loading={preview.loading}
        error={preview.error}
        onClose={closePreview}
      />
    </div>
  );
}
