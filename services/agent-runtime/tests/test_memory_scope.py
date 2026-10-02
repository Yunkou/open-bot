"""Scope resolution and per-scene memory quotas."""

from __future__ import annotations

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))

from app.mem0_store import _row_matches_scope
from app.memory import (
    MemoryItem,
    merge_scoped_snippets,
    normalize_pair,
    resolve_write_scope,
    scene_kind,
)


def _item(scope: str, content: str, tier: str = "note") -> MemoryItem:
    return MemoryItem(id=content, tier=tier, content=content, scope=scope)


def test_normalize_pair_is_order_independent():
    assert normalize_pair("b", "a") == ("a", "b")
    assert normalize_pair("a", "b") == normalize_pair("b", "a")


def test_resolve_write_scope_defaults():
    assert resolve_write_scope("", agent_id="bot-1") == ("bot", "bot-1", "", "")
    assert resolve_write_scope("", agent_id="bot-1", channel_id="ch-1") == ("channel", "", "ch-1", "")
    assert resolve_write_scope("user", agent_id="bot-1", channel_id="ch-1") == ("user", "", "", "")
    assert resolve_write_scope("agent_pair", agent_id="b", peer_agent_id="a") == ("agent_pair", "a", "", "b")


def test_scene_kind():
    assert scene_kind("", "") == "dm"
    assert scene_kind("ch", "") == "channel"
    assert scene_kind("ch", "peer") == "agent_pair"


def test_dm_quota_spills_to_user():
    explicit = {
        "bot": [_item("bot", f"bot-{i}") for i in range(2)],
        "user": [_item("user", f"user-{i}") for i in range(6)],
    }
    out = merge_scoped_snippets(explicit, {}, "dm")
    assert len(out) == 8
    assert sum(1 for s in out if s.startswith("[bot]")) == 2
    assert sum(1 for s in out if s.startswith("[user]")) == 6


def test_explicit_beats_mem0_duplicate_inside_scope():
    explicit = {"bot": [_item("bot", "same fact", tier="profile")], "user": []}
    mem0 = {"bot": ["same fact", "extra"], "user": []}
    out = merge_scoped_snippets(explicit, mem0, "dm")
    assert out[0] == "[bot] [profile] same fact"
    assert "[bot] [mem0] extra" in out
    assert not any("[mem0] same fact" in s for s in out)


def test_mem0_bot_scope_reads_identity_agent_id():
    row = {"memory": "likes tea", "agent_id": "bot-1", "metadata": {"scope": "bot"}}
    assert _row_matches_scope(row, scope="bot", agent_id="bot-1")
    assert _row_matches_scope(row, scope="bot")
    assert not _row_matches_scope(row, scope="bot", agent_id="other")
    assert not _row_matches_scope(row, scope="user")


def test_channel_order_and_cap():
    explicit = {
        "channel": [_item("channel", f"c-{i}") for i in range(6)],
        "bot": [_item("bot", f"b-{i}") for i in range(6)],
        "user": [_item("user", f"u-{i}") for i in range(6)],
    }
    out = merge_scoped_snippets(explicit, {}, "channel")
    assert len(out) == 8
    assert out[0].startswith("[channel]")
    assert sum(1 for s in out if s.startswith("[channel]")) == 4
    assert sum(1 for s in out if s.startswith("[bot]")) == 3
    assert sum(1 for s in out if s.startswith("[user]")) == 1


def test_build_recall_payload_matches_injected_snippets():
    from app.memory import build_recall_payload, merge_scoped_snippets

    explicit = {
        "bot": [_item("bot", "likes tea", tier="profile")],
        "user": [_item("user", "lives in shanghai")],
    }
    mem0 = {"bot": ["likes coffee"], "user": []}
    snippets = merge_scoped_snippets(explicit, mem0, "dm")
    payload = build_recall_payload(explicit, mem0, "dm", recalled=explicit["bot"] + explicit["user"], mem0_hits=mem0["bot"])
    assert payload["scene"] == "dm"
    assert payload["explicit_count"] == 2
    assert payload["mem0_count"] == 1
    assert [i["snippet"] for i in payload["items"]] == snippets
    assert payload["items"][0]["source"] == "explicit"
    assert payload["items"][0]["tier"] == "profile"
    assert any(i["source"] == "mem0" for i in payload["items"])


def test_build_recall_records_clips_long_content():
    from app.memory import MAX_RECALL_SNIPPET_CHARS, build_recall_records

    long = "x" * (MAX_RECALL_SNIPPET_CHARS + 80)
    explicit = {"bot": [_item("bot", long)], "user": []}
    items = build_recall_records(explicit, {}, "dm")
    assert len(items) == 1
    assert len(items[0]["content"]) <= MAX_RECALL_SNIPPET_CHARS
    assert items[0]["content"].endswith("…")
