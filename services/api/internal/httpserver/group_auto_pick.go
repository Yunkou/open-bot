package httpserver

import (
	"context"
	"encoding/json"
	"log"
	"sort"
	"strings"
	"unicode"

	"github.com/tangxin/open-bot/services/api/internal/db"
)

const (
	autoPickDefaultK        = 1
	autoPickMaxK            = 3
	autoPickConfidenceFloor = 0.4 // below → fail-open to lead
)

// groupAutoPickResult is the A-tier heuristic (or fail-open) outcome for a no-@ turn.
type groupAutoPickResult struct {
	AgentIDs   []string
	Confidence float64
	Method     string // "heuristic"
	Fallback   string // "" | "lead" | "all"
	// LeadFallback marks agents that should get the "you are the fallback, please answer" prompt.
	LeadFallback map[string]bool
	// AllowSecondFailOpen: if every AutoPick candidate PASS-es, run lead (or all).
	AllowSecondFailOpen bool
}

// groupAutoPickOpts configures A-tier AutoPick for a no-@ group turn.
type groupAutoPickOpts struct {
	Enabled        bool
	K              int
	RecentSynopsis string // optional last 1–2 msg synopsis
}

func clampAutoPickK(k int) int {
	if k <= 0 {
		return autoPickDefaultK
	}
	if k > autoPickMaxK {
		return autoPickMaxK
	}
	return k
}

// groupHasExplicitAddressing is true when @ / @everyone / a proper subset of client
// agent_ids already chose targets. AutoPick must never replace those forced sets.
//
// Legacy web clients send every channel member as agent_ids on no-@ turns ("all
// candidates, each may PASS"). That full-member dump is NOT explicit addressing —
// treating it as such skipped AutoPick and ran every bot (duplicate UX + wrong pick).
func groupHasExplicitAddressing(content string, explicit []string, members []string, agents []*db.Agent) bool {
	if mentionIncludesEveryone(content) {
		return true
	}
	if len(resolveMentionedAgents(parseMentionTokens(content), members, agents)) > 0 {
		return true
	}
	memberSet := map[string]struct{}{}
	for _, m := range members {
		m = strings.TrimSpace(m)
		if m != "" {
			memberSet[m] = struct{}{}
		}
	}
	if len(memberSet) == 0 {
		return false
	}
	seen := map[string]struct{}{}
	n := 0
	for _, id := range explicit {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := memberSet[id]; !ok {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		n++
	}
	if n == 0 {
		return false
	}
	// Full-member dump = legacy candidate list; AutoPick still applies.
	if n == len(memberSet) {
		return false
	}
	return true
}

// autoPickHeuristic picks ≤K members by keyword / role-description match; else fail-open to lead.
// No extra LLM. leadID is conv.AgentID / group lead assistant.
func autoPickHeuristic(userText string, members []string, agents []*db.Agent, leadID string, k int, synopsis string) groupAutoPickResult {
	return autoPickHeuristicAs("heuristic", userText, members, agents, leadID, k, synopsis)
}

// autoPickHeuristicAs is autoPickHeuristic with an explicit log/Method label
// (e.g. "decision->heuristic" when B-tier fail-opens).
func autoPickHeuristicAs(method, userText string, members []string, agents []*db.Agent, leadID string, k int, synopsis string) groupAutoPickResult {
	k = clampAutoPickK(k)
	leadID = strings.TrimSpace(leadID)
	if strings.TrimSpace(method) == "" {
		method = "heuristic"
	}

	memberSet := map[string]struct{}{}
	for _, m := range members {
		memberSet[m] = struct{}{}
	}
	leadInMembers := false
	if leadID != "" {
		_, leadInMembers = memberSet[leadID]
	}

	failOpenLead := func() groupAutoPickResult {
		if !leadInMembers {
			out := copyStringSlice(members)
			log.Printf("auto_pick=%s ids=%v conf=0.00 fallback=all", method, out)
			return groupAutoPickResult{
				AgentIDs:            out,
				Confidence:          0,
				Method:              method,
				Fallback:            "all",
				LeadFallback:        map[string]bool{},
				AllowSecondFailOpen: false,
			}
		}
		log.Printf("auto_pick=%s ids=%v conf=0.00 fallback=lead", method, []string{leadID})
		return groupAutoPickResult{
			AgentIDs:            []string{leadID},
			Confidence:          0,
			Method:              method,
			Fallback:            "lead",
			LeadFallback:        map[string]bool{leadID: true},
			AllowSecondFailOpen: true,
		}
	}

	byID := map[string]*db.Agent{}
	for _, a := range agents {
		if a != nil {
			byID[a.ID] = a
		}
	}

	haystack := strings.ToLower(strings.TrimSpace(userText))
	if syn := strings.TrimSpace(synopsis); syn != "" {
		haystack = haystack + "\n" + strings.ToLower(syn)
	}
	if haystack == "" {
		return failOpenLead()
	}

	type scored struct {
		id    string
		score int
	}
	var ranked []scored
	for _, id := range members {
		a := byID[id]
		name := id
		role := ""
		if a != nil {
			if n := strings.TrimSpace(a.Name); n != "" {
				name = n
			}
			role = groupParticipationRoleHint(a)
		}
		sc := scoreAutoPickMatch(haystack, name, role)
		if sc > 0 {
			ranked = append(ranked, scored{id: id, score: sc})
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].id < ranked[j].id
	})

	if len(ranked) == 0 {
		return failOpenLead()
	}

	best := ranked[0].score
	conf := autoPickConfidence(best)
	if conf < autoPickConfidenceFloor {
		return failOpenLead()
	}

	n := k
	if n > len(ranked) {
		n = len(ranked)
	}
	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		ids = append(ids, ranked[i].id)
	}
	log.Printf("auto_pick=%s ids=%v conf=%.2f fallback=", method, ids, conf)
	return groupAutoPickResult{
		AgentIDs:            ids,
		Confidence:          conf,
		Method:              method,
		Fallback:            "",
		LeadFallback:        map[string]bool{},
		AllowSecondFailOpen: true,
	}
}

