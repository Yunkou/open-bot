package httpserver

import (
	"context"
	"fmt"
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

func TestAutoPickHeuristicWhoWritesCodePicksCoder(t *testing.T) {
	members := []string{"research", "coder"}
	agents := []*db.Agent{
		{ID: "research", Name: "22", Description: "负责查资料与总结：搜索、阅读、整理要点。"},
		{ID: "coder", Name: "33", Description: "负责写代码与排障：改项目、查 bug、联调。"},
	}
	got := autoPickHeuristic("谁会写代码", members, agents, "coder", 1, "")
	if len(got.AgentIDs) != 1 || got.AgentIDs[0] != "coder" {
		t.Fatalf("want coder(33) only, got %#v conf=%.2f fallback=%s", got.AgentIDs, got.Confidence, got.Fallback)
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
		t.Fatal("subset agent_ids should count as explicit")
	}
	// Legacy client: no @ but agent_ids = all members → not explicit (AutoPick must run).
	if groupHasExplicitAddressing("谁会写代码", members, members, agents) {
		t.Fatal("full-member agent_ids without @ must not skip AutoPick")
	}
	if groupHasExplicitAddressing("谁会写代码", []string{"lead", "coder", "writer", "lead"}, members, agents) {
		t.Fatal("deduped full-member dump must not skip AutoPick")
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

func TestOrgAutoPickModeDefaultHeuristic(t *testing.T) {
	if orgAutoPickMode("") != db.AutoPickHeuristic || orgAutoPickMode("{}") != db.AutoPickHeuristic {
		t.Fatal("default heuristic")
	}
	if orgAutoPickMode(`{"auto_pick":false}`) != db.AutoPickOff {
		t.Fatal("false → off")
	}
	if orgAutoPickMode(`{"auto_pick":true}`) != db.AutoPickHeuristic {
		t.Fatal("true → heuristic")
	}
	if orgAutoPickMode(`{"auto_pick":"off"}`) != db.AutoPickOff {
		t.Fatal("string off")
	}
	if orgAutoPickMode(`{"auto_pick":"heuristic"}`) != db.AutoPickHeuristic {
		t.Fatal("string heuristic")
	}
	if orgAutoPickMode(`{"auto_pick":"decision"}`) != db.AutoPickDecision {
		t.Fatal("string decision")
	}
	if !orgAutoPickEnabled("") || orgAutoPickEnabled(`{"auto_pick":"off"}`) {
		t.Fatal("orgAutoPickEnabled wrapper")
	}
}

func TestRunAutoPickByModeDecisionNilDepsFailOpenHeuristic(t *testing.T) {
	members, agents := testGroupAgents()
	got := runAutoPickByMode(db.AutoPickDecision, "帮我写个函数", members, agents, "lead", 1, "", nil)
	if len(got.AgentIDs) != 1 || got.AgentIDs[0] != "coder" {
		t.Fatalf("decision should fail-open to heuristic pick, got %#v", got.AgentIDs)
	}
	if got.Method != "decision->heuristic" {
		t.Fatalf("method=%s want decision->heuristic", got.Method)
	}
	// Must NOT collapse to lead-only when heuristic has a match.
	if got.Fallback == "lead" && got.AgentIDs[0] == "lead" {
		t.Fatal("decision unimplemented must not force lead-only when heuristic matches")
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

func TestChannelSettingsEffectiveAutoPickMode(t *testing.T) {
	s := db.ChannelSettings{}
	if s.EffectiveAutoPickMode(db.AutoPickHeuristic) != db.AutoPickHeuristic {
		t.Fatal("org heuristic")
	}
	if s.EffectiveAutoPickMode(db.AutoPickOff) != db.AutoPickOff {
		t.Fatal("org off")
	}
	if s.EffectiveAutoPickMode(db.AutoPickDecision) != db.AutoPickDecision {
		t.Fatal("org decision")
	}
	if s.EffectiveAutoPickMode("") != db.AutoPickHeuristic {
		t.Fatal("empty org → heuristic default")
	}
	// Channel auto_pick is ignored; org feature_flags only.
	s.AutoPick = db.AutoPickOff
	if s.EffectiveAutoPickMode(db.AutoPickHeuristic) != db.AutoPickHeuristic {
		t.Fatal("channel off must not override org heuristic")
	}
	s.AutoPick = db.AutoPickDecision
	if s.EffectiveAutoPickMode(db.AutoPickOff) != db.AutoPickOff {
		t.Fatal("channel decision must not override org off")
	}
	s.AutoPick = db.AutoPickHeuristic
	if s.EffectiveAutoPickMode(db.AutoPickDecision) != db.AutoPickDecision {
		t.Fatal("org decision wins")
	}
	// Legacy EffectiveAutoPick bool helper (org only)
	s.AutoPick = db.AutoPickOff
	if !s.EffectiveAutoPick(true) || s.EffectiveAutoPick(false) {
		t.Fatal("EffectiveAutoPick bool wrapper ignores channel")
	}
}

func TestParseAutoPickModeMigration(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{true, db.AutoPickHeuristic},
		{false, db.AutoPickOff},
		{"heuristic", db.AutoPickHeuristic},
		{"decision", db.AutoPickDecision},
		{"off", db.AutoPickOff},
		{"", ""},
		{nil, ""},
	}
	for _, c := range cases {
		got, err := db.ParseAutoPickMode(c.in)
		if err != nil || got != c.want {
			t.Fatalf("ParseAutoPickMode(%v)=%q err=%v want %q", c.in, got, err, c.want)
		}
	}
	if _, err := db.ParseAutoPickMode("nope"); err == nil {
		t.Fatal("unknown should error")
	}
}

func TestAutoPickDecisionChoiceMapsToCoder(t *testing.T) {
	members, agents := testGroupAgents()
	deps := &autoPickDecisionDeps{
		Provider: "unsloth",
		Decide: func(ctx context.Context, state any, questions map[string]any) (map[string]any, error) {
			pick, _ := questions["pick"].(map[string]any)
			crit, _ := pick["criteria"].(map[string]string)
			if crit == nil {
				// JSON round-trip may use map[string]any
				if raw, ok := pick["criteria"].(map[string]any); ok {
					crit = map[string]string{}
					for k, v := range raw {
						crit[k] = fmt.Sprint(v)
					}
				}
			}
			if len(crit) != 4 { // all + m0..m2
				t.Fatalf("want 4 criteria (all+members), got %#v", pick["criteria"])
			}
			if _, ok := crit[autoPickDecisionAllKey]; !ok {
				t.Fatalf("missing all criterion: %#v", crit)
			}
			// coder is members[1] → m1
			return map[string]any{
				"pick": map[string]any{
					"type":       "choice",
					"choice":     "m1",
					"confidence": 0.82,
					"probabilities": map[string]any{
						"m0": 0.1, "m1": 0.82, "m2": 0.08,
					},
				},
			}, nil
		},
	}
	got := runAutoPickByMode(db.AutoPickDecision, "帮我写个函数", members, agents, "lead", 1, "", deps)
	if got.Method != "decision" {
		t.Fatalf("method=%s want decision", got.Method)
	}
	if len(got.AgentIDs) != 1 || got.AgentIDs[0] != "coder" {
		t.Fatalf("want coder, got %#v", got.AgentIDs)
	}
	if got.Confidence < autoPickConfidenceFloor {
		t.Fatalf("conf=%.2f", got.Confidence)
	}
	if got.Fallback != "" {
		t.Fatalf("fallback=%s", got.Fallback)
	}
}

func TestAutoPickDecisionKFromProbabilities(t *testing.T) {
	members, agents := testGroupAgents()
	deps := &autoPickDecisionDeps{
		Provider: "jev",
		Decide: func(ctx context.Context, state any, questions map[string]any) (map[string]any, error) {
			return map[string]any{
				"pick": map[string]any{
					"type":   "choice",
					"choice": "m1",
					"probabilities": map[string]any{
						"m0": 0.05, "m1": 0.7, "m2": 0.55,
					},
					"confidence": 0.7,
				},
			}, nil
		},
	}
	got := runAutoPickByMode(db.AutoPickDecision, "写代码顺便润色文案", members, agents, "lead", 2, "", deps)
	if got.Method != "decision" || len(got.AgentIDs) != 2 {
		t.Fatalf("got %#v", got)
	}
	if got.AgentIDs[0] != "coder" || got.AgentIDs[1] != "writer" {
		t.Fatalf("order want coder,writer got %#v", got.AgentIDs)
	}
}

func TestAutoPickDecisionErrorsFailOpenHeuristic(t *testing.T) {
	members, agents := testGroupAgents()
	cases := []struct {
		name string
		deps *autoPickDecisionDeps
	}{
		{"provider_off", &autoPickDecisionDeps{Provider: "off", Decide: func(ctx context.Context, state any, questions map[string]any) (map[string]any, error) {
			t.Fatal("should not call")
			return nil, nil
		}}},
		{"timeout_err", &autoPickDecisionDeps{Provider: "unsloth", Decide: func(ctx context.Context, state any, questions map[string]any) (map[string]any, error) {
			return nil, context.DeadlineExceeded
		}}},
		{"empty_answers", &autoPickDecisionDeps{Provider: "unsloth", Decide: func(ctx context.Context, state any, questions map[string]any) (map[string]any, error) {
			return map[string]any{}, nil
		}}},
		{"bad_choice_key", &autoPickDecisionDeps{Provider: "unsloth", Decide: func(ctx context.Context, state any, questions map[string]any) (map[string]any, error) {
			return map[string]any{"pick": map[string]any{"choice": "coder", "confidence": 0.9}}, nil
		}}},
		{"low_conf", &autoPickDecisionDeps{Provider: "unsloth", Decide: func(ctx context.Context, state any, questions map[string]any) (map[string]any, error) {
			return map[string]any{"pick": map[string]any{"choice": "m1", "confidence": 0.2}}, nil
		}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := runAutoPickByMode(db.AutoPickDecision, "帮我写个函数", members, agents, "lead", 1, "", c.deps)
			if got.Method != "decision->heuristic" {
				t.Fatalf("method=%s", got.Method)
			}
			if len(got.AgentIDs) != 1 || got.AgentIDs[0] != "coder" {
				t.Fatalf("should still heuristic-pick coder, got %#v", got.AgentIDs)
			}
		})
	}
}

func TestBuildAutoPickDecisionPayloadOpaqueKeys(t *testing.T) {
	members, agents := testGroupAgents()
	keyToID, questions, state := buildAutoPickDecisionPayload("修 bug", members, agents, "上次提到 API", false)
	if keyToID["m0"] != "lead" || keyToID["m1"] != "coder" || keyToID["m2"] != "writer" {
		t.Fatalf("keyToID=%#v", keyToID)
	}
	pick := questions["pick"].(map[string]any)
	if pick["type"] != "choice" {
		t.Fatal("want choice")
	}
	crit := pick["criteria"].(map[string]string)
	if crit[autoPickDecisionAllKey] == "" {
		t.Fatalf("want opaque %q in criteria, got %#v", autoPickDecisionAllKey, crit)
	}
	if !strings.Contains(crit["m1"], "码农") {
		t.Fatalf("criteria=%#v", crit)
	}
	instr := fmt.Sprint(pick["instructions"])
	if !strings.Contains(instr, `"all"`) && !strings.Contains(instr, "all") {
		t.Fatalf("instructions should mention all, got %q", instr)
	}
	if !strings.Contains(state, "修 bug") || !strings.Contains(state, "上次提到 API") {
		t.Fatalf("state=%q", state)
	}
	// Must not put raw agent ids as criteria keys
	for k := range crit {
		if k == "coder" || k == "lead" || k == "writer" {
			t.Fatalf("leaked agent id as key: %s", k)
		}
	}
	// allHands strengthens instructions
	_, q2, _ := buildAutoPickDecisionPayload("都介绍下自己", members, agents, "", true)
	pick2 := q2["pick"].(map[string]any)
	if !strings.Contains(fmt.Sprint(pick2["instructions"]), "all-hands") {
		t.Fatalf("allHands instructions missing hint: %v", pick2["instructions"])
	}
}

func TestMapDecisionChoiceRejectsAgentIDEcho(t *testing.T) {
	keyToID := map[string]string{"m0": "lead", "m1": "coder"}
	_, _, _, ok := mapDecisionChoiceToAgentIDs(map[string]any{
		"pick": map[string]any{"choice": "coder", "confidence": 0.9},
	}, keyToID, []string{"lead", "coder"}, 1)
	if ok {
		t.Fatal("must reject raw agent id echo")
	}
}

func TestMapDecisionChoiceAllExpandsMembers(t *testing.T) {
	keyToID := map[string]string{"m0": "lead", "m1": "coder", "m2": "writer"}
	ids, conf, allSel, ok := mapDecisionChoiceToAgentIDs(map[string]any{
		"pick": map[string]any{"choice": "all", "confidence": 0.91},
	}, keyToID, []string{"lead", "coder", "writer"}, 1)
	if !ok || !allSel {
		t.Fatalf("ok=%v allSel=%v", ok, allSel)
	}
	if len(ids) != 3 || ids[0] != "lead" || ids[1] != "coder" || ids[2] != "writer" {
		t.Fatalf("ids=%#v", ids)
	}
	if conf < 0.9 {
		t.Fatalf("conf=%.2f", conf)
	}
}

func TestDecisionEffectiveKAllHandsRaises(t *testing.T) {
	if got := decisionEffectiveK(1, 5, false); got != 1 {
		t.Fatalf("specialty k=1 → %d", got)
	}
	if got := decisionEffectiveK(2, 5, false); got != 2 {
		t.Fatalf("specialty k=2 → %d", got)
	}
	if got := decisionEffectiveK(1, 5, true); got != 5 {
		t.Fatalf("all-hands 5 members → %d", got)
	}
	if got := decisionEffectiveK(1, 12, true); got != autoPickAllHandsMaxK {
		t.Fatalf("all-hands 12 members capped → %d", got)
	}
}

func TestIsAllHandsIntent(t *testing.T) {
	positives := []string{
		"都介绍下自己",
		"都介绍一下自己",
		"大家介绍一下",
		"大家介绍自己",
		"各位介绍一下",
		"各自介绍一下",
		"都说说",
		"大家都说说",
		"每人介绍一下",
		"轮流介绍",
		"everyone introduce yourselves",
		"Please all introduce yourselves",
		"everybody introduce",
	}
	for _, p := range positives {
		if !isAllHandsIntent(p) {
			t.Fatalf("want all-hands for %q", p)
		}
	}
	negatives := []string{
		"",
		"帮我写个函数",
		"谁会写代码",
		"介绍一下码农",
		"介绍一下",
		"都很好",
		"大家好",
		"随便聊聊天气",
		"@码农 介绍一下",
	}
	for _, n := range negatives {
		if isAllHandsIntent(n) {
			t.Fatalf("want NOT all-hands for %q", n)
		}
	}
}

func TestRunAutoPickAllHandsDecisionCallsDecide(t *testing.T) {
	members, agents := testGroupAgents()
	decideCalled := false
	var sawAllCriterion bool
	var sawAllHandsHint bool
	deps := &autoPickDecisionDeps{
		Provider: "unsloth",
		Decide: func(ctx context.Context, state any, questions map[string]any) (map[string]any, error) {
			decideCalled = true
			pick, _ := questions["pick"].(map[string]any)
			crit, _ := pick["criteria"].(map[string]string)
			if crit == nil {
				if raw, ok := pick["criteria"].(map[string]any); ok {
					crit = map[string]string{}
					for k, v := range raw {
						crit[k] = fmt.Sprint(v)
					}
				}
			}
			if _, ok := crit[autoPickDecisionAllKey]; ok {
				sawAllCriterion = true
			}
			if strings.Contains(fmt.Sprint(pick["instructions"]), "all-hands") {
				sawAllHandsHint = true
			}
			return map[string]any{
				"pick": map[string]any{
					"type":       "choice",
					"choice":     "all",
					"confidence": 0.93,
				},
			}, nil
		},
	}
	got := runAutoPickByMode(db.AutoPickDecision, "都介绍下自己", members, agents, "lead", 1, "", deps)
	if !decideCalled {
		t.Fatal("decision mode must call decide for all-hands (no phrase short-circuit)")
	}
	if !sawAllCriterion || !sawAllHandsHint {
		t.Fatalf("payload: allCriterion=%v allHandsHint=%v", sawAllCriterion, sawAllHandsHint)
	}
	if got.Method != "decision" {
		t.Fatalf("method=%s want decision", got.Method)
	}
	if got.Fallback != "all" || len(got.AgentIDs) != 3 {
		t.Fatalf("want all 3 candidates, got %#v fallback=%s", got.AgentIDs, got.Fallback)
	}
	if got.AgentIDs[0] != "lead" || got.AgentIDs[1] != "coder" || got.AgentIDs[2] != "writer" {
		t.Fatalf("order=%#v", got.AgentIDs)
	}
	if got.AllowSecondFailOpen {
		t.Fatal("all candidates already; second fail-open should be off")
	}
	if len(got.LeadFallback) != 0 {
		t.Fatalf("LeadFallback=%#v", got.LeadFallback)
	}
}

func TestRunAutoPickAllHandsDecisionFailOpenAll(t *testing.T) {
	members, agents := testGroupAgents()
	deps := &autoPickDecisionDeps{
		Provider: "unsloth",
		Decide: func(ctx context.Context, state any, questions map[string]any) (map[string]any, error) {
			return nil, context.DeadlineExceeded
		},
	}
	got := runAutoPickByMode(db.AutoPickDecision, "大家介绍一下", members, agents, "lead", 1, "", deps)
	if got.Method != "decision->heuristic->all_hands" {
		t.Fatalf("method=%s", got.Method)
	}
	if len(got.AgentIDs) != 3 || got.Fallback != "all" {
		t.Fatalf("want all members on fail-open, got %#v fallback=%s", got.AgentIDs, got.Fallback)
	}
}

func TestRunAutoPickAllHandsDecisionMultiViaRaisedK(t *testing.T) {
	members, agents := testGroupAgents()
	deps := &autoPickDecisionDeps{
		Provider: "jev",
		Decide: func(ctx context.Context, state any, questions map[string]any) (map[string]any, error) {
			// Model ranks individuals instead of "all"; raised K should still return everyone.
			return map[string]any{
				"pick": map[string]any{
					"type":   "choice",
					"choice": "m0",
					"probabilities": map[string]any{
						"m0": 0.5, "m1": 0.45, "m2": 0.42,
					},
					"confidence": 0.5,
				},
			}, nil
		},
	}
	got := runAutoPickByMode(db.AutoPickDecision, "everyone introduce yourselves", members, agents, "lead", 1, "", deps)
	if got.Method != "decision" {
		t.Fatalf("method=%s", got.Method)
	}
	if len(got.AgentIDs) != 3 {
		t.Fatalf("raised K should return 3, got %#v", got.AgentIDs)
	}
}

func TestRunAutoPickAllHandsHeuristic(t *testing.T) {
	members, agents := testGroupAgents()
	got := runAutoPickByMode(db.AutoPickHeuristic, "大家介绍一下", members, agents, "lead", 1, "", nil)
	if got.Method != "heuristic->all_hands" {
		t.Fatalf("method=%s", got.Method)
	}
	if len(got.AgentIDs) != 3 || got.Fallback != "all" {
		t.Fatalf("want all members, got %#v fallback=%s", got.AgentIDs, got.Fallback)
	}
	// Contrast: specialty question still picks coder, not all.
	got2 := runAutoPickByMode(db.AutoPickHeuristic, "帮我写个函数", members, agents, "lead", 1, "", nil)
	if len(got2.AgentIDs) != 1 || got2.AgentIDs[0] != "coder" {
		t.Fatalf("specialty should still pick coder, got %#v", got2.AgentIDs)
	}
}
