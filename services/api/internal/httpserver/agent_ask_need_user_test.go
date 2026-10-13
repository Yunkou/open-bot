package httpserver

import "testing"

func TestSplitAskNeedUser(t *testing.T) {
	visible, needs := splitAskNeedUser("NEED_USER\n\n要改哪几个文件？")
	if !needs || visible != "要改哪几个文件？" {
		t.Fatalf("got needs=%v visible=%q", needs, visible)
	}
	visible, needs = splitAskNeedUser("直接回答就好。")
	if needs || visible != "直接回答就好。" {
		t.Fatalf("plain reply changed: needs=%v visible=%q", needs, visible)
	}
	visible, needs = splitAskNeedUser("need_user:")
	if !needs || visible == "" {
		t.Fatalf("empty questions should still ask the user: needs=%v visible=%q", needs, visible)
	}
}

func TestReplyEchoesColleague(t *testing.T) {
	colleague := "要改哪几个文件？预算是多少？"
	if !replyEchoesColleague(colleague, colleague) {
		t.Fatal("exact copy should echo")
	}
	if !replyEchoesColleague("同事需要你确认：\n"+colleague, colleague) {
		t.Fatal("short lead-in plus the same questions should echo")
	}
	long := colleague + "另外我已经按现有仓库结构列出了可能涉及的目录，并建议先从 API 层改起，再补一组回归。这段是调用方自己的新内容，不应被当成重复。"
	if replyEchoesColleague(long, colleague) {
		t.Fatal("a reply that adds its own content should stay")
	}
	if replyEchoesColleague("好的，我来处理。", colleague) {
		t.Fatal("unrelated short reply should stay")
	}
	colleagueLink := "已做好简易计算器页面，支持加减乘除、百分比和键盘操作：[打开计算器](sandbox:/workspace/shared/simple-calculator.html)"
	if !replyEchoesColleague("[打开简易计算器](sandbox:/workspace/shared/simple-calculator.html)", colleagueLink) {
		t.Fatal("a shorter link to the same file should echo")
	}
	if replyEchoesColleague("配色改成深色，按钮加大，并另存一版：[打开草稿](sandbox:/workspace/shared/other.html)", colleagueLink) {
		t.Fatal("a different file plus new direction should stay")
	}
}
