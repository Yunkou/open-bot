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
// On any soft failure the caller should fail-open to heuristic (not lead-only).
func autoPickDecision(ctx context.Context, deps *autoPickDecisionDeps, userText string, members []string, agents []*db.Agent, leadID string, k int, synopsis string) (groupAutoPickResult, bool) {
	method := "decision"
	allHands := isAllHandsIntent(userText)
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

	keyToID, questions, state := buildAutoPickDecisionPayload(userText, members, agents, synopsis, allHands)
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
	// Explicit count (even 1) wins; only infer multi from probs when count was omitted.
	allowMultiFromProbs := countHint == 0
	ids, conf, allSelected, ok := mapDecisionChoiceToAgentIDs(answers, keyToID, members, effectiveK, allowMultiFromProbs)
	if !ok || len(ids) == 0 {
		log.Printf("auto_pick=decision note=parse_or_empty_failopen_heuristic")
		return groupAutoPickResult{}, false
	}
	if conf < autoPickConfidenceFloor {
		log.Printf("auto_pick=decision ids=%v conf=%.2f note=low_conf_failopen_heuristic", ids, conf)
		return groupAutoPickResult{}, false
	}

	fallback := ""
	allowSecond := true
	if allSelected {
		fallback = "all"
		allowSecond = false
		log.Printf("auto_pick=%s ids=%v conf=%.2f fallback=all", method, ids, conf)
	} else {
		log.Printf("auto_pick=%s ids=%v conf=%.2f fallback=", method, ids, conf)
	}
	return groupAutoPickResult{
		AgentIDs:            ids,
		Confidence:          conf,
		Method:              method,
		Fallback:            fallback,
		LeadFallback:        map[string]bool{},
		AllowSecondFailOpen: allowSecond,
	}, true
}

func buildAutoPickDecisionPayload(userText string, members []string, agents []*db.Agent, synopsis string, allHands bool) (keyToID map[string]string, questions map[string]any, state string) {
	byID := map[string]*db.Agent{}
	for _, a := range agents {
		if a != nil {
			byID[a.ID] = a
		}
	}
	keyToID = map[string]string{}
	criteria := map[string]string{}
	// Opaque "all" first so models can select every member (intro / round-robin).
	criteria[autoPickDecisionAllKey] = "Everyone — all members should reply (self-intro, round-robin, or explicit all-hands)"
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
		instr += " The user message looks like an all-hands / everyone-speak request: prefer choice \"all\" " +
			"unless a single specialist is clearly intended."
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
	if syn := strings.TrimSpace(synopsis); syn != "" {
		b.WriteString("\n\nRecent context:\n")
		b.WriteString(syn)
	}
	return keyToID, questions, b.String()
}

// mapDecisionChoiceToAgentIDs reads answers.pick (choice) and optional probabilities for multi.
// Opaque "all" → every member in channel order. Only mN keys in keyToID are accepted otherwise.
// k is the already-clamped / raised effective K (may exceed autoPickMaxK for multi / all-hands).
// When soft K is 1 but several mN probs clear the threshold, still returns a multi subset
// (top-N by probability) without requiring choice=all — e.g. 2 of 3 specialists.
func mapDecisionChoiceToAgentIDs(answers map[string]any, keyToID map[string]string, memberOrder []string, k int, allowMultiFromProbs bool) (ids []string, conf float64, allSelected bool, ok bool) {
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
		if p < autoPickConfidenceFloor {
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

	// Multi from top probabilities without requiring "all": if count was omitted and
	// soft K is low but several specialists clear the threshold, raise want up to member cap.
	want := k
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
	if c, ok := optionalFloat(raw["confidence"]); ok {
		return c
	}
	if p, ok := probs[choiceKey]; ok && p > 0 {
		return p
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
