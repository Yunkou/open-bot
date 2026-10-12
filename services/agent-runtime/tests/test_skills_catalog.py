"""Skill catalog prompt shaping: desc cap + find-skills discovery entry."""

from __future__ import annotations

import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))

from app.llm import build_system_prompt  # noqa: E402
from app.skills import CATALOG_DESC_MAX, SkillMeta, SkillRegistry  # noqa: E402


def test_catalog_desc_max_constant() -> None:
    assert CATALOG_DESC_MAX == 120


def test_catalog_truncates_long_descriptions() -> None:
    reg = SkillRegistry.__new__(SkillRegistry)
    reg.root = Path("/tmp")
    reg.user_id = None
    long_desc = "A" * 300
    reg._by_name = {
        "short": SkillMeta(name="short", description="ok", path=None),
        "long": SkillMeta(name="long", description=long_desc, path=None),
        "find-skills": SkillMeta(
            name="find-skills",
            description="Discover and install skills when the user asks how do I do X.",
            path=None,
        ),
    }
    catalog = reg.catalog_for_prompt()
    lines = {line.split(":", 1)[0].removeprefix("- ").strip(): line for line in catalog.splitlines()}
    assert lines["short"].endswith("ok")
    long_line = lines["long"]
    desc = long_line.split(": ", 1)[1]
    assert len(desc) <= CATALOG_DESC_MAX
    assert desc.endswith("…")
    assert len(desc) >= CATALOG_DESC_MAX - 3
    assert "find-skills" in lines


def test_skills_header_points_find_skills_for_deep_search() -> None:
    prompt = build_system_prompt(
        agent_id="bot",
        skills_catalog="- find-skills: discover\n- other: x",
        memory_snippets=[],
        tools_enabled=True,
        available_tool_names=["load_skill"],
    )
    assert "find-skills" in prompt
    assert "深搜" in prompt or "生态" in prompt
