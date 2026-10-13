from app.llm import TOOL_DEFS, iter_tool_defs


def test_ask_agent_is_defined():
    names = {str((t.get("function") or {}).get("name") or "") for t in TOOL_DEFS}
    assert "ask_agent" in names
    ask = next(t for t in TOOL_DEFS if (t.get("function") or {}).get("name") == "ask_agent")
    required = set((ask.get("function") or {}).get("parameters", {}).get("required") or [])
    assert required == {"to_agent_id", "question"}


def test_ask_agent_hidden_unless_allowed():
    hidden = {str((t.get("function") or {}).get("name") or "") for t in iter_tool_defs()}
    shown = {str((t.get("function") or {}).get("name") or "") for t in iter_tool_defs(allow_ask_agent=True)}
    assert "ask_agent" not in hidden
    assert "send_to_agent" in hidden
    assert "ask_agent" in shown
