package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/tangxin/open-bot/services/api/internal/db"
)

const (
	autoPickDecisionTimeout  = 8 * time.Second
	autoPickDecisionQKey     = "pick"
	autoPickDecisionCountKey = "count" // noul: how many should reply (→ 1..members)
	autoPickDecisionAllKey   = "all"   // opaque criteria: every member should reply
	autoPickAllHandsMaxK     = 8       // multi/all-hands ceiling; soft specialty K stays ≤ autoPickMaxK
)

// autoPickDecisionDeps wires org SystemOne settings + an injectable decide call.
// Nil Decide or provider off → caller fail-opens to heuristic.
type autoPickDecisionDeps struct {
	Provider string
	BaseURL  string
	APIKey   string
	Model    string
	// Decide posts state+questions and returns the answers map (DecisionResult.answers).
	// Tests inject a mock; production uses Server.callRuntimeDecision.
	Decide func(ctx context.Context, state any, questions map[string]any) (map[string]any, error)
}

// decisionMemberCap is the max agents decision mapping may return (multi / all-hands).
func decisionMemberCap(memberCount int) int {
	capN := autoPickAllHandsMaxK
	if memberCount > 0 && memberCount < capN {
		capN = memberCount
	}
	if capN < 1 {
		return 1
	}
	return capN
}

// decisionEffectiveK keeps auto_pick_k as the soft specialty default (≤ autoPickMaxK).
// countHint (from noul, 1..members) or all-hands may raise K up to min(members, autoPickAllHandsMaxK).
func decisionEffectiveK(k, memberCount int, allHands bool, countHint int) int {
	base := clampAutoPickK(k)
	capN := decisionMemberCap(memberCount)
	want := base
	if countHint > 0 {
		want = countHint
	}
	if allHands {
		want = capN
	}
	if want < 1 {
		want = 1
	}
	if want > capN {
		want = capN
	}
	if want < base {
		return base
	}
	return want
}

// autoPickDecision asks org decision model (choice + optional count) for members.
// Opaque criteria keys (m0..) map back to agent_ids; "all" → every candidate.
// count noul → how many (1..members); specialty soft K; multi/all-hands may raise to members.
// Trusts model choice+count+probs (no keyword role-match remap; no all-hands underpick→roster expand).
// All-hands intent is prompt-driven (choice=all + full count); who-can stays a capable subset.
// On any soft failure the caller should fail-open to heuristic (not lead-only).
func autoPickDecision(ctx context.Context, deps *autoPickDecisionDeps, userText string, members []string, agents []*db.Agent, leadID string, k int, synopsis string) (groupAutoPickResult, bool) {
	method := "decision"
	allHands := isAllHandsIntent(userText)
	whoCan := isWhoCanIntent(userText)
	if deps == nil {
		log.Printf("auto_pick=decision note=deps_nil_failopen_heuristic")
		return groupAutoPickResult{}, false
	}
	provider := db.NormalizeDecisionProvider(deps.Provider)
	if provider == "off" {
		log.Printf("auto_pick=decision note=provider_off_failopen_heuristic")
		return groupAutoPickResult{}, false
	}
	if deps.Decide == nil {
		log.Printf("auto_pick=decision note=decide_nil_failopen_heuristic")
		return groupAutoPickResult{}, false
	}

	keyToID, questions, state := buildAutoPickDecisionPayload(userText, members, agents, synopsis, allHands, whoCan)
	if len(keyToID) == 0 {
		return groupAutoPickResult{}, false
	}

	if ctx == nil {
		ctx = context.Background()
	}
	dctx, cancel := context.WithTimeout(ctx, autoPickDecisionTimeout)
	defer cancel()

	answers, err := deps.Decide(dctx, state, questions)
	if err != nil {
		log.Printf("auto_pick=decision note=decide_err err=%v", err)
		return groupAutoPickResult{}, false
	}
	countHint := countHintFromAnswers(answers, len(members))
	effectiveK := decisionEffectiveK(k, len(members), allHands, countHint)
	// Who-can census: allow multi from mN probs even when count says 1 (model often undershoots).
	// Explicit count still raises K; who-can also lifts K to member cap so mapping can keep all capable.
	if whoCan {
		capN := decisionMemberCap(len(members))
		if effectiveK < capN {
			effectiveK = capN
		}
	}
	// Explicit count (even 1) wins for specialty; who-can / omitted count may infer multi from probs.
	allowMultiFromProbs := countHint == 0 || whoCan
	ids, conf, allSelected, ok := mapDecisionChoiceToAgentIDs(answers, keyToID, members, effectiveK, allowMultiFromProbs, whoCan, countHint)
	choiceKey, probs := decisionPickMeta(answers)
	// Trust model choice+count+probs (no keyword / role-match remap). Who-can structural
	// safety below still forbids PASS→second-wave dump of incapable peers.
	if !ok || len(ids) == 0 {
		log.Printf("auto_pick=decision note=parse_or_empty_failopen_heuristic choice=%s count=%d probs=%v", choiceKey, countHint, probs)
		return groupAutoPickResult{}, false
	}
	if conf < autoPickConfidenceFloor {
		log.Printf("auto_pick=decision ids=%v conf=%.2f choice=%s count=%d probs=%v note=low_conf_failopen_heuristic", ids, conf, choiceKey, countHint, probs)
		return groupAutoPickResult{}, false
	}

	fallback := ""
	allowSecond := true
	leadFB := map[string]bool{}
	if allSelected {
		fallback = "all"
		allowSecond = false
		log.Printf("auto_pick=%s ids=%v conf=%.2f choice=%s count=%d probs=%v fallback=all", method, ids, conf, choiceKey, countHint, probs)
	} else {
		// Who-can census: capable picks must answer (leadFallback prompt) and must not
		// second-fail-open into the remaining channel (incapable peers). Production
		// 11:41 CST 「谁会写代码」mapped to 33 only, then 33 [PASS] → 22+44 spoke.
		if whoCan {
			allowSecond = false
			for _, id := range ids {
				leadFB[id] = true
			}
		}
		log.Printf("auto_pick=%s ids=%v conf=%.2f choice=%s count=%d probs=%v fallback=", method, ids, conf, choiceKey, countHint, probs)
	}
	return groupAutoPickResult{
		AgentIDs:            ids,
		Confidence:          conf,
		Method:              method,
		Fallback:            fallback,
		LeadFallback:        leadFB,
		AllowSecondFailOpen: allowSecond,
	}, true
}

