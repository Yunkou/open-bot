package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tangxin/open-bot/services/api/internal/auth"
	"github.com/tangxin/open-bot/services/api/internal/db"
)

// A2A gateway: agent-card + JSON-RPC with persisted task lifecycle,
// optional SSE streaming (message/stream), GetTask, Cancel, and push webhooks.
// Auth: optional A2A_TOKEN (X-A2A-Token or Bearer).

func (s *Server) a2aPublicBase(r *http.Request) string {
	if v := strings.TrimSpace(os.Getenv("A2A_PUBLIC_BASE")); v != "" {
		return strings.TrimRight(v, "/")
	}
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = "127.0.0.1:18080"
	}
	return scheme + "://" + host
}

func (s *Server) requireA2AAuth(w http.ResponseWriter, r *http.Request) bool {
	want := strings.TrimSpace(os.Getenv("A2A_TOKEN"))
	if want == "" {
		return true
	}
	got := strings.TrimSpace(r.Header.Get("X-A2A-Token"))
	if got == "" {
		got = auth.BearerToken(r.Header.Get("Authorization"))
	}
	if got == "" || got != want {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "a2a unauthorized"})
		return false
	}
	return true
}

func (s *Server) ensureA2AUser() (*db.User, error) {
	hash, err := auth.HashPassword("a2a-system-internal")
	if err != nil {
		return nil, err
	}
	u, err := s.db.EnsureA2ASystemUser(hash)
	if err != nil {
		return nil, err
	}
	if _, err := s.db.GetDefaultLLM(u.ID); errors.Is(err, db.ErrNotFound) {
		base := strings.TrimSpace(os.Getenv("OPENAI_BASE_URL"))
		key := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
		model := strings.TrimSpace(os.Getenv("OPENAI_MODEL"))
		enableTools := false
		switch strings.ToLower(strings.TrimSpace(os.Getenv("OPENAI_ENABLE_TOOLS"))) {
		case "1", "true", "yes", "on":
			enableTools = true
		}
		_, _ = s.db.CreateLLMConnection(u.ID, "A2A 默认", base, key, model, enableTools, true, nil)
	}
	return u, nil
}

