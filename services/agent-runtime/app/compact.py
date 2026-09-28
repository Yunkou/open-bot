"""History compaction: token-budget primary trigger; message/char thresholds as fallback."""

from __future__ import annotations

import os
import re
from typing import Any

from .llm import strip_think

# CJK Unified Ideographs + common CJK punctuation / fullwidth forms.
_CJK_RE = re.compile(
    r"[\u3400-\u4dbf\u4e00-\u9fff\uf900-\ufaff"
    r"\u3000-\u303f\uff00-\uffef]"
)

_MSG_OVERHEAD_TOKENS = 4
_MIN_CONTEXT_WINDOW = 4096
_MAX_CONTEXT_WINDOW = 1_000_000


def _int_env(name: str, default: int) -> int:
    raw = (os.getenv(name) or "").strip()
    if not raw:
        return default
    try:
        return int(raw)
    except ValueError:
        return default


def _float_env(name: str, default: float) -> float:
    raw = (os.getenv(name) or "").strip()
    if not raw:
        return default
    try:
        return float(raw)
    except ValueError:
        return default


def max_messages() -> int:
    return max(4, _int_env("COMPACT_MAX_MESSAGES", 20))


def max_chars() -> int:
    return max(500, _int_env("COMPACT_MAX_CHARS", 6000))


def keep_recent() -> int:
    # Keep enough recent turns; clamp so we always leave room to summarize.
    k = _int_env("COMPACT_KEEP_RECENT", 8)
    return max(2, min(k, max_messages() - 2))


def default_context_window() -> int:
    return clamp_context_window(_int_env("COMPACT_DEFAULT_CONTEXT_WINDOW", 32768))


def reserve_output_tokens() -> int:
    return max(0, _int_env("COMPACT_RESERVE_OUTPUT_TOKENS", 2048))


def budget_ratio() -> float:
    r = _float_env("COMPACT_BUDGET_RATIO", 0.75)
    return max(0.1, min(r, 1.0))


def clamp_context_window(n: int) -> int:
    return max(_MIN_CONTEXT_WINDOW, min(int(n), _MAX_CONTEXT_WINDOW))


def guess_context_window(model: str | None) -> int:
    """Heuristic context window from model name (case-insensitive)."""
    name = (model or "").strip().lower()
    if not name:
        return default_context_window()

    # Explicit size tokens in the name win first.
    if "1m" in name or "1000k" in name:
        return clamp_context_window(1_000_000)
    if "200k" in name:
        return clamp_context_window(200_000)
    if "128k" in name:
        return clamp_context_window(131_072)
    if "64k" in name:
        return clamp_context_window(65_536)
    if "32k" in name:
        return clamp_context_window(32_768)
    if "16k" in name:
        return clamp_context_window(16_384)
    if "8k" in name:
        return clamp_context_window(8192)

    # Known families / documented defaults.
    if any(x in name for x in ("gpt-4o", "gpt-4.1", "gpt-4-turbo", "chatgpt-4o")):
        return clamp_context_window(128_000)
    if any(
        x in name
        for x in (
            "claude-3-5",
            "claude-3.5",
            "claude-sonnet-4",
            "claude-opus-4",
            "claude-haiku-4",
            "claude-3-opus",
            "claude-3-sonnet",
            "claude-3-haiku",
        )
    ):
        return clamp_context_window(200_000)
    if "gpt-3.5" in name or "gpt-35" in name:
        return clamp_context_window(16_384)
    if name.startswith("gpt-4") or "gpt-4-" in name:
        return clamp_context_window(8192)

    # Qwen / long-context variants without explicit *k* already handled above.
    if "qwen" in name:
        if "long" in name or "plus" in name or "max" in name:
            return clamp_context_window(131_072)
        return clamp_context_window(32_768)

    if "gemini" in name and ("1.5" in name or "2." in name or "pro" in name or "flash" in name):
        return clamp_context_window(128_000)

    return default_context_window()


def resolve_context_window(
    explicit: int | None = None,
    model: str | None = None,
) -> int:
    """Prefer explicit llm.context_window if > 0; else guess from model; else env default."""
    if explicit is not None and int(explicit) > 0:
        return clamp_context_window(int(explicit))
    return guess_context_window(model)


