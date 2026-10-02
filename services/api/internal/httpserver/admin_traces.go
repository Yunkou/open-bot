package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type langfuseConfig struct {
	BaseURL    string
	PublicKey  string
	SecretKey  string
	PublicUI   string
	ProjectID  string
	Configured bool
	Reason     string
}

func loadLangfuseConfig() langfuseConfig {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("LANGFUSE_BASE_URL")), "/")
	pk := strings.TrimSpace(os.Getenv("LANGFUSE_PUBLIC_KEY"))
	sk := strings.TrimSpace(os.Getenv("LANGFUSE_SECRET_KEY"))
	ui := strings.TrimRight(strings.TrimSpace(os.Getenv("LANGFUSE_PUBLIC_UI_URL")), "/")
	if ui == "" {
		ui = base
	}
	projectID := strings.TrimSpace(os.Getenv("LANGFUSE_PROJECT_ID"))
	cfg := langfuseConfig{
		BaseURL:   base,
		PublicKey: pk,
		SecretKey: sk,
		PublicUI:  ui,
		ProjectID: projectID,
	}
	enabledEnv := strings.TrimSpace(os.Getenv("LANGFUSE_ENABLED"))
	explicitOff := enabledEnv == "0" || strings.EqualFold(enabledEnv, "false") || strings.EqualFold(enabledEnv, "off")
	if explicitOff {
		cfg.Reason = "LANGFUSE_ENABLED=0"
		return cfg
	}
	if base == "" || pk == "" || sk == "" {
		cfg.Reason = "未配置 LANGFUSE_BASE_URL / LANGFUSE_PUBLIC_KEY / LANGFUSE_SECRET_KEY；本地可 make compose-langfuse 并写入 .env"
		return cfg
	}
	cfg.Configured = true
	return cfg
}

func (s *Server) langfuseGET(path string, query url.Values) (int, []byte, error) {
	return s.langfuseRequest(context.Background(), http.MethodGet, path, query, nil)
}

// langfuseRequest calls Langfuse public API with Basic auth (pk/sk).
// body may be nil for GET/DELETE-without-body.
func (s *Server) langfuseRequest(ctx context.Context, method, path string, query url.Values, body []byte) (int, []byte, error) {
	cfg := loadLangfuseConfig()
	if !cfg.Configured {
		return 0, nil, fmt.Errorf("%s", cfg.Reason)
	}
	u := cfg.BaseURL + path
	if len(query) > 0 {
		u = u + "?" + query.Encode()
	}
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rdr)
	if err != nil {
		return 0, nil, err
	}
	req.SetBasicAuth(cfg.PublicKey, cfg.SecretKey)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 20 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	respBody, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return res.StatusCode, nil, err
	}
	return res.StatusCode, respBody, nil
}

const (
	langfusePurgePageLimit   = 100
	langfusePurgeMaxPages    = 50
	langfusePurgeDeleteBatch = 100 // Langfuse allows up to 1000; keep moderate batches
)