func (s *Server) handleAgentCard(w http.ResponseWriter, r *http.Request) {
	if !s.requireA2AAuth(w, r) {
		return
	}
	agentID := strings.TrimSpace(r.URL.Query().Get("agent_id"))
	u, err := s.ensureA2AUser()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if agentID == "" {
		agentID, err = s.db.FirstAgentID(u.ID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
	}
	agent, err := s.db.GetAgent(u.ID, agentID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "agent not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	base := s.a2aPublicBase(r)
	endpoint := base + "/a2a/v1"

	skills := []map[string]any{}
	if list, err := db.ListGlobalSkillsFromDisk(); err == nil {
		for _, sk := range list {
			skills = append(skills, map[string]any{
				"id":          sk.Name,
				"name":        sk.Name,
				"description": sk.Description,
				"tags":        []string{"open-bot"},
			})
		}
	}
	if len(skills) == 0 {
		skills = append(skills, map[string]any{
			"id":          "chat",
			"name":        "chat",
			"description": "通用对话",
			"tags":        []string{"open-bot"},
		})
	}

	card := map[string]any{
		"name":        agent.Name,
		"description": agent.Description,
		"skills":      skills,
		"supportedInterfaces": []map[string]any{
			{
				"url":             endpoint,
				"protocolBinding": "JSONRPC",
				"protocolVersion": "1.0",
			},
			{
				"url":             endpoint,
				"protocolBinding": "HTTP+JSON",
				"protocolVersion": "1.0",
			},
		},
		"capabilities": map[string]any{
			"streaming":              true,
			"pushNotifications":      true,
			"stateTransitionHistory": false,
			"extendedAgentCard":      false,
		},
		"url":               endpoint,
		"protocolVersion":   "0.3",
		"preferredTransport": "JSONRPC",
		"provider": map[string]any{
			"organization": "open-bot",
			"url":          base,
		},
		"version":           "0.2.0",
		"documentationUrl":  base + "/docs/a2a-adapter",
		"notes":             "open-bot A2A：message/send、message/stream、tasks/get、tasks/cancel、push config。可选 A2A_TOKEN。",
		"defaultInputModes":  []string{"text"},
		"defaultOutputModes": []string{"text"},
		"metadata": map[string]any{
			"agent_id": agent.ID,
		},
	}
	writeJSON(w, http.StatusOK, card)
}

type jsonRPCReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

func (s *Server) handleA2AJSONRPC(w http.ResponseWriter, r *http.Request) {
	if !s.requireA2AAuth(w, r) {
		return
	}
	var req jsonRPCReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"jsonrpc": "2.0",
			"id":      nil,
			"error":   map[string]any{"code": -32700, "message": "Parse error"},
		})
		return
	}
	if req.JSONRPC == "" {
		req.JSONRPC = "2.0"
	}
	method := strings.TrimSpace(req.Method)
	switch method {
	case "message/send", "SendMessage":
		result, err := s.a2aMessageSend(r, req.Params, false)
		s.writeA2ARPC(w, req.ID, result, err)
	case "message/stream", "SendStreamingMessage":
		s.a2aMessageStream(w, r, req)
	case "tasks/get", "GetTask":
		result, err := s.a2aTasksGet(req.Params)
		s.writeA2ARPC(w, req.ID, result, err)
	case "tasks/cancel", "CancelTask":
		result, err := s.a2aTasksCancel(req.Params)
		s.writeA2ARPC(w, req.ID, result, err)
	case "tasks/pushNotificationConfig/set", "SetTaskPushNotificationConfig":
		result, err := s.a2aPushSet(req.Params)
		s.writeA2ARPC(w, req.ID, result, err)
	case "tasks/pushNotificationConfig/get", "GetTaskPushNotificationConfig":
		result, err := s.a2aPushGet(req.Params)
		s.writeA2ARPC(w, req.ID, result, err)
	default:
		writeJSON(w, http.StatusOK, map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"error": map[string]any{
				"code": -32601,
				"message": fmt.Sprintf(
					"Method not found: %s (supported: message/send, message/stream, tasks/get, tasks/cancel, tasks/pushNotificationConfig/set|get)",
					method,
				),
			},
		})
	}
}

func (s *Server) writeA2ARPC(w http.ResponseWriter, id any, result map[string]any, err error) {
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"jsonrpc": "2.0",
			"id":      id,
			"error": map[string]any{
				"code":    -32000,
				"message": err.Error(),
			},
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
}

func extractA2AText(params map[string]any) (text, agentID, contextID string) {
	if v, ok := params["text"].(string); ok {
		text = strings.TrimSpace(v)
	}
	if v, ok := params["agent_id"].(string); ok {
		agentID = strings.TrimSpace(v)
	}
	if v, ok := params["contextId"].(string); ok {
		contextID = strings.TrimSpace(v)
	}
	if v, ok := params["context_id"].(string); ok && contextID == "" {
		contextID = strings.TrimSpace(v)
	}
	if meta, ok := params["metadata"].(map[string]any); ok {
		if v, ok := meta["agent_id"].(string); ok && agentID == "" {
			agentID = strings.TrimSpace(v)
		}
	}
	if msg, ok := params["message"].(map[string]any); ok {
		if v, ok := msg["contextId"].(string); ok && contextID == "" {
			contextID = strings.TrimSpace(v)
		}
		if parts, ok := msg["parts"].([]any); ok {
			var b strings.Builder
			for _, p := range parts {
				pm, ok := p.(map[string]any)
				if !ok {
					continue
				}
				if t, ok := pm["text"].(string); ok && strings.TrimSpace(t) != "" {
					if b.Len() > 0 {
						b.WriteString("\n")
					}
					b.WriteString(strings.TrimSpace(t))
					continue
				}
				if kind, _ := pm["kind"].(string); kind == "text" {
					if t, ok := pm["text"].(string); ok && strings.TrimSpace(t) != "" {
						if b.Len() > 0 {
							b.WriteString("\n")
						}
						b.WriteString(strings.TrimSpace(t))
					}
				}
			}
			if text == "" {
				text = strings.TrimSpace(b.String())
			}
		}
	}
	return text, agentID, contextID
}

