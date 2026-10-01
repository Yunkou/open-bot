"""Local Dream-equivalent memory consolidation (OSS path).

Mem0 Platform / Dream is a hosted product. This module provides an optional
local consolidation job that:
  1) lists recent Mem0 facts (when Mem0 OSS is enabled) and/or MemoryStore notes
  2) asks the chat LLM to distill a durable profile summary
  3) upserts it into MemoryStore as tier=profile tag=dream

Enable with MEM0_DREAM_ENABLED=1 (default off). Prefer Mem0 Platform when
MEM0_PLATFORM_API_KEY / MEM0_API_KEY is set and MEM0_PLATFORM_DREAM=1.

Honesty: this is NOT Mem0 Platform Dream; it is an OSS-compatible substitute.
"""

from __future__ import annotations

import logging
import os
from datetime import datetime, timezone
from typing import Any

import httpx

from . import mem0_store
from .llm import openai_config
from .memory import get_store

logger = logging.getLogger("open-bot.dream")


def _truthy(name: str, default: str = "0") -> bool:
    return (os.getenv(name) or default).strip().lower() in {"1", "true", "yes", "on"}


def platform_dream_available() -> bool:
    key = (os.getenv("MEM0_PLATFORM_API_KEY") or os.getenv("MEM0_API_KEY") or "").strip()
    return bool(key) and _truthy("MEM0_PLATFORM_DREAM", "0")


def local_dream_enabled() -> bool:
    return _truthy("MEM0_DREAM_ENABLED", "0")


def dream_mode() -> str:
    """Return platform | local | off."""
    if platform_dream_available():
        return "platform"
    if local_dream_enabled():
        return "local"
    return "off"


def status() -> dict[str, Any]:
    return {
        "dream_mode": dream_mode(),
        "mem0_wanted": mem0_store.mem0_wanted(),
        "platform_available": platform_dream_available(),
        "local_enabled": local_dream_enabled(),
        "note": (
            "Platform Dream requires MEM0_PLATFORM_API_KEY + MEM0_PLATFORM_DREAM=1; "
            "otherwise optional local consolidation via MEM0_DREAM_ENABLED=1."
        ),
    }


def _llm_summarize(facts: list[str]) -> str:
    api_key, base, model = openai_config(None)
    if not api_key and not base:
        raise RuntimeError("OPENAI_* not configured")
    bullet = "\n".join(f"- {f}" for f in facts[:80])
    messages = [
        {
            "role": "system",
            "content": (
                "你是记忆整理助手。根据事实列表，写出简洁、稳定的用户画像摘要（中文优先）。"
                "只保留长期有用信息；去掉一次性闲聊。输出纯文本，分短段落，不超过 800 字。"
            ),
        },
        {
            "role": "user",
            "content": f"请整理以下记忆事实为 durable profile：\n{bullet}",
        },
    ]
    url = f"{base.rstrip('/')}/chat/completions"
    headers = {"Content-Type": "application/json"}
    if api_key:
        headers["Authorization"] = f"Bearer {api_key}"
    payload = {"model": model, "stream": False, "messages": messages, "max_tokens": 900}
    with httpx.Client(timeout=90.0) as client:
        resp = client.post(url, headers=headers, json=payload)
        resp.raise_for_status()
        data = resp.json()
    choices = data.get("choices") or []
    if not choices:
        return ""
    msg = choices[0].get("message") or {}
    return str(msg.get("content") or "").strip()


def consolidate_user(user_id: str, *, max_facts: int = 60) -> dict[str, Any]:
    """Run one consolidation for a user. Returns status dict."""
    uid = (user_id or "").strip()
    if not uid:
        return {"ok": False, "error": "user_id required"}

    mode = dream_mode()
    if mode == "off":
        return {"ok": False, "error": "dream disabled", "status": status()}

    if mode == "platform":
        return {
            "ok": False,
            "error": (
                "Mem0 Platform Dream configured but open-bot OSS runtime does not call "
                "hosted Dream API yet; use MEM0_DREAM_ENABLED=1 for local consolidation "
                "or run Dream in the Mem0 Platform console."
            ),
            "status": status(),
        }

    facts: list[str] = []
    listed = mem0_store.list_for_user(uid, limit=max_facts)
    for item in listed.get("memories") or []:
        text = str(item.get("content") or "").strip()
        if text:
            facts.append(text)
        if len(facts) >= max_facts:
            break

    if len(facts) < max_facts:
        store = get_store(uid)
        try:
            legacy = store.list(limit=max_facts)
        except Exception:  # noqa: BLE001
            legacy = []
        for row in legacy or []:
            content = ""
            tier = ""
            if hasattr(row, "content"):
                content = str(getattr(row, "content") or "").strip()
                tier = str(getattr(row, "tier") or "")
            elif isinstance(row, dict):
                content = str(row.get("content") or "").strip()
                tier = str(row.get("tier") or "")
            if not content or tier == "profile":
                continue
            facts.append(content)
            if len(facts) >= max_facts:
                break

    # dedupe
    seen: set[str] = set()
    uniq: list[str] = []
    for f in facts:
        key = " ".join(f.lower().split())
        if key in seen:
            continue
        seen.add(key)
        uniq.append(f)
    facts = uniq

    if not facts:
        return {"ok": False, "error": "no facts to consolidate", "user_id": uid}

    try:
        summary = _llm_summarize(facts)
    except Exception as e:  # noqa: BLE001
        logger.warning("dream LLM failed user=%s: %s", uid, e)
        return {"ok": False, "error": f"llm failed: {e}", "user_id": uid}

    if not summary:
        return {"ok": False, "error": "empty summary", "user_id": uid}

    stamp = datetime.now(timezone.utc).strftime("%Y-%m-%d")
    content = f"[dream:{stamp}] {summary}"
    store = get_store(uid)
    try:
        item = store.upsert_tagged(content, tag="dream", tier="profile", scope="user")
        mid = getattr(item, "id", "") or ""
    except Exception as e:  # noqa: BLE001
        return {"ok": False, "error": f"memory write failed: {e}", "user_id": uid}

    return {
        "ok": True,
        "mode": "local",
        "user_id": uid,
        "facts_used": len(facts),
        "memory_id": mid,
        "summary_chars": len(summary),
    }


def main(argv: list[str] | None = None) -> int:
    import argparse

    ap = argparse.ArgumentParser(description="Local Dream consolidation")
    ap.add_argument("--user-id", required=True)
    ap.add_argument("--max-facts", type=int, default=60)
    args = ap.parse_args(argv)
    # Allow one-shot CLI even if env flag off
    os.environ.setdefault("MEM0_DREAM_ENABLED", "1")
    result = consolidate_user(args.user_id, max_facts=args.max_facts)
    print(result)
    return 0 if result.get("ok") else 1


if __name__ == "__main__":
    raise SystemExit(main())
