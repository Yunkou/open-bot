"""Unit tests for explicit-reply quote injection (PM acceptance #3)."""

from __future__ import annotations

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

from app.reply_quote import (  # noqa: E402
    REPLY_QUOTE_HEADER,
    REPLY_QUOTE_MAX_CHARS,
    apply_reply_quote,
)


def _ok(cond: bool, msg: str) -> None:
    if not cond:
        raise AssertionError(msg)
    print(f"  OK  {msg}")


def test_empty_is_noop() -> None:
    msgs = [{"role": "assistant", "content": "a"}, {"role": "user", "content": "hi"}]
    for q in (None, "", "   \n\t "):
        c, m = apply_reply_quote("hi", msgs, q)
        _ok(c == "hi", f"content unchanged for quote={q!r}")
        _ok(m == msgs, f"messages unchanged for quote={q!r}")


def test_prefixes_content_and_last_user() -> None:
    msgs = [
        {"role": "user", "content": "first"},
        {"role": "assistant", "content": "原始回答 X"},
        {"role": "user", "content": "这个怎么理解？"},
    ]
    c, m = apply_reply_quote("这个怎么理解？", msgs, "  原始回答 X  ")
    expected = f"{REPLY_QUOTE_HEADER}\n「原始回答 X」\n\n这个怎么理解？"
    _ok(c == expected, "content prefixed with quote block")
    _ok(m[-1]["content"] == expected, "last user message patched")
    _ok(m[0]["content"] == "first", "earlier user message untouched")
    _ok(m[1]["content"] == "原始回答 X", "assistant untouched")
    _ok(msgs[-1]["content"] == "这个怎么理解？", "input list not mutated")
    # idempotent
    c2, m2 = apply_reply_quote(c, m, "原始回答 X")
    _ok(c2 == c and m2[-1]["content"] == expected, "idempotent (no double prefix)")


def test_appends_user_when_missing() -> None:
    c, m = apply_reply_quote("q", [{"role": "assistant", "content": "a"}], "a")
    _ok(m[-1] == {"role": "user", "content": c}, "user turn appended when none exists")


def test_truncation() -> None:
    long = "字" * (REPLY_QUOTE_MAX_CHARS + 500)
    c, m = apply_reply_quote("ok", [{"role": "user", "content": "ok"}], long)
    quote = c.split("「", 1)[1].split("」", 1)[0]
    _ok(len(quote) <= REPLY_QUOTE_MAX_CHARS + 1, "quote truncated to limit (+ellipsis)")
    _ok(quote.endswith("…"), "truncated quote ends with ellipsis")
    _ok(c.endswith("\n\nok"), "original content preserved after quote")


if __name__ == "__main__":
    test_empty_is_noop()
    test_prefixes_content_and_last_user()
    test_appends_user_when_missing()
    test_truncation()
    print("all reply_quote tests passed")
