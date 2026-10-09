from app.openbot_api import ROUTINE_TOOL_DEFS, ROUTINE_TOOL_NAMES


def test_routine_tool_defs_cover_crud_and_defer():
    names = ROUTINE_TOOL_NAMES
    for required in (
        "defer_work",
        "list_routines",
        "create_routine",
        "update_routine",
        "pause_routine",
        "resume_routine",
        "delete_routine",
    ):
        assert required in names
    assert len(ROUTINE_TOOL_DEFS) >= 7
