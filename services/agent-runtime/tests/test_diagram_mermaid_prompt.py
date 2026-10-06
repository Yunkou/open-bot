from pathlib import Path

from app.llm import DIAGRAM_SKILL_TRIGGER, build_system_prompt
from app.skills import SkillRegistry


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
    assert "load_skill" in DIAGRAM_SKILL_TRIGGER
    assert "技能不可用" in DIAGRAM_SKILL_TRIGGER
    assert "===|" in DIAGRAM_SKILL_TRIGGER
    assert "英文" in DIAGRAM_SKILL_TRIGGER or "节点" in DIAGRAM_SKILL_TRIGGER


def test_trigger_mentions_diagram_kinds():
    for word in ("流程图", "架构图", "时序图", "关系图"):
        assert word in DIAGRAM_SKILL_TRIGGER


def test_diagram_skill_on_disk_has_hard_rules():
    root = Path(__file__).resolve().parents[3] / "skills"
    reg = SkillRegistry(root=root)
    skill = reg.load("画图")
    assert skill is not None, f"missing 画图 under {root}; have={[m.name for m in reg.list_meta()]}"
    body = skill.body
    assert "===|" in body or "禁止" in body
    assert "成对" in body
    assert "人物" in body or "关系图" in body
    assert "技能不可用" in body
    assert "英文" in body or "节点 ID" in body
    assert "\\" in body or "转义" in body or "反斜杠" in body