func autoPickConfidence(bestScore int) float64 {
	if bestScore <= 0 {
		return 0
	}
	// 1 → 0.55, 2 → 0.75, 3+ → 0.95
	c := 0.35 + 0.20*float64(bestScore)
	if c > 0.95 {
		c = 0.95
	}
	return c
}

// scoreAutoPickMatch scores how well name/role fit the user haystack (lowercased).
func scoreAutoPickMatch(haystack, name, role string) int {
	score := 0
	nameL := strings.ToLower(strings.TrimSpace(name))
	roleL := strings.ToLower(strings.TrimSpace(role))

	if nameL != "" && len([]rune(nameL)) >= 2 && strings.Contains(haystack, nameL) {
		score += 3
	}

	for _, tok := range tokenizeRoleHints(roleL) {
		if strings.Contains(haystack, tok) {
			score += 2
		}
	}
	for _, tok := range tokenizeUserNeedles(haystack) {
		if roleL != "" && strings.Contains(roleL, tok) {
			score += 1
		}
		if nameL != "" && strings.Contains(nameL, tok) {
			score += 1
		}
	}

	score += scoreRoleSynonyms(haystack, nameL+" "+roleL)
	return score
}

func tokenizeRoleHints(role string) []string {
	role = strings.TrimSpace(role)
	if role == "" {
		return nil
	}
	parts := strings.FieldsFunc(role, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r) || r == '、' || r == '，' || r == '/' || r == '|' || r == '：' || r == ':'
	})
	seen := map[string]struct{}{}
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(strings.ToLower(s))
		if s == "" || len([]rune(s)) < 2 {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	for _, p := range parts {
		add(p)
		runes := []rune(p)
		if len(runes) >= 4 {
			for i := 0; i+2 <= len(runes) && i < 8; i++ {
				add(string(runes[i : i+2]))
			}
		}
	}
	return out
}