func extractA2ATaskID(params map[string]any) string {
	if v, ok := params["id"].(string); ok {
		return strings.TrimSpace(v)
	}
	if v, ok := params["taskId"].(string); ok {
		return strings.TrimSpace(v)
	}
	if v, ok := params["task_id"].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func (s *Server) a2aResolveAgent(u *db.User, agentID string) (string, error) {
	if agentID == "" {
		id, err := s.db.FirstAgentID(u.ID)
		if err != nil {
			return "", err
		}
		agentID = id
	}
	if _, err := s.db.GetAgent(u.ID, agentID); err != nil {
		return "", fmt.Errorf("agent not found: %s", agentID)
	}
	return agentID, nil
}

func a2aHistoryArtifacts(text, reply string) (historyJSON, artifactsJSON string) {
	hist := []map[string]any{
		{
			"messageId": uuid.NewString(),
			"role":      "user",
			"parts":     []map[string]any{{"kind": "text", "text": text}},
		},
		{
			"messageId": uuid.NewString(),
			"role":      "agent",
			"parts":     []map[string]any{{"kind": "text", "text": reply}},
		},
	}
	arts := []map[string]any{
		{
			"artifactId": uuid.NewString(),
			"name":       "reply",
			"parts":      []map[string]any{{"kind": "text", "text": reply}},
		},
	}
	hb, _ := json.Marshal(hist)
	ab, _ := json.Marshal(arts)
	return string(hb), string(ab)
}

func (s *Server) a2aMessageSend(r *http.Request, raw json.RawMessage, async bool) (map[string]any, error) {
	params := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return nil, errors.New("invalid params")
		}
	}
	text, agentID, contextID := extractA2AText(params)
	if text == "" {
		return nil, errors.New("message text required (params.text or message.parts[].text)")
	}
	u, err := s.ensureA2AUser()
	if err != nil {
		return nil, err
	}
	agentID, err = s.a2aResolveAgent(u, agentID)
	if err != nil {
		return nil, err
	}

	task, err := s.db.CreateA2ATask(u.ID, agentID, contextID, text)
	if err != nil {
		return nil, err
	}
	// Optional inline push registration on send
	if push, ok := params["pushNotificationConfig"].(map[string]any); ok {
		url, _ := push["url"].(string)
		token, _ := push["token"].(string)
		if strings.TrimSpace(url) != "" {
			_, _ = s.db.UpsertA2APushConfig(task.ID, agentID, url, token)
		}
	}

	_, _ = s.db.UpdateA2ATaskState(task.ID, db.A2AStateWorking, "", "", "", "")

	if async {
		go s.a2aExecuteTask(context.Background(), task.ID, u.ID, agentID, text, task.ContextID)
		t2, _ := s.db.GetA2ATask(task.ID)
		return db.A2ATaskPublicMap(t2), nil
	}

	s.a2aExecuteTask(r.Context(), task.ID, u.ID, agentID, text, task.ContextID)
	t2, err := s.db.GetA2ATask(task.ID)
	if err != nil {
		return nil, err
	}
	return db.A2ATaskPublicMap(t2), nil
}

func (s *Server) a2aExecuteTask(ctx context.Context, taskID, userID, agentID, text, contextID string) {
	title := "A2A " + agentID
	res, err := s.runAgentOnce(ctx, userID, agentID, text, title, contextID, "")
	if err != nil {
		_, _ = s.db.UpdateA2ATaskState(taskID, db.A2AStateFailed, "", err.Error(), "", "")
		s.a2aMaybePush(taskID)
		return
	}
	reply := res.Reply
	if strings.TrimSpace(reply) == "" {
		reply = "(empty reply)"
	}
	hist, arts := a2aHistoryArtifacts(text, reply)
	_, _ = s.db.UpdateA2ATaskState(taskID, db.A2AStateCompleted, reply, "", arts, hist)
	s.a2aMaybePush(taskID)
}

