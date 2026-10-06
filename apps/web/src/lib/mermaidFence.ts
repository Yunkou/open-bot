/**
 * Pure helpers for diagram fences inside chat markdown (no DOM, no mermaid import).
 *
 * Streaming rule (design v1/v2): an unfinished fence must NOT mount the renderer / iframe.
 * CommonMark lets an unclosed fence run to the end of its container, so react-markdown happily
 * renders it as a code block. We decide "closed" from the fence's own source slice
 * (`node.position` offsets): it is closed iff its last line is a closing fence of the same char
 * and at least the opener's length.
 */

export const MERMAID_LANGS = new Set(["mermaid", "mmd"]);
/** Acceptance language is `html`; `html-diagram` is an optional alias (design v2 §1.1). */
export const HTML_DIAGRAM_LANGS = new Set(["html", "html-diagram"]);

export function isMermaidLang(lang: string | null | undefined): boolean {
  return Boolean(lang) && MERMAID_LANGS.has(String(lang).toLowerCase());
}

export function isHtmlDiagramLang(lang: string | null | undefined): boolean {
  return Boolean(lang) && HTML_DIAGRAM_LANGS.has(String(lang).toLowerCase());
}

/** Strip container prefixes (blockquote `>` markers and indentation) from a fence line. */
function stripContainer(line: string): string {
  return line.replace(/^(?:[ \t]*>)*[ \t]*/, "");
}

/**
 * `slice` = markdown source of one fenced code node (opener … closer).
 * Returns true when the fence has a matching closing line.
 */
export function isFenceClosed(slice: string): boolean {
  const lines = (slice || "").replace(/\r\n?/g, "\n").replace(/\n+$/, "").split("\n");
  if (lines.length < 2) return false;
  const open = /^(`{3,}|~{3,})/.exec(stripContainer(lines[0]));
  if (!open) return false;
  const close = /^(`{3,}|~{3,})[ \t]*$/.exec(stripContainer(lines[lines.length - 1]));
  return Boolean(close) && close![1][0] === open[1][0] && close![1].length >= open[1].length;
}

export type SourcePosition = {
  start?: { offset?: number | null } | null;
  end?: { offset?: number | null } | null;
} | null | undefined;

/**
 * Should this diagram fence stay in "pending" (source preview, no renderer / iframe)?
 * - stream ended → never pending (render once, even if the model never closed the fence)
 * - streaming + known position → pending until the closing fence arrives
 * - streaming + no position info → pending (safe default: render after stream ends)
 */
export function isFencePending(
  markdown: string,
  position: SourcePosition,
  streaming: boolean | undefined,
): boolean {
  if (!streaming) return false;
  const s = position?.start?.offset;
  const e = position?.end?.offset;
  if (typeof s !== "number" || typeof e !== "number" || e <= s) return true;
  return !isFenceClosed(markdown.slice(s, e));
}

/** @deprecated alias — use isFencePending */
export const isMermaidPending = isFencePending;

function pad2(n: number): string {
  return String(n).padStart(2, "0");
}

/** `diagram-YYYYMMDD-HHmmss` in local time (design v1 §2 / v2). */
export function diagramFilename(date = new Date()): string {
  return (
    `diagram-${date.getFullYear()}${pad2(date.getMonth() + 1)}${pad2(date.getDate())}` +
    `-${pad2(date.getHours())}${pad2(date.getMinutes())}${pad2(date.getSeconds())}`
  );
}
