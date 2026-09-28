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


def format_environment_block(client: ClientContext | None) -> str:
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
        lines.append("，".join(bits) + "）。")
        if client.platform == "web" or client.app == "browser":
            lines.append(
                "用户正在浏览器中使用；浏览器本身不是「主机电脑」。"
                "「我的电脑 / Downloads / 桌面」需先 list_machines；"
                "若没有已注册且在线的主机，请如实说明无法直接访问本机文件，"
                "不要用内部运行环境目录假装成 Downloads。"
            )
        elif client.app in ("tauri", "capacitor"):
            lines.append(
                "用户正在桌面/移动客户端。capabilities.host_tools="
                f"{str(client.capabilities.host_tools).lower()}："
                "Phase 1 本机文件读写尚未接通；请用 list_machines 确认主机在线，"
                "若用户要访问本机路径，诚实说明「本机文件访问尚未接通」。"
            )
        else:
            lines.append(
                "解释「我的电脑」时以 list_machines 结果为准；"
                "未在线或未注册则不要假装能访问本机。"
            )

    lines.append(
        "内部工具 sandbox_*（仅模型可见）：用于脚本、临时文件与安全执行。"
        "对用户完全透明：禁止提及 sandbox / Docker / 容器 / /workspace，"
        "禁止把内部环境说成用户的另一台电脑；只谈结果（已生成、可预览、可下载）。"
        "该环境 ≠ 用户本机 Downloads/桌面；不要把内部路径说成用户的下载文件夹。"
    )
    return "\n".join(lines)


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
    if "list_machines" in names:
        lines.append(
            "主机路径：「我的电脑 / Downloads / 桌面」→ 先调用 list_machines；"
            "仅当有在线主机且 host 能力可用时才谈本机操作。"
            "Phase 1：本机 shell/读写尚未接通；list_machines 仅列出已注册电脑。"
            "若在网页且无注册主机，诚实告知，勿用内部运行环境冒充本机。"
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