func buildAutoPickDecisionPayload(userText string, members []string, agents []*db.Agent, synopsis string, allHands bool, whoCan bool) (keyToID map[string]string, questions map[string]any, state string) {
	byID := map[string]*db.Agent{}
	for _, a := range agents {
		if a != nil {
			byID[a.ID] = a
		}
	}
	keyToID = map[string]string{}
	criteria := map[string]string{}
	// Opaque "all" first so models can select every member (intro / round-robin).
	// Who-can: keep the key but discourage full-roster parking (Langfuse 21ed840c 「谁会写代码」).
	if whoCan {
		criteria[autoPickDecisionAllKey] = "ONLY if EVERY member's role can do X — rare for specialty skills (写代码/coding). Prefer capable mN keys; never use all to mean the whole channel when only a subset fits"
	} else {
		criteria[autoPickDecisionAllKey] = "Everyone — all members should reply (self-intro, round-robin, or explicit all-hands)"
	}
	for i, id := range members {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		key := fmt.Sprintf("m%d", i)
		name := id
		role := ""
		if a := byID[id]; a != nil {
			if n := strings.TrimSpace(a.Name); n != "" {
				name = n
			}
			role = strings.TrimSpace(groupParticipationRoleHint(a))
		}
		label := name
		if role != "" {
			label = name + " — " + role
		}
		keyToID[key] = id
		criteria[key] = label
	}
	if len(keyToID) == 0 {
		return nil, nil, ""
	}
	instr := "Who should reply this turn? Pick one opaque key as the primary: a single member (m0, m1, …) by specialty, " +
		"OR \"all\" when every member should speak (introductions, round-robin, everyone contribute). " +
		"You may pick one specialist, several complementary specialists when the ask needs multiple skills, or all. " +
		"Prefer a specialist when the request matches their role; prefer \"all\" for collective asks; " +
		"prefer the generalist only for vague single-responder requests. " +
		"When several specialists fit, pick the best primary — also set count and probabilities so extras can be included (multi subset, not only all-or-one)."
	if allHands {
		instr += " ALL-HANDS / EVERYONE-SPEAK (intro-everyone): the user asks every member to introduce or speak " +
			"(e.g. 「都介绍下自己」「大家都介绍一下」「introduce yourselves」). " +
			"You MUST set choice=\"all\" — never a single mN as primary. " +
			"Set count to the full roster (noul near 1). Put HIGH probability mass on \"all\" " +
			"and keep every mN probability high/even; never concentrate mass on one mN. " +
			"Do not pick a specialist primary for collective intro / round-robin asks."
	}
	if whoCan {
		instr += " CAPABILITY CENSUS (who-can): the user asks who can do X (e.g. 「谁会写代码」「谁会查资料」「who can code」). " +
			"Match X ONLY to the role labels in criteria (m0, m1, …) — not to Recent-context chat intros. " +
			"Do NOT choose \"all\" unless EVERY member's role can do X. " +
			"You MUST choose a capable mN as choice — never park on \"all\" for a specialty census. " +
			"Set count to the number of members whose roles share that capability (1 if only one fits; 2 if two researchers fit; etc.). " +
			"Put HIGH probabilities on EVERY capable mN and NEAR-ZERO on incapable ones and on \"all\". " +
			"Hard examples: 「谁会写代码」/ write/code/编程 → ONLY the coding-role mN; choice=that mN, noul near 0 (→1), high mass on coder, low on research and all. " +
			"「谁会查资料」/ research/搜索 → every research/资料-role mN; choice=one researcher, count=how many researchers, high probs on all researchers, low on coder and all. " +
			"Never pick \"all\" or pad count/noul to the full roster when only a subset is capable. " +
			"Never let a peer's self-intro in Recent context override the criteria role labels."
	}
	if !allHands && !whoCan {
		instr += " SPECIALTY TASK (do-the-job, not who-can / not intro): when the user asks bots to perform work " +
			"(e.g. 「帮我查下…」「查一下…发展/资料」「搜索/调研/research/look up»; or 「修 bug」「写代码」/code), " +
			"match the task to criteria role labels — never park on \"all\" and never pick an unrelated specialty. " +
			"Research/lookup asks → every research/资料/搜索-role mN; choice=one researcher as primary; " +
			"count=how many researchers; HIGH probs on all researchers; NEAR-ZERO on coding-role and on \"all\". " +
			"Coding/debug asks → ONLY the coding-role mN; count near 1 (noul≈0); high mass on coder; low on research and all. " +
			"Hard example: 「帮我查下2026年优锘科技的发展」/ look up a company → research mNs only (e.g. 22+44), not the coder, not all. " +
			"Reserve choice=\"all\" for introductions / round-robin / explicit everyone — not for a concrete specialty task. " +
			"Recent-context peer intros do not override criteria roles."
	}

	memberN := len(keyToID)
	frac2 := 0.0
	if memberN > 1 {
		frac2 = 1.0 / float64(memberN-1) // noul for exactly 2 of N
	}
	countInstr := fmt.Sprintf(
		"How many members should reply this turn? Output noul in [0,1]: near 0 → one specialist; "+
			"around 0.5 → several complementary specialists (multi subset, not necessarily everyone); "+
			"near 1 → all %d members. Example: for exactly 2 of %d, use about %.2f.",
		memberN, memberN, frac2,
	)
	if allHands {
		countInstr = fmt.Sprintf(
			"ALL-HANDS count: every member should reply (intro-everyone / round-robin). "+
				"Output noul near 1 → all %d members. Do not use near 0 (single specialist) "+
				"for 「都介绍下自己」「大家都介绍一下」 / introduce-yourselves asks.",
			memberN,
		)
	}
	if whoCan {
		countInstr = fmt.Sprintf(
			"CAPABILITY CENSUS count: how many members' roles can do what the user asked? "+
				"Output noul in [0,1] mapped as 1 + round(noul*(%d-1)) → member count. "+
				"「谁会写代码」/ only the coder → near 0 (→1). "+
				"「谁会查资料」/ two researchers → about %.2f (→2). "+
				"Use near 1 ONLY when every member's role is capable. "+
				"FORBIDDEN: noul near 1 / full roster when the ask is a specialty census (写代码, 查资料 subset).",
			memberN, frac2,
		)
	}
	if !allHands && !whoCan {
		countInstr = fmt.Sprintf(
			"SPECIALTY TASK count: how many members' roles match the job? "+
				"Output noul in [0,1] mapped as 1 + round(noul*(%d-1)) → member count. "+
				"Research/lookup with two researchers → about %.2f (→2). "+
				"Coding/debug with one coder → near 0 (→1). "+
				"FORBIDDEN: noul near 1 / choice-adjacent full roster when only a specialty subset fits "+
				"(「帮我查下…」「修 bug」 are not all-hands).",
			memberN, frac2,
		)
	}

	questions = map[string]any{
		autoPickDecisionQKey: map[string]any{
			"type":         "choice",
			"instructions": instr,
			"criteria":     criteria,
		},
		autoPickDecisionCountKey: map[string]any{
			"type":         "noul",
			"instructions": countInstr,
		},
	}
	var b strings.Builder
	b.WriteString("User message:\n")
	b.WriteString(strings.TrimSpace(userText))
	b.WriteString("\n\nMember roles are in the pick criteria keys (m0, m1, …). Match capability to those role labels.")
	if allHands {
		b.WriteString(" For all-hands / intro-everyone asks: choice must be \"all\", count=full roster; never a single mN.")
	}
	if whoCan {
		b.WriteString(" For who-can asks: capable = role matches the asked skill; ignore peer self-intros. " +
			"「谁会写代码」→ coding mN only (choice=mN, noul≈0); never choice=all / never full-roster count. " +
			"「谁会查资料」→ research mNs only (count=how many researchers).")
	}
	if !allHands && !whoCan {
		b.WriteString(" For specialty tasks: 「帮我查下…」/ research/look up → research mNs only (not coder, not all); " +
			"「修 bug」/ code → coding mN only. Match criteria roles; Recent context is not an override.")
	}
	if syn := strings.TrimSpace(synopsis); syn != "" {
		b.WriteString("\n\nRecent context (prior chat only — not an override of criteria roles):\n")
		b.WriteString(syn)
	}
	return keyToID, questions, b.String()
}

