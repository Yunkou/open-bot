from app import openbot_api as obapi
from app.client_env import format_tools_routing_block, tool_display_label
from app.llm import TOOL_DEFS


def test_clone_tool_registered_and_labelled():
    names = {str((t.get("function") or {}).get("name") or "") for t in TOOL_DEFS}
    assert "clone_agent" in names
    assert "host_file_query" not in names
    assert tool_display_label("clone_agent") == "复制助手"


def test_routing_mentions_clone_only_when_available():
    with_clone = format_tools_routing_block(tools_enabled=True, available_tool_names=["clone_agent"])
    assert "clone_agent" in with_clone and "follow_up" in with_clone
    assert "专属记忆" in with_clone or "copy_memory" in with_clone
    without = format_tools_routing_block(tools_enabled=True, available_tool_names=["calculator"])
    assert "clone_agent" not in without


def test_clone_agent_args_normalizes():
    out = obapi.clone_agent_args(
        {
            "name": "  周报助手 ",
            "description": "",
            "system_prompt_append": "专写周报",
            "computer_mode": "PRIVATE",
            "copy_memory": 1,
            "enable_skills": "daily-brief， summarize-text",
            "disable_skills": ["", "tdd"],
            "follow_up": "",
            "bogus": "x",
        }
    )
    assert out == {
        "name": "周报助手",
        "system_prompt_append": "专写周报",
        "computer_mode": "private",
        "copy_memory": True,
        "enable_skills": ["daily-brief", "summarize-text"],
        "disable_skills": ["tdd"],
    }


def test_clone_agent_args_defaults_copy_memory_true():
    out = obapi.clone_agent_args({"name": "副本"})
    assert out["copy_memory"] is True
    assert "copy_routines" not in out


def test_clone_agent_args_follow_up_forces_copy_memory():
    out = obapi.clone_agent_args({"copy_memory": False, "follow_up": "接着写上周报告"})
    assert out["follow_up"] == "接着写上周报告"
    assert out["copy_memory"] is True
    # handoff_task alias
    out2 = obapi.clone_agent_args({"copy_memory": False, "handoff_task": "继续旧任务"})
    assert out2["follow_up"] == "继续旧任务"
    assert out2["copy_memory"] is True
    # explicit false without follow-up stays false
    out3 = obapi.clone_agent_args({"copy_memory": False})
    assert out3["copy_memory"] is False


def test_clone_agent_posts_internal_endpoint(monkeypatch):
    seen = {}

    def fake_post(path, payload, timeout=60.0):
        seen["path"] = path
        seen["payload"] = payload
        return {"ok": True, "agent": {"id": "new", "name": "A 副本"}}

    monkeypatch.setattr(obapi, "_post", fake_post)
    # Mirror main.py: normalize args first (default/force copy_memory).
    obapi.clone_agent("u1", "a1", **obapi.clone_agent_args({"name": "A 副本", "follow_up": "写日报"}))
    assert seen["path"] == "/internal/agents/clone"
    assert seen["payload"] == {
        "user_id": "u1",
        "source_agent_id": "a1",
        "name": "A 副本",
        "follow_up": "写日报",
        "copy_memory": True,
    }


def test_summarize_clone_result():
    s = obapi.summarize_clone_result(
        {
            "ok": True,
            "agent": {"id": "n1", "name": "写手 副本"},
            "conversation_id": "c1",
            "skills_copied": 3,
            "memories_copied": 0,
            "routine_names": ["日报"],
            "follow_up_status": "queued",
        }
    )
    assert s["new_agent_id"] == "n1" and s["new_agent_name"] == "写手 副本"
    assert s["copied"]["routines_paused"] == ["日报"]
    assert s["follow_up_status"] == "queued"
    assert "skill_errors" not in s