func (s *Server) a2aMessageStream(w http.ResponseWriter, r *http.Request, req jsonRPCReq) {
	params := map[string]any{}
	if len(req.Params) > 0 {
		_ = json.Unmarshal(req.Params, &params)
	}
	text, agentID, contextID := extractA2AText(params)
	if text == "" {
		s.writeA2ARPC(w, req.ID, nil, errors.New("message text required"))
		return
	}
	u, err := s.ensureA2AUser()
	if err != nil {
		s.writeA2ARPC(w, req.ID, nil, err)
		return
	}
	agentID, err = s.a2aResolveAgent(u, agentID)
	if err != nil {
		s.writeA2ARPC(w, req.ID, nil, err)
		return
	}
	task, err := s.db.CreateA2ATask(u.ID, agentID, contextID, text)
	if err != nil {
		s.writeA2ARPC(w, req.ID, nil, err)
		return
	}
	if push, ok := params["pushNotificationConfig"].(map[string]any); ok {
		url, _ := push["url"].(string)
		token, _ := push["token"].(string)
		if strings.TrimSpace(url) != "" {
			_, _ = s.db.UpsertA2APushConfig(task.ID, agentID, url, token)
		}
	}
	_, _ = s.db.UpdateA2ATaskState(task.ID, db.A2AStateWorking, "", "", "", "")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeA2ARPC(w, req.ID, nil, errors.New("streaming unsupported"))
		return
	}
	emitRPC := func(result map[string]any) {
		payload := map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result}
		b, _ := json.Marshal(payload)
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
	}

	working := db.A2ATaskPublicMap(task)
	working["status"] = map[string]any{
		"state":     db.A2AStateWorking,
		"timestamp": time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
	}
	emitRPC(map[string]any{
		"kind":      "status-update",
		"taskId":    task.ID,
		"contextId": task.ContextID,
		"status":    working["status"],
		"final":     false,
	})

	title := "A2A " + agentID
	res, runErr := s.runAgentOnce(r.Context(), u.ID, agentID, text, title, task.ContextID, "")
	if runErr != nil {
		t2, _ := s.db.UpdateA2ATaskState(task.ID, db.A2AStateFailed, "", runErr.Error(), "", "")
		emitRPC(map[string]any{
			"kind":      "status-update",
			"taskId":    task.ID,
			"contextId": task.ContextID,
			"status":    db.A2ATaskPublicMap(t2)["status"],
			"final":     true,
			"task":      db.A2ATaskPublicMap(t2),
		})
		s.a2aMaybePush(task.ID)
		return
	}
	reply := res.Reply
	if strings.TrimSpace(reply) == "" {
		reply = "(empty reply)"
	}
	hist, arts := a2aHistoryArtifacts(text, reply)
	t2, _ := s.db.UpdateA2ATaskState(task.ID, db.A2AStateCompleted, reply, "", arts, hist)
	emitRPC(map[string]any{
		"kind":      "artifact-update",
		"taskId":    task.ID,
		"contextId": task.ContextID,
		"artifact": map[string]any{
			"artifactId": uuid.NewString(),
			"name":       "reply",
			"parts":      []map[string]any{{"kind": "text", "text": reply}},
		},
		"lastChunk": true,
	})
	emitRPC(map[string]any{
		"kind":      "status-update",
		"taskId":    task.ID,
		"contextId": task.ContextID,
		"status":    db.A2ATaskPublicMap(t2)["status"],
		"final":     true,
		"task":      db.A2ATaskPublicMap(t2),
	})
	s.a2aMaybePush(task.ID)
}

func (s *Server) a2aTasksGet(raw json.RawMessage) (map[string]any, error) {
	params := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return nil, errors.New("invalid params")
		}
	}
	id := extractA2ATaskID(params)
	if id == "" {
		return nil, errors.New("task id required")
	}
	t, err := s.db.GetA2ATask(id)
	if err != nil {
		return nil, err
	}
	return db.A2ATaskPublicMap(t), nil
}