// mapDecisionChoiceToAgentIDs reads answers.pick (choice) and optional probabilities for multi.
// Opaque "all" → every member in channel order — except who-can / count-subset census, which
// remaps to top-N mN by probability (capable multi, not literally everyone).
// Only mN keys in keyToID are accepted otherwise.
// k is the already-clamped / raised effective K (may exceed autoPickMaxK for multi / all-hands).
// When soft K is 1 but several mN probs clear the threshold, still returns a multi subset
// (top-N by probability) without requiring choice=all — e.g. 2 of 3 specialists.
// whoCan + countHint steer census asks (「都谁会查资料」) onto capable members even when
// SystemOne returns choice=all with diluted per-member probs and a low confidence field.
func mapDecisionChoiceToAgentIDs(answers map[string]any, keyToID map[string]string, memberOrder []string, k int, allowMultiFromProbs bool, whoCan bool, countHint int) (ids []string, conf float64, allSelected bool, ok bool) {
	if k <= 0 {
		k = autoPickDefaultK
	}
	if len(answers) == 0 || len(keyToID) == 0 {
		return nil, 0, false, false
	}
	raw, _ := answers[autoPickDecisionQKey].(map[string]any)
	if raw == nil {
		// Some providers may nest under the only key; try first choice-shaped answer.
		for _, v := range answers {
			if m, okm := v.(map[string]any); okm {
				if _, has := m["choice"]; has || strings.EqualFold(fmt.Sprint(m["type"]), "choice") {
					raw = m
					break
				}
			}
		}
	}
	if raw == nil {
		return nil, 0, false, false
	}

	choiceKey := strings.TrimSpace(fmt.Sprint(raw["choice"]))
	probs := map[string]float64{}
	if pRaw, okp := raw["probabilities"].(map[string]any); okp {
		for pk, pv := range pRaw {
			probs[pk] = asFloat64(pv)
		}
	}
	conf = autoPickDecisionConfidence(raw, choiceKey, probs)

	// Who-can / multi-subset: choice=all with count < members means "all who can", not every seat.
	// Remap via mN probabilities + count so both capable specialists run (e.g. 22+44 research).
	remapAllToCapable := strings.EqualFold(choiceKey, autoPickDecisionAllKey) && whoCan
	if strings.EqualFold(choiceKey, autoPickDecisionAllKey) && !remapAllToCapable {
		// True all-hands (or model insists on everyone without census intent).
		if countHint > 0 && countHint < len(keyToID) && allowMultiFromProbs {
			// Soft: count says subset — prefer mN multi over literal everyone.
			remapAllToCapable = true
		}
	}
	if remapAllToCapable {
		mapCount := countHint
		// Inflated full-roster count on who-can (Langfuse 21ed840c / 12:18 CST 「谁会写代码」):
		// SystemOne returned choice=all + noul≈0.91 → count=N; trusting count expanded everyone.
		// Already-approved who-can remap: never literal full roster — infer from relative mN probs.
		if whoCan && countHint >= len(keyToID) && len(keyToID) > 0 {
			mapCount = 0
		}
		ids, conf2 := mapCapableMultiFromProbs(probs, keyToID, memberOrder, k, mapCount)
		if len(ids) == 0 {
			return nil, 0, false, false
		}
		if conf2 > conf {
			conf = conf2
		}
		// Valid census multi: don't discard on diluted per-member probs / low confidence field.
		if conf < autoPickConfidenceFloor && len(ids) >= 1 {
			conf = autoPickDecisionConfidence(raw, choiceKey, probs)
			if conf < autoPickConfidenceFloor {
				if pAll := probs[autoPickDecisionAllKey]; pAll > conf {
					conf = pAll
				}
				if conf < autoPickConfidenceFloor {
					conf = 0.55
				}
			}
		}
		return ids, conf, false, true
	}

	if strings.EqualFold(choiceKey, autoPickDecisionAllKey) {
		ids = orderedDecisionMemberIDs(memberOrder, keyToID)
		if len(ids) == 0 {
			return nil, 0, false, false
		}
		return ids, conf, true, true
	}

	primaryID, havePrimary := keyToID[choiceKey]
	if !havePrimary {
		return nil, 0, false, false
	}

	ids = []string{primaryID}
	seen := map[string]struct{}{primaryID: {}}

	type ranked struct {
		key  string
		prob float64
	}
	var extra []ranked
	for key, p := range probs {
		if strings.EqualFold(key, autoPickDecisionAllKey) {
			continue
		}
		id, okm := keyToID[key]
		if !okm {
			continue
		}
		if _, did := seen[id]; did {
			continue
		}
		// Who-can: accept relative matches (mass often sits on "all"); specialty keeps absolute floor.
		if whoCan {
			if p <= 0 {
				continue
			}
		} else if p < autoPickConfidenceFloor {
			continue
		}
		extra = append(extra, ranked{key: key, prob: p})
	}
	sort.SliceStable(extra, func(i, j int) bool {
		if extra[i].prob != extra[j].prob {
			return extra[i].prob > extra[j].prob
		}
		return extra[i].key < extra[j].key
	})
	if whoCan && len(extra) > 0 {
		maxP := probs[choiceKey]
		for _, e := range extra {
			if e.prob > maxP {
				maxP = e.prob
			}
		}
		if maxP <= 0 {
			maxP = extra[0].prob
		}
		relFloor := maxP * 0.4
		filtered := extra[:0]
		for _, e := range extra {
			if e.prob >= relFloor {
				filtered = append(filtered, e)
			}
		}
		extra = filtered
	}

	// Multi from top probabilities without requiring "all": if count was omitted / who-can
	// and soft K is low but several specialists clear the threshold, raise want up to member cap.
	want := k
	if countHint > 0 && countHint < want {
		// Explicit smaller count still respected for specialty; who-can may keep raised want.
		if !whoCan {
			want = countHint
		}
	}
	if countHint > want {
		want = countHint
		capN := decisionMemberCap(len(keyToID))
		if want > capN {
			want = capN
		}
	}
	if allowMultiFromProbs && len(extra) > 0 {
		multiFromProbs := 1 + len(extra) // primary + extras above floor
		capN := decisionMemberCap(len(keyToID))
		if multiFromProbs > want && want < capN {
			want = multiFromProbs
			if want > capN {
				want = capN
			}
		}
	}
	for _, e := range extra {
		if len(ids) >= want {
			break
		}
		id := keyToID[e.key]
		ids = append(ids, id)
		seen[id] = struct{}{}
	}
	return ids, conf, false, true
}

