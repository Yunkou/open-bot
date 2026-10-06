import {
  createContext,
  isValidElement,
  useCallback,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from "react";
import ReactMarkdown, { defaultUrlTransform, type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import { downloadSandboxFile, downloadTextFile, readSandboxFile } from "../api";
import { isMermaidLang, isMermaidPending, type SourcePosition } from "../lib/mermaidFence";
import { DiagramCard } from "./DiagramCard";
import {
  HtmlPreviewModal,
  friendlyFileError,
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
  path: string | null;
  html: string | null;
  loading: boolean;
  error: string | null;
};

const PREVIEW_CLOSED: PreviewState = {
  open: false,
  title: "",
  path: null,
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

/**
 * Current markdown text + streaming flag for block renderers. Passed by context (not closure) so
 * the `components` map stays referentially stable across stream ticks — otherwise every token
 * would give react-markdown new component types and remount every block (diagram flicker).
 */
const MdRenderContext = createContext<{ markdown: string; streaming: boolean }>({
  markdown: "",
  streaming: false,
});

/** ```mermaid fence → DiagramCard. Pending (no renderer) until the fence closes or the stream ends. */
function MermaidBlock({ code, position }: { code: string; position: SourcePosition }) {
  const { markdown, streaming } = useContext(MdRenderContext);
  const pending = isMermaidPending(markdown, position, streaming);
  return <DiagramCard kind="mermaid" source={code} pending={pending} />;
}

export function MarkdownMessage({ content, streaming, agentId }: Props) {
  const text = content || (streaming ? "…" : "");
  const [preview, setPreview] = useState<PreviewState>(PREVIEW_CLOSED);
  const [downloading, setDownloading] = useState(false);
  const [downloadError, setDownloadError] = useState<string | null>(null);

  const closePreview = useCallback(() => {
    setPreview(PREVIEW_CLOSED);
    setDownloadError(null);
  }, []);

  const openHtmlPreview = useCallback((html: string, title: string) => {
    setDownloadError(null);
    setPreview({ open: true, title, path: null, html, loading: false, error: null });
  }, []);

  const downloadPreview = useCallback(async () => {
    setDownloading(true);
    setDownloadError(null);
    try {
      if (preview.path) {
        await downloadSandboxFile(preview.path, agentId ? { agent_id: agentId } : undefined);
      } else if (preview.html) {
        const name = preview.title && !preview.title.includes("/") ? preview.title : "preview.html";
        const filename = /\.html?$/i.test(name) ? name : `${name}.html`;
        downloadTextFile(filename, preview.html, "text/html;charset=utf-8");
      }
    } catch (err) {
      setDownloadError(friendlyFileError(err, "下载"));
    } finally {
      setDownloading(false);
    }
  }, [agentId, preview.html, preview.path, preview.title]);

  const openSandboxLink = useCallback(async (path: string) => {
    const wp = normalizeWorkspacePath(path);
    setDownloadError(null);
    setPreview({
      open: true,
      title: previewTitleFromPath(wp),
      path: wp,
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
          path: wp,
          html: wrapped,
          loading: false,
          error: null,
        });
        return;
      }
      setPreview({
        open: true,
        title: previewTitleFromPath(wp),
        path: wp,
        html: fileContent,
        loading: false,
        error: null,
      });
    } catch (err) {
      setPreview({
        open: true,
        title: previewTitleFromPath(wp),
        path: wp,
        html: null,
        loading: false,
        error: friendlyOpenError(err),
      });
    }
  }, [agentId]);

  const components = useMemo<Components>(
    () => ({
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
      code: ({ className, children, node }) => {
        const raw = extractText(children);
        const isBlock =
          Boolean(className && /language-/.test(className)) || raw.includes("\n");
        if (!isBlock) {
          return <code className="md-inline-code">{children}</code>;
        }
        const lang = /language-([\w-]+)/.exec(className || "")?.[1];
        if (isMermaidLang(lang)) {
          return <MermaidBlock code={raw.replace(/\n$/, "")} position={node?.position} />;
        }
        return (
          <CodeBlock className={className} onPreview={openHtmlPreview}>
            {children}
          </CodeBlock>
        );
      },
    }),
    [openHtmlPreview, openSandboxLink],
  );
  const renderCtx = useMemo(() => ({ markdown: text, streaming: Boolean(streaming) }), [text, streaming]);

  return (
    <div className={`md-body${streaming ? " streaming" : ""}`}>
      <MdRenderContext.Provider value={renderCtx}>
        <ReactMarkdown
          remarkPlugins={[remarkGfm]}
          urlTransform={urlTransform}
          components={components}
        >
          {text}
        </ReactMarkdown>
      </MdRenderContext.Provider>
      {streaming ? <span className="caret" /> : null}
      <HtmlPreviewModal
        open={preview.open}
        title={preview.title}
        html={preview.html}
        loading={preview.loading}
        error={preview.error}
        downloading={downloading}
        downloadError={downloadError}
        onDownload={preview.path || preview.html ? () => void downloadPreview() : undefined}
        onClose={closePreview}
      />
    </div>
  );
}
