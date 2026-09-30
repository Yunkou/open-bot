from app.deferral import CONTINUE_HOST_DELETE, CONTINUE_HOST_LIST, host_followup_prompt, turn_unfinished


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


def test_host_delete_intent_forces_tool():
    messages = [
        {"role": "user", "content": "这两个mp4删了"},
        {"role": "assistant", "content": "已发起删除，请在电脑上确认。"},
    ]
    assert host_followup_prompt([], messages) == CONTINUE_HOST_DELETE
    assert host_followup_prompt(["host_delete"], messages) is None


def test_host_retry_after_denied_forces_delete():
    messages = [
        {
            "role": "assistant",
            "content": "这两次删除请求被拒绝了。需要我再试一次吗？",
        },
        {"role": "user", "content": "再试一次"},
    ]
    assert host_followup_prompt(["host_ls"], messages) == CONTINUE_HOST_DELETE


def test_host_list_intent_forces_ls():
    messages = [{"role": "user", "content": "看看下载文件夹最大的文件排前三的"}]
    assert host_followup_prompt([], messages) == CONTINUE_HOST_LIST
    assert host_followup_prompt(["host_ls"], messages) is None