// mapCapableMultiFromProbs picks top-N opaque mN members by probability for who-can / subset-all.
// When countHint >= 2, takes that many; countHint 0/<2 keeps members within 40% of the best mN prob.
// Callers should pass countHint=0 when who-can choice=all claimed a full-roster count (inflated).
func mapCapableMultiFromProbs(probs map[string]float64, keyToID map[string]string, memberOrder []string, k, countHint int) ([]string, float64) {
	type ranked struct {
		key  string
		id   string
		prob float64
	}
	var list []ranked
	maxP := 0.0
	for key, id := range keyToID {
		p := probs[key]
		list = append(list, ranked{key: key, id: id, prob: p})
		if p > maxP {
			maxP = p
		}
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].prob != list[j].prob {
			return list[i].prob > list[j].prob
		}
		return list[i].key < list[j].key
	})
	want := k
	if countHint > 0 {
		want = countHint
	}
	capN := decisionMemberCap(len(keyToID))
	if want < 1 {
		want = 1
	}
	if want > capN {
		want = capN
	}
	if countHint < 2 && maxP > 0 {
		// Infer capable set by relative probability (SystemOne often parks mass on "all").
		relFloor := maxP * 0.4
		n := 0
		for _, e := range list {
			if e.prob >= relFloor {
				n++
			}
		}
		if n > want {
			want = n
		}
		if want > capN {
			want = capN
		}
	}
	ids := make([]string, 0, want)
	seen := map[string]struct{}{}
	var sum float64
	for _, e := range list {
		if len(ids) >= want {
			break
		}
		if _, did := seen[e.id]; did {
			continue
		}
		if countHint < 2 && maxP > 0 && e.prob < maxP*0.4 && len(ids) >= 1 {
			break
		}
		seen[e.id] = struct{}{}
		ids = append(ids, e.id)
		sum += e.prob
	}
	// Prefer channel order for stability when reporting multi census.
	if len(ids) > 1 && len(memberOrder) > 0 {
		ids = orderIDsByMembers(ids, memberOrder)
	}
	conf := maxP
	if sum > conf {
		conf = sum
		if conf > 1 {
			conf = 1
		}
	}
	if pAll := probs[autoPickDecisionAllKey]; pAll > conf {
		conf = pAll
	}
	return ids, conf
}