func tokenizeUserNeedles(haystack string) []string {
	var out []string
	seen := map[string]struct{}{}
	add := func(s string) {
		s = strings.ToLower(strings.TrimSpace(s))
		if len([]rune(s)) < 2 {
			return
		}
		if _, ok := seen[s]; ok {
			return
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	var buf []rune
	flushLatin := func() {
		if len(buf) >= 2 {
			add(string(buf))
		}
		buf = buf[:0]
	}
	for _, r := range haystack {
		switch {
		case unicode.IsLetter(r) && r < 0x80:
			buf = append(buf, unicode.ToLower(r))
		case unicode.Is(unicode.Han, r):
			flushLatin()
			buf = append(buf, r)
			if len(buf) >= 2 {
				add(string(buf[len(buf)-2:]))
			}
			if len(buf) > 6 {
				buf = buf[len(buf)-4:]
			}
		default:
			flushLatin()
			buf = buf[:0]
		}
	}
	flushLatin()
	return out
}

var roleSynonymBuckets = []struct {
	userNeedles []string
	roleNeedles []string
}{
	{
		userNeedles: []string{"函数", "代码", "bug", "修bug", "编程", "实现", "编译", "报错", "排障", "refactor", "code", "function", "api", "脚本", "单元测试", "写个函数", "修一下"},
		roleNeedles: []string{"码", "程序", "代码", "编程", "开发", "工程", "排障", "tech", "code", "engineer", "软件", "后端", "前端"},
	},
	{
		userNeedles: []string{"写诗", "文案", "改稿", "小说", "散文", "写作", "润色", "起标题"},
		roleNeedles: []string{"写", "文案", "写作", "编辑", "内容", "诗人", "作家"},
	},
	{
		userNeedles: []string{"查资料", "搜索", "研究", "调研", "检索", "百科"},
		roleNeedles: []string{"研究", "调研", "资料", "搜索", "分析"},
	},
	{
		userNeedles: []string{"翻译", "英文", "中译", "localize"},
		roleNeedles: []string{"翻译", "译", "语言", "英语"},
	},
}

func scoreRoleSynonyms(haystack, roleBlob string) int {
	score := 0
	for _, b := range roleSynonymBuckets {
		userHit := false
		for _, n := range b.userNeedles {
			if n != "" && strings.Contains(haystack, strings.ToLower(n)) {
				userHit = true
				break
			}
		}
		if !userHit {
			continue
		}
		for _, n := range b.roleNeedles {
			if n != "" && strings.Contains(roleBlob, strings.ToLower(n)) {
				score += 2
				break
			}
		}
	}
	return score
}

func copyStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

// secondFailOpenTargets chooses who runs after AutoPick candidates all PASS.
// Prefer lead if in members and not already tried; else remaining members.
func secondFailOpenTargets(members []string, alreadyRan []string, leadID string) (targets []string, leadFallback map[string]bool, fallback string) {
	leadFallback = map[string]bool{}
	ran := map[string]struct{}{}
	for _, id := range alreadyRan {
		ran[id] = struct{}{}
	}
	leadID = strings.TrimSpace(leadID)
	memberSet := map[string]struct{}{}
	for _, m := range members {
		memberSet[m] = struct{}{}
	}
	if leadID != "" {
		if _, ok := memberSet[leadID]; ok {
			if _, did := ran[leadID]; !did {
				leadFallback[leadID] = true
				return []string{leadID}, leadFallback, "lead"
			}
		}
	}
	var rest []string
	for _, m := range members {
		if _, did := ran[m]; !did {
			rest = append(rest, m)
		}
	}
	if len(rest) == 0 {
		return nil, leadFallback, "all"
	}
	for _, id := range rest {
		leadFallback[id] = true
	}
	return rest, leadFallback, "all"
}

// displayNamesForAgents returns member display names (no agent_id) for debug UI.
func displayNamesForAgents(ids []string, agents []*db.Agent) []string {
	byID := map[string]*db.Agent{}
	for _, a := range agents {
		if a != nil {
			byID[a.ID] = a
		}
	}
	var names []string
	for _, id := range ids {
		name := id
		if a := byID[id]; a != nil {
			if n := strings.TrimSpace(a.Name); n != "" {
				name = n
			}
		}
		names = append(names, name)
	}
	return names
}

// orgAutoPickMode reads feature_flags_json.auto_pick as off|heuristic|decision.
// Missing key → heuristic. Legacy bool true→heuristic, false→off.
func orgAutoPickMode(flagsJSON string) string {
	flagsJSON = strings.TrimSpace(flagsJSON)
	if flagsJSON == "" || flagsJSON == "{}" {
		return db.AutoPickHeuristic
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(flagsJSON), &m); err != nil {
		return db.AutoPickHeuristic
	}
	v, ok := m["auto_pick"]
	if !ok {
		return db.AutoPickHeuristic
	}
	mode, err := db.ParseAutoPickMode(v)
	if err != nil || mode == "" {
		return db.AutoPickHeuristic
	}
	return mode
}

// orgAutoPickEnabled is true when org mode is not off (legacy helper).
func orgAutoPickEnabled(flagsJSON string) bool {
	return orgAutoPickMode(flagsJSON) != db.AutoPickOff
}

// isAllHandsIntent detects no-@ phrasing that asks every member to speak
// (self-intro / round-robin), e.g. 「都介绍下自己」「大家介绍一下」「everyone introduce」.
// MVP: heuristic substring / collective+action match — enough to avoid decision/heuristic
// collapsing to K=1 when the user clearly wants all bots.
func isAllHandsIntent(userText string) bool {
	text := strings.ToLower(strings.TrimSpace(userText))
	if text == "" {
		return false
	}
	for _, p := range allHandsFixedPhrases {
		if p != "" && strings.Contains(text, strings.ToLower(p)) {
			return true
		}
	}
	hasCollective := false
	for _, c := range allHandsCollectiveMarkers {
		if c != "" && strings.Contains(text, strings.ToLower(c)) {
			hasCollective = true
			break
		}
	}
	if !hasCollective {
		return false
	}
	for _, a := range allHandsActionMarkers {
		if a != "" && strings.Contains(text, strings.ToLower(a)) {
			return true
		}
	}
	return false
}

// Fixed multi-char phrases (ZH/EN). Checked first for precision.
var allHandsFixedPhrases = []string{
	"都介绍", "大家都介绍", "各位介绍", "各自介绍",
	"都自我介绍", "大家自我介绍", "各位自我介绍", "各自自我介绍",
	"都说说", "大家都说说", "各位说说", "各自说说",
	"都说一下", "大家都说一下", "各位说一下",
	"都讲讲", "大家讲讲", "各位讲讲",
	"都讲一下", "大家讲一下",
	"每人介绍", "每位介绍", "每个人介绍", "每一位介绍",
	"逐个介绍", "轮流介绍", "全员介绍", "所有人介绍",
	"都来介绍", "一起来介绍", "都来自我介绍",
	"轮流发言", "每人发言", "各自发言",
	"introduce yourselves", "everyone introduce", "everybody introduce",
	"all introduce", "all of you introduce", "each of you introduce",
	"everyone introduce yourselves", "introduce yourself everyone",
}

// Collective markers that must co-occur with an action marker.
var allHandsCollectiveMarkers = []string{
	"都", "大家", "各位", "各自", "每人", "每位", "每个人", "每一位",
	"所有人", "全员", "轮流", "逐个",
	"everyone", "everybody", "all of you", "each of you",
}

// Speak / intro actions paired with collective markers.
var allHandsActionMarkers = []string{
	"介绍", "自我介绍", "说说", "说一下", "讲讲", "讲一下", "发言",
	"introduce", "say hi", "say hello",
}

// autoPickAllCandidates returns every member as a non-forced candidate (same as auto_pick=off).
// Each may PASS; Method is mode-prefixed for logs (heuristic->all_hands / decision->all_hands).
func autoPickAllCandidates(method string, members []string) groupAutoPickResult {
	if strings.TrimSpace(method) == "" {
		method = "all_hands"
	}
	out := copyStringSlice(members)
	log.Printf("auto_pick=%s ids=%v conf=1.00 fallback=all", method, out)
	return groupAutoPickResult{
		AgentIDs:            out,
		Confidence:          1.0,
		Method:              method,
		Fallback:            "all",
		LeadFallback:        map[string]bool{},
		AllowSecondFailOpen: false,
	}
}

// runAutoPickByMode applies org auto_pick mode for a no-@ turn.
// Heuristic all-hands phrasing (「都介绍下自己」…) short-circuits to all members as
// candidates (non-forced, may PASS) — same candidacy as off.
// Decision always calls SystemOne (no phrase skip): schema includes opaque "all" + mN;
// all-hands raises effective K so the model can return multiple / all agent_ids.
// On parse/timeout/empty/low-conf/off fail-open to heuristic (or all-hands if phrase).
func runAutoPickByMode(mode, userText string, members []string, agents []*db.Agent, leadID string, k int, synopsis string, deps *autoPickDecisionDeps) groupAutoPickResult {
	mode = db.NormalizeAutoPickMode(mode)
	allHands := isAllHandsIntent(userText)
	if allHands && mode != db.AutoPickDecision {
		return autoPickAllCandidates("heuristic->all_hands", members)
	}
	switch mode {
	case db.AutoPickDecision:
		if pick, ok := autoPickDecision(context.Background(), deps, userText, members, agents, leadID, k, synopsis); ok {
			return pick
		}
		// Fail-open: all-hands phrase → all candidates (decision path already tried);
		// else heuristic (docs §6), Method decision->heuristic — not lead-only.
		if allHands {
			return autoPickAllCandidates("decision->heuristic->all_hands", members)
		}
		return autoPickHeuristicAs("decision->heuristic", userText, members, agents, leadID, k, synopsis)
	default: // heuristic
		return autoPickHeuristic(userText, members, agents, leadID, k, synopsis)
	}
}

// recentMessagesSynopsis builds a short optional synopsis from the last 1–2 messages.
func recentMessagesSynopsis(msgs []db.Message, maxMsgs int) string {
	if maxMsgs <= 0 {
		maxMsgs = 2
	}
	if len(msgs) == 0 {
		return ""
	}
	start := len(msgs) - maxMsgs
	if start < 0 {
		start = 0
	}
	var parts []string
	for _, m := range msgs[start:] {
		c := strings.TrimSpace(m.Content)
		if c == "" || isGroupPassReply(c) {
			continue
		}
		runes := []rune(c)
		if len(runes) > 80 {
			c = string(runes[:80]) + "…"
		}
		parts = append(parts, c)
	}
	return strings.Join(parts, " | ")
}
