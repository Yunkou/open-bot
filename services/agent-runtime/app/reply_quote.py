"""Explicit-reply quote injection (PM acceptance #3).

When the user explicitly replies (「回复」) to a message, the API passes the
parent message text as ``reply_to_content``. We prefix the current user turn
with the quoted text so the model sees what is being replied to — both in
``content`` and in the last ``user`` message of the history (so compaction /
the actual LLM payload carry it too).

Pure helper: no third-party deps, safe to unit test standalone.
"""

from __future__ import annotations

from typing import Any

REPLY_QUOTE_MAX_CHARS = 2000
REPLY_QUOTE_HEADER = "用户正在回复以下消息："


def truncate_quote(text: str, limit: int = REPLY_QUOTE_MAX_CHARS) -> str:
    t = (text or "").strip()
    if len(t) <= limit:
        return t
    return t[:limit].rstrip() + "…"


def format_reply_prefix(quote: str, content: str) -> str:
    return f"{REPLY_QUOTE_HEADER}\n「{quote}」\n\n{content}"


def apply_reply_quote(
    content: str,
    messages: list[dict[str, Any]],
    reply_to_content: str | None,
) -> tuple[str, list[dict[str, Any]]]:
    """Return (content, messages) with the quote injected into the current user turn.

    - blank ``reply_to_content`` → unchanged inputs (no-op)
    - quote truncated to ~REPLY_QUOTE_MAX_CHARS chars
    - ``content`` is prefixed (if non-empty)
    - the **last** ``role == "user"`` message is prefixed (a copy; input list is
      not mutated). If no user message exists, one is appended.
    - idempotent: already-prefixed text is not prefixed twice
    """
    quote = truncate_quote(reply_to_content or "")
    if not quote:
        return content, messages

    def _wrap(text: str) -> str:
        if text.startswith(REPLY_QUOTE_HEADER):
            return text
        return format_reply_prefix(quote, text)

    raw = content or ""
    new_content = _wrap(raw.strip()) if raw.strip() else raw

    out = [dict(m) for m in (messages or [])]
    for i in range(len(out) - 1, -1, -1):
        if out[i].get("role") == "user":
            out[i]["content"] = _wrap(str(out[i].get("content") or ""))
            break
    else:
        if raw.strip():
            out.append({"role": "user", "content": new_content})
    return new_content, out
