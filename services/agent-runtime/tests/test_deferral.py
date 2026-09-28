from app.deferral import turn_unfinished


def test_chat_without_a_file_is_finished():
    messages = [
        {"role": "user", "content": "你好"},
        {"role": "assistant", "content": "我来把这一版补上战斗动画，做好后给你。"},
    ]
    assert turn_unfinished([], messages) is False


def test_file_thread_without_a_tool_is_still_running():
    messages = [
        {"role": "assistant", "content": "打开 [文件](sandbox:/workspace/bots/x/web_rpg.html)"},
        {"role": "user", "content": "做完了么"},
        {
            "role": "assistant",
            "content": "还没完全做完，确认后我再把更新版发给你。",
        },
    ]
    assert turn_unfinished([], messages) is True


def test_delivery_tool_finishes_the_turn():
    messages = [
        {"role": "assistant", "content": "sandbox:/workspace/bots/x/web_rpg.html"},
    ]
    assert turn_unfinished(["sandbox_write"], messages) is False


def test_onboarding_without_a_file_is_finished():
    messages = [
        {"role": "user", "content": "先用你来写代码"},
        {"role": "assistant", "content": "好的，我会优先帮你写代码和排障。"},
    ]
    assert turn_unfinished([], messages) is False
