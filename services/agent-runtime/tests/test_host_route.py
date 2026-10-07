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


MAC = {
    "id": "mac",
    "label": "我的 Mac",
    "platform": "macos",
    "device_type": "desktop",
    "host_eligible": True,
    "connected": True,
    "file_op_count": 4,
}
PHONE = {
    "id": "phone",
    "label": "我的 Android",
    "platform": "android",
    "device_type": "mobile",
    "host_eligible": False,
    "connected": True,
    "file_op_count": 0,
}
OFFLINE = {
    "id": "mac",
    "label": "我的 Mac",
    "platform": "macos",
    "device_type": "desktop",
    "host_eligible": True,
    "connected": False,
    "file_op_count": 4,
}
MAC2 = {
    "id": "mac2",
    "label": "公司 Mac",
    "platform": "macos",
    "device_type": "desktop",
    "host_eligible": True,
    "connected": True,
    "file_op_count": 9,
}


def test_named_machine() -> None:
    got = select_machine([MAC, PHONE], explicit_id="mac", current_id="phone")
    _ok(got["ok"] and got["machine"]["id"] == "mac", "named mac wins over current phone")


def test_named_phone_rejected() -> None:
    got = select_machine([MAC, PHONE], explicit_id="phone")
    _ok(not got["ok"] and "手机" in got["error"], "named phone cannot host")


def test_session_mobile_rejected() -> None:
    got = select_machine([MAC, PHONE], current_id="phone", preferred_id="mac")
    _ok(not got["ok"] and "手机" in got["error"], "session phone is not a host")


def test_offline_current_does_not_fall_back() -> None:
    got = select_machine([OFFLINE, PHONE], current_id="mac", preferred_id="phone")
    _ok(
        not got["ok"] and "当前电脑未连接" in got["error"],
        "offline session host is not replaced by preferred/phone",
    )


def test_browser_preferred_phone_ignored() -> None:
    got = select_machine([MAC, PHONE], current_id="", preferred_id="phone")
    _ok(got["ok"] and got["machine"]["id"] == "mac", "preferred phone ignored → sole desktop")


def test_browser_preferred_offline_phone_only_refuses() -> None:
    got = select_machine([OFFLINE, PHONE], current_id="", preferred_id="mac")
    _ok(not got["ok"] and "桌面" in got["error"], "preferred offline + phone-only → need desktop")


def test_browser_multi_no_preferred_asks() -> None:
    got = select_machine([MAC, MAC2], current_id="", preferred_id="")
    _ok(not got["ok"] and "多台" in got["error"], "browser multi online without preferred asks")


def test_browser_single_connected() -> None:
    got = select_machine([MAC], current_id="")
    _ok(got["ok"] and got["machine"]["id"] == "mac", "browser with one connected computer uses it")


def test_phone_only_connected_refuses() -> None:
    got = select_machine([PHONE], current_id="")
    _ok(not got["ok"], "phone-only connected cannot host")


if __name__ == "__main__":
    test_named_machine()
    test_named_phone_rejected()
    test_session_mobile_rejected()
    test_offline_current_does_not_fall_back()
    test_browser_preferred_phone_ignored()
    test_browser_preferred_offline_phone_only_refuses()
    test_browser_multi_no_preferred_asks()
    test_browser_single_connected()
    test_phone_only_connected_refuses()
    print("all passed")
