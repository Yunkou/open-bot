package db

import "testing"

func TestTriggerMatchesSlackMention(t *testing.T) {
	tr := RoutineTrigger{Source: "slack", Type: "app_mention"}
	ev := EventMatch{Source: "slack", Type: "app_mention", Text: "<@U1> hello"}
	if !TriggerMatches(tr, ev) {
		t.Fatal("expected mention match")
	}
}

func TestTriggerMatchesSlackKeyword(t *testing.T) {
	tr := RoutineTrigger{Source: "slack", Type: "keyword", Keywords: []string{"日报"}}
	ev := EventMatch{Source: "slack", Type: "message", Text: "请写今日日报"}
	if !TriggerMatches(tr, ev) {
		t.Fatal("expected keyword match")
	}
	ev2 := EventMatch{Source: "slack", Type: "message", Text: "无关消息"}
	if TriggerMatches(tr, ev2) {
		t.Fatal("should not match")
	}
}

func TestTriggerMatchesGitHubPR(t *testing.T) {
	tr := RoutineTrigger{
		Source:  "github",
		Type:    "pull_request",
		Actions: []string{"opened", "reopened"},
		Repo:    "acme/app",
	}
	ev := EventMatch{Source: "github", Type: "pull_request", Action: "opened", Repo: "acme/app"}
	if !TriggerMatches(tr, ev) {
		t.Fatal("expected pr match")
	}
	evBad := EventMatch{Source: "github", Type: "pull_request", Action: "closed", Repo: "acme/app"}
	if TriggerMatches(tr, evBad) {
		t.Fatal("closed should not match")
	}
	evRepo := EventMatch{Source: "github", Type: "pull_request", Action: "opened", Repo: "other/app"}
	if TriggerMatches(tr, evRepo) {
		t.Fatal("wrong repo")
	}
}

func TestRoutineMatchesEvent(t *testing.T) {
	r := &Routine{
		Enabled: true,
		Triggers: []RoutineTrigger{
			{Source: "github", Type: "issues", Actions: []string{"opened"}},
		},
	}
	if !RoutineMatchesEvent(r, EventMatch{Source: "github", Type: "issues", Action: "opened"}) {
		t.Fatal("expected match")
	}
	r.Enabled = false
	if RoutineMatchesEvent(r, EventMatch{Source: "github", Type: "issues", Action: "opened"}) {
		t.Fatal("disabled should not match")
	}
}

func TestNormalizeTimezone(t *testing.T) {
	if NormalizeTimezone("") != "Asia/Shanghai" {
		t.Fatal("default")
	}
	if NormalizeTimezone("Asia/Tokyo") != "Asia/Tokyo" {
		t.Fatal("tokyo")
	}
	if NormalizeTimezone("Not/AZone") != "" {
		t.Fatal("invalid should be empty")
	}
}

func TestParseTriggersJSON(t *testing.T) {
	tr, err := ParseTriggersJSON(`[{"source":"Slack","type":"App_Mention"}]`)
	if err != nil || len(tr) != 1 {
		t.Fatalf("parse: %v %#v", err, tr)
	}
	if tr[0].Source != "slack" || tr[0].Type != "app_mention" {
		t.Fatalf("normalized: %#v", tr[0])
	}
}

func TestHashResultTextStable(t *testing.T) {
	a := HashResultText(" hello ")
	b := HashResultText("hello")
	if a != b || a == "" {
		t.Fatalf("hash %s %s", a, b)
	}
}