func orderIDsByMembers(ids []string, memberOrder []string) []string {
	want := map[string]struct{}{}
	for _, id := range ids {
		want[id] = struct{}{}
	}
	var out []string
	for _, id := range memberOrder {
		if _, ok := want[id]; ok {
			out = append(out, id)
			delete(want, id)
		}
	}
	for _, id := range ids {
		if _, ok := want[id]; ok {
			out = append(out, id)
		}
	}
	return out
}

// decisionPickMeta extracts choice + probabilities for auto_pick logs.
func decisionPickMeta(answers map[string]any) (choice string, probs map[string]float64) {
	probs = map[string]float64{}
	if len(answers) == 0 {
		return "", probs
	}
	raw, _ := answers[autoPickDecisionQKey].(map[string]any)
	if raw == nil {
		return "", probs
	}
	choice = strings.TrimSpace(fmt.Sprint(raw["choice"]))
	if pRaw, okp := raw["probabilities"].(map[string]any); okp {
		for pk, pv := range pRaw {
			probs[pk] = asFloat64(pv)
		}
	}
	return choice, probs
}

// countHintFromAnswers maps answers.count noul ∈ [0,1] → how many members (1..memberCount).
// 0 unset / missing → 0 (caller keeps soft K). Mapping: 1 + round(noul*(N-1)).
func countHintFromAnswers(answers map[string]any, memberCount int) int {
	if memberCount <= 0 || len(answers) == 0 {
		return 0
	}
	raw, _ := answers[autoPickDecisionCountKey].(map[string]any)
	if raw == nil {
		return 0
	}
	noul, ok := optionalFloat(raw["noul"])
	if !ok {
		return 0
	}
	if noul < 0 {
		noul = 0
	}
	if noul > 1 {
		noul = 1
	}
	if memberCount == 1 {
		return 1
	}
	n := 1 + int(math.Round(noul*float64(memberCount-1)))
	if n < 1 {
		n = 1
	}
	if n > memberCount {
		n = memberCount
	}
	return n
}

