package httpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/tangxin/open-bot/services/api/internal/auth"
	"github.com/tangxin/open-bot/services/api/internal/db"
)

// A2A 适配层说明：
// 这是面向演示/联调的轻量适配，不是完整 A2A 认证、流式任务生命周期或 SDK 互操作实现。
// 支持 JSON-RPC 方法 message/send（v0.3 风格）与 SendMessage（v1 别名）。

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
			"streaming":         false,
			"pushNotifications": false,
			"extendedAgentCard": false,
		},
		// 兼容旧字段（适配层）
		"url":             endpoint,
		"protocolVersion": "0.3",
		"preferredTransport": "JSONRPC",
		"provider": map[string]any{
			"organization": "open-bot",
			"url":          base,
		},
		"version": "0.1.0",
		"documentationUrl": base + "/docs/a2a-adapter",
		"notes": "open-bot A2A 适配层：非完整认证与流式任务生命周期。方法支持 message/send 与 SendMessage。",
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
		result, err := s.a2aMessageSend(r, req.Params)
		if err != nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"error": map[string]any{
					"code":    -32000,
					"message": err.Error(),
				},
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  result,
		})
	default:
		writeJSON(w, http.StatusOK, map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"error": map[string]any{
				"code":    -32601,
				"message": fmt.Sprintf("Method not found: %s (supported: message/send, SendMessage)", method),
			},
		})
	}
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
				// v0.3 TextPart
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

func (s *Server) a2aMessageSend(r *http.Request, raw json.RawMessage) (map[string]any, error) {
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
	if agentID == "" {
		agentID, err = s.db.FirstAgentID(u.ID)
		if err != nil {
			return nil, err
		}
	}
	if _, err := s.db.GetAgent(u.ID, agentID); err != nil {
		return nil, fmt.Errorf("agent not found: %s", agentID)
	}

	taskID := uuid.NewString()
	convID := contextID
	if convID == "" {
		convID = "a2a-" + taskID
	}
	title := "A2A " + agentID

	ctx := r.Context()
	res, err := s.runAgentOnce(ctx, u.ID, agentID, text, title, convID, "")
	if err != nil {
		return map[string]any{
			"id":        taskID,
			"contextId": convID,
			"status": map[string]any{
				"state":     "TASK_STATE_FAILED",
				"timestamp": time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
				"message": map[string]any{
					"role": "ROLE_AGENT",
					"parts": []map[string]any{
						{"text": err.Error(), "mediaType": "text/plain"},
					},
				},
			},
			"kind": "task",
		}, nil
	}

	reply := res.Reply
	if strings.TrimSpace(reply) == "" {
		reply = "(empty reply)"
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	return map[string]any{
		"id":        taskID,
		"contextId": res.ConversationID,
		"status": map[string]any{
			"state":     "TASK_STATE_COMPLETED",
			"timestamp": now,
		},
		"artifacts": []map[string]any{
			{
				"artifactId": uuid.NewString(),
				"name":       "reply",
				"parts": []map[string]any{
					{"text": reply, "mediaType": "text/plain"},
				},
			},
		},
		"history": []map[string]any{
			{
				"messageId": uuid.NewString(),
				"role":      "ROLE_USER",
				"parts":     []map[string]any{{"text": text, "mediaType": "text/plain"}},
			},
			{
				"messageId": uuid.NewString(),
				"role":      "ROLE_AGENT",
				"parts":     []map[string]any{{"text": reply, "mediaType": "text/plain"}},
			},
		},
		"kind": "task",
		// 简化 result 便捷字段
		"result_text": reply,
		"agent_id":    agentID,
	}, nil
}
