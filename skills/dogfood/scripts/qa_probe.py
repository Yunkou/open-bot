#!/usr/bin/env python3
"""Headless QA probe for exploratory web testing (open-bot dogfood skill).

Loads a URL in Chromium (Playwright), optionally runs a list of actions,
and prints a JSON report: console errors, page errors, failed requests,
visible interactive elements, and a screenshot path.

Install once:  pip install playwright && python -m playwright install chromium

Usage:
  python3 qa_probe.py https://example.com --out ./dogfood-output
  python3 qa_probe.py https://example.com/login --out ./dogfood-output \
      --actions '[{"fill":"input[name=email]","text":"a@b.c"},{"click":"text=Sign in"},{"wait":1}]'

Action keys: goto (url), click (selector), fill (selector) + text,
press (key, optional selector), wait (seconds), scroll (pixels).
Selectors are Playwright selectors (css, text=..., role=button[name="..."]).
"""
from __future__ import annotations

import argparse
import json
import pathlib
import re
import sys
import time


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("url")
    ap.add_argument("--out", default="./dogfood-output")
    ap.add_argument("--actions", default="[]", help="JSON list of actions")
    ap.add_argument("--width", type=int, default=1280)
    ap.add_argument("--height", type=int, default=800)
    ap.add_argument("--max-elements", type=int, default=60)
    args = ap.parse_args()

    try:
        from playwright.sync_api import sync_playwright
    except ImportError:
        print(json.dumps({"error": "playwright not installed",
                          "hint": "pip install playwright && python -m playwright install chromium"}))
        return 2

    out = pathlib.Path(args.out).expanduser()
    (out / "screenshots").mkdir(parents=True, exist_ok=True)
    actions = json.loads(args.actions)

    report: dict = {"url": args.url, "console": [], "page_errors": [], "failed_requests": [],
                    "http_errors": [], "actions": [], "elements": []}
    with sync_playwright() as p:
        browser = p.chromium.launch()
        page = browser.new_page(viewport={"width": args.width, "height": args.height})
        page.on("console", lambda m: report["console"].append({"type": m.type, "text": m.text[:500]})
                if m.type in ("error", "warning") else None)
        page.on("pageerror", lambda e: report["page_errors"].append(str(e)[:500]))
        page.on("requestfailed", lambda r: report["failed_requests"].append(
            {"url": r.url[:300], "failure": (r.failure or "")[:200]}))
        page.on("response", lambda r: report["http_errors"].append({"url": r.url[:300], "status": r.status})
                if r.status >= 400 else None)

        resp = page.goto(args.url, wait_until="networkidle", timeout=45000)
        report["status"] = resp.status if resp else None
        for a in actions:
            entry = {"action": a}
            try:
                if "goto" in a:
                    page.goto(a["goto"], wait_until="networkidle", timeout=45000)
                elif "click" in a:
                    page.click(a["click"], timeout=10000)
                elif "fill" in a:
                    page.fill(a["fill"], a.get("text", ""), timeout=10000)
                elif "press" in a:
                    if a.get("selector"):
                        page.press(a["selector"], a["press"])
                    else:
                        page.keyboard.press(a["press"])
                elif "scroll" in a:
                    page.mouse.wheel(0, int(a["scroll"]))
                if "wait" in a:
                    time.sleep(float(a["wait"]))
                else:
                    page.wait_for_load_state("networkidle", timeout=15000)
                entry["ok"] = True
            except Exception as e:  # noqa: BLE001
                entry["ok"] = False
                entry["error"] = str(e)[:300]
            report["actions"].append(entry)

        report["final_url"] = page.url
        report["title"] = page.title()
        els = page.query_selector_all("a, button, input, select, textarea, [role=button], [role=link]")
        for el in els:
            if len(report["elements"]) >= args.max_elements:
                break
            try:
                if not el.is_visible():
                    continue
                tag = el.evaluate("e => e.tagName.toLowerCase()")
                label = (el.inner_text() or el.get_attribute("aria-label") or el.get_attribute("placeholder")
                         or el.get_attribute("name") or el.get_attribute("href") or "")
                report["elements"].append({"tag": tag, "label": re.sub(r"\s+", " ", label)[:80]})
            except Exception:  # noqa: BLE001
                continue
        slug = re.sub(r"[^a-zA-Z0-9]+", "-", page.url)[-60:].strip("-") or "page"
        shot = out / "screenshots" / f"{int(time.time())}-{slug}.png"
        page.screenshot(path=str(shot), full_page=True)
        report["screenshot"] = str(shot)
        browser.close()

    print(json.dumps(report, ensure_ascii=False, indent=1))
    return 0


if __name__ == "__main__":
    sys.exit(main())
