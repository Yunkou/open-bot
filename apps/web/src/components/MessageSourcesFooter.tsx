import type { MessageSource } from "../api";
import { normalizeMessageSources } from "../api";
import { citationAnchorId } from "../lib/citationMarkers";

function displayLabel(src: MessageSource): string {
  if (src.title && src.title.trim()) return src.title.trim();
  try {
    const u = new URL(src.url);
    return u.hostname.replace(/^www\./, "") || src.url;
  } catch {
    return src.url;
  }
}

type Props = {
  sources?: MessageSource[] | null;
  /** Message id — used for citation jump anchors ([1] → footer). */
  messageId?: string;
};

/** Numbered 「来源」 footer; markers in the answer jump here via #cite-{msg}-{n}. */
export function MessageSourcesFooter({ sources, messageId }: Props) {
  const list = normalizeMessageSources(sources);
  if (list.length === 0) return null;
  const mid = (messageId || "").trim();
  return (
    <div className="msg-sources" aria-label="来源">
      <div className="msg-sources-label">来源</div>
      <ol className="msg-sources-list">
        {list.map((s, i) => {
          const n = i + 1;
          const id = mid ? citationAnchorId(mid, n) : undefined;
          return (
            <li
              key={s.url}
              id={id}
              className="msg-sources-item"
              data-source-url={s.url}
            >
              <a
                className="msg-sources-link"
                href={s.url}
                target="_blank"
                rel="noopener noreferrer"
                title={s.url}
              >
                <span className="msg-sources-num" aria-hidden>
                  [{n}]
                </span>{" "}
                <span className="msg-sources-title">{displayLabel(s)}</span>
              </a>
            </li>
          );
        })}
      </ol>
    </div>
  );
}
