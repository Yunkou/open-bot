package httpserver

import (
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
	cfg := loadLangfuseConfig()
	if !cfg.Configured {
		return 0, nil, fmt.Errorf("%s", cfg.Reason)
	}
	u := cfg.BaseURL + path
	if len(query) > 0 {
		u = u + "?" + query.Encode()
	}
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return 0, nil, err
	}
	req.SetBasicAuth(cfg.PublicKey, cfg.SecretKey)
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 20 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return res.StatusCode, nil, err
	}
	return res.StatusCode, body, nil
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
	traceID, _ := row["traceId"].(string)
	if traceID == "" {
		traceID, _ = row["id"].(string)
	}
	name, _ := row["traceName"].(string)
	if name == "" {
		name, _ = row["name"].(string)
	}
	userID, _ := row["userId"].(string)
	sessionID, _ := row["sessionId"].(string)
	ts, _ := row["startTime"].(string)
	projectID, _ := row["projectId"].(string)
	if projectID == "" {
		projectID = cfg.ProjectID
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
	}
	if u := buildLangfuseTraceURL(cfg.PublicUI, projectID, traceID); u != "" {
		out["langfuse_url"] = u
	}
	return out
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
	q.Set("fields", "core,basic,trace_context")
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
	q.Set("fields", "core,basic,io,trace_context")
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
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":       true,
		"reason":        "",
		"trace":         summary,
		"observations":  observations,
		"public_ui_url": cfg.PublicUI,
		"project_id":    cfg.ProjectID,
	})
}