// purgeLangfuseUserTraces best-effort deletes Langfuse traces for userID.
//
// Langfuse v4 events_only: list via GET /api/public/v2/observations (userId
// filter, plus metadata.user_id fallback), then DELETE /api/public/traces
// {"traceIds":[…]}. Deletion is asynchronous on the Langfuse worker (often
// seconds, up to ~15m). Never fails the caller — returns a status string for
// side_effects.langfuse (like mem0).
func (s *Server) purgeLangfuseUserTraces(ctx context.Context, userID string) string {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return "skipped: empty user"
	}
	cfg := loadLangfuseConfig()
	if !cfg.Configured {
		reason := cfg.Reason
		if reason == "" {
			reason = "not configured"
		}
		return "skipped: " + reason
	}

	now := time.Now().UTC()
	from := now.Add(-10 * 365 * 24 * time.Hour) // ~10y window; purge should cover history
	to := now.Add(time.Minute)
	fromStr := from.Format(time.RFC3339)
	toStr := to.Format(time.RFC3339)

	seen := map[string]struct{}{}
	var allIDs []string

	collect := func(label string, q url.Values) string {
		cursor := ""
		for page := 0; page < langfusePurgeMaxPages; page++ {
			qq := cloneURLValues(q)
			qq.Set("limit", strconv.Itoa(langfusePurgePageLimit))
			qq.Set("fields", "core,basic,metadata")
			qq.Set("fromStartTime", fromStr)
			qq.Set("toStartTime", toStr)
			if cursor != "" {
				qq.Set("cursor", cursor)
			}
			code, body, err := s.langfuseRequest(ctx, http.MethodGet, "/api/public/v2/observations", qq, nil)
			if err != nil {
				return fmt.Sprintf("%s_list_err=%s", label, err.Error())
			}
			if code < 200 || code >= 300 {
				return fmt.Sprintf("%s_list_http=%d %s", label, code, truncateStr(string(body), 160))
			}
			var raw map[string]any
			if err := json.Unmarshal(body, &raw); err != nil {
				return fmt.Sprintf("%s_parse_err=%s", label, err.Error())
			}
			data, _ := raw["data"].([]any)
			for _, item := range data {
				row, ok := item.(map[string]any)
				if !ok {
					continue
				}
				traceID := strings.TrimSpace(asString(row["traceId"]))
				if traceID == "" {
					continue
				}
				if _, ok := seen[traceID]; ok {
					continue
				}
				seen[traceID] = struct{}{}
				allIDs = append(allIDs, traceID)
			}
			meta, _ := raw["meta"].(map[string]any)
			next := ""
			if meta != nil {
				next = strings.TrimSpace(asString(meta["cursor"]))
			}
			if next == "" || len(data) == 0 {
				break
			}
			cursor = next
		}
		return ""
	}

	// Primary: exact userId on the trace (SDK propagate_attributes).
	qUser := url.Values{}
	qUser.Set("userId", userID)
	if errMsg := collect("userId", qUser); errMsg != "" && len(allIDs) == 0 {
		switch {
		case strings.HasPrefix(errMsg, "userId_list_err="):
			return "unreachable: " + strings.TrimPrefix(errMsg, "userId_list_err=")
		case strings.HasPrefix(errMsg, "userId_list_http="):
			return "list HTTP " + strings.TrimPrefix(errMsg, "userId_list_http=")
		case strings.HasPrefix(errMsg, "userId_parse_err="):
			return "parse error: " + strings.TrimPrefix(errMsg, "userId_parse_err=")
		default:
			return errMsg
		}
	}

	// Fallback: metadata.user_id (always written by open-bot even if propagate fails).
	qMeta := url.Values{}
	filter, _ := json.Marshal([]map[string]any{{
		"type":     "stringObject",
		"column":   "metadata",
		"key":      "user_id",
		"operator": "=",
		"value":    userID,
	}})
	qMeta.Set("filter", string(filter))
	_ = collect("metadata", qMeta) // best-effort; ignore errors if primary already found IDs

	if len(allIDs) == 0 {
		return fmt.Sprintf("deleted:0 (no traces matched userId=%s or metadata.user_id)", userID)
	}

	deleted := 0
	for i := 0; i < len(allIDs); i += langfusePurgeDeleteBatch {
		end := i + langfusePurgeDeleteBatch
		if end > len(allIDs) {
			end = len(allIDs)
		}
		chunk := allIDs[i:end]
		payload, _ := json.Marshal(map[string]any{"traceIds": chunk})
		dCode, dBody, dErr := s.langfuseRequest(ctx, http.MethodDelete, "/api/public/traces", nil, payload)
		if dErr != nil {
			return fmt.Sprintf("partial: deleted=%d delete_err=%s", deleted, dErr.Error())
		}
		if dCode < 200 || dCode >= 300 {
			return fmt.Sprintf("partial: deleted=%d delete_http=%d %s", deleted, dCode, truncateStr(string(dBody), 200))
		}
		deleted += len(chunk)
	}

	// Langfuse applies deletes asynchronously (worker → ClickHouse events table).
	return fmt.Sprintf("deleted:%d queued (async; typically visible within minutes)", deleted)
}

func cloneURLValues(in url.Values) url.Values {
	out := make(url.Values, len(in))
	for k, vs := range in {
		out[k] = append([]string(nil), vs...)
	}
	return out
}

// buildLangfuseTraceURL returns the Langfuse v4 UI deep link for a trace.
// Legacy "/trace/{id}" returns notFound on v4 events-only installs; use project-scoped path.
func buildLangfuseTraceURL(ui, projectID, traceID string) string {
	ui = strings.TrimRight(strings.TrimSpace(ui), "/")
	projectID = strings.TrimSpace(projectID)
	traceID = strings.TrimSpace(traceID)
	if ui == "" || projectID == "" || traceID == "" {
		return ""
	}
	return ui + "/project/" + url.PathEscape(projectID) + "/traces/" + url.PathEscape(traceID)
}

