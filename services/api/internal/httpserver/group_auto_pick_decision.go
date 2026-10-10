package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/tangxin/open-bot/services/api/internal/db"
)

const (
	autoPickDecisionTimeout = 8 * time.Second
	autoPickDecisionQKey    = "pick"
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

// autoPickDecision asks org decision model (choice question) for ≤K members.
// Opaque criteria keys (m0..) map back to agent_ids — never trust free-text ids/names.
// On any soft failure the caller should fail-open to heuristic (not lead-only).
func autoPickDecision(ctx context.Context, deps *autoPickDecisionDeps, userText string, members []string, agents []*db.Agent, leadID string, k int, synopsis string) (groupAutoPickResult, bool) {
	method := "decision"
	k = clampAutoPickK(k)
	if deps == nil || deps.Decide == nil {
		return groupAutoPickResult{}, false
	}
	provider := db.NormalizeDecisionProvider(deps.Provider)
	if provider == "off" {
		log.Printf("auto_pick=decision note=provider_off_failopen_heuristic")
		return groupAutoPickResult{}, false
	}

	keyToID, questions, state := buildAutoPickDecisionPayload(userText, members, agents, synopsis)
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
	ids, conf, ok := mapDecisionChoiceToAgentIDs(answers, keyToID, k)
	if !ok || len(ids) == 0 {
		log.Printf("auto_pick=decision note=parse_or_empty_failopen_heuristic")
		return groupAutoPickResult{}, false
	}
	if conf < autoPickConfidenceFloor {
		log.Printf("auto_pick=decision ids=%v conf=%.2f note=low_conf_failopen_heuristic", ids, conf)
		return groupAutoPickResult{}, false
	}

	log.Printf("auto_pick=%s ids=%v conf=%.2f fallback=", method, ids, conf)
	return groupAutoPickResult{
		AgentIDs:            ids,
		Confidence:          conf,
		Method:              method,
		Fallback:            "",
		LeadFallback:        map[string]bool{},
		AllowSecondFailOpen: true,
	}, true
}

func buildAutoPickDecisionPayload(userText string, members []string, agents []*db.Agent, synopsis string) (keyToID map[string]string, questions map[string]any, state string) {
	byID := map[string]*db.Agent{}
	for _, a := range agents {
		if a != nil {
			byID[a.ID] = a
		}
	}
	keyToID = map[string]string{}
	criteria := map[string]string{}
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
	if len(criteria) == 0 {
		return nil, nil, ""
	}
	questions = map[string]any{
		autoPickDecisionQKey: map[string]any{
			"type": "choice",
			"instructions": "Which group member should reply to the user this turn? " +
				"Pick the single best fit by specialty and role. " +
				"Prefer a specialist when the request matches their role; " +
				"prefer the generalist only for vague or cross-cutting requests.",
			"criteria": criteria,
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

// mapDecisionChoiceToAgentIDs reads answers.pick (choice) and optional probabilities for K>1.
// Only opaque criteria keys in keyToID are accepted.
func mapDecisionChoiceToAgentIDs(answers map[string]any, keyToID map[string]string, k int) (ids []string, conf float64, ok bool) {
	k = clampAutoPickK(k)
	if len(answers) == 0 || len(keyToID) == 0 {
		return nil, 0, false
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
		return nil, 0, false
	}

	choiceKey := strings.TrimSpace(fmt.Sprint(raw["choice"]))
	primaryID, havePrimary := keyToID[choiceKey]
	if !havePrimary {
		// Soft: allow accidental echo of display label only if unique exact criteria match already keyed.
		return nil, 0, false
	}

	probs := map[string]float64{}
	if pRaw, okp := raw["probabilities"].(map[string]any); okp {
		for pk, pv := range pRaw {
			probs[pk] = asFloat64(pv)
		}
	}
	conf = autoPickDecisionConfidence(raw, choiceKey, probs)
	ids = []string{primaryID}
	seen := map[string]struct{}{primaryID: {}}

	if k > 1 && len(probs) > 0 {
		type ranked struct {
			key  string
			prob float64
		}
		var extra []ranked
		for key, p := range probs {
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
		for _, e := range extra {
			if len(ids) >= k {
				break
			}
			id := keyToID[e.key]
			ids = append(ids, id)
			seen[id] = struct{}{}
		}
	}
	return ids, conf, true
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
