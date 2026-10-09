/**
 * Sanitize HTML diagram fence content for a strict sandbox iframe (sandbox="").
 * - Strip scripts and external script/link/font http(s) resources
 * - Allow inline <style> and semantic HTML
 * - img only data: (or same-origin auth attachment URLs when provided)
 * - Neutralize external navigations so the iframe cannot leave its blank origin
 */
export const HTML_DIAGRAM_MAX_BYTES = 200 * 1024;

export type HtmlSanitizeOptions = {
  /** Allowed non-data image URL prefixes (e.g. authenticated attachment GET). */
  allowImageUrlPrefixes?: string[];
};

export type HtmlSanitizeResult = {
  html: string;
  blockedExternal: number;
  empty: boolean;
};

const RESOURCE_ATTRS = ["src", "href", "xlink:href", "poster", "data"] as const;

function isHttpUrl(value: string): boolean {
  return /^https?:\/\//i.test(value.trim());
}

function isDataUrl(value: string): boolean {
  return /^data:/i.test(value.trim());
}

function isAllowedImageSrc(value: string, allowPrefixes: string[]): boolean {
  const v = value.trim();
  if (!v || v.startsWith("#")) return false;
  if (isDataUrl(v)) return true;
  if (!isHttpUrl(v) && !v.startsWith("//")) {
    // relative / root-relative — only if an allow prefix matches when absolutized is unknown;
    // without an attachment base we reject non-data to avoid leaking to arbitrary hosts via <base>.
    return allowPrefixes.some((p) => v.startsWith(p));
  }
  if (v.startsWith("//")) return false;
  return allowPrefixes.some((p) => v.startsWith(p));
}

function stripImportsFromCss(css: string): { css: string; blocked: number } {
  let blocked = 0;
  const next = css.replace(/@import\s+(?:url\s*\(\s*)?["']?([^"')\s]+)["']?\s*\)?\s*;?/gi, (full, url: string) => {
    if (isHttpUrl(url) || url.startsWith("//")) {
      blocked += 1;
      return "/* stripped external @import */";
    }
    return full;
  });
  // url(https://...) inside CSS (fonts/backgrounds)
  const next2 = next.replace(/url\s*\(\s*(['"]?)(https?:\/\/[^)'"]+|\/\/[^)'"]+)\1\s*\)/gi, () => {
    blocked += 1;
    return "url(about:blank)";
  });
  return { css: next2, blocked };
}

function neutralizeAnchor(el: HTMLAnchorElement): number {
  const href = (el.getAttribute("href") || "").trim();
  if (!href || href.startsWith("#")) return 0;
  const external = isHttpUrl(href) || href.startsWith("//") || /^(mailto|tel|javascript):/i.test(href);
  // Neutralize every non-fragment href so the opaque iframe cannot navigate itself.
  el.setAttribute("data-blocked-href", href);
  el.setAttribute("href", "#");
  el.removeAttribute("target");
  el.setAttribute("rel", "nofollow noopener");
  // Toast counter: only real network / protocol exits count as 「外链」.
  return external ? 1 : 0;
}

/**
 * DOM-based sanitize. Safe to call in browser; returns empty doc shell if input is blank.
 */