func normalizeRootObs(row map[string]any, cfg langfuseConfig) map[string]any {
	traceID := strings.TrimSpace(asString(row["traceId"]))
	if traceID == "" {
		traceID = strings.TrimSpace(asString(row["id"]))
	}
	name := strings.TrimSpace(asString(row["traceName"]))
	if name == "" {
		name = strings.TrimSpace(asString(row["name"]))
	}
	userID := strings.TrimSpace(asString(row["userId"]))
	sessionID := strings.TrimSpace(asString(row["sessionId"]))
	ts := strings.TrimSpace(asString(row["startTime"]))
	projectID := strings.TrimSpace(asString(row["projectId"]))
	if projectID == "" {
		projectID = cfg.ProjectID
	}
	agentID := ""
	if meta, ok := row["metadata"].(map[string]any); ok {
		agentID = strings.TrimSpace(asString(meta["agent_id"]))
		if userID == "" {
			userID = strings.TrimSpace(asString(meta["user_id"]))
		}
		if sessionID == "" {
			sessionID = strings.TrimSpace(asString(meta["conversation_id"]))
		}
	}
	var latency any
	if v, ok := row["latency"]; ok {
		latency = v
	}
	out := map[string]any{
		"id":            traceID,
		"name":          name,
		"userId":        userID,
		"sessionId":     sessionID,
		"timestamp":     ts,
		"latency":       latency,
		"observationId": row["id"],
		"type":          row["type"],
		"level":         row["level"],
		"projectId":     projectID,
		"agentId":       agentID,
	}
	if u := buildLangfuseTraceURL(cfg.PublicUI, projectID, traceID); u != "" {
		out["langfuse_url"] = u
	}
	return out
}

// enrichTracesWithNames fills userName / agentName (bot display name) from local DB.
func (s *Server) enrichTracesWithNames(traces []map[string]any) {
	if s == nil || s.db == nil || len(traces) == 0 {
		return
	}
	userIDs := make([]string, 0, len(traces))
	agentIDs := make([]string, 0, len(traces))
	seenU := map[string]struct{}{}
	seenA := map[string]struct{}{}
	for _, t := range traces {
		uid := strings.TrimSpace(asString(t["userId"]))
		if uid != "" {
			if _, ok := seenU[uid]; !ok {
				seenU[uid] = struct{}{}
				userIDs = append(userIDs, uid)
			}
		}
		aid := strings.TrimSpace(asString(t["agentId"]))
		if aid != "" {
			if _, ok := seenA[aid]; !ok {
				seenA[aid] = struct{}{}
				agentIDs = append(agentIDs, aid)
			}
		}
	}
	users := s.db.LookupUsernames(userIDs)
	agents := s.db.LookupAgentNames(agentIDs)
	for _, t := range traces {
		uid := strings.TrimSpace(asString(t["userId"]))
		if uid != "" {
			if name := users[uid]; name != "" {
				t["userName"] = name
			}
		}
		aid := strings.TrimSpace(asString(t["agentId"]))
		if aid != "" {
			if name := agents[aid]; name != "" {
				t["agentName"] = name
			}
		}
	}
}

func (s *Server) handleAdminTracesStatus(w http.ResponseWriter, r *http.Request) {
	cfg := loadLangfuseConfig()
	out := map[string]any{
		"enabled":       cfg.Configured,
		"public_ui_url": cfg.PublicUI,
		"base_url":      cfg.BaseURL,
		"project_id":    cfg.ProjectID,
		"reason":        cfg.Reason,
	}
	if !cfg.Configured {
		writeJSON(w, http.StatusOK, out)
		return
	}
	code, body, err := s.langfuseGET("/api/public/health", nil)
	if err != nil {
		out["ready"] = false
		out["reason"] = err.Error()
		writeJSON(w, http.StatusOK, out)
		return
	}
	out["ready"] = code >= 200 && code < 300
	if !out["ready"].(bool) {
		out["reason"] = fmt.Sprintf("Langfuse health HTTP %d: %s", code, truncateStr(string(body), 200))
	} else {
		out["reason"] = ""
	}
	writeJSON(w, http.StatusOK, out)
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func (s *Server) handleAdminListTraces(w http.ResponseWriter, r *http.Request) {
	cfg := loadLangfuseConfig()
	if !cfg.Configured {
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled":       false,
			"reason":        cfg.Reason,
			"traces":        []any{},
			"public_ui_url": cfg.PublicUI,
		})
		return
	}

	limit := 50
	if q := strings.TrimSpace(r.URL.Query().Get("limit")); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			limit = n
			if limit > 200 {
				limit = 200
			}
		}
	}
	// Optional page → approximate via larger fetch window; prefer cursor when provided.
	cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
	page := 1
	if q := strings.TrimSpace(r.URL.Query().Get("page")); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			page = n
		}
	}

	now := time.Now().UTC()
	from := now.Add(-7 * 24 * time.Hour)
	if q := strings.TrimSpace(r.URL.Query().Get("from")); q != "" {
		if t, err := time.Parse(time.RFC3339, q); err == nil {
			from = t.UTC()
		}
	}
	to := now.Add(time.Minute)
	if q := strings.TrimSpace(r.URL.Query().Get("to")); q != "" {
		if t, err := time.Parse(time.RFC3339, q); err == nil {
			to = t.UTC()
		}
	}

	q := url.Values{}
	q.Set("limit", strconv.Itoa(limit))
	q.Set("fields", "core,basic,trace_context,metadata")
	q.Set("isRootObservation", "true")
	q.Set("fromStartTime", from.Format(time.RFC3339))
	q.Set("toStartTime", to.Format(time.RFC3339))
	if cursor != "" {
		q.Set("cursor", cursor)
	} else if page > 1 {
		// v2 is cursor-based; page>1 without cursor is soft-ignored but kept for API compat.
		_ = page
	}

	code, body, err := s.langfuseGET("/api/public/v2/observations", q)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled":       false,
			"reason":        "Langfuse 不可达: " + err.Error(),
			"traces":        []any{},
			"public_ui_url": cfg.PublicUI,
		})
		return
	}
	if code < 200 || code >= 300 {
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled":       false,
			"reason":        fmt.Sprintf("Langfuse HTTP %d: %s", code, truncateStr(string(body), 300)),
			"traces":        []any{},
			"public_ui_url": cfg.PublicUI,
		})
		return
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled":       false,
			"reason":        "解析 Langfuse 响应失败",
			"traces":        []any{},
			"public_ui_url": cfg.PublicUI,
		})
		return
	}
	data, _ := raw["data"].([]any)
	traces := make([]map[string]any, 0, len(data))
	for _, item := range data {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		traces = append(traces, normalizeRootObs(row, cfg))
	}
	sort.SliceStable(traces, func(i, j int) bool {
		ti, _ := traces[i]["timestamp"].(string)
		tj, _ := traces[j]["timestamp"].(string)
		return ti > tj
	})
	s.enrichTracesWithNames(traces)
	meta, _ := raw["meta"].(map[string]any)
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":       true,
		"reason":        "",
		"traces":        traces,
		"meta":          meta,
		"public_ui_url": cfg.PublicUI,
		"project_id":    cfg.ProjectID,
		"page":          page,
		"limit":         limit,
	})
}

