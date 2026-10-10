import type { MessageSource } from "../api";
import { normalizeMessageSources } from "../api";

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
};

/** Minimal 「来源」 footer under Bot replies that used retrieval. */
export function MessageSourcesFooter({ sources }: Props) {
  const list = normalizeMessageSources(sources);
  if (list.length === 0) return null;
  return (
    <div className="msg-sources" aria-label="来源">
      <span className="msg-sources-label">来源</span>
      <ul className="msg-sources-list">
        {list.map((s) => (
          <li key={s.url} className="msg-sources-item">
            <a
              className="msg-sources-link"
              href={s.url}
              target="_blank"
              rel="noopener noreferrer"
              title={s.url}
            >
              {displayLabel(s)}
            </a>
          </li>
        ))}
      </ul>
    </div>
  );
}
