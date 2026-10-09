"""Lessons injection: only status=active lessons reach the system prompt."""

from __future__ import annotations

import json
from unittest import mock

from app import lessons


class _Resp:
    def __init__(self, payload: dict):
        self._raw = json.dumps(payload).encode("utf-8")

    def read(self) -> bytes:
        return self._raw

    def __enter__(self):
        return self

    def __exit__(self, *exc):
        return False


def test_fetch_active_lessons_filters_non_active_and_empty():
    payload = {
        "lessons": [
            {"id": "a", "title": "简洁", "body": "先给结论", "status": "active", "tags": ["verbose"]},
            {"id": "p", "title": "待定", "body": "不应注入", "status": "pending"},
            {"id": "i", "title": "忽略", "body": "不应注入", "status": "ignored"},
            {"id": "e", "title": "", "body": "缺标题", "status": "active"},
        ]
    }
    with mock.patch.object(lessons.urllib.request, "urlopen", return_value=_Resp(payload)) as op:
        got = lessons.fetch_active_lessons("u1", "bot1")
    assert [x["id"] for x in got] == ["a"]
    req = op.call_args[0][0]
    assert "/internal/lessons/active?" in req.full_url
    assert "user_id=u1" in req.full_url and "agent_id=bot1" in req.full_url


def test_fetch_active_lessons_without_user_skips_http():
    with mock.patch.object(lessons.urllib.request, "urlopen") as op:
        assert lessons.fetch_active_lessons("", "bot1") == []
    op.assert_not_called()


def test_fetch_active_lessons_swallows_errors():
    with mock.patch.object(lessons.urllib.request, "urlopen", side_effect=OSError("down")):
        assert lessons.fetch_active_lessons("u1", "bot1") == []


def test_format_and_block():
    assert lessons.format_lessons_for_prompt([]) == ""
    block = lessons.format_lessons_for_prompt(
        [{"title": "简洁", "body": "先给结论", "tags": ["太啰嗦"]}]
    )
    assert "用户已确认的经验" in block
    assert "- **简洁** [太啰嗦]：先给结论" in block
    payload = {"lessons": [{"id": "a", "title": "T", "body": "B", "status": "active"}]}
    with mock.patch.object(lessons.urllib.request, "urlopen", return_value=_Resp(payload)):
        got, blk = lessons.active_lessons_with_block("u1", "bot1")
    assert [x["id"] for x in got] == ["a"] and "**T**" in blk