func (s *Server) handleAdminGetTrace(w http.ResponseWriter, r *http.Request) {
	cfg := loadLangfuseConfig()
	if !cfg.Configured {
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled": false,
			"reason":  cfg.Reason,
			"trace":   nil,
		})
		return
	}
	traceID := strings.TrimSpace(r.PathValue("id"))
	if traceID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "trace id required"})
		return
	}

	now := time.Now().UTC()
	from := now.Add(-30 * 24 * time.Hour)
	to := now.Add(time.Minute)
	q := url.Values{}
	q.Set("traceId", traceID)
	q.Set("limit", "100")
	q.Set("fields", "core,basic,io,trace_context,metadata")
	q.Set("fromStartTime", from.Format(time.RFC3339))
	q.Set("toStartTime", to.Format(time.RFC3339))

	code, body, err := s.langfuseGET("/api/public/v2/observations", q)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled": false,
			"reason":  "Langfuse 不可达: " + err.Error(),
			"trace":   nil,
		})
		return
	}
	if code < 200 || code >= 300 {
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled": false,
			"reason":  fmt.Sprintf("Langfuse HTTP %d: %s", code, truncateStr(string(body), 300)),
			"trace":   nil,
		})
		return
	}
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled": false,
			"reason":  "解析 Langfuse 响应失败",
			"trace":   nil,
		})
		return
	}
	data, _ := raw["data"].([]any)
	var root map[string]any
	observations := make([]map[string]any, 0, len(data))
	for _, item := range data {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		observations = append(observations, row)
		isRoot, _ := row["isRootObservation"].(bool)
		parent, _ := row["parentObservationId"]
		if isRoot || parent == nil {
			if root == nil {
				root = row
			}
		}
	}
	summary := map[string]any{"id": traceID}
	if root != nil {
		summary = normalizeRootObs(root, cfg)
	} else if u := buildLangfuseTraceURL(cfg.PublicUI, cfg.ProjectID, traceID); u != "" {
		summary["langfuse_url"] = u
		summary["projectId"] = cfg.ProjectID
	}
	// Prefer projectId from observations when env unset.
	if _, ok := summary["langfuse_url"]; !ok {
		for _, row := range observations {
			pid, _ := row["projectId"].(string)
			if u := buildLangfuseTraceURL(cfg.PublicUI, pid, traceID); u != "" {
				summary["langfuse_url"] = u
				summary["projectId"] = pid
				break
			}
		}
	}
	s.enrichTracesWithNames([]map[string]any{summary})
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":       true,
		"reason":        "",
		"trace":         summary,
		"observations":  observations,
		"public_ui_url": cfg.PublicUI,
		"project_id":    cfg.ProjectID,
	})
}
