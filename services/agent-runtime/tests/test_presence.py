"""Unit tests for PresencePublisher min-hold and soft-fail posting."""

from __future__ import annotations

import asyncio
import sys
import time
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

from app.presence import PresencePublisher  # noqa: E402


def _ok(cond: bool, msg: str) -> None:
    if not cond:
        raise AssertionError(msg)
    print(f"  OK  {msg}")


async def test_min_hold_and_same_skip() -> None:
    posts: list[str] = []

    def fake_post(payload: dict) -> None:
        posts.append(str(payload.get("status")))

    pub = PresencePublisher(
        "conv-1",
        "agent-1",
        user_id="u1",
        min_hold_ms=300,
        post_fn=fake_post,
    )

    t0 = time.monotonic()
    await pub.set("thinking")
    await pub.set("thinking")  # same → no second post, no wait
    _ok(posts == ["thinking"], f"same-status skipped post: {posts}")

    await pub.set("working")
    elapsed = (time.monotonic() - t0) * 1000
    _ok(elapsed >= 280, f"hold before thinking→working ≥~300ms (got {elapsed:.0f}ms)")
    _ok(posts == ["thinking", "working"], f"posted working: {posts}")

    t1 = time.monotonic()
    await pub.set("thinking")
    elapsed2 = (time.monotonic() - t1) * 1000
    _ok(elapsed2 >= 280, f"hold before working→thinking ≥~300ms (got {elapsed2:.0f}ms)")
    _ok(posts == ["thinking", "working", "thinking"], f"back to thinking: {posts}")


async def test_post_failure_soft() -> None:
    def boom(_payload: dict) -> None:
        raise OSError("api down")

    pub = PresencePublisher("c", "a", min_hold_ms=0, post_fn=boom)
    await pub.set("thinking")  # must not raise
    await pub.set("error")
    _ok(pub._current == "error", "local state advances despite post failure")


async def test_missing_ids_no_post() -> None:
    posts: list[dict] = []

    def fake_post(payload: dict) -> None:
        posts.append(payload)

    pub = PresencePublisher("", "", user_id=None, min_hold_ms=0, post_fn=fake_post)
    await pub.set("thinking")
    _ok(posts == [], "no post without conversation_id/user_id")
    _ok(pub._current == "thinking", "local status still tracked")


async def main() -> None:
    print("test_presence")
    await test_min_hold_and_same_skip()
    await test_post_failure_soft()
    await test_missing_ids_no_post()
    print("all passed")


if __name__ == "__main__":
    asyncio.run(main())
