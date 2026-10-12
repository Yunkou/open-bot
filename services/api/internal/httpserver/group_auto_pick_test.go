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
	if got.AllowSecondFailOpen || !got.LeadFallback["coder"] {
		t.Fatalf("specialty coding: LeadFallback coder, no second-wave dump, got %#v", got)
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
	if strings.Contains(prompt, "Auto") || strings.Contains(prompt, "选人") {
		t.Fatalf("lead-fallback prompt must not leak Auto/选人: %q", prompt)
	}
	if !strings.Contains(prompt, "回答") {
		t.Fatalf("should ask to answer: %q", prompt)
	}
	if !strings.Contains(prompt, "不要输出 [PASS]") {
		t.Fatalf("lead-fallback must forbid [PASS] like forced: %q", prompt)
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
	keyToID, questions, state := buildAutoPickDecisionPayload("修 bug", members, agents, "上次提到 API", false, false)
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
	if !strings.Contains(instr, "several") {
		t.Fatalf("instructions should allow several specialists, got %q", instr)
	}
	countQ, okCount := questions[autoPickDecisionCountKey].(map[string]any)
	if !okCount || countQ["type"] != "noul" {
		t.Fatalf("want count noul question, got %#v", questions[autoPickDecisionCountKey])
	}
	countInstr0 := fmt.Sprint(countQ["instructions"])
	if !strings.Contains(countInstr0, "How many") && !strings.Contains(countInstr0, "SPECIALTY TASK count") {
		t.Fatalf("count instructions=%v", countQ["instructions"])
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
	// allHands strengthens instructions: must force choice=all / full roster / high mass on all
	_, q2, st2 := buildAutoPickDecisionPayload("都介绍下自己", members, agents, "", true, false)
	pick2 := q2["pick"].(map[string]any)
	instr2 := fmt.Sprint(pick2["instructions"])
	for _, needle := range []string{"ALL-HANDS", "都介绍下自己", "大家都介绍一下", `choice="all"`, "never a single mN", "HIGH probability"} {
		if !strings.Contains(instr2, needle) {
			t.Fatalf("all-hands instructions missing %q: %s", needle, instr2)
		}
	}
	count2 := q2[autoPickDecisionCountKey].(map[string]any)
	countInstr2 := fmt.Sprint(count2["instructions"])
	for _, needle := range []string{"ALL-HANDS count", "near 1", "都介绍下自己"} {
		if !strings.Contains(countInstr2, needle) {
			t.Fatalf("all-hands count instructions missing %q: %s", needle, countInstr2)
		}
	}
	if !strings.Contains(st2, `choice must be "all"`) || !strings.Contains(st2, "never a single mN") {
		t.Fatalf("all-hands state missing roster mandate: %q", st2)
	}
	// who-can: prompt must drive specific mN + capable count (no role-match remap later)
	_, q3, st3 := buildAutoPickDecisionPayload("谁会写代码", members, agents, "同伴自介查资料", false, true)
	pick3 := q3["pick"].(map[string]any)
	instr3 := fmt.Sprint(pick3["instructions"])
	for _, needle := range []string{"CAPABILITY CENSUS", "谁会写代码", "写代码", "查资料", "criteria", "never park on", "NEAR-ZERO"} {
		if !strings.Contains(instr3, needle) {
			t.Fatalf("who-can instructions missing %q: %s", needle, instr3)
		}
	}
	criteria3 := pick3["criteria"].(map[string]string)
	if !strings.Contains(criteria3[autoPickDecisionAllKey], "ONLY if EVERY") {
		t.Fatalf("who-can criteria[all] should discourage full roster, got %q", criteria3[autoPickDecisionAllKey])
	}
	count3 := q3[autoPickDecisionCountKey].(map[string]any)
	countInstr3 := fmt.Sprint(count3["instructions"])
	if !strings.Contains(countInstr3, "CAPABILITY CENSUS count") || !strings.Contains(countInstr3, "FORBIDDEN") {
		t.Fatalf("who-can count instructions=%v", count3["instructions"])
	}
	if !strings.Contains(st3, "Member roles are in the pick criteria") {
		t.Fatalf("state should point at criteria roles, got %q", st3)
	}
	if !strings.Contains(st3, "not an override of criteria roles") {
		t.Fatalf("state should warn Recent context is not role override, got %q", st3)
	}
	if !strings.Contains(st3, "谁会写代码") || !strings.Contains(st3, "never choice=all") {
		t.Fatalf("state should mandate coder-only for 谁会写代码, got %q", st3)
	}
}

func TestMapDecisionChoiceRejectsAgentIDEcho(t *testing.T) {
	keyToID := map[string]string{"m0": "lead", "m1": "coder"}
	_, _, _, ok := mapDecisionChoiceToAgentIDs(map[string]any{
		"pick": map[string]any{"choice": "coder", "confidence": 0.9},
	}, keyToID, []string{"lead", "coder"}, 1, true, false, 0)
	if ok {
		t.Fatal("must reject raw agent id echo")
	}
}

func TestMapDecisionChoiceAllExpandsMembers(t *testing.T) {
	keyToID := map[string]string{"m0": "lead", "m1": "coder", "m2": "writer"}
	ids, conf, allSel, ok := mapDecisionChoiceToAgentIDs(map[string]any{
		"pick": map[string]any{"choice": "all", "confidence": 0.91},
	}, keyToID, []string{"lead", "coder", "writer"}, 1, true, false, 0)
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
	if got := decisionEffectiveK(1, 5, false, 0); got != 1 {
		t.Fatalf("specialty k=1 → %d", got)
	}
	if got := decisionEffectiveK(2, 5, false, 0); got != 2 {
		t.Fatalf("specialty k=2 → %d", got)
	}
	if got := decisionEffectiveK(1, 5, true, 0); got != 5 {
		t.Fatalf("all-hands 5 members → %d", got)
	}
	if got := decisionEffectiveK(1, 12, true, 0); got != autoPickAllHandsMaxK {
		t.Fatalf("all-hands 12 members capped → %d", got)
	}
	if got := decisionEffectiveK(1, 3, false, 2); got != 2 {
		t.Fatalf("countHint=2 of 3 → %d", got)
	}
	if got := decisionEffectiveK(1, 3, false, 3); got != 3 {
		t.Fatalf("countHint=3 of 3 → %d", got)
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
			instr := fmt.Sprint(pick["instructions"])
			if strings.Contains(instr, "ALL-HANDS") || strings.Contains(strings.ToLower(instr), "all-hands") {
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
	for _, id := range got.AgentIDs {
		if !got.LeadFallback[id] {
			t.Fatalf("all-hands must LeadFallback every member (forbid PASS), missing %s got %#v", id, got.LeadFallback)
		}
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
	for _, id := range got.AgentIDs {
		if !got.LeadFallback[id] {
			t.Fatalf("fail-open all-hands must LeadFallback %s, got %#v", id, got.LeadFallback)
		}
	}
	if got.AllowSecondFailOpen {
		t.Fatal("all-hands must not AllowSecondFailOpen")
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
	for _, id := range got.AgentIDs {
		if !got.LeadFallback[id] {
			t.Fatalf("all-hands multi must LeadFallback %s, got %#v", id, got.LeadFallback)
		}
	}
}


// Production 14:25 CST 「大家都介绍下自己」 (Langfuse decision peek=f24b88df,
// runs 9bd33f4a/aeb5cffa): SystemOne choice=all + noul≈0.89 + confidence≈0.10 +
// probs[all]≈0.46. Must keep decision mapping (not low-conf discard) and mark
// every member LeadFallback so participation forbids [PASS].
func TestAllHandsIntroLowConfStillLeadFallback(t *testing.T) {
	members, agents := testGroupAgents()
	// Two-member roster like production 文章+代码.
	members = []string{"writer", "coder"}
	agents = []*db.Agent{agents[2], agents[1]} // writer, coder from testGroupAgents order lead/coder/writer
	deps := &autoPickDecisionDeps{
		Provider: "unsloth",
		Decide: func(ctx context.Context, state any, questions map[string]any) (map[string]any, error) {
			return map[string]any{
				"pick": map[string]any{
					"type":   "choice",
					"choice": "all",
					"probabilities": map[string]any{
						// stringified like SystemOne/Langfuse sometimes emit
						"all": "0.4607", "m0": "0.127", "m1": "0.4123",
					},
					"confidence": "0.1039",
				},
				"count": map[string]any{"type": "noul", "noul": "0.8896"},
			}, nil
		},
	}
	got := runAutoPickByMode(db.AutoPickDecision, "大家都介绍下自己", members, agents, "writer", 1, "", deps)
	if got.Method != "decision" {
		t.Fatalf("method=%s want decision (not low_conf fail-open)", got.Method)
	}
	if got.Fallback != "all" || len(got.AgentIDs) != 2 {
		t.Fatalf("want both members fallback=all, got %#v fallback=%s", got.AgentIDs, got.Fallback)
	}
	if got.AllowSecondFailOpen {
		t.Fatal("all-hands must not AllowSecondFailOpen")
	}
	for _, id := range []string{"writer", "coder"} {
		if !got.LeadFallback[id] {
			t.Fatalf("LeadFallback missing %s: %#v", id, got.LeadFallback)
		}
	}
	if got.Confidence < autoPickConfidenceFloor {
		t.Fatalf("confidence salvage failed: %.4f", got.Confidence)
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
	for _, id := range got.AgentIDs {
		if !got.LeadFallback[id] {
			t.Fatalf("heuristic all-hands must LeadFallback %s, got %#v", id, got.LeadFallback)
		}
	}
	// Contrast: specialty question still picks coder, not all.
	got2 := runAutoPickByMode(db.AutoPickHeuristic, "帮我写个函数", members, agents, "lead", 1, "", nil)
	if len(got2.AgentIDs) != 1 || got2.AgentIDs[0] != "coder" {
		t.Fatalf("specialty should still pick coder, got %#v", got2.AgentIDs)
	}
}

func TestCountHintFromAnswers(t *testing.T) {
	if got := countHintFromAnswers(nil, 3); got != 0 {
		t.Fatalf("nil → %d", got)
	}
	if got := countHintFromAnswers(map[string]any{}, 3); got != 0 {
		t.Fatalf("empty → %d", got)
	}
	// noul 0 → 1; 0.5 → 2 of 3; 1 → 3
	if got := countHintFromAnswers(map[string]any{
		"count": map[string]any{"type": "noul", "noul": 0.0},
	}, 3); got != 1 {
		t.Fatalf("noul=0 → %d", got)
	}
	if got := countHintFromAnswers(map[string]any{
		"count": map[string]any{"type": "noul", "noul": 0.5},
	}, 3); got != 2 {
		t.Fatalf("noul=0.5 → %d want 2", got)
	}
	if got := countHintFromAnswers(map[string]any{
		"count": map[string]any{"type": "noul", "noul": 1.0},
	}, 3); got != 3 {
		t.Fatalf("noul=1 → %d", got)
	}
}

func TestMapDecisionChoiceTwoOfThreeFromProbs(t *testing.T) {
	keyToID := map[string]string{"m0": "lead", "m1": "coder", "m2": "writer"}
	// Soft K=1 but two specialists clear threshold → multi subset (not all).
	ids, conf, allSel, ok := mapDecisionChoiceToAgentIDs(map[string]any{
		"pick": map[string]any{
			"type":   "choice",
			"choice": "m1",
			"probabilities": map[string]any{
				"m0": 0.05, "m1": 0.7, "m2": 0.55, "all": 0.1,
			},
			"confidence": 0.7,
		},
	}, keyToID, []string{"lead", "coder", "writer"}, 1, true, false, 0)
	if !ok || allSel {
		t.Fatalf("ok=%v allSel=%v", ok, allSel)
	}
	if len(ids) != 2 || ids[0] != "coder" || ids[1] != "writer" {
		t.Fatalf("want coder,writer (2 of 3), got %#v", ids)
	}
	if conf < 0.6 {
		t.Fatalf("conf=%.2f", conf)
	}
	// Explicit count=1 must not expand via probs.
	ids1, _, allSel1, ok1 := mapDecisionChoiceToAgentIDs(map[string]any{
		"pick": map[string]any{
			"choice": "m1",
			"probabilities": map[string]any{
				"m0": 0.05, "m1": 0.7, "m2": 0.55,
			},
			"confidence": 0.7,
		},
	}, keyToID, []string{"lead", "coder", "writer"}, 1, false, false, 1)
	if !ok1 || allSel1 || len(ids1) != 1 || ids1[0] != "coder" {
		t.Fatalf("count=1 must stay single, got %#v", ids1)
	}
}

func TestAutoPickDecisionTwoOfThreeViaCount(t *testing.T) {
	members, agents := testGroupAgents()
	deps := &autoPickDecisionDeps{
		Provider: "unsloth",
		Decide: func(ctx context.Context, state any, questions map[string]any) (map[string]any, error) {
			if _, ok := questions[autoPickDecisionCountKey]; !ok {
				t.Fatal("payload must ask count")
			}
			return map[string]any{
				"pick": map[string]any{
					"type":   "choice",
					"choice": "m1",
					"probabilities": map[string]any{
						"m0": 0.08, "m1": 0.65, "m2": 0.5,
					},
					"confidence": 0.65,
				},
				"count": map[string]any{
					"type": "noul",
					"noul": 0.5, // → 2 of 3
				},
			}, nil
		},
	}
	// Soft auto_pick_k=1; count raises to 2 — not all, not 1.
	got := runAutoPickByMode(db.AutoPickDecision, "写代码顺便润色文案", members, agents, "lead", 1, "", deps)
	if got.Method != "decision" {
		t.Fatalf("method=%s", got.Method)
	}
	if got.Fallback == "all" {
		t.Fatalf("must not be all-hands fallback, got %#v", got)
	}
	if len(got.AgentIDs) != 2 {
		t.Fatalf("want exactly 2 of 3, got %#v", got.AgentIDs)
	}
	if got.AgentIDs[0] != "coder" || got.AgentIDs[1] != "writer" {
		t.Fatalf("order want coder,writer got %#v", got.AgentIDs)
	}
	// Phrase hits coding specialty synonym → structural safety (no PASS→incapable dump).
	if got.AllowSecondFailOpen {
		t.Fatal("specialty synonym phrase must not AllowSecondFailOpen")
	}
	if !got.LeadFallback["coder"] || !got.LeadFallback["writer"] {
		t.Fatalf("LeadFallback want coder+writer, got %#v", got.LeadFallback)
	}
}

func TestAutoPickDecisionTwoOfThreeViaProbsNoCount(t *testing.T) {
	members, agents := testGroupAgents()
	deps := &autoPickDecisionDeps{
		Provider: "jev",
		Decide: func(ctx context.Context, state any, questions map[string]any) (map[string]any, error) {
			// No count answer — multi inferred from top probabilities above floor.
			return map[string]any{
				"pick": map[string]any{
					"type":   "choice",
					"choice": "m1",
					"probabilities": map[string]any{
						"m0": 0.05, "m1": 0.72, "m2": 0.48,
					},
					"confidence": 0.72,
				},
			}, nil
		},
	}
	got := runAutoPickByMode(db.AutoPickDecision, "写代码并润色文案", members, agents, "lead", 1, "", deps)
	if got.Method != "decision" || len(got.AgentIDs) != 2 {
		t.Fatalf("want 2 of 3 from probs, got %#v method=%s", got.AgentIDs, got.Method)
	}
	if got.AgentIDs[0] != "coder" || got.AgentIDs[1] != "writer" {
		t.Fatalf("got %#v", got.AgentIDs)
	}
	if got.Fallback != "" {
		t.Fatalf("fallback=%s want empty (not all)", got.Fallback)
	}
}

func TestIsWhoCanIntent(t *testing.T) {
	positives := []string{
		"都谁会查资料",
		"谁会查资料",
		"谁能写代码",
		"哪些人会搜索",
		"哪位会写诗",
		"who can research",
		"which of you can code",
		"both can",
	}
	for _, p := range positives {
		if !isWhoCanIntent(p) {
			t.Fatalf("want who-can for %q", p)
		}
	}
	negatives := []string{
		"",
		"帮我写个函数",
		"都介绍下自己",
		"查一下资料",
		"大家介绍一下",
		"随便聊聊天气",
	}
	for _, n := range negatives {
		if isWhoCanIntent(n) {
			t.Fatalf("want NOT who-can for %q", n)
		}
	}
}

func TestMapDecisionWhoCanRemapsAllToCapableMulti(t *testing.T) {
	// Production Unsloth response for 「谁会查资料」: choice=all, low confidence field,
	// count≈0.25→2, mass on all + two researchers (m0/m2); coder m1 low.
	keyToID := map[string]string{"m0": "22", "m1": "33", "m2": "44"}
	ids, conf, allSel, ok := mapDecisionChoiceToAgentIDs(map[string]any{
		"pick": map[string]any{
			"type":   "choice",
			"choice": "all",
			"probabilities": map[string]any{
				"all": 0.3913, "m0": 0.1618, "m1": 0.0786, "m2": 0.3683,
			},
			"confidence": 0.113,
		},
		"count": map[string]any{"type": "noul", "noul": 0.2511},
	}, keyToID, []string{"22", "33", "44"}, 3, true, true, 2)
	if !ok || allSel {
		t.Fatalf("ok=%v allSel=%v (must not expand literally everyone)", ok, allSel)
	}
	if len(ids) != 2 {
		t.Fatalf("want 22+44 (2 capable), got %#v", ids)
	}
	got := map[string]bool{}
	for _, id := range ids {
		got[id] = true
	}
	if !got["22"] || !got["44"] || got["33"] {
		t.Fatalf("want research 22+44 only, got %#v", ids)
	}
	if conf < autoPickConfidenceFloor {
		t.Fatalf("conf=%.2f should clear floor after who-can remap", conf)
	}
}

func TestMapDecisionWhoCanInflatedAllCountNotFullRoster(t *testing.T) {
	// Langfuse 21ed840c / 12:18 CST 「谁会写代码」: choice=all, noul≈0.91→count=3,
	// probs all:0.4591 m0:0.2663 m1:0.1758 m2:0.0989 (m0=coder). Old mapping trusted
	// count=3 and expanded everyone. Inflated full-roster count must use relative mN
	// inference (approved who-can remap) — never literal roster.
	keyToID := map[string]string{"m0": "33", "m1": "22", "m2": "44"}
	ids, conf, allSel, ok := mapDecisionChoiceToAgentIDs(map[string]any{
		"pick": map[string]any{
			"type":   "choice",
			"choice": "all",
			"probabilities": map[string]any{
				"all": 0.4591, "m0": 0.2663, "m1": 0.1758, "m2": 0.0989,
			},
			"confidence": 0.1025,
		},
		"count": map[string]any{"type": "noul", "noul": 0.908},
	}, keyToID, []string{"33", "22", "44"}, 3, true, true, 3)
	if !ok || allSel {
		t.Fatalf("ok=%v allSel=%v (must not expand literally everyone)", ok, allSel)
	}
	if len(ids) >= 3 {
		t.Fatalf("inflated who-can count must not pick full roster, got %#v", ids)
	}
	got := map[string]bool{}
	for _, id := range ids {
		got[id] = true
	}
	if !got["33"] {
		t.Fatalf("coder 33 (top mN) must be included, got %#v", ids)
	}
	if conf < autoPickConfidenceFloor {
		t.Fatalf("conf=%.2f should clear floor after who-can remap", conf)
	}
}

func TestAutoPickDecisionWhoCanCodeInflatedAllFailsNotEveryone(t *testing.T) {
	// End-to-end: same Langfuse shape through autoPickDecision — must not return all 3.
	members := []string{"33", "22", "44"}
	agents := []*db.Agent{
		{ID: "33", Name: "33", Description: "负责写代码与排障：改项目、查 bug、联调。"},
		{ID: "22", Name: "22", Description: "负责查资料与总结：搜索、阅读、整理要点。"},
		{ID: "44", Name: "44", Description: "负责查资料与总结：搜索、阅读、整理要点。"},
	}
	deps := &autoPickDecisionDeps{
		Provider: "unsloth",
		Decide: func(ctx context.Context, state any, questions map[string]any) (map[string]any, error) {
			pick := questions["pick"].(map[string]any)
			if !strings.Contains(fmt.Sprint(pick["instructions"]), "谁会写代码") {
				t.Fatalf("who-can decide must get strengthened 写代码 census prompt")
			}
			return map[string]any{
				"pick": map[string]any{
					"type":   "choice",
					"choice": "all",
					"probabilities": map[string]any{
						"all": 0.4591, "m0": 0.2663, "m1": 0.1758, "m2": 0.0989,
					},
					"confidence": 0.1025,
				},
				"count": map[string]any{"type": "noul", "noul": 0.908},
			}, nil
		},
	}
	got := runAutoPickByMode(db.AutoPickDecision, "谁会写代码", members, agents, "22", 1, "我叫 44，主要负责查资料、阅读和整理总结。 | 谁会写代码", deps)
	if got.Method != "decision" {
		t.Fatalf("method=%s want decision (not fail-open)", got.Method)
	}
	if got.Fallback == "all" || len(got.AgentIDs) >= 3 {
		t.Fatalf("must not expand inflated who-can all to roster, got %#v fallback=%s", got.AgentIDs, got.Fallback)
	}
	set := map[string]bool{}
	for _, id := range got.AgentIDs {
		set[id] = true
	}
	if !set["33"] {
		t.Fatalf("coder 33 must be selected, got %#v", got.AgentIDs)
	}
	if got.AllowSecondFailOpen {
		t.Fatalf("who-can must not AllowSecondFailOpen")
	}
}

func TestAutoPickDecisionWhoCanBothResearchers(t *testing.T) {
	members := []string{"22", "33", "44"}
	agents := []*db.Agent{
		{ID: "22", Name: "22", Description: "负责查资料与总结：搜索、阅读、整理要点。"},
		{ID: "33", Name: "33", Description: "负责写代码与排障：改项目、查 bug、联调。"},
		{ID: "44", Name: "44", Description: "负责查资料与总结：搜索、阅读、整理要点。"},
	}
	deps := &autoPickDecisionDeps{
		Provider: "unsloth",
		Decide: func(ctx context.Context, state any, questions map[string]any) (map[string]any, error) {
			return map[string]any{
				"pick": map[string]any{
					"type":   "choice",
					"choice": "all",
					"probabilities": map[string]any{
						"all": 0.5413, "m0": 0.1661, "m1": 0.0641, "m2": 0.2284,
					},
					"confidence": 0.1749,
				},
				"count": map[string]any{"type": "noul", "noul": 0.2675},
			}, nil
		},
	}
	got := runAutoPickByMode(db.AutoPickDecision, "都谁会查资料", members, agents, "22", 1, "", deps)
	if got.Method != "decision" {
		t.Fatalf("method=%s want decision (not fail-open heuristic)", got.Method)
	}
	if got.Fallback == "all" {
		t.Fatalf("who-can must not fallback=all, got %#v", got)
	}
	if len(got.AgentIDs) != 2 {
		t.Fatalf("want both researchers, got %#v", got.AgentIDs)
	}
	set := map[string]bool{}
	for _, id := range got.AgentIDs {
		set[id] = true
	}
	if !set["22"] || !set["44"] || set["33"] {
		t.Fatalf("want 22+44, got %#v", got.AgentIDs)
	}
	if got.AllowSecondFailOpen {
		t.Fatalf("who-can must not AllowSecondFailOpen")
	}
	if !got.LeadFallback["22"] || !got.LeadFallback["44"] || got.LeadFallback["33"] {
		t.Fatalf("leadFallback want 22+44, got %#v", got.LeadFallback)
	}
}

func TestAutoPickDecisionWhoCanResearchViaModelNotChannelOrder(t *testing.T) {
	// Production 2026-10-12 10:35 CST: 「谁能帮我查下资料？」— old SystemOne returned
	// choice=all count≈2 with diluted mN ranking coder first (would map to 33+22 by
	// channel-order top-N). Prompt now teaches role-matched mN; mock that output.
	// Mapping must trust choice+count+probs — no capableIDsFromRoleMatch override.
	members := []string{"33", "22", "44"}
	agents := []*db.Agent{
		{ID: "33", Name: "33", Description: "负责写代码与排障：改项目、查 bug、联调。"},
		{ID: "22", Name: "22", Description: "负责查资料与总结：搜索、阅读、整理要点。"},
		{ID: "44", Name: "44", Description: "负责查资料与总结：搜索、阅读、整理要点。"},
	}
	deps := &autoPickDecisionDeps{
		Provider: "unsloth",
		Decide: func(ctx context.Context, state any, questions map[string]any) (map[string]any, error) {
			return map[string]any{
				"pick": map[string]any{
					"type":   "choice",
					"choice": "m1",
					"probabilities": map[string]any{
						"all": 0.05, "m0": 0.05, "m1": 0.48, "m2": 0.42,
					},
					"confidence": 0.75,
				},
				"count": map[string]any{"type": "noul", "noul": 0.5},
			}, nil
		},
	}
	got := runAutoPickByMode(db.AutoPickDecision, "谁能帮我查下资料？", members, agents, "22", 1, "22 和 44。", deps)
	if got.Method != "decision" {
		t.Fatalf("method=%s want decision", got.Method)
	}
	if got.Fallback == "all" {
		t.Fatalf("who-can must not fallback=all, got %#v", got)
	}
	set := map[string]bool{}
	for _, id := range got.AgentIDs {
		set[id] = true
	}
	if !set["22"] || !set["44"] || set["33"] || len(got.AgentIDs) != 2 {
		t.Fatalf("want research 22+44 only (not channel-order 33+22), got %#v", got.AgentIDs)
	}
	if got.AllowSecondFailOpen {
		t.Fatalf("who-can must not AllowSecondFailOpen")
	}
}

func TestAutoPickDecisionWhoCanUsesCriteriaRolesNotSynopsis(t *testing.T) {
	// Production 2026-10-12 11:34 CST: 「你们谁能帮我查下资料」— model wrongly chose
	// m0=coder while Recent context had 44's research intro. Prompt now says match
	// criteria roles, not synopsis. Mock correct researchers; mapping trusts model.
	members := []string{"33", "22", "44"}
	agents := []*db.Agent{
		{ID: "33", Name: "33", Description: "负责写代码与排障：改项目、查 bug、联调。"},
		{ID: "22", Name: "22", Description: "负责查资料与总结：搜索、阅读、整理要点。"},
		{ID: "44", Name: "44", Description: "负责查资料与总结：搜索、阅读、整理要点。"},
	}
	synopsis := "我是44，主要负责查资料、阅读和整理总结，尽量把结论与来源依据说清楚。 | 你们谁能帮我查下资料"
	var sawState string
	deps := &autoPickDecisionDeps{
		Provider: "unsloth",
		Decide: func(ctx context.Context, state any, questions map[string]any) (map[string]any, error) {
			sawState = fmt.Sprint(state)
			pick := questions["pick"].(map[string]any)
			if !strings.Contains(fmt.Sprint(pick["instructions"]), "CAPABILITY CENSUS") {
				t.Fatalf("who-can decide must get census instructions, got %v", pick["instructions"])
			}
			return map[string]any{
				"pick": map[string]any{
					"type":   "choice",
					"choice": "m1",
					"probabilities": map[string]any{
						"all": 0.04, "m0": 0.06, "m1": 0.46, "m2": 0.44,
					},
					"confidence": 0.78,
				},
				"count": map[string]any{"type": "noul", "noul": 0.5},
			}, nil
		},
	}
	got := runAutoPickByMode(db.AutoPickDecision, "你们谁能帮我查下资料", members, agents, "22", 1, synopsis, deps)
	if got.Method != "decision" {
		t.Fatalf("method=%s want decision", got.Method)
	}
	if got.Fallback == "all" {
		t.Fatalf("who-can must not fallback=all, got %#v", got)
	}
	set := map[string]bool{}
	for _, id := range got.AgentIDs {
		set[id] = true
	}
	if !set["22"] || !set["44"] || set["33"] || len(got.AgentIDs) != 2 {
		t.Fatalf("want research 22+44 only (criteria roles, not synopsis bleed), got %#v", got.AgentIDs)
	}
	if !strings.Contains(sawState, "not an override of criteria roles") {
		t.Fatalf("state must warn synopsis is not role override, got %q", sawState)
	}
}

func TestAutoPickDecisionWhoCanSingleCoderNotAllHands(t *testing.T) {
	// 「谁会写代码」with members ordered coder first: model picks m0 + count=1.
	members := []string{"33", "22", "44"}
	agents := []*db.Agent{
		{ID: "33", Name: "33", Description: "负责写代码与排障：改项目、查 bug、联调。"},
		{ID: "22", Name: "22", Description: "负责查资料与总结：搜索、阅读、整理要点。"},
		{ID: "44", Name: "44", Description: "负责查资料与总结：搜索、阅读、整理要点。"},
	}
	deps := &autoPickDecisionDeps{
		Provider: "unsloth",
		Decide: func(ctx context.Context, state any, questions map[string]any) (map[string]any, error) {
			return map[string]any{
				"pick": map[string]any{
					"type":   "choice",
					"choice": "m0",
					"probabilities": map[string]any{
						"all": 0.03, "m0": 0.88, "m1": 0.05, "m2": 0.04,
					},
					"confidence": 0.86,
				},
				"count": map[string]any{"type": "noul", "noul": 0.02},
			}, nil
		},
	}
	got := runAutoPickByMode(db.AutoPickDecision, "谁会写代码", members, agents, "22", 1, "", deps)
	if got.Method != "decision" || len(got.AgentIDs) != 1 || got.AgentIDs[0] != "33" {
		t.Fatalf("want only coder 33, got %#v method=%s", got.AgentIDs, got.Method)
	}
	if got.AllowSecondFailOpen || !got.LeadFallback["33"] {
		t.Fatalf("structural safety: LeadFallback 33, no second-wave, got %#v", got)
	}
}

func TestAutoPickDecisionAllHandsPromptNeedlesNoStructuralExpand(t *testing.T) {
	// Prompt-only fix (no underpick→roster remap): all-hands payload must force choice=all.
	// Langfuse 38726dc4 shape (choice=m0 diluted) is trusted as-is — do NOT assert expand.
	members := []string{"635d1077-5611-41bb-9fcf-2a4454d43d5a", "bfcea5bc-91b6-4bf7-b31a-ea45f2fcb0bf", "f634df70-8e30-4f71-9415-c7f80a8c0ec3"}
	agents := []*db.Agent{
		{ID: members[0], Name: "33", Description: "负责写代码与排障：改项目、查 bug、联调。"},
		{ID: members[1], Name: "22", Description: "负责查资料与总结：搜索、阅读、整理要点。"},
		{ID: members[2], Name: "44", Description: "负责查资料与总结：搜索、阅读、整理要点。"},
	}
	var sawAllHandsPrompt bool
	deps := &autoPickDecisionDeps{
		Provider: "unsloth",
		Decide: func(ctx context.Context, state any, questions map[string]any) (map[string]any, error) {
			pick := questions["pick"].(map[string]any)
			instr := fmt.Sprint(pick["instructions"])
			countQ := questions[autoPickDecisionCountKey].(map[string]any)
			countInstr := fmt.Sprint(countQ["instructions"])
			st := fmt.Sprint(state)
			for _, needle := range []string{"ALL-HANDS", "都介绍下自己", "大家都介绍一下", `choice="all"`, "never a single mN"} {
				if !strings.Contains(instr, needle) {
					t.Fatalf("all-hands pick instructions missing %q: %s", needle, instr)
				}
			}
			if !strings.Contains(countInstr, "ALL-HANDS count") || !strings.Contains(countInstr, "near 1") {
				t.Fatalf("all-hands count instructions weak: %s", countInstr)
			}
			if !strings.Contains(st, `choice must be "all"`) {
				t.Fatalf("all-hands state missing mandate: %q", st)
			}
			sawAllHandsPrompt = true
			// Langfuse 38726dc4 underpick shape — mapping must trust model (no structural expand).
			return map[string]any{
				"pick": map[string]any{
					"type":   "choice",
					"choice": "m0",
					"probabilities": map[string]any{
						"all": 0.2909, "m0": 0.4469, "m1": 0.209, "m2": 0.0532,
					},
					"confidence": 0.1327,
				},
				"count": map[string]any{"type": "noul", "noul": 0.8364},
			}, nil
		},
	}
	got := runAutoPickByMode(db.AutoPickDecision, "都介绍下自己", members, agents, members[0], 1, "都介绍下自己", deps)
	if !sawAllHandsPrompt {
		t.Fatal("decide must receive all-hands-strengthened payload")
	}
	// Trust model: do not remap underpick to full roster (prompt should make live model return choice=all).
	if got.Fallback == "all" && len(got.AgentIDs) == 3 {
		t.Fatalf("must not structurally expand underpick to roster; got %#v (prompt-only fix)", got.AgentIDs)
	}
	if got.Method != "decision" && !strings.HasPrefix(got.Method, "decision->heuristic") {
		t.Fatalf("unexpected method=%s got=%#v", got.Method, got)
	}
	// Contrast: who-can with single capable must NOT expand to all.
	deps2 := &autoPickDecisionDeps{
		Provider: "unsloth",
		Decide: func(ctx context.Context, state any, questions map[string]any) (map[string]any, error) {
			return map[string]any{
				"pick": map[string]any{
					"type":   "choice",
					"choice": "m0",
					"probabilities": map[string]any{
						"all": 0.03, "m0": 0.88, "m1": 0.05, "m2": 0.04,
					},
					"confidence": 0.86,
				},
				"count": map[string]any{"type": "noul", "noul": 0.02},
			}, nil
		},
	}
	got2 := runAutoPickByMode(db.AutoPickDecision, "谁会写代码", members, agents, members[1], 1, "", deps2)
	if len(got2.AgentIDs) != 1 || got2.AgentIDs[0] != members[0] || got2.Fallback == "all" {
		t.Fatalf("who-can must stay coder-only subset, got %#v fallback=%s", got2.AgentIDs, got2.Fallback)
	}
	if got2.AllowSecondFailOpen || !got2.LeadFallback[members[0]] {
		t.Fatalf("who-can structural safety: LeadFallback + no second-wave, got %#v", got2)
	}
}

func TestAutoPickDecisionAllHandsStillCallsDecide(t *testing.T) {
	// 「都介绍下自己」— still call decide; choice=all → everyone OK.
	members := []string{"22", "33", "44"}
	agents := []*db.Agent{
		{ID: "22", Name: "22", Description: "负责查资料与总结"},
		{ID: "33", Name: "33", Description: "负责写代码与排障"},
		{ID: "44", Name: "44", Description: "负责查资料与总结"},
	}
	called := false
	deps := &autoPickDecisionDeps{
		Provider: "unsloth",
		Decide: func(ctx context.Context, state any, questions map[string]any) (map[string]any, error) {
			called = true
			pick := questions["pick"].(map[string]any)
			instr := fmt.Sprint(pick["instructions"])
			if !strings.Contains(instr, "ALL-HANDS") && !strings.Contains(strings.ToLower(instr), "all-hands") {
				t.Fatalf("all-hands decide must get all-hands instructions, got %s", instr)
			}
			return map[string]any{
				"pick": map[string]any{
					"type": "choice", "choice": "all", "confidence": 0.9,
					"probabilities": map[string]any{"all": 0.9, "m0": 0.03, "m1": 0.03, "m2": 0.04},
				},
				"count": map[string]any{"type": "noul", "noul": 0.95},
			}, nil
		},
	}
	got := runAutoPickByMode(db.AutoPickDecision, "都介绍下自己", members, agents, "22", 1, "", deps)
	if !called {
		t.Fatal("all-hands must still call decide")
	}
	if got.Method != "decision" || got.Fallback != "all" || len(got.AgentIDs) != 3 {
		t.Fatalf("want decision fallback=all all members, got %#v", got)
	}
}

func TestAutoPickHeuristicWhoCanBothResearchers(t *testing.T) {
	members := []string{"22", "33", "44"}
	agents := []*db.Agent{
		{ID: "22", Name: "22", Description: "负责查资料与总结：搜索、阅读、整理要点。"},
		{ID: "33", Name: "33", Description: "负责写代码与排障：改项目、查 bug、联调。"},
		{ID: "44", Name: "44", Description: "负责查资料与总结：搜索、阅读、整理要点。"},
	}
	got := autoPickHeuristic("都谁会查资料", members, agents, "22", 1, "")
	if len(got.AgentIDs) != 2 {
		t.Fatalf("who-can heuristic should pick both research bots, got %#v conf=%.2f", got.AgentIDs, got.Confidence)
	}
	set := map[string]bool{}
	for _, id := range got.AgentIDs {
		set[id] = true
	}
	if !set["22"] || !set["44"] || set["33"] {
		t.Fatalf("want 22+44, got %#v", got.AgentIDs)
	}
}


func TestBuildAutoPickDecisionPayloadSpecialtyResearchTask(t *testing.T) {
	members, agents := testGroupAgents()
	_, q, st := buildAutoPickDecisionPayload("帮我查下2026年优锘科技的发展", members, agents, "谁不会写代码：22和44查资料", false, false)
	pick := q["pick"].(map[string]any)
	instr := fmt.Sprint(pick["instructions"])
	for _, needle := range []string{"SPECIALTY TASK", "帮我查下", "research", "never park on", "22+44", "not the coder"} {
		if !strings.Contains(instr, needle) {
			t.Fatalf("specialty instructions missing %q: %s", needle, instr)
		}
	}
	count := q[autoPickDecisionCountKey].(map[string]any)
	countInstr := fmt.Sprint(count["instructions"])
	if !strings.Contains(countInstr, "SPECIALTY TASK count") || !strings.Contains(countInstr, "FORBIDDEN") {
		t.Fatalf("specialty count instructions=%v", count["instructions"])
	}
	if !strings.Contains(st, "specialty tasks") || !strings.Contains(st, "research mNs only") {
		t.Fatalf("specialty state missing mandate: %q", st)
	}
	// who-can / all-hands must NOT get specialty block
	_, qWho, _ := buildAutoPickDecisionPayload("谁会写代码", members, agents, "", false, true)
	if strings.Contains(fmt.Sprint(qWho["pick"].(map[string]any)["instructions"]), "SPECIALTY TASK") {
		t.Fatal("who-can must not append SPECIALTY TASK block")
	}
	_, qAll, _ := buildAutoPickDecisionPayload("都介绍下自己", members, agents, "", true, false)
	if strings.Contains(fmt.Sprint(qAll["pick"].(map[string]any)["instructions"]), "SPECIALTY TASK") {
		t.Fatal("all-hands must not append SPECIALTY TASK block")
	}
}

// Production 2026-10-12 12:28 CST: 「帮我查下2026年优锘科技的发展」— decision
// returned choice=all conf≈0.04 → fail-open heuristic; old needles missed bare「查下」
// so lead/coder 33 answered. Needles must route research lookup to 22+44.
func TestAutoPickHeuristicResearchLookupYouNuo(t *testing.T) {
	members := []string{"33", "22", "44"}
	agents := []*db.Agent{
		{ID: "33", Name: "33", Description: "负责写代码与排障：改项目、查 bug、联调。"},
		{ID: "22", Name: "22", Description: "负责查资料与总结：搜索、阅读、整理要点。"},
		{ID: "44", Name: "44", Description: "负责查资料与总结：搜索、阅读、整理要点。"},
	}
	// Synopsis mentions 写代码 (prior census) — must NOT pull coder 33 when ask is 查下.
	got := autoPickHeuristic("帮我查下2026年优锘科技的发展", members, agents, "33", 1, "要的。你刚才问的是谁不会写代码：22 和 44 主要负责查资料与总结；33 负责写代码与排障。")
	if len(got.AgentIDs) != 2 {
		t.Fatalf("research lookup heuristic want 22+44, got %#v conf=%.2f", got.AgentIDs, got.Confidence)
	}
	set := map[string]bool{}
	for _, id := range got.AgentIDs {
		set[id] = true
	}
	if !set["22"] || !set["44"] || set["33"] {
		t.Fatalf("want research 22+44 only (not lead/coder 33), got %#v", got.AgentIDs)
	}
}


// Production 2026-10-12 12:35 CST Langfuse peek/obs=3569c5ef947ad07a (trace 38cb71ea…):
// 「帮我查下2025年格灵深瞳的年报」— c5c5231 SPECIALTY TASK prompt was loaded, but SystemOne
// still chose m0=coder (pick-prob 0.61, calibrated conf 0.23, count noul≈0.99).
// Specialty mismatch vs role-capable 22+44 → fail-open heuristic (not trust m0).
// Companion run peek/obs=f258fadb… / trace 042ffb71… answered by 33.
func TestAutoPickDecisionSpecialtyResearchMismatchGeLing(t *testing.T) {
	members := []string{"33", "22", "44"}
	agents := []*db.Agent{
		{ID: "33", Name: "33", Description: "负责写代码与排障：改项目、查 bug、联调。"},
		{ID: "22", Name: "22", Description: "负责查资料与总结：搜索、阅读、整理要点。"},
		{ID: "44", Name: "44", Description: "负责查资料与总结：搜索、阅读、整理要点。"},
	}
	deps := &autoPickDecisionDeps{
		Provider: "unsloth",
		Decide: func(ctx context.Context, state any, questions map[string]any) (map[string]any, error) {
			return map[string]any{
				"pick": map[string]any{
					"type":       "choice",
					"choice":     "m0", // coder 33
					"confidence": 0.2288,
					"probabilities": map[string]any{
						"all": 0.1798, "m0": 0.6116, "m1": 0.1388, "m2": 0.0698,
					},
				},
				"count": map[string]any{"type": "noul", "noul": 0.9883},
			}, nil
		},
	}
	synopsis := "要的。你刚才问的是谁不会写代码：22 和 44 主要负责查资料与总结；33 负责写代码与排障。"
	got := runAutoPickByMode(db.AutoPickDecision, "帮我查下2025年格灵深瞳的年报", members, agents, "33", 1, synopsis, deps)
	if got.Method != "decision->heuristic" {
		t.Fatalf("want decision->heuristic fail-open, got method=%s ids=%#v conf=%.2f", got.Method, got.AgentIDs, got.Confidence)
	}
	if len(got.AgentIDs) != 2 {
		t.Fatalf("research specialty want 22+44, got %#v", got.AgentIDs)
	}
	set := map[string]bool{}
	for _, id := range got.AgentIDs {
		set[id] = true
	}
	if !set["22"] || !set["44"] || set["33"] {
		t.Fatalf("want research 22+44 only (not coder 33), got %#v", got.AgentIDs)
	}
}

// Production 2026-10-12 13:45 CST Langfuse peek/obs=5808f45a342469dc
// (trace a32fde27da3cf91c5cf57a042b456afa): 「写个js的代码，格式化日期为hh:mm:ss。输入是时间戳」
// SystemOne choice=m0 (代码与排障助手) noul≈0.98 conf field 0.05 pick-prob 0.38 → below
// floor → decision→heuristic both coders; both [PASS]; second fail-open dumped to 文档助手
// who answered. Specialty picks must LeadFallback + AllowSecondFailOpen=false.
func TestSpecialtyCodingNoSecondWaveDocBot(t *testing.T) {
	members := []string{
		"1d818a58-4f4a-48f3-9bcd-1de613d6dcec", // 代码与排障助手 (m0 / lead)
		"550cda95-1e8f-4abd-911a-eead2c4803e3", // coder (m1)
		"a21d3e74-a9a1-4b30-8e5f-042074819ccf", // 文档助手 (m2)
	}
	agents := []*db.Agent{
		{ID: members[0], Name: "代码与排障助手", Description: "负责写代码与排障：改项目、查 bug、联调。小步验证；危险操作需确认。远程 SSH 先 load_skill host-ssh。"},
		{ID: members[1], Name: "coder", Description: "负责写代码与排障：改项目、查 bug、联调。小步验证；危险操作需确认。远程 SSH 先 load_skill host-ssh。"},
		{ID: members[2], Name: "文档助手", Description: "负责写东西与改稿：邮件、文档、文案。尊重用户语气；定稿外发前先给草稿。"},
	}
	phrase := "写个js的代码，格式化日期为hh:mm:ss。输入是时间戳"
	deps := &autoPickDecisionDeps{
		Provider: "unsloth",
		Decide: func(ctx context.Context, state any, questions map[string]any) (map[string]any, error) {
			// Exact Langfuse 5808f45a shape.
			return map[string]any{
				"pick": map[string]any{
					"type":       "choice",
					"choice":     "m0",
					"confidence": 0.0463,
					"probabilities": map[string]any{
						"all": 0.2593, "m0": 0.3837, "m1": 0.2219, "m2": 0.1352,
					},
				},
				"count": map[string]any{"type": "noul", "noul": 0.9824},
			}, nil
		},
	}
	got := runAutoPickByMode(db.AutoPickDecision, phrase, members, agents, members[0], 1, phrase+" | "+phrase, deps)
	if got.Method != "decision->heuristic" {
		t.Fatalf("want low-conf fail-open decision->heuristic, got method=%s ids=%#v conf=%.2f", got.Method, got.AgentIDs, got.Confidence)
	}
	set := map[string]bool{}
	for _, id := range got.AgentIDs {
		set[id] = true
	}
	if set[members[2]] {
		t.Fatalf("must not pick 文档助手 for JS coding ask, got %#v", got.AgentIDs)
	}
	if !set[members[0]] && !set[members[1]] {
		t.Fatalf("want at least one coding bot, got %#v", got.AgentIDs)
	}
	if got.AllowSecondFailOpen {
		t.Fatalf("specialty must not AllowSecondFailOpen (else PASS→文档助手), got %#v", got)
	}
	for _, id := range got.AgentIDs {
		if !got.LeadFallback[id] {
			t.Fatalf("LeadFallback missing for capable %s, got %#v", id, got.LeadFallback)
		}
	}
	if got.LeadFallback[members[2]] {
		t.Fatalf("文档助手 must not be LeadFallback, got %#v", got.LeadFallback)
	}

	// Heuristic alone (same channel) must also block second-wave.
	h := autoPickHeuristic(phrase, members, agents, members[0], 1, "")
	if h.AllowSecondFailOpen {
		t.Fatalf("heuristic specialty must not AllowSecondFailOpen, got %#v", h)
	}
	for _, id := range h.AgentIDs {
		if id == members[2] {
			t.Fatalf("heuristic must not pick 文档助手, got %#v", h.AgentIDs)
		}
		if !h.LeadFallback[id] {
			t.Fatalf("heuristic LeadFallback missing %s, got %#v", id, h.LeadFallback)
		}
	}
}