def estimate_tokens(text: str) -> int:
    """Deterministic token estimate without tiktoken.

    Formula (blended CJK / ASCII):
    - Count CJK ideographs / fullwidth chars as ``cjk``.
    - Remaining chars are ``ascii_ish = len(text) - cjk``.
    - ``tokens ≈ max(1 if text else 0, round(cjk * 0.6 + ascii_ish / 4))``.

    CJK-heavy text uses ~0.6 tokens/char; ASCII ~chars/4. Equivalent to
    ``max(len//4, cjk_count)``-style floors when CJK dominates.
    """
    if not text:
        return 0
    cjk = len(_CJK_RE.findall(text))
    ascii_ish = max(0, len(text) - cjk)
    est = int(round(cjk * 0.6 + ascii_ish / 4.0))
    # Floor: never under-count dense CJK relative to a crude char/4 split.
    est = max(est, len(text) // 4, cjk)
    return max(1, est)


def estimate_messages_tokens(messages: list[dict[str, Any]]) -> int:
    total = 0
    for m in messages:
        total += estimate_tokens(str(m.get("content") or ""))
        total += _MSG_OVERHEAD_TOKENS
    return total


def token_budget(
    context_window: int,
    *,
    reserve_output: int | None = None,
    ratio: float | None = None,
) -> int:
    """Budget for prompt messages: int(window * ratio) - reserve_output."""
    cw = clamp_context_window(context_window)
    r = budget_ratio() if ratio is None else max(0.1, min(float(ratio), 1.0))
    reserve = reserve_output_tokens() if reserve_output is None else max(0, int(reserve_output))
    raw = int(cw * r) - reserve
    # Floor: enough room for keep_recent short turns.
    floor = keep_recent() * (_MSG_OVERHEAD_TOKENS + 16)
    return max(floor, raw)


def estimate_chars(messages: list[dict[str, Any]]) -> int:
    return sum(len(str(m.get("content") or "")) for m in messages)


def compact_thresholds(
    *,
    context_window: int | None = None,
    model: str | None = None,
) -> dict[str, Any]:
    cw = (
        clamp_context_window(context_window)
        if context_window is not None and context_window > 0
        else resolve_context_window(None, model)
    )
    budget = token_budget(cw)
    return {
        "max_messages": max_messages(),
        "max_chars": max_chars(),
        "keep_recent": keep_recent(),
        "token_mode": True,
        "default_context_window": default_context_window(),
        "context_window": cw,
        "reserve_output_tokens": reserve_output_tokens(),
        "budget_ratio": budget_ratio(),
        "token_budget": budget,
        "min_context_window": _MIN_CONTEXT_WINDOW,
        "max_context_window": _MAX_CONTEXT_WINDOW,
    }


def compact_config(
    *,
    context_window: int | None = None,
    model: str | None = None,
) -> dict[str, Any]:
    """Alias for settings/health endpoints (same payload as compact_thresholds)."""
    return compact_thresholds(context_window=context_window, model=model)


def needs_compact(
    messages: list[dict[str, Any]],
    *,
    context_window: int | None = None,
    model: str | None = None,
    extra_prompt_tokens: int = 0,
    reserve_output: int | None = None,
    budget_ratio_override: float | None = None,
) -> tuple[bool, str]:
    """Return (needed, reason).

    Primary: estimated tokens vs token budget from context window.
    Fallback safety net: legacy message_count / char_count thresholds.
    """
    cw = resolve_context_window(context_window, model)
    budget = token_budget(
        cw,
        reserve_output=reserve_output,
        ratio=budget_ratio_override,
    )
    est = estimate_messages_tokens(messages) + max(0, int(extra_prompt_tokens))
    if est > budget:
        return True, "token_budget"
    if len(messages) > max_messages():
        return True, "message_count"
    if estimate_chars(messages) > max_chars():
        return True, "char_count"
    return False, ""


def fake_summary(older: list[dict[str, Any]]) -> str:
    bits: list[str] = []
    for m in older[-12:]:
        role = m.get("role") or "user"
        content = str(m.get("content") or "").replace("\n", " ").strip()
        if len(content) > 120:
            content = content[:117] + "…"
        if content:
            bits.append(f"{role}: {content}")
    joined = " | ".join(bits)
    if len(joined) > 1500:
        joined = joined[:1497] + "…"
    return f"[对话摘要·假压缩] 更早共 {len(older)} 条消息。" + (
        f" 要点：{joined}" if joined else ""
    )


async def summarize_older(
    older: list[dict[str, Any]],
    *,
    api_key: str | None,
    chat_fn,
) -> str:
    """chat_fn(messages)->str optional; without key use fake_summary."""
    if not older:
        return ""
    if not api_key:
        return fake_summary(older)
    prompt = [
        {
            "role": "system",
            "content": "将以下对话历史压缩为一段简洁中文摘要，保留事实、偏好与未完成事项。不要编造。",
        },
        {
            "role": "user",
            "content": "\n".join(
                f"{m.get('role')}: {m.get('content')}" for m in older
            )[:12000],
        },
    ]
    try:
        text = await chat_fn(prompt)
        text = strip_think((text or "").strip())
        if text:
            if len(text) > 1200:
                text = text[:1197] + "…"
            return f"[对话摘要] {text}"
    except Exception:  # noqa: BLE001
        pass
    return fake_summary(older)


def _is_dialog_role(role: str) -> bool:
    return role in ("user", "assistant", "summary")


def split_at_last_summary(
    messages: list[dict[str, Any]],
) -> tuple[str | None, list[dict[str, Any]]]:
    """Prefer stored summary: return (summary_text|None, messages_after_summary)."""
    last_idx = -1
    for i, m in enumerate(messages):
        if str(m.get("role") or "") == "summary":
            last_idx = i
    if last_idx < 0:
        dialog = [m for m in messages if _is_dialog_role(str(m.get("role") or ""))]
        # drop any stray summary already filtered; keep user/assistant only for fresh
        dialog = [m for m in dialog if str(m.get("role")) != "summary"]
        return None, dialog
    summary_text = str(messages[last_idx].get("content") or "").strip()
    after = [
        m
        for m in messages[last_idx + 1 :]
        if str(m.get("role") or "") in ("user", "assistant")
    ]
    return summary_text or None, after


def select_keep_recent_count(
    dialog: list[dict[str, Any]],
    *,
    context_window: int,
) -> int:
    """Prefer keeping recent turns by a token slice of the budget; else message count."""
    fallback = keep_recent()
    if not dialog:
        return fallback
    # Reserve ~1/3 of the prompt budget for verbatim recent turns.
    recent_budget = max(64, token_budget(context_window) // 3)
    used = 0
    keep = 0
    for m in reversed(dialog):
        t = estimate_tokens(str(m.get("content") or "")) + _MSG_OVERHEAD_TOKENS
        if keep > 0 and used + t > recent_budget:
            break
        used += t
        keep += 1
        if keep >= len(dialog):
            break
    # Never keep everything (need room to summarize); clamp to fallback floor/ceiling.
    if keep <= 0:
        return fallback
    if keep >= len(dialog):
        return min(fallback, max(2, len(dialog) - 1))
    # Prefer token-based keep when it retains at least 2 turns; else fall back.
    if keep < 2:
        return fallback
    return keep


async def compact_messages(
    messages: list[dict[str, Any]],
    *,
    api_key: str | None = None,
    chat_fn=None,
    context_window: int | None = None,
    model: str | None = None,
    extra_prompt_tokens: int = 0,
) -> tuple[list[dict[str, Any]], dict[str, Any]]:
    """Return possibly compacted messages + meta flags.

    Prefer an existing role=summary message so restarts skip full re-summarization.
    When a new summary is produced, meta['summary'] holds the text for persistence.
    """
    prior_summary, dialog = split_at_last_summary(messages)
    cw = resolve_context_window(context_window, model)
    budget = token_budget(cw)
    est = estimate_messages_tokens(dialog) + max(0, int(extra_prompt_tokens))
    keep = select_keep_recent_count(dialog, context_window=cw)

    # Prefer stored summary: only re-summarize when *dialog after summary* overflows.
    # The summary itself is already the compressed form of older turns.
    check_target = list(dialog)
    meta: dict[str, Any] = {
        "compacted": False,
        "compact_reason": "",
        "original_count": len(dialog) + (1 if prior_summary else 0),
        "original_chars": estimate_chars(
            ([{"role": "system", "content": prior_summary}] if prior_summary else [])
            + dialog
        ),
        "thresholds": compact_thresholds(context_window=cw, model=model),
        "reused_summary": bool(prior_summary),
        "summary": None,
        "summary_new": False,
        "context_window": cw,
        "estimated_tokens": est,
        "token_budget": budget,
    }

    needed, reason = needs_compact(
        check_target,
        context_window=cw,
        model=model,
        extra_prompt_tokens=extra_prompt_tokens,
    )
    if not needed:
        if prior_summary:
            out = [{"role": "system", "content": prior_summary}, *dialog]
            meta.update(
                {
                    "compacted": True,
                    "compact_reason": "reused_summary",
                    "summary": prior_summary,
                    "result_count": len(out),
                    "result_chars": estimate_chars(out),
                    "estimated_tokens": estimate_messages_tokens(out),
                }
            )
            return out, meta
        return dialog, meta

    if len(dialog) <= keep:
        # Over budget but few messages: fold prior summary + all dialog.
        older = list(dialog)
        if prior_summary:
            older = [{"role": "system", "content": prior_summary}, *older]
        summary = await summarize_older(
            older,
            api_key=api_key,
            chat_fn=chat_fn,
        )
        out = [{"role": "system", "content": summary}]
        meta.update(
            {
                "compacted": True,
                "compact_reason": reason,
                "kept_recent": 0,
                "summarized_count": len(older),
                "result_count": len(out),
                "result_chars": estimate_chars(out),
                "summary": summary,
                "summary_new": True,
                "estimated_tokens": estimate_messages_tokens(out),
            }
        )
        return out, meta

    older = dialog[:-keep]
    recent = dialog[-keep:]
    to_summarize: list[dict[str, Any]] = list(older)
    if prior_summary:
        to_summarize = [{"role": "system", "content": prior_summary}, *older]

    summary = await summarize_older(
        to_summarize,
        api_key=api_key,
        chat_fn=chat_fn,
    )
    out = [{"role": "system", "content": summary}, *recent]
    meta.update(
        {
            "compacted": True,
            "compact_reason": reason,
            "kept_recent": len(recent),
            "summarized_count": len(to_summarize),
            "result_count": len(out),
            "result_chars": estimate_chars(out),
            "summary": summary,
            "summary_new": True,
            "estimated_tokens": estimate_messages_tokens(out),
        }
    )
    return out, meta
