"""Routing for host file ops across logged-in computers."""

from __future__ import annotations

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

from app.machines import select_machine  # noqa: E402


def _ok(cond: bool, msg: str) -> None:
    if not cond:
        raise AssertionError(msg)
    print(f"  OK  {msg}")


MAC = {"id": "mac", "label": "我的 Mac", "platform": "macos", "connected": True, "file_op_count": 4}
PHONE = {"id": "phone", "label": "我的 Android", "platform": "android", "connected": True, "file_op_count": 0}
OFFLINE = {"id": "mac", "label": "我的 Mac", "platform": "macos", "connected": False, "file_op_count": 4}


def test_named_machine() -> None:
    got = select_machine([MAC, PHONE], explicit_id="mac", current_id="phone")
    _ok(got["ok"] and got["machine"]["id"] == "mac", "named mac wins over current phone")


def test_unnamed_uses_usual() -> None:
    got = select_machine([MAC, PHONE], current_id="phone")
    _ok(got["ok"] and got["machine"]["id"] == "mac", "unnamed path uses most-used mac")


def test_usual_offline_does_not_fall_back() -> None:
    got = select_machine([OFFLINE, PHONE], current_id="phone")
    _ok(not got["ok"] and "不要改到" in got["error"], "offline usual machine is not replaced by the phone")


def test_no_history_uses_current() -> None:
    fresh = {"id": "phone", "label": "我的 Android", "connected": True, "file_op_count": 0}
    got = select_machine([fresh], current_id="phone")
    _ok(got["ok"] and got["machine"]["id"] == "phone", "no history uses the current device")


def test_browser_single_connected() -> None:
    got = select_machine([MAC], current_id="")
    _ok(got["ok"] and got["machine"]["id"] == "mac", "browser with one connected computer uses it")


if __name__ == "__main__":
    test_named_machine()
    test_unnamed_uses_usual()
    test_usual_offline_does_not_fall_back()
    test_no_history_uses_current()
    test_browser_single_connected()
    print("all passed")
