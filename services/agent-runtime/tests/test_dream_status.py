"""Dream status helpers (no live LLM)."""

from __future__ import annotations

import os
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

from app import dream  # noqa: E402


def test_dream_mode_off_by_default(monkeypatch=None) -> None:
    os.environ.pop("MEM0_DREAM_ENABLED", None)
    os.environ.pop("MEM0_PLATFORM_DREAM", None)
    os.environ.pop("MEM0_PLATFORM_API_KEY", None)
    os.environ.pop("MEM0_API_KEY", None)
    assert dream.dream_mode() == "off"
    st = dream.status()
    assert st["dream_mode"] == "off"
    assert "Platform Dream" in st["note"] or "local" in st["note"]


def test_local_dream_flag() -> None:
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


if __name__ == "__main__":
    test_dream_mode_off_by_default()
    test_local_dream_flag()
    test_platform_takes_precedence()
    print("ok")
