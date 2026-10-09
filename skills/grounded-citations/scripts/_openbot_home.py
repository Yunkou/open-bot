"""Resolve the open-bot per-user data dir for standalone skill scripts.

Adapted from Hermes Agent's _hermes_home.py (MIT). Default: ~/.open-bot,
override with OPEN_BOT_HOME.
"""

from __future__ import annotations

import os
from pathlib import Path


def get_openbot_home() -> Path:
    val = os.environ.get("OPEN_BOT_HOME", "").strip()
    home = Path(val).expanduser() if val else Path.home() / ".open-bot"
    home.mkdir(parents=True, exist_ok=True)
    return home


def display_openbot_home() -> str:
    home = get_openbot_home()
    try:
        return "~/" + str(home.relative_to(Path.home()))
    except ValueError:
        return str(home)