export function sanitizeHtmlDiagram(
  source: string,
  theme: "light" | "dark" = "light",
  opts: HtmlSanitizeOptions = {},
): HtmlSanitizeResult {
  const raw = source ?? "";
  if (!raw.trim()) {
    return { html: wrapHtmlDocument("", theme), blockedExternal: 0, empty: true };
  }

  const allowPrefixes = opts.allowImageUrlPrefixes ?? [];
  let blocked = 0;

  const parser = new DOMParser();
  // Parse as full document so <style> in head survives.
  const looksFull = /^\s*(<!doctype|html[\s>])/i.test(raw);
  const parsed = parser.parseFromString(looksFull ? raw : `<body>${raw}</body>`, "text/html");

  // Remove dangerous / external resource nodes
  parsed.querySelectorAll("script, iframe, object, embed, applet, frame, frameset").forEach((n) => {
    n.remove();
    blocked += 1;
  });
  // Drop meta refresh / CSP-injection vectors (avoid attr selector for linkedom/jsdom quirks)
  parsed.querySelectorAll("meta").forEach((n) => {
    if (n.hasAttribute("http-equiv")) {
      n.remove();
      blocked += 1;
    }
  });

  // <link> — drop stylesheets/fonts/preloads that hit the network
  parsed.querySelectorAll("link").forEach((link) => {
    const rel = (link.getAttribute("rel") || "").toLowerCase();
    const href = (link.getAttribute("href") || "").trim();
    const as = (link.getAttribute("as") || "").toLowerCase();
    const isStyle = rel.includes("stylesheet") || as === "style";
    const isFont = rel.includes("font") || as === "font";
    const isPreload = rel.includes("preload") || rel.includes("prefetch") || rel.includes("modulepreload");
    if (isStyle || isFont || isPreload || isHttpUrl(href) || href.startsWith("//")) {
      link.remove();
      blocked += 1;
    }
  });

  // Inline <style> — strip @import / external url()
  parsed.querySelectorAll("style").forEach((style) => {
    const { css, blocked: b } = stripImportsFromCss(style.textContent || "");
    style.textContent = css;
    blocked += b;
  });

  // style= attributes with external url()
  parsed.querySelectorAll("[style]").forEach((el) => {
    const style = el.getAttribute("style") || "";
    const { css, blocked: b } = stripImportsFromCss(style);
    if (b) el.setAttribute("style", css);
    blocked += b;
  });

  // Event handler attributes
  parsed.querySelectorAll("*").forEach((el) => {
    for (const attr of Array.from(el.attributes)) {
      if (/^on/i.test(attr.name)) {
        el.removeAttribute(attr.name);
        blocked += 1;
      }
    }
  });

  // Resource URLs (iterate elements — avoid `[xlink:href]` selectors that some parsers reject)
  parsed.querySelectorAll("*").forEach((el) => {
    const tag = el.tagName.toLowerCase();
    for (const attr of RESOURCE_ATTRS) {
      if (!el.hasAttribute(attr)) continue;
      const value = el.getAttribute(attr) || "";
      if (!value.trim()) continue;

      if (tag === "img" || tag === "source" || tag === "image") {
        if (!isAllowedImageSrc(value, allowPrefixes)) {
          el.setAttribute(attr, "about:blank");
          el.setAttribute("data-blocked-src", value);
          blocked += 1;
        }
        continue;
      }

      if (tag === "a" && attr === "href") {
        blocked += neutralizeAnchor(el as HTMLAnchorElement);
        continue;
      }

      if (tag === "base" || isHttpUrl(value) || value.startsWith("//") || /^javascript:/i.test(value)) {
        if (tag === "base") {
          el.remove();
        } else {
          el.setAttribute(attr, attr === "href" ? "#" : "about:blank");
        }
        blocked += 1;
      }
    }
  });

  // Forms: neutralize action
  parsed.querySelectorAll("form").forEach((form) => {
    form.setAttribute("action", "#");
    form.removeAttribute("target");
  });

  const headHtml = parsed.head?.innerHTML ?? "";
  const bodyHtml = parsed.body?.innerHTML ?? "";
  const doc = wrapHtmlDocument(bodyHtml, theme, headHtml);
  return { html: doc, blockedExternal: blocked, empty: !bodyHtml.trim() };
}

export function wrapHtmlDocument(bodyInner: string, theme: "light" | "dark", extraHead = ""): string {
  const scheme = theme === "dark" ? "dark" : "light";
  const fg = theme === "dark" ? "#e5e5e5" : "#0f172a";
  const muted = theme === "dark" ? "#a1a1aa" : "#64748b";
  return `<!DOCTYPE html>
<html data-theme="${scheme}" style="color-scheme:${scheme}">
<head>
<meta charset="utf-8"/>
<meta name="viewport" content="width=device-width,initial-scale=1"/>
<style>
  html,body{margin:0;padding:0;background:transparent;color:${fg};color-scheme:${scheme};
    font:13px/1.5 "Inter","SF Pro Text","PingFang SC","Noto Sans SC",system-ui,sans-serif;}
  body{padding:12px;overflow:auto;}
  a[href="#"][data-blocked-href],a[data-blocked-href]{color:${muted};text-decoration:underline;cursor:not-allowed;}
  table{border-collapse:collapse;max-width:100%;}
  th,td{border:1px solid ${theme === "dark" ? "rgba(255,255,255,.14)" : "rgba(15,23,42,.12)"};padding:6px 10px;}
  img{max-width:100%;height:auto;}
</style>
${extraHead}
</head>
<body data-theme="${scheme}">${bodyInner}</body>
</html>`;
}

export function htmlSourceByteLength(source: string): number {
  if (typeof TextEncoder !== "undefined") return new TextEncoder().encode(source).length;
  // Fallback approximate
  return unescape(encodeURIComponent(source)).length;
}