// orderedDecisionMemberIDs expands keyToID into channel member order (stable, no dups).
func orderedDecisionMemberIDs(memberOrder []string, keyToID map[string]string) []string {
	want := map[string]struct{}{}
	for _, id := range keyToID {
		want[id] = struct{}{}
	}
	var out []string
	seen := map[string]struct{}{}
	for _, id := range memberOrder {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := want[id]; !ok {
			continue
		}
		if _, did := seen[id]; did {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) == len(want) {
		return out
	}
	// Fallback: any remaining keys sorted by opaque mN order.
	type pair struct {
		key string
		id  string
	}
	var rest []pair
	for key, id := range keyToID {
		if _, did := seen[id]; did {
			continue
		}
		rest = append(rest, pair{key: key, id: id})
	}
	sort.SliceStable(rest, func(i, j int) bool { return rest[i].key < rest[j].key })
	for _, p := range rest {
		out = append(out, p.id)
	}
	return out
}

func autoPickDecisionConfidence(raw map[string]any, choiceKey string, probs map[string]float64) float64 {
	conf := 0.0
	have := false
	if c, ok := optionalFloat(raw["confidence"]); ok {
		conf = c
		have = true
	}
	if p, ok := probs[choiceKey]; ok && p > 0 {
		if !have || p > conf {
			conf = p
		}
		have = true
	}
	if have {
		return conf
	}
	// Valid mapped choice without explicit conf → treat as moderate (above floor).
	return 0.55
}

func asFloat64(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case json.Number:
		f, _ := t.Float64()
		return f
	case string:
		var f float64
		fmt.Sscanf(strings.TrimSpace(t), "%f", &f)
		return f
	default:
		return 0
	}
}

