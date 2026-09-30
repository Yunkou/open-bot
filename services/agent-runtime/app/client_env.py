"""Client environment envelope shared with Go/TS (platform / app / capabilities)."""

from __future__ import annotations

from typing import Any

from pydantic import BaseModel, Field


class ClientCapabilities(BaseModel):
    host_tools: bool = False
    workspace_tools: bool = True


class ClientContext(BaseModel):
    platform: str = Field(default="web")  # web|macos|windows|linux|ios|android
    app: str = Field(default="browser")  # browser|tauri|capacitor
    os: str = Field(default="")
    arch: str = Field(default="")
    app_version: str = Field(default="")
    locale: str = Field(default="")
    machine_id: str = Field(default="")
    machine_label: str = Field(default="")  # display name of current device
    capabilities: ClientCapabilities = Field(default_factory=ClientCapabilities)

    @classmethod
    def from_any(cls, raw: Any) -> ClientContext | None:
        if raw is None:
            return None
        if isinstance(raw, ClientContext):
            return raw
        if not isinstance(raw, dict):
            return None
        try:
            caps_raw = raw.get("capabilities") or {}
            if not isinstance(caps_raw, dict):
                caps_raw = {}
            caps = ClientCapabilities(
                host_tools=bool(caps_raw.get("host_tools", False)),
                workspace_tools=bool(caps_raw.get("workspace_tools", True)),
            )
            return ClientContext(
                platform=str(raw.get("platform") or "web").strip() or "web",
                app=str(raw.get("app") or "browser").strip() or "browser",
                os=str(raw.get("os") or "").strip(),
                arch=str(raw.get("arch") or "").strip(),
                app_version=str(raw.get("app_version") or "").strip(),
                locale=str(raw.get("locale") or "").strip(),
                machine_id=str(raw.get("machine_id") or "").strip(),
                machine_label=str(
                    raw.get("machine_label") or raw.get("display_name") or ""
                ).strip(),
                capabilities=caps,
            )
        except Exception:  # noqa: BLE001
            return None


_PLATFORM_LABEL = {
    "web": "网页浏览器",
    "macos": "macOS 桌面应用",
    "windows": "Windows 桌面应用",
    "linux": "Linux 桌面应用",
    "ios": "iOS 应用",
    "android": "Android 应用",
}


