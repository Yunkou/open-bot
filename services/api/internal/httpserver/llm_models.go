package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// LLMListModelsResult is returned by POST /v1/llm/list-models and the admin twin.
type LLMListModelsResult struct {
	OK         bool     `json:"ok"`
	Models     []string `json:"models"`
	Detail     string   `json:"detail,omitempty"`
	Error      string   `json:"error,omitempty"`
	HTTPStatus int      `json:"http_status,omitempty"`
}

type llmListModelsBody struct {
	BaseURL      string `json:"base_url"`
	APIKey       string `json:"api_key"`
	ConnectionID string `json:"connection_id"`
}

func modelsURLCandidates(baseURL string) []string {
	base := normalizeLLMBaseURL(baseURL)
	if base == "" {
		return nil
	}
	out := []string{base + "/models"}
	// Some gateways take a host root (no /v1); chat uses {base}/chat/completions so
	// prefer {base}/models first, then fall back to {base}/v1/models.
	if !strings.HasSuffix(base, "/v1") {
		out = append(out, base+"/v1/models")
	}
	return out
}

func parseOpenAIModelsBody(body []byte) ([]string, error) {
	var parsed struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(parsed.Data))
	ids := make([]string, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		id := strings.TrimSpace(m.ID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

// fetchLLMModels calls OpenAI-compatible GET {base}/models (with /v1/models fallback).
// The API key is never written into returned error/detail strings.
func fetchLLMModels(ctx context.Context, baseURL, apiKey string) LLMListModelsResult {
	baseURL = normalizeLLMBaseURL(baseURL)
	if baseURL == "" {
		return LLMListModelsResult{
			OK: false, Models: []string{}, Detail: "需要 base_url", Error: "需要 base_url",
		}
	}
	if strings.TrimSpace(apiKey) == "" {
		return LLMListModelsResult{
			OK: false, Models: []string{}, Detail: "需要 api_key", Error: "需要 api_key",
		}
	}

	client := &http.Client{Timeout: 20 * time.Second}
	var lastStatus int
	var lastBody string
	var lastErr error

	for _, url := range modelsURLCandidates(baseURL) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
		req.Header.Set("Accept", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
		lastStatus = resp.StatusCode
		lastBody = string(bodyBytes)

		if resp.StatusCode == http.StatusNotFound {
			continue
		}
		if resp.StatusCode >= 300 {
			clip := lastBody
			if len(clip) > 280 {
				clip = clip[:280] + "…"
			}
			// Never echo credentials; body from upstream should not contain the key we sent.
			return LLMListModelsResult{
				OK: false, Models: []string{},
				Detail:     fmt.Sprintf("上游 models 接口失败（HTTP %d）", resp.StatusCode),
				Error:      clip,
				HTTPStatus: resp.StatusCode,
			}
		}
		ids, err := parseOpenAIModelsBody(bodyBytes)
		if err != nil {
			return LLMListModelsResult{
				OK: false, Models: []string{},
				Detail:     "无法解析上游 models 响应",
				Error:      "invalid models json",
				HTTPStatus: resp.StatusCode,
			}
		}
		detail := fmt.Sprintf("已拉取 %d 个模型", len(ids))
		return LLMListModelsResult{
			OK: true, Models: ids, Detail: detail, HTTPStatus: resp.StatusCode,
		}
	}

	if lastErr != nil {
		return LLMListModelsResult{
			OK: false, Models: []string{},
			Detail: "无法连接上游：" + lastErr.Error(),
			Error:  lastErr.Error(),
		}
	}
	if lastStatus == http.StatusNotFound {
		return LLMListModelsResult{
			OK: false, Models: []string{},
			Detail:     "上游未找到 models 接口（尝试了 /models 与 /v1/models）",
			Error:      "not found",
			HTTPStatus: lastStatus,
		}
	}
	return LLMListModelsResult{
		OK: false, Models: []string{},
		Detail: "无法拉取模型列表", Error: "unknown", HTTPStatus: lastStatus,
	}
}

func (s *Server) resolveListModelsCredentials(r *http.Request, body llmListModelsBody) (baseURL, apiKey string, err error) {
	baseURL = strings.TrimSpace(body.BaseURL)
	apiKey = strings.TrimSpace(body.APIKey)
	uid := userIDFrom(r.Context())

	if cid := strings.TrimSpace(body.ConnectionID); cid != "" {
		c, gerr := s.db.GetLLMConnection(uid, cid)
		if gerr != nil {
			return "", "", gerr
		}
		if baseURL == "" {
			baseURL = c.BaseURL
		}
		if apiKey == "" {
			apiKey = c.APIKey
		}
	}
	return baseURL, apiKey, nil
}

func (s *Server) handleListLLMModels(w http.ResponseWriter, r *http.Request) {
	var body llmListModelsBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	baseURL, apiKey, err := s.resolveListModelsCredentials(r, body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if strings.TrimSpace(apiKey) == "" && strings.TrimSpace(body.ConnectionID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "需要 api_key，或提供已有 connection_id 以复用密钥"})
		return
	}
	if strings.TrimSpace(apiKey) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "该连接没有保存 api_key"})
		return
	}
	result := fetchLLMModels(r.Context(), baseURL, apiKey)
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleAdminListLLMModels(w http.ResponseWriter, r *http.Request) {
	u, ok := s.loadAuthUser(w, r)
	if !ok {
		return
	}
	var body llmListModelsBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	baseURL := strings.TrimSpace(body.BaseURL)
	apiKey := strings.TrimSpace(body.APIKey)
	if apiKey == "" || baseURL == "" {
		settings, serr := s.db.GetOrgSettings(u.OrgID)
		if serr != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": serr.Error()})
			return
		}
		if baseURL == "" {
			baseURL = settings.LLMBaseURL
		}
		if apiKey == "" {
			apiKey = settings.LLMAPIKey
		}
	}
	if apiKey == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "需要 api_key，或组织里已保存密钥"})
		return
	}
	if baseURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "需要 base_url"})
		return
	}
	result := fetchLLMModels(r.Context(), baseURL, apiKey)
	writeJSON(w, http.StatusOK, result)
}
