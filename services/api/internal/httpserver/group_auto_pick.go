package httpserver

import (
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

// groupHasExplicitAddressing is true when @ / @everyone / client agent_ids already chose targets.
// AutoPick must never replace those forced sets.
func groupHasExplicitAddressing(content string, explicit []string, members []string, agents []*db.Agent) bool {
	for _, id := range explicit {
		if strings.TrimSpace(id) != "" {
			return true
		}
	}
	if mentionIncludesEveryone(content) {
		return true
	}
	if len(resolveMentionedAgents(parseMentionTokens(content), members, agents)) > 0 {
		return true
	}
	return false
}

// autoPickHeuristic picks ≤K members by keyword / role-description match; else fail-open to lead.
// No extra LLM. leadID is conv.AgentID / group lead assistant.
func autoPickHeuristic(userText string, members []string, agents []*db.Agent, leadID string, k int, synopsis string) groupAutoPickResult {
	k = clampAutoPickK(k)
	leadID = strings.TrimSpace(leadID)
	method := "heuristic"

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

// runAutoPickByMode applies channel/org auto_pick mode for a no-@ turn.
// decision (B-tier) is not implemented yet: fail-open to heuristic (not lead-only) + log note.
func runAutoPickByMode(mode, userText string, members []string, agents []*db.Agent, leadID string, k int, synopsis string) groupAutoPickResult {
	mode = db.NormalizeAutoPickMode(mode)
	switch mode {
	case db.AutoPickDecision:
		log.Printf("auto_pick=decision note=unimplemented_failopen_heuristic")
		pick := autoPickHeuristic(userText, members, agents, leadID, k, synopsis)
		if pick.Method == "heuristic" {
			pick.Method = "decision->heuristic"
		}
		return pick
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
