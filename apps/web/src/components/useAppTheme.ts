import { useSyncExternalStore } from "react";
import type { DiagramTheme } from "../lib/mermaidRender";

/** App theme from `data-theme` on <html> (or <body>); anything but "dark" → light. */
function readTheme(): DiagramTheme {
  if (typeof document === "undefined") return "light";
  const t = document.documentElement.dataset.theme || document.body?.dataset.theme || "";
  return t === "dark" ? "dark" : "light";
}

function subscribe(onChange: () => void): () => void {
  if (typeof MutationObserver === "undefined") return () => {};
  const mo = new MutationObserver(onChange);
  mo.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme"] });
  if (document.body) mo.observe(document.body, { attributes: true, attributeFilter: ["data-theme"] });
  return () => mo.disconnect();
}

export function useAppTheme(): DiagramTheme {
  return useSyncExternalStore(subscribe, readTheme, () => "light");
}
