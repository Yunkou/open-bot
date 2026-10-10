package httpserver

import (
	"strings"
	"testing"

	"github.com/tangxin/open-bot/services/api/internal/db"
)

func testGroupAgents() (members []string, agents []*db.Agent) {
	members = []string{"lead", "coder", "writer"}
	agents = []*db.Agent{
		{ID: "lead", Name: "通用助手", Description: "综合问答与兜底"},
		{ID: "coder", Name: "码农", Description: "负责写代码与排障"},
		{ID: "writer", Name: "写手", Description: "负责写诗与文案"},
	}
	return members, agents
}

func TestAutoPickHeuristicPrefersCoderForFunction(t *testing.T) {
	members, agents := testGroupAgents()
	got := autoPickHeuristic("帮我写个函数", members, agents, "lead", 1, "")
	if len(got.AgentIDs) != 1 || got.AgentIDs[0] != "coder" {
		t.Fatalf("want coder, got %#v conf=%.2f fallback=%s", got.AgentIDs, got.Confidence, got.Fallback)
	}
	if got.Fallback != "" || got.Method != "heuristic" {
		t.Fatalf("unexpected meta %#v", got)
	}
	if !got.AllowSecondFailOpen {
		t.Fatal("pick should allow second fail-open")
	}
	if got.Confidence < autoPickConfidenceFloor {
		t.Fatalf("confidence too low: %.2f", got.Confidence)
	}
}

func TestAutoPickHeuristicFailOpenLead(t *testing.T) {
	members, agents := testGroupAgents()
	got := autoPickHeuristic("随便聊聊天气", members, agents, "lead", 1, "")
	if len(got.AgentIDs) != 1 || got.AgentIDs[0] != "lead" {
		t.Fatalf("want lead fail-open, got %#v", got.AgentIDs)
	}
	if got.Fallback != "lead" || !got.LeadFallback["lead"] {
		t.Fatalf("want lead fallback meta, got %#v", got)
	}
}

func TestAutoPickHeuristicLeadMissingFallsToAll(t *testing.T) {
	members, agents := testGroupAgents()
	got := autoPickHeuristic("随便聊聊天气", members, agents, "missing-lead", 1, "")
	if len(got.AgentIDs) != 3 {
		t.Fatalf("want all members, got %#v", got.AgentIDs)
	}
	if got.Fallback != "all" {
		t.Fatalf("fallback=%s", got.Fallback)
	}
}

func TestGroupHasExplicitAddressing(t *testing.T) {
	members, agents := testGroupAgents()
	if groupHasExplicitAddressing("帮我写个函数", nil, members, agents) {
		t.Fatal("plain text should not be explicit")
	}
	if !groupHasExplicitAddressing("@码农 修 bug", nil, members, agents) {
		t.Fatal("@ should be explicit")
	}
	if !groupHasExplicitAddressing("大家介绍自己 @everyone", nil, members, agents) {
		t.Fatal("@everyone should be explicit")
	}
	if !groupHasExplicitAddressing("hi", []string{"coder"}, members, agents) {
		t.Fatal("explicit agent_ids should count")
	}
}

func TestResolveGroupSendTargetsAtUnchanged(t *testing.T) {
	members, agents := testGroupAgents()
	got := resolveGroupSendTargets(members, agents, nil, "@码农 修 bug", nil, "lead")
	if len(got) != 1 || got[0] != "coder" {
		t.Fatalf("@码农 → coder only, got %#v", got)
	}
	got2 := resolveGroupSendTargets(members, agents, nil, "大家介绍自己 @everyone", nil, "lead")
	if len(got2) != 3 {
		t.Fatalf("@everyone → all, got %#v", got2)
	}
}

func TestSecondFailOpenTargets(t *testing.T) {
	members := []string{"lead", "coder", "writer"}
	targets, fb, kind := secondFailOpenTargets(members, []string{"coder"}, "lead")
	if kind != "lead" || len(targets) != 1 || targets[0] != "lead" || !fb["lead"] {
		t.Fatalf("want lead, got targets=%#v fb=%#v kind=%s", targets, fb, kind)
	}
	targets2, fb2, kind2 := secondFailOpenTargets(members, []string{"coder", "lead"}, "lead")
	if kind2 != "all" || len(targets2) != 1 || targets2[0] != "writer" || !fb2["writer"] {
		t.Fatalf("want remaining writer, got targets=%#v fb=%#v kind=%s", targets2, fb2, kind2)
	}
}

func TestOrgAutoPickEnabledDefaultOn(t *testing.T) {
	if !orgAutoPickEnabled("") || !orgAutoPickEnabled("{}") {
		t.Fatal("default on")
	}
	if orgAutoPickEnabled(`{"auto_pick":false}`) {
		t.Fatal("false should disable")
	}
	if !orgAutoPickEnabled(`{"auto_pick":true}`) {
		t.Fatal("true should enable")
	}
}

func TestGroupParticipationLeadFallbackCopy(t *testing.T) {
	members, agents := testGroupAgents()
	prompt := groupParticipationExtraSystemMode(false, true, agents[0], members, agents)
	if strings.Contains(prompt, "Auto") || strings.Contains(prompt, "选人") || strings.Contains(prompt, "PASS") {
		t.Fatalf("lead-fallback prompt must not leak internal words: %q", prompt)
	}
	if !strings.Contains(prompt, "回答") {
		t.Fatalf("should ask to answer: %q", prompt)
	}
}

func TestDisplayNamesForAgentsNoIDs(t *testing.T) {
	_, agents := testGroupAgents()
	names := displayNamesForAgents([]string{"coder", "lead"}, agents)
	if len(names) != 2 || names[0] != "码农" || names[1] != "通用助手" {
		t.Fatalf("names=%#v", names)
	}
}

func TestChannelSettingsEffectiveAutoPick(t *testing.T) {
	off := false
	on := true
	s := db.ChannelSettings{}
	if !s.EffectiveAutoPick(true) {
		t.Fatal("inherit org on")
	}
	if s.EffectiveAutoPick(false) {
		t.Fatal("inherit org off")
	}
	s.AutoPick = &off
	if s.EffectiveAutoPick(true) {
		t.Fatal("channel off wins")
	}
	s.AutoPick = &on
	if !s.EffectiveAutoPick(false) {
		t.Fatal("channel on wins")
	}
}
