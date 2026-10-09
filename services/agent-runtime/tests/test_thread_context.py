from app.thread_context import augment_mem0_turn, build_recall_query, format_reply_prefix


def test_format_reply_prefix():
    p = format_reply_prefix("你好世界", who="码农助手")
    assert p.startswith("【回复 码农助手：「你好世界」】")


def test_build_recall_query_includes_snippet_and_user():
    q = build_recall_query("再确认端口", reply_snippet="用 make dev", thread_tail=["端口呢？", "8080"])
    assert "回复：用 make dev" in q
    assert "再确认端口" in q
    assert "8080" in q


def test_augment_mem0_turn():
    turn = [{"role": "user", "content": "继续"}, {"role": "assistant", "content": "好"}]
    out = augment_mem0_turn(turn, "回复：旧话\n继续")
    assert out[0]["content"].startswith("[线程上下文]")
    assert out[-1]["content"] == "好"
    assert augment_mem0_turn(turn, "") == turn
