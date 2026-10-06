/** SVG / PNG export + clipboard helpers for DiagramCard. Browser-only. */
import type { DiagramTheme } from "./mermaidRender";

export const DIAGRAM_EXPORT_BG: Record<DiagramTheme, string> = {
  light: "#ffffff",
  dark: "#1f1f1f",
};

function saveBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  a.remove();
  window.setTimeout(() => URL.revokeObjectURL(url), 1000);
}

/** Standalone SVG: natural px size, no max-width clamp, xmlns, opaque theme background. */
export function standaloneSvg(svg: string, width: number, height: number, theme: DiagramTheme): string {
  const doc = new DOMParser().parseFromString(svg, "image/svg+xml");
  const root = doc.documentElement;
  if (!root || root.nodeName.toLowerCase() !== "svg" || doc.getElementsByTagName("parsererror").length) {
    return svg;
  }
  root.setAttribute("xmlns", "http://www.w3.org/2000/svg");
  root.setAttribute("xmlns:xlink", "http://www.w3.org/1999/xlink");
  root.setAttribute("width", String(width));
  root.setAttribute("height", String(height));
  root.style.removeProperty("max-width");
  root.style.setProperty("background-color", DIAGRAM_EXPORT_BG[theme]);
  const xml = new XMLSerializer().serializeToString(root);
  return `<?xml version="1.0" encoding="UTF-8"?>\n${xml}`;
}

export function downloadSvg(svg: string, width: number, height: number, theme: DiagramTheme, base: string) {
  const text = standaloneSvg(svg, width, height, theme);
  saveBlob(new Blob([text], { type: "image/svg+xml;charset=utf-8" }), `${base}.svg`);
}

/** Rasterize the current SVG (2x, capped at 8192px per side). Rejects on tainted canvas etc. */
export async function downloadPng(
  svg: string,
  width: number,
  height: number,
  theme: DiagramTheme,
  base: string,
): Promise<void> {
  const text = standaloneSvg(svg, width, height, theme);
  const img = new Image();
  img.decoding = "async";
  img.src = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(text)}`;
  await new Promise<void>((resolve, reject) => {
    img.onload = () => resolve();
    img.onerror = () => reject(new Error("svg image load failed"));
  });
  const MAX = 8192;
  const scale = Math.max(0.1, Math.min(2, MAX / width, MAX / height));
  const canvas = document.createElement("canvas");
  canvas.width = Math.max(1, Math.round(width * scale));
  canvas.height = Math.max(1, Math.round(height * scale));
  const ctx = canvas.getContext("2d");
  if (!ctx) throw new Error("no 2d context");
  ctx.fillStyle = DIAGRAM_EXPORT_BG[theme];
  ctx.fillRect(0, 0, canvas.width, canvas.height);
  ctx.drawImage(img, 0, 0, canvas.width, canvas.height);
  const blob = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, "image/png"));
  if (!blob) throw new Error("png encode failed");
  saveBlob(blob, `${base}.png`);
}

/** Clipboard with a textarea fallback for non-secure (http) deployments. */
export async function copyText(text: string): Promise<void> {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      return;
    }
  } catch {
    /* fall through */
  }
  const ta = document.createElement("textarea");
  ta.value = text;
  ta.setAttribute("readonly", "");
  ta.style.position = "fixed";
  ta.style.opacity = "0";
  document.body.appendChild(ta);
  ta.select();
  const ok = document.execCommand("copy");
  ta.remove();
  if (!ok) throw new Error("copy failed");
}
