from pathlib import Path

from app.client_env import tool_display_label
from app.llm import (
    DIAGRAM_SKILL_TRIGGER,
    USER_FACING_INTERNAL_HIDE,
    build_system_prompt,
)
from app.skills import SkillRegistry


def test_system_prompt_hides_internal_words_rule():
    prompt = build_system_prompt(
        agent_id="open-bot",
        skills_catalog="- 画图: 用户要流程图时先加载",
        memory_snippets=[],
        tools_enabled=True,
        available_tool_names=["load_skill"],
    )
    assert USER_FACING_INTERNAL_HIDE in prompt
    assert "禁止出现" in USER_FACING_INTERNAL_HIDE
    assert "skill" in USER_FACING_INTERNAL_HIDE.lower()


def test_system_prompt_triggers_diagram_skill():
    prompt = build_system_prompt(
        agent_id="open-bot",
        skills_catalog="- 画图: 用户要流程图时先加载",
        memory_snippets=[],
        tools_enabled=True,
        available_tool_names=["load_skill"],
    )
    assert DIAGRAM_SKILL_TRIGGER in prompt
    assert "画图" in prompt
    assert "内部加载" in DIAGRAM_SKILL_TRIGGER
    assert "不要向用户解释加载失败" in DIAGRAM_SKILL_TRIGGER
    assert "===|" in DIAGRAM_SKILL_TRIGGER
    assert "对用户只说" in DIAGRAM_SKILL_TRIGGER


def test_trigger_mentions_diagram_kinds():
    for word in ("流程图", "架构图", "时序图", "关系图"):
        assert word in DIAGRAM_SKILL_TRIGGER


def test_status_labels_hide_raw_tool_ids():
    assert "load_skill" not in tool_display_label("load_skill")
    assert "skill" not in tool_display_label("load_skill").lower()
    assert "技能" not in tool_display_label("load_skill")
    assert tool_display_label("load_skill") == "正在准备…"
    assert "mcp__" not in tool_display_label("mcp__foo_bar")
    assert "skill" not in tool_display_label("some_skill_loader").lower()


def test_diagram_skill_forbids_user_jargon():
    root = Path(__file__).resolve().parents[3] / "skills"
    reg = SkillRegistry(root=root)
    skill = reg.load("画图")
    assert skill is not None, f"missing 画图 under {root}; have={[m.name for m in reg.list_meta()]}"
    body = skill.body
    assert "对用户可见文案" in body or "禁止写" in body
    assert "mermaid" in body.lower()
    assert "===|" in body or "禁止" in body
    assert "成对" in body
