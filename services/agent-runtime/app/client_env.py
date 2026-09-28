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
    if client is None:
        lines.append(
            "当前客户端信息未知。不要假设用户在某台具体电脑上；"
            "若用户提到「我的电脑 / Downloads / 桌面」，先调用 list_machines 查看已注册主机。"
        )
    else:
        plat = _PLATFORM_LABEL.get(client.platform, client.platform)
        bits = [f"当前客户端：{plat}（app={client.app}"]
        if client.os:
            bits.append(f"os={client.os}")
        if client.arch:
            bits.append(f"arch={client.arch}")
        if client.app_version:
            bits.append(f"version={client.app_version}")
        if client.locale:
            bits.append(f"locale={client.locale}")
        if client.machine_id:
            bits.append(f"machine_id={client.machine_id}")
        lines.append("，".join(bits) + "）。")
        lines.append(
            "本机文件只在已连接的电脑上读写，范围是 Downloads、Desktop、Documents。"
            "用户点名某台电脑时，用 list_machines 里对应且 connected 的 machine_id。"
            "没点名、只说打开某个路径时，用最常用的工作设备；它没连接就说明要打开那台，不要改到当前手机。"
            "还没有常用设备时才用当前 machine_id；当前是浏览器且只有一台已连接电脑时用那一台。"
            "对不上或有多台都像时先问用户。打开软件用 host_open；"
            "运行命令或 ssh 用 host_shell，交互式会话设 terminal=true，会打开终端。"
            "覆盖已有文件、删除、移动，以及 host_shell，会在对话里等用户点允许或拒绝；"
            "人在网页或手机上发起时，告诉用户直接在当前对话里确认，操作仍在目标电脑上执行。"
            "不要用内部运行环境冒充本机 Downloads。"
        )
        lines.extend(_usual_machine_lines(machines))

    lines.append(
        "内部工具 sandbox_*（仅模型可见）：用于脚本、临时文件与安全执行。"
        "对用户完全透明：禁止提及 sandbox / Docker / 容器 / /workspace，"
        "禁止把内部环境说成用户的另一台电脑；只谈结果（已生成、可预览、可下载）。"
        "该环境 ≠ 用户本机 Downloads/桌面；不要把内部路径说成用户的下载文件夹。"
    )
    return "\n".join(lines)


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
            "主机文件：先 list_machines 看 connected。"
            "用户点名某台电脑就用那台的 machine_id；"
            "没点名时用最常用的工作设备（环境块里的 machine_id），它未连接就说明要打开，不要改用当前手机；"
            "还没有常用设备才用当前设备。浏览器且只有一台已连接电脑时用那一台。"
            "host_open 打开本机软件。host_shell 运行命令；ssh 等交互命令设 terminal=true，在终端里打开。"
            "host_ls / host_read / host_write 在 Downloads、Desktop、Documents 内执行；"
            "覆盖、host_delete、host_move、host_shell 会在对话里等待确认，任意已登录端都能点。"
            "没有已连接电脑时如实说明，勿用内部运行环境冒充本机。"
        )
    lines.append(
        "Skills：仍通过目录 + load_skill 加载全文；与桌面/本机相关的技能先 load_skill 再按说明执行。"
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
