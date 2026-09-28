"""Unit/smoke tests for builtin_tools (no pytest required — runnable as script)."""

from __future__ import annotations

import asyncio
import json
import os
import sys
from pathlib import Path

# Allow `python tests/test_builtin_tools.py` from agent-runtime/
ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

from app import builtin_tools as bt  # noqa: E402


def _ok(cond: bool, msg: str) -> None:
    if not cond:
        raise AssertionError(msg)
    print(f"  OK  {msg}")


async def test_calculator() -> None:
    out = await bt.calculator({"expression": "1+2*3"})
    _ok(out == "7", f"1+2*3 -> 7 (got {out!r})")

    out = await bt.calculator({"expression": "(10-3)//2"})
    _ok(out == "3", f"(10-3)//2 -> 3 (got {out!r})")

    out = await bt.calculator({"expression": "2**10"})
    _ok(out == "1024", f"2**10 -> 1024 (got {out!r})")

    bad = await bt.calculator({"expression": "import os"})
    data = json.loads(bad)
    _ok("error" in data, f"reject import os: {bad}")

    bad2 = await bt.calculator({"expression": "__import__('os').system('id')"})
    data2 = json.loads(bad2)
    _ok("error" in data2, f"reject __import__: {bad2}")

    bad3 = await bt.calculator({"expression": "open('/etc/passwd').read()"})
    data3 = json.loads(bad3)
    _ok("error" in data3, f"reject open(): {bad3}")


async def test_get_current_time() -> None:
    out = await bt.get_current_time({})
    data = json.loads(out)
    _ok("iso8601" in data, f"has iso8601: {out[:200]}")
    _ok(data.get("timezone") in ("Asia/Shanghai", os.getenv("OPENBOT_TZ"), os.getenv("TZ"))
        or data.get("timezone") == "Asia/Shanghai"
        or "+" in data.get("iso8601", "") or data.get("iso8601", "").endswith("+08:00")
        or "Asia/Shanghai" in str(data.get("timezone")),
        f"default tz Asia/Shanghai-ish: {data.get('timezone')} iso={data.get('iso8601')}")
    # Stronger check: offset +08:00 when default Shanghai and no TZ override
    if not (os.getenv("OPENBOT_TZ") or os.getenv("TZ")):
        _ok(
            data.get("timezone") == "Asia/Shanghai"
            and ("+08:00" in data.get("iso8601", "") or "+08" in data.get("local", "")),
            f"Shanghai offset: {data}",
        )
    _ok("weekday" in data and "weekday_zh" in data, "has weekday fields")

    bad = await bt.get_current_time({"timezone": "Not/A_Real_Zone"})
    data_bad = json.loads(bad)
    _ok("error" in data_bad, f"invalid tz error: {bad}")


async def test_http_fetch_ssrf() -> None:
    # Ensure private blocked by default
    os.environ.pop("HTTP_FETCH_ALLOW_PRIVATE", None)
    out = await bt.http_fetch({"url": "http://127.0.0.1/"})
    data = json.loads(out)
    _ok("error" in data, f"reject 127.0.0.1: {out}")
    _ok(
        "blocked" in data["error"].lower() or "blocked" in out.lower(),
        f"error mentions blocked: {out}",
    )

    out2 = await bt.http_fetch({"url": "http://169.254.169.254/latest/meta-data/"})
    data2 = json.loads(out2)
    _ok("error" in data2, f"reject metadata IP: {out2}")

    out3 = await bt.http_fetch({"url": "file:///etc/passwd"})
    data3 = json.loads(out3)
    _ok("error" in data3, f"reject file://: {out3}")


async def main() -> None:
    print("test_calculator")
    await test_calculator()
    print("test_get_current_time")
    await test_get_current_time()
    print("test_http_fetch_ssrf")
    await test_http_fetch_ssrf()
    print("ALL PASSED")


if __name__ == "__main__":
    asyncio.run(main())