def format_environment_block(
    client: ClientContext | None,
    machines: list[dict[str, Any]] | None = None,
) -> str:
    """Build the 「环境」 system-prompt section (user-facing wording; no sandbox/Docker)."""
    lines = ["## 环境"]
    connected_labels = _connected_labels(machines)
    if client is None:
        lines.append(
            "当前客户端信息未知。不要假设用户在某台具体电脑上；"
            "若用户提到「我的电脑 / Downloads / 桌面」，先调用 list_machines 查看已注册主机。"
        )
    else:
        is_browser = client.app == "browser" or client.platform == "web"
        display = "" if is_browser else _current_device_display_name(client, machines)
        plat = _PLATFORM_LABEL.get(client.platform, client.platform)
        if is_browser:
            lines.append(
                "用户当前正在网页浏览器里聊天（不是桌面应用）。"
                "浏览器进程本身不能直接读写本机文件；"
                "但若 list_machines 显示另有已连接（connected）的电脑应用，"
                "必须用 host_ls / host_shell 等通过那台电脑代操作 Downloads / 桌面（不要拒绝、不要改用 sandbox）。"
                "没有任何已连接电脑时，才说明需要先打开桌面应用。"
            )
        elif display:
            lines.append(f"用户当前正在这台设备上聊天：{display}。")
        bits = [f"当前客户端：{plat}（app={client.app}"]
        if client.os:
            bits.append(f"os={client.os}")
        if client.arch:
            bits.append(f"arch={client.arch}")
        if client.app_version:
            bits.append(f"version={client.app_version}")
        if client.locale:
            bits.append(f"locale={client.locale}")
        if display:
            bits.append(f"device={display}")
        if client.machine_id and not is_browser:
            bits.append(f"machine_id={client.machine_id}")
        lines.append("，".join(bits) + "）。")
        lines.append(
            "本机文件只在已连接的电脑上读写；可用绝对路径或 ~/...。"
            "用户点名某台电脑时，用 list_machines 里对应且 connected 的 machine_id（以 label 识别）。"
            "没点名时用最常用的工作设备；它没连接就说明要打开那台，不要改到当前手机。"
            "还没有常用设备时才用当前 machine_id；浏览器且只有一台已连接电脑时用那一台。"
            "对不上或有多台都像时先问用户。打开软件用 host_open；本机命令用 host_shell（不要用它跑 ssh）。"
            "远程 SSH / 远程文件：先 load_skill host-ssh，再按说明调用 host_ssh_*（桌面应用无界面拨号）。"
            "覆盖、删除、移动、主目录外写入、host_shell，以及远程写入/删除/执行："
            "必须立刻调用对应 host_* / host_ssh_* 工具；对话里会出现「允许/拒绝」卡片，"
            "用户在网页或任意已登录端点按钮即可（不必去电脑本地另确认）。"
            "禁止用纯文字让用户「去电脑上确认」或「回复好/是」代替该卡片；调用后等待工具返回。"
            "操作仍在目标电脑执行。不要用内部运行环境冒充本机文件。"
        )
        lines.extend(_usual_machine_lines(machines))
        if not connected_labels:
            lines.append(
                "【硬性】此刻没有任何已连接的电脑应用。"
                "用户问 Downloads / 桌面 / 本机文件 / 本机命令时："
                "必须先 list_machines（或直接说明没有已打开的电脑应用）；"
                "禁止编造文件名、大小、修改时间；禁止用 sandbox_* 或记忆里的旧结果冒充本机。"
                "正确说法示例：「要看你电脑上的下载文件夹，请先打开桌面应用并保持在线。」"
            )

    lines.append(
        "内部工具 sandbox_*（仅模型可见）：用于脚本、临时文件与安全执行。"
        "对用户完全透明：禁止提及 sandbox / Docker / 容器 / /workspace，"
        "禁止把内部环境说成用户的另一台电脑；只谈结果（已生成、可预览、可下载）。"
        "该环境 ≠ 用户本机 Downloads/桌面；不要把内部路径说成用户的下载文件夹。"
    )
    lines.append(
        "关于本机文件的长期记忆可能过时；必须以本次 list_machines / host_* 的实时结果为准，"
        "不能凭记忆报「最大的文件是 xxx」。"
    )
    return "\n".join(lines)


def _connected_labels(machines: list[dict[str, Any]] | None) -> list[str]:
    if not machines:
        return []
    out: list[str] = []
    for m in machines:
        if not isinstance(m, dict):
            continue
        if not (m.get("connected") or m.get("online")):
            continue
        label = str(m.get("label") or m.get("id") or "").strip()
        if label:
            out.append(label)
    return out



def _current_device_display_name(
    client: ClientContext | None,
    machines: list[dict[str, Any]] | None,
) -> str:
    """Prefer server-side renamed label for the current machine_id."""
    if client is None:
        return ""
    mid = (client.machine_id or "").strip()
    if mid and machines:
        for m in machines:
            if not isinstance(m, dict):
                continue
            if str(m.get("id") or "").strip() == mid:
                label = str(m.get("label") or "").strip()
                if label:
                    return label
    return (client.machine_label or "").strip()


def _usual_machine_lines(machines: list[dict[str, Any]] | None) -> list[str]:
    if not machines:
        return []
    best: dict[str, Any] | None = None
    best_count = 0
    connected: list[str] = []
    for m in machines:
        if not isinstance(m, dict):
            continue
        label = str(m.get("label") or m.get("id") or "").strip()
        if m.get("connected") or m.get("online"):
            if label:
                connected.append(label)
        try:
            count = int(m.get("file_op_count") or 0)
        except (TypeError, ValueError):
            count = 0
        if count > best_count:
            best = m
            best_count = count
    lines: list[str] = []
    named: list[str] = []
    for m in machines:
        if not isinstance(m, dict):
            continue
        label = str(m.get("label") or m.get("id") or "").strip()
        mid = str(m.get("id") or "").strip()
        if not label:
            continue
        state = "已连接" if (m.get("connected") or m.get("online")) else "未连接"
        named.append(f"{label}（machine_id={mid}，{state}）")
    if named:
        lines.append("已登记的电脑：" + "；".join(named) + "。")
    if connected:
        lines.append("当前已连接、可打开软件和运行命令的电脑：" + "、".join(connected) + "。")
    if best is not None:
        label = str(best.get("label") or "工作设备")
        mid = str(best.get("id") or "")
        state = "已连接" if (best.get("connected") or best.get("online")) else "未连接"
        lines.append(f"最常用的工作设备：{label}（machine_id={mid}，{state}）。")
    return lines


