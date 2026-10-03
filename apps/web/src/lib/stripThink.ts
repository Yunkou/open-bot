/** Hide model reasoning so it is not rendered as chat text. Mirrors runtime strip_think_tags. */

const THINK_BLOCK =
  /<(?:think|thinking|redacted_thinking)\b[^>]*>[\s\S]*?<\/(?:think|thinking|redacted_thinking)>/gi;
const THINK_UNCLOSED = /<(?:think|thinking|redacted_thinking)\b[^>]*>[\s\S]*$/gi;
const THINK_PARTIAL = /<(?:think|thinking|redacted_thinking)\b[^>]*$/gi;

export function stripThinkTags(text: string): string {
  if (!text) return text;
  const cleaned = text.replace(THINK_BLOCK, "").replace(THINK_UNCLOSED, "").replace(THINK_PARTIAL, "");
  return cleaned.trim();
}
