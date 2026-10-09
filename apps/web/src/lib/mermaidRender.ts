/**
 * Lazy, serialized Mermaid renderer with a small result cache.
 *
 * - `mermaid` is loaded via dynamic import on first diagram → not in the main chunk.
 * - Renders are queued (mermaid.initialize/render share global state).
 * - Cache key = theme + source, so a remounted bubble (e.g. local id → server id swap at stream end)
 *   shows the SVG synchronously instead of flashing back to "rendering".
 * - Every failure resolves to `{ ok: false }`; callers never see a throw (no white screen).
 */

export type DiagramTheme = "light" | "dark";

export type DiagramRenderResult =
  | { ok: true; svg: string; width: number; height: number }
  | { ok: false; reason: "syntax" | "load"; message: string };

type MermaidApi = {
  initialize: (config: Record<string, unknown>) => void;
  render: (id: string, text: string) => Promise<{ svg: string }>;
};

const CACHE_LIMIT = 60;
const cache = new Map<string, DiagramRenderResult>();
const inflight = new Map<string, Promise<DiagramRenderResult>>();
let loader: Promise<MermaidApi> | null = null;
let queue: Promise<unknown> = Promise.resolve();
let initializedTheme: DiagramTheme | null = null;
let seq = 0;

const FONT_STACK = '"Inter", "SF Pro Text", "PingFang SC", "Noto Sans SC", system-ui, sans-serif';

function keyOf(source: string, theme: DiagramTheme): string {
  return `${theme}\u0000${source}`;
}

function loadMermaid(): Promise<MermaidApi> {
  if (!loader) {
    loader = import("mermaid")
      .then((mod) => (mod.default ?? mod) as unknown as MermaidApi)
      .catch((err) => {
        loader = null; // allow retry on next diagram (e.g. chunk load hiccup)
        throw err;
      });
  }
  return loader;
}

function initFor(mermaid: MermaidApi, theme: DiagramTheme) {
  if (initializedTheme === theme) return;
  mermaid.initialize({
    startOnLoad: false,
    securityLevel: "strict", // DOMPurify labels, no click callbacks / scripts
    theme: theme === "dark" ? "dark" : "default",
    fontFamily: FONT_STACK,
    // Match chat body size (mermaid default 16px makes in-bubble graphs oversized)
    themeVariables: { fontSize: "14px", fontFamily: FONT_STACK },
    // SVG <text> labels instead of <foreignObject>: keeps PNG export canvas untainted (Safari)
    htmlLabels: false,
    flowchart: { htmlLabels: false },
    suppressErrorRendering: true,
  });
  initializedTheme = theme;
}

/** Natural size from viewBox (fallback width/height attrs). */
export function measureSvg(svg: string): { width: number; height: number } {
  const vb = /viewBox\s*=\s*"([^"]+)"/i.exec(svg);
  if (vb) {
    const parts = vb[1].trim().split(/[\s,]+/).map(Number);
    if (parts.length === 4 && parts[2] > 0 && parts[3] > 0) {
      return { width: Math.ceil(parts[2]), height: Math.ceil(parts[3]) };
    }
  }
  const w = /<svg[^>]*\swidth\s*=\s*"([\d.]+)(?:px)?"/i.exec(svg);
  const h = /<svg[^>]*\sheight\s*=\s*"([\d.]+)(?:px)?"/i.exec(svg);
  return { width: w ? Math.ceil(Number(w[1])) : 600, height: h ? Math.ceil(Number(h[1])) : 400 };
}

function remember(key: string, result: DiagramRenderResult) {
  if (result.ok === false && result.reason === "load") return; // retryable, do not pin
  cache.delete(key);
  cache.set(key, result);
  while (cache.size > CACHE_LIMIT) {
    const oldest = cache.keys().next().value;
    if (oldest === undefined) break;
    cache.delete(oldest);
  }
}

function cleanupTemp(id: string) {
  if (typeof document === "undefined") return;
  for (const el of [document.getElementById(id), document.getElementById(`d${id}`)]) {
    el?.remove();
  }
}

/** Synchronous cache lookup (used for first paint to avoid a loading flash on remount). */
export function peekDiagram(source: string, theme: DiagramTheme): DiagramRenderResult | undefined {
  return cache.get(keyOf(source, theme));
}

export function renderDiagram(source: string, theme: DiagramTheme): Promise<DiagramRenderResult> {
  const key = keyOf(source, theme);
  const hit = cache.get(key);
  if (hit) return Promise.resolve(hit);
  const pending = inflight.get(key);
  if (pending) return pending;

  const job: Promise<DiagramRenderResult> = queue.then(async () => {
    let mermaid: MermaidApi;
    try {
      mermaid = await loadMermaid();
    } catch (err) {
      return { ok: false, reason: "load", message: String((err as Error)?.message || err) } as const;
    }
    const id = `ob-mmd-${Date.now().toString(36)}-${++seq}`;
    try {
      initFor(mermaid, theme);
      const { svg } = await mermaid.render(id, source);
      if (!svg || !/<svg[\s>]/i.test(svg)) throw new Error("empty svg");
      return { ok: true, svg, ...measureSvg(svg) } as const;
    } catch (err) {
      return { ok: false, reason: "syntax", message: String((err as Error)?.message || err) } as const;
    } finally {
      cleanupTemp(id);
    }
  });

  queue = job.catch(() => undefined);
  const tracked = job.then((result) => {
    remember(key, result);
    inflight.delete(key);
    return result;
  });
  inflight.set(key, tracked);
  return tracked;
}