def format_tools_routing_block(*, tools_enabled: bool, available_tool_names: list[str] | None) -> str:
    """Clarify builtins vs workspace vs host vs skills when tools are on."""
    if not tools_enabled:
        return ""
    names = [n for n in (available_tool_names or []) if n]
    lines = ["## 工具路由"]
    lines.append(
        "始终可用的内置工具（若已列出）：get_current_time / calculator / http_fetch / "
        "load_skill / memory_write / memory_recall 等；另有 MCP（mcp__*）与密钥工具。"
    )
    if any(n.startswith("sandbox_") for n in names):
        lines.append(
            "sandbox_*（内部名）：读写/执行临时文件与脚本；"
            "用户要跑脚本、写文件、生成可预览内容时用它们；回复只谈结果，不提工具或路径。"
        )
    if "list_machines" in names or any(n.startswith("host_") for n in names):
        lines.append(
            "主机：先 list_machines 看 connected；点名用对应 machine_id，没点名用最常用工作设备。"
            "host_open / host_shell / host_ls… 操作本机；远程 SSH 先 load_skill host-ssh 再调 host_ssh_*。"
            "覆盖、主目录外写入、删除、移动、shell 与远程写/删/exec 需对话确认。"
            "没有已连接电脑时如实说明（请先打开桌面应用），禁止编造 Downloads 文件名/大小；勿用 sandbox 冒充本机。"
            "删除等危险操作：必须先调用 host_delete 等工具；确认卡由系统弹出（网页可点），"
            "禁止用文字假装「已发起删除 / 请在电脑上确认 / 已批准」；工具结果是 denied 就是用户拒绝了。"
            "用户说「再试一次」且上下文是删除时，必须再次调用 host_delete，不要只 host_ls 后编造批准状态。"
            "问下载目录最大/最新/前几名时必须 host_ls（或先 list_machines），禁止用摘要或记忆编文件名和大小。"
            "同时删除多个文件时，一次调用 host_delete 并传 paths 数组；不要每个文件单独调用（否则会出多张确认卡）。"
        )
    lines.append(
        "Skills：目录 + load_skill；远程 SSH、本机复杂流程先 load_skill 再执行。"
    )
    return "\n".join(lines)


# User-visible aliases for tool status events (API names unchanged).
TOOL_DISPLAY_ALIASES: dict[str, str] = {
    "sandbox_ensure": "准备运行环境",
    "sandbox_shell": "执行命令",
    "sandbox_read": "读取文件",
    "sandbox_write": "写入文件",
    "sandbox_ls": "列出目录",
    "list_machines": "查看我的电脑",
    "host_ls": "列出本机目录",
    "host_read": "读取本机文件",
    "host_write": "写入本机文件",
    "host_delete": "删除本机文件",
    "host_move": "移动本机文件",
    "host_open": "打开软件",
    "host_shell": "在本机运行命令",
    "host_ssh_ls": "列出远程目录",
    "host_ssh_read": "读取远程文件",
    "host_ssh_write": "写入远程文件",
    "host_ssh_delete": "删除远程文件",
    "host_ssh_exec": "在远程主机上运行",
    "get_current_time": "获取当前时间",
    "calculator": "计算",
    "http_fetch": "获取网页",
    "load_skill": "加载技能",
    "memory_write": "写入记忆",
    "memory_recall": "回忆",
    "send_to_agent": "发送给助手",
    "request_secret": "请求密钥",
    "secret_http": "带密钥请求",
}


def tool_display_label(name: str) -> str:
    n = (name or "").strip()
    if not n:
        return "正在运行命令"
    if n in TOOL_DISPLAY_ALIASES:
        return TOOL_DISPLAY_ALIASES[n]
    if n.startswith("mcp__"):
        return f"调用 MCP · {n.removeprefix('mcp__')}"
    return f"正在运行 · {n}"
