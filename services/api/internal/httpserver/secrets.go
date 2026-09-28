package httpserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tangxin/open-bot/services/api/internal/crypto"
	"github.com/tangxin/open-bot/services/api/internal/db"
)

func (s *Server) handleListBotSecrets(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	agentID := r.URL.Query().Get("agent_id")
	list, err := s.db.ListBotSecrets(uid, agentID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if list == nil {
		list = []*db.BotSecretMeta{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"secrets": list})
}

func (s *Server) handleCreateBotSecret(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	var body struct {
		AgentID  string `json:"agent_id"`
		Name     string `json:"name"`
		Origin   string `json:"origin"`
		AuthType string `json:"auth_type"`
		Value    string `json:"value"` // plaintext once; never returned
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if strings.TrimSpace(body.Value) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "value required"})
		return
	}
	key, err := crypto.LoadKey()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	ct, err := crypto.Encrypt(key, []byte(body.Value))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	meta, err := s.db.CreateBotSecret(uid, body.AgentID, body.Name, body.Origin, body.AuthType, ct)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, meta)
}

func (s *Server) handleDeleteBotSecret(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	id := r.PathValue("id")
	if err := s.db.DeleteBotSecret(uid, id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListSecretRequests(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	list, err := s.db.ListPendingSecretRequests(uid)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if list == nil {
		list = []*db.BotSecretRequest{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"requests": list})
}

func (s *Server) handleResolveSecretRequest(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	id := r.PathValue("id")
	var body struct {
		Value    string `json:"value"`
		Name     string `json:"name"`
		Origin   string `json:"origin"`
		AuthType string `json:"auth_type"`
		AgentID  string `json:"agent_id"`
		Dismiss  bool   `json:"dismiss"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if body.Dismiss {
		_ = s.db.ResolveSecretRequest(uid, id, "dismissed")
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	if strings.TrimSpace(body.Value) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "value required"})
		return
	}
	key, err := crypto.LoadKey()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	ct, err := crypto.Encrypt(key, []byte(body.Value))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	name := body.Name
	if name == "" {
		name = "secret"
	}
	meta, err := s.db.CreateBotSecret(uid, body.AgentID, name, body.Origin, body.AuthType, ct)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	_ = s.db.ResolveSecretRequest(uid, id, "resolved")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "secret": meta})
}

// Internal: request_secret from runtime — records pending request; never returns plaintext.
func (s *Server) handleInternalRequestSecret(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID         string `json:"user_id"`
		AgentID        string `json:"agent_id"`
		ConversationID string `json:"conversation_id"`
		Name           string `json:"name"`
		Origin         string `json:"origin"`
		AuthType       string `json:"auth_type"`
		Reason         string `json:"reason"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	uid := strings.TrimSpace(body.UserID)
	if uid == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id required"})
		return
	}
	req, err := s.db.CreateSecretRequest(uid, body.AgentID, body.ConversationID, body.Name, body.Origin, body.AuthType, body.Reason)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"need_user_input": true,
		"event":           "secret_needed",
		"request":         req,
		"message":         "需要用户提供密钥：" + req.Name + "（不会回传明文给模型）",
	})
}

// Internal decrypt-for-request: returns plaintext only to internal callers (runtime secret_http).
func (s *Server) handleInternalDecryptSecret(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID  string `json:"user_id"`
		AgentID string `json:"agent_id"`
		Name    string `json:"name"`
		ID      string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	uid := strings.TrimSpace(body.UserID)
	if uid == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id required"})
		return
	}
	var sec *db.BotSecret
	var err error
	if strings.TrimSpace(body.ID) != "" {
		sec, err = s.db.GetBotSecretByID(uid, body.ID)
	} else {
		sec, err = s.db.GetBotSecretDecrypt(uid, body.AgentID, body.Name)
	}
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "secret not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	key, err := crypto.LoadKey()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	pt, err := crypto.Decrypt(key, sec.Ciphertext)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "decrypt failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"id":        sec.ID,
		"name":      sec.Name,
		"origin":    sec.Origin,
		"auth_type": sec.AuthType,
		"value":     string(pt),
	})
}

// secret_http: HTTPS only to saved origin with injected auth. Never returns secret to model.
func (s *Server) handleInternalSecretHTTP(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID  string            `json:"user_id"`
		AgentID string            `json:"agent_id"`
		Name    string            `json:"name"`
		Method  string            `json:"method"`
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
		Body    string            `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	uid := strings.TrimSpace(body.UserID)
	if uid == "" || strings.TrimSpace(body.Name) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id and name required"})
		return
	}
	sec, err := s.db.GetBotSecretDecrypt(uid, body.AgentID, body.Name)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "secret not found"})
		return
	}
	key, err := crypto.LoadKey()
	if err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	pt, err := crypto.Decrypt(key, sec.Ciphertext)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "decrypt failed"})
		return
	}
	target := strings.TrimSpace(body.URL)
	if target == "" {
		target = sec.Origin
	}
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "https" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url must be https"})
		return
	}
	origin := strings.TrimSpace(sec.Origin)
	if origin != "" {
		ou, e := url.Parse(origin)
		if e != nil || !strings.EqualFold(ou.Host, u.Host) {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "url host must match saved origin"})
			return
		}
	}
	method := strings.ToUpper(strings.TrimSpace(body.Method))
	if method == "" {
		method = http.MethodGet
	}
	var reqBody io.Reader
	if body.Body != "" {
		reqBody = strings.NewReader(body.Body)
	}
	req, err := http.NewRequestWithContext(r.Context(), method, u.String(), reqBody)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	for k, v := range body.Headers {
		req.Header.Set(k, v)
	}
	token := string(pt)
	switch strings.ToLower(sec.AuthType) {
	case "basic":
		req.Header.Set("Authorization", "Basic "+token)
	case "header":
		req.Header.Set("X-Api-Key", token)
	default:
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	writeJSON(w, http.StatusOK, map[string]any{
		"status":      resp.StatusCode,
		"body":        string(raw),
		"secret_name": sec.Name,
		// plaintext never included
	})
}

// Internal: worker triggers a routine run by id.
func (s *Server) handleInternalRunRoutine(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RoutineID string `json:"routine_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	rt, err := s.db.GetRoutineByID(strings.TrimSpace(body.RoutineID))
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	run, err := s.executeRoutine(r.Context(), rt)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error(), "run": run})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run": run, "routine_id": rt.ID})
}
