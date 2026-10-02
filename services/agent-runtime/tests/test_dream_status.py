"""Dream status helpers (no live LLM)."""

from __future__ import annotations

import os
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

from app import dream  # noqa: E402


def test_dream_mode_local_by_default() -> None:
    os.environ.pop("MEM0_DREAM_ENABLED", None)
    os.environ.pop("MEM0_PLATFORM_DREAM", None)
    os.environ.pop("MEM0_PLATFORM_API_KEY", None)
    os.environ.pop("MEM0_API_KEY", None)
    assert dream.dream_mode() == "local"
    st = dream.status()
    assert st["dream_mode"] == "local"
    assert st["local_enabled"] is True
    assert "interval_seconds" in st
    assert "Local Dream defaults on" in st["note"] or "MEM0_DREAM_ENABLED" in st["note"]


def test_local_dream_can_disable() -> None:
    os.environ["MEM0_DREAM_ENABLED"] = "0"
    os.environ.pop("MEM0_PLATFORM_DREAM", None)
    os.environ.pop("MEM0_PLATFORM_API_KEY", None)
    os.environ.pop("MEM0_API_KEY", None)
    assert dream.dream_mode() == "off"
    assert dream.local_dream_enabled() is False
    os.environ.pop("MEM0_DREAM_ENABLED", None)


def test_local_dream_flag_explicit_on() -> None:
    os.environ["MEM0_DREAM_ENABLED"] = "1"
    os.environ.pop("MEM0_PLATFORM_DREAM", None)
    os.environ.pop("MEM0_PLATFORM_API_KEY", None)
    os.environ.pop("MEM0_API_KEY", None)
    assert dream.dream_mode() == "local"
    os.environ.pop("MEM0_DREAM_ENABLED", None)


def test_platform_takes_precedence() -> None:
    os.environ["MEM0_DREAM_ENABLED"] = "1"
    os.environ["MEM0_PLATFORM_DREAM"] = "1"
    os.environ["MEM0_API_KEY"] = "pk-test"
    assert dream.dream_mode() == "platform"
    os.environ.pop("MEM0_DREAM_ENABLED", None)
    os.environ.pop("MEM0_PLATFORM_DREAM", None)
    os.environ.pop("MEM0_API_KEY", None)


def test_maybe_consolidate_bg_respects_disable() -> None:
    dream.reset_rate_limits_for_tests()
    os.environ["MEM0_DREAM_ENABLED"] = "0"
    os.environ.pop("MEM0_PLATFORM_DREAM", None)
    os.environ.pop("MEM0_PLATFORM_API_KEY", None)
    os.environ.pop("MEM0_API_KEY", None)
    assert dream.maybe_consolidate_user_bg("user-test") is False
    os.environ.pop("MEM0_DREAM_ENABLED", None)


def test_maybe_consolidate_bg_rate_limit() -> None:
    dream.reset_rate_limits_for_tests()
    os.environ["MEM0_DREAM_ENABLED"] = "1"
    os.environ["MEM0_DREAM_INTERVAL_SECONDS"] = "3600"
    os.environ.pop("MEM0_PLATFORM_DREAM", None)
    os.environ.pop("MEM0_PLATFORM_API_KEY", None)
    os.environ.pop("MEM0_API_KEY", None)
    # First call starts a thread (may fail later with no facts/LLM — ok)
    started = dream.maybe_consolidate_user_bg("user-rate")
    assert started is True
    # Immediate second call should be rate-limited / in-flight
    assert dream.maybe_consolidate_user_bg("user-rate") is False
    dream.reset_rate_limits_for_tests()
    os.environ.pop("MEM0_DREAM_ENABLED", None)
    os.environ.pop("MEM0_DREAM_INTERVAL_SECONDS", None)


def test_dream_interval_bounds() -> None:
    os.environ["MEM0_DREAM_INTERVAL_SECONDS"] = "10"
    assert dream.dream_interval_seconds() == 60  # floor
    os.environ["MEM0_DREAM_INTERVAL_SECONDS"] = "99999999"
    assert dream.dream_interval_seconds() == 86400 * 7  # cap
    os.environ.pop("MEM0_DREAM_INTERVAL_SECONDS", None)
    assert dream.dream_interval_seconds() == 3600


if __name__ == "__main__":
    test_dream_mode_local_by_default()
    test_local_dream_can_disable()
    test_local_dream_flag_explicit_on()
    test_platform_takes_precedence()
    test_maybe_consolidate_bg_respects_disable()
    test_maybe_consolidate_bg_rate_limit()
    test_dream_interval_bounds()
    print("ok")
