"""Thread reply helpers for recall / Mem0 (mirrors Go thread_history.go).

Go injects the Chinese reply prefix into the user message before calling runtime.
Runtime uses ``reply_context`` for memory recall and Mem0 writes.
"""

from __future__ import annotations


def truncate_text(text: str, max_chars: int = 240) -> str:
    s = (text or "").strip()
    if max_chars <= 0 or len(s) <= max_chars:
        return s
    return s[:max_chars] + "…"


def format_reply_prefix(snippet: str, who: str = "助手") -> str:
    snip = truncate_text(snippet, 240)
    if not snip:
        return ""
    label = (who or "助手").strip() or "助手"
    return f"【回复 {label}：「{snip}」】\n"


def build_recall_query(user_text: str, *, reply_snippet: str = "", thread_tail: list[str] | None = None) -> str:
    parts: list[str] = []
    snip = truncate_text(reply_snippet, 240)
    if snip:
        parts.append(f"回复：{snip}")
    for t in thread_tail or []:
        bit = truncate_text(t, 120)
        if bit:
            parts.append(bit)
    if (user_text or "").strip():
        parts.append(user_text.strip())
    return "\n".join(parts).strip()


def augment_mem0_turn(
    turn: list[dict[str, str]],
    reply_context: str | None,
) -> list[dict[str, str]]:
    ctx = (reply_context or "").strip()
    if not ctx:
        return list(turn)
    return [{"role": "user", "content": f"[线程上下文]\n{ctx}"}, *list(turn)]
