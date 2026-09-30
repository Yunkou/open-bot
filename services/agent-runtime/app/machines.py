"""ListMachines helper: call Go internal API (same auth as sandbox/agent_bus)."""

from __future__ import annotations

import json
import os
import urllib.error
import urllib.request
from typing import Any

DEFAULT_API_URL = "http://127.0.0.1:18080"
DEFAULT_INTERNAL_TOKEN = "open-bot-dev-internal"


def _api_base() -> str:
    return (os.getenv("OPENBOT_API_URL") or os.getenv("API_BASE_URL") or DEFAULT_API_URL).rstrip(
        "/"
    )


def _internal_token() -> str:
    return (
        (os.getenv("INTERNAL_TOKEN") or os.getenv("OPENBOT_INTERNAL_TOKEN") or DEFAULT_INTERNAL_TOKEN)
        .strip()
        or DEFAULT_INTERNAL_TOKEN
    )


def list_machines(user_id: str, timeout: float = 15.0) -> dict[str, Any]:
    """Return {"machines": [...], "count": N} for the user."""
    uid = (user_id or "").strip()
    if not uid:
        return {"error": "user_id required", "machines": [], "count": 0}
    url = f"{_api_base()}/internal/machines/list"
    data = json.dumps({"user_id": uid}).encode("utf-8")
    req = urllib.request.Request(
        url,
        data=data,
        method="POST",
        headers={
            "Content-Type": "application/json",
            "X-Internal-Token": _internal_token(),
            "Accept": "application/json",
        },
    )
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read().decode("utf-8")
            return json.loads(raw) if raw else {"machines": [], "count": 0}
    except urllib.error.HTTPError as e:
        detail = e.read().decode("utf-8", errors="replace")
        return {"error": f"machines api HTTP {e.code}: {detail}", "machines": [], "count": 0}
    except urllib.error.URLError as e:
        return {"error": f"machines api unreachable: {e}", "machines": [], "count": 0}


def _connected(machine: dict[str, Any]) -> bool:
    if "connected" in machine:
        return bool(machine.get("connected"))
    return bool(machine.get("online"))


def usual_work_machine(machines: list[dict[str, Any]]) -> dict[str, Any] | None:
    best: dict[str, Any] | None = None
    best_count = 0
    for m in machines:
        try:
            count = int(m.get("file_op_count") or 0)
        except (TypeError, ValueError):
            count = 0
        if count > best_count:
            best = m
            best_count = count
    return best


def select_machine(
    machines: list[dict[str, Any]],
    *,
    explicit_id: str = "",
    current_id: str = "",
) -> dict[str, Any]:
    """Pick the host for a file op.

    Named machine_id wins. Otherwise the most-used work machine when it is
    connected. With no history, fall back to the current device, then the only
    connected computer.
    """
    rows = [m for m in machines if isinstance(m, dict) and str(m.get("id") or "").strip()]
    by_id = {str(m["id"]): m for m in rows}
    explicit = (explicit_id or "").strip()
    current = (current_id or "").strip()
    if explicit:
        chosen = by_id.get(explicit)
        if chosen is None:
            return {"ok": False, "error": "没有这台已登记的电脑", "machines": rows}
        if not _connected(chosen):
            label = str(chosen.get("label") or "那台电脑")
            return {
                "ok": False,
                "error": f"「{label}」的应用没开着，请先在那台电脑上打开",
                "machine_id": explicit,
                "label": label,
                "machines": rows,
            }
        return {"ok": True, "machine": chosen}

    usual = usual_work_machine(rows)
    if usual is not None:
        label = str(usual.get("label") or "最常用的工作设备")
        if not _connected(usual):
            return {
                "ok": False,
                "error": f"最常用的工作设备「{label}」没开着，请先打开它，不要改到当前这台手机",
                "machine_id": str(usual.get("id") or ""),
                "label": label,
                "machines": rows,
            }
        return {"ok": True, "machine": usual}

    connected = [m for m in rows if _connected(m)]
    if current and current in by_id and _connected(by_id[current]):
        return {"ok": True, "machine": by_id[current]}
    if len(connected) == 1:
        return {"ok": True, "machine": connected[0]}
    if not connected:
        return {"ok": False, "error": "没有已打开的电脑应用，无法访问本机文件", "machines": rows}
    labels = "、".join(str(m.get("label") or m.get("id")) for m in connected)
    return {
        "ok": False,
        "error": f"有多台电脑开着（{labels}），请说明要操作哪一台",
        "machines": rows,
    }


def exec_host(
    user_id: str,
    machine_id: str,
    op: str,
    *,
    path: str = "",
    dest: str = "",
    content: str = "",
    conversation_id: str = "",
    ssh_host: str = "",
    ssh_user: str = "",
    ssh_port: int = 0,
    timeout: float = 100.0,
) -> dict[str, Any]:
    uid = (user_id or "").strip()
    mid = (machine_id or "").strip()
    if not uid or not mid:
        return {"ok": False, "error": "user_id and machine_id required"}
    url = f"{_api_base()}/internal/machines/exec"
    payload: dict[str, Any] = {
        "user_id": uid,
        "machine_id": mid,
        "op": op,
        "path": path,
        "dest": dest,
        "content": content,
        "conversation_id": conversation_id,
    }
    if ssh_host:
        payload["ssh_host"] = ssh_host
        payload["ssh_user"] = ssh_user
        payload["ssh_port"] = ssh_port
    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(
        url,
        data=data,
        method="POST",
        headers={
            "Content-Type": "application/json",
            "X-Internal-Token": _internal_token(),
            "Accept": "application/json",
        },
    )
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read().decode("utf-8")
            return json.loads(raw) if raw else {"ok": False, "error": "empty response"}
    except urllib.error.HTTPError as e:
        detail = e.read().decode("utf-8", errors="replace")
        return {"ok": False, "error": f"host exec HTTP {e.code}: {detail}"}
    except urllib.error.URLError as e:
        return {"ok": False, "error": f"host exec unreachable: {e}"}


WORK_DEVICE_TAG = "host-work-device"


def remember_usual_device(mem: Any, usual: dict[str, Any] | None) -> None:
    if mem is None or not isinstance(usual, dict):
        return
    label = str(usual.get("label") or "").strip()
    mid = str(usual.get("id") or "").strip()
    if not label or not mid:
        return
    platform = str(usual.get("platform") or "").strip()
    text = (
        f"最常用的工作设备是{label}（machine_id={mid}"
        + (f"，platform={platform}" if platform else "")
        + "）。用户没点名电脑、只说打开某个路径时用这台；"
        "它没连接就告诉用户打开这台电脑，不要改用当前手机。"
    )
    try:
        mem.upsert_tagged(text, WORK_DEVICE_TAG, tier="profile", scope="user")
    except Exception:  # noqa: BLE001
        return

