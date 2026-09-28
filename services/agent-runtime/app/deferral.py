"""Whether this agent turn is still running.

Completion is the run itself, not the wording of the reply. A delivery tool
means the work has been done. A text answer in a thread that already has a
file, with no delivery tool this turn, means the agent stopped too early.
"""

from __future__ import annotations

# Reading or listing does not count as handing the user a result.
DELIVERY_TOOLS = frozenset({"sandbox_write", "sandbox_shell"})

# Fed back into the same run. Not shown to the user and not matched against
# the model's previous sentence.
CONTINUE_WORK = (
    "这一轮还没做完。请直接调用工具把文件改完，"
    "完成后在回复里给出可打开的文件链接。"
    "如果确实不需要改文件，就用一句话回答，不要再说稍后。"
)


def history_has_artifact(messages: list[dict] | None) -> bool:
    for message in messages or []:
        content = message.get("content") if isinstance(message, dict) else ""
        if isinstance(content, str) and "sandbox:" in content:
            return True
    return False


def turn_unfinished(tools_used: list[str] | None, messages: list[dict] | None) -> bool:
    """True when the agent has not finished and should keep running.

    Wording is ignored. Delivery tools finish the turn. With no file in the
    thread yet, a text reply is the result (chat). A file already in the
    thread, and no delivery tool this turn, means the job is still open.
    """
    if any(name in DELIVERY_TOOLS for name in (tools_used or [])):
        return False
    return history_has_artifact(messages)