func optionalFloat(v any) (float64, bool) {
	if v == nil {
		return 0, false
	}
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case json.Number:
		f, err := t.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

// callRuntimeDecision invokes agent-runtime /v1/decision/decide (factory → SystemOne).
func (s *Server) callRuntimeDecision(ctx context.Context, provider, baseURL, apiKey, model string, state any, questions map[string]any) (map[string]any, error) {
	if s == nil || strings.TrimSpace(s.runtimeURL) == "" {
		return nil, fmt.Errorf("runtime unavailable")
	}
	provider, baseURL, model = db.ApplyDecisionDefaults(provider, baseURL, model)
	payload, err := json.Marshal(map[string]any{
		"provider":  provider,
		"base_url":  baseURL,
		"api_key":   apiKey,
		"model":     model,
		"state":     state,
		"questions": questions,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.runtimeURL+"/v1/decision/decide", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := s.client
	if client == nil {
		client = &http.Client{Timeout: autoPickDecisionTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(raw))
		var parsed map[string]any
		if json.Unmarshal(raw, &parsed) == nil {
			if d, ok := parsed["detail"].(string); ok && d != "" {
				msg = d
			} else if e, ok := parsed["error"].(string); ok && e != "" {
				msg = e
			}
		}
		if msg == "" {
			msg = fmt.Sprintf("decision status %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("%s", msg)
	}
	var out struct {
		Enabled bool           `json:"enabled"`
		Answers map[string]any `json:"answers"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("decision response parse: %w", err)
	}
	if !out.Enabled || len(out.Answers) == 0 {
		return nil, fmt.Errorf("decision disabled or empty answers")
	}
	return out.Answers, nil
}

// decisionDepsForUser loads org decision settings and binds runtime decide.
// Returns nil when org decision is off (caller fail-opens to heuristic).
func (s *Server) decisionDepsForUser(uid string) *autoPickDecisionDeps {
	if s == nil || s.db == nil {
		return nil
	}
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return nil
	}
	u, err := s.db.GetUserByID(uid)
	if err != nil || u == nil || strings.TrimSpace(u.OrgID) == "" {
		return nil
	}
	settings, err := s.db.GetOrgSettings(u.OrgID)
	if err != nil || settings == nil {
		return nil
	}
	provider := db.NormalizeDecisionProvider(settings.DecisionProvider)
	if provider == "off" {
		return &autoPickDecisionDeps{Provider: "off"}
	}
	deps := &autoPickDecisionDeps{
		Provider: provider,
		BaseURL:  settings.DecisionBaseURL,
		APIKey:   settings.DecisionAPIKey,
		Model:    settings.DecisionModel,
	}
	deps.Decide = func(ctx context.Context, state any, questions map[string]any) (map[string]any, error) {
		return s.callRuntimeDecision(ctx, deps.Provider, deps.BaseURL, deps.APIKey, deps.Model, state, questions)
	}
	return deps
}