func (s *Server) a2aTasksCancel(raw json.RawMessage) (map[string]any, error) {
	params := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return nil, errors.New("invalid params")
		}
	}
	id := extractA2ATaskID(params)
	if id == "" {
		return nil, errors.New("task id required")
	}
	t, err := s.db.GetA2ATask(id)
	if err != nil {
		return nil, err
	}
	if db.A2AStateTerminal(t.State) {
		return db.A2ATaskPublicMap(t), nil
	}
	t2, err := s.db.UpdateA2ATaskState(id, db.A2AStateCanceled, "", "canceled by client", "", "")
	if err != nil {
		return nil, err
	}
	s.a2aMaybePush(id)
	return db.A2ATaskPublicMap(t2), nil
}

func (s *Server) a2aPushSet(raw json.RawMessage) (map[string]any, error) {
	params := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return nil, errors.New("invalid params")
		}
	}
	taskID := extractA2ATaskID(params)
	agentID, _ := params["agent_id"].(string)
	url := ""
	token := ""
	if cfg, ok := params["pushNotificationConfig"].(map[string]any); ok {
		url, _ = cfg["url"].(string)
		token, _ = cfg["token"].(string)
	}
	if url == "" {
		url, _ = params["url"].(string)
	}
	if token == "" {
		token, _ = params["token"].(string)
	}
	if taskID == "" && strings.TrimSpace(agentID) == "" {
		return nil, errors.New("task id or agent_id required")
	}
	cfg, err := s.db.UpsertA2APushConfig(taskID, agentID, url, token)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"id":                     cfg.ID,
		"taskId":                 cfg.TaskID,
		"agent_id":               cfg.AgentID,
		"pushNotificationConfig": map[string]any{"url": cfg.URL, "token": cfg.Token},
	}, nil
}

func (s *Server) a2aPushGet(raw json.RawMessage) (map[string]any, error) {
	params := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &params); err != nil {
			return nil, errors.New("invalid params")
		}
	}
	taskID := extractA2ATaskID(params)
	agentID, _ := params["agent_id"].(string)
	list, err := s.db.ListA2APushConfigs(taskID, agentID)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		if taskID != "" {
			if t, err := s.db.GetA2ATask(taskID); err == nil && t.PushURL != "" {
				return map[string]any{
					"taskId": taskID,
					"pushNotificationConfig": map[string]any{
						"url":   t.PushURL,
						"token": t.PushToken,
					},
				}, nil
			}
		}
		return nil, errors.New("push config not found")
	}
	c := list[0]
	return map[string]any{
		"id":       c.ID,
		"taskId":   c.TaskID,
		"agent_id": c.AgentID,
		"pushNotificationConfig": map[string]any{
			"url":   c.URL,
			"token": c.Token,
		},
	}, nil
}

func (s *Server) a2aMaybePush(taskID string) {
	t, err := s.db.GetA2ATask(taskID)
	if err != nil || t == nil || !db.A2AStateTerminal(t.State) {
		return
	}
	targets := []struct{ URL, Token string }{}
	if t.PushURL != "" {
		targets = append(targets, struct{ URL, Token string }{t.PushURL, t.PushToken})
	}
	if cfgs, err := s.db.ListA2APushConfigs(taskID, t.AgentID); err == nil {
		for _, c := range cfgs {
			if c.URL == "" {
				continue
			}
			dup := false
			for _, x := range targets {
				if x.URL == c.URL {
					dup = true
					break
				}
			}
			if !dup {
				targets = append(targets, struct{ URL, Token string }{c.URL, c.Token})
			}
		}
	}
	if len(targets) == 0 {
		return
	}
	body, _ := json.Marshal(map[string]any{
		"kind":      "task",
		"task":      db.A2ATaskPublicMap(t),
		"timestamp": time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
	})
	client := &http.Client{Timeout: 10 * time.Second}
	for _, tgt := range targets {
		go func(url, token string, payload []byte) {
			req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(payload))
			if err != nil {
				return
			}
			req.Header.Set("Content-Type", "application/json")
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
				req.Header.Set("X-A2A-Notification-Token", token)
			}
			resp, err := client.Do(req)
			if err != nil {
				return
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}(tgt.URL, tgt.Token, body)
	}
}
