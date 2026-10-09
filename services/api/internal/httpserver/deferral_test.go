package httpserver

import "testing"

func TestClassifySendIgnoresWording(t *testing.T) {
	if got := classifySend("做完了么", false); got != gateNone {
		t.Fatalf("no running agent, start a turn: %v", got)
	}
	if got := classifySend("我来把这一版补上战斗动画，做好后给你", false); got != gateNone {
		t.Fatalf("wording must not open a gate: %v", got)
	}
	if got := classifySend("做完了么", true); got != gateFollowUp {
		t.Fatalf("running agent stays running: %v", got)
	}
	if got := classifySend("再加一个背包", true); got != gateFollowUp {
		t.Fatalf("follow-up while running: %v", got)
	}
	if got := classifySend("别做了", true); got != gateCancel {
		t.Fatalf("cancel: %v", got)
	}
}
