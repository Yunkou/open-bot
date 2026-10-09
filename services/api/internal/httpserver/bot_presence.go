package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

var allowedBotPresence = map[string]struct{}{
	"idle":              {},
	"thinking":          {},
	"working":           {},
	"awaiting_approval": {},
	"error":             {},
}

// publishBotPresence pushes bot_presence on conversation SSE
// (GET /v1/conversations/{id}/events) and the user's chat WS (sidebar shows
// presence for every bot, not just the open thread). Separate from handoff notes.
func (s *Server) publishBotPresence(userID, conversationID, agentID, status string) {
	if s == nil {
		return
	}
	userID = strings.TrimSpace(userID)
	conversationID = strings.TrimSpace(conversationID)
	agentID = strings.TrimSpace(agentID)
	status = strings.TrimSpace(status)
	if conversationID == "" || agentID == "" {
		return
	}
	if _, ok := allowedBotPresence[status]; !ok {
		return
	}
	evt := map[string]any{
		"type":            "bot_presence",
		"conversation_id": conversationID,
		"agent_id":        agentID,
		"status":          status,
		"updated_at":      time.Now().UTC().Format(time.RFC3339Nano),
	}
	if s.convEvents != nil {
		if payload, err := json.Marshal(evt); err == nil {
			s.convEvents.Publish(conversationID, payload)
		}
	}
	if s.events != nil && userID != "" {
		s.events.Publish(userID, evt)
	}
}

// Presence states: idle | thinking | working | awaiting_approval | error.
// The 300ms minimum dwell (avoid thinking/working flicker) is enforced by the
// runtime when it pushes; the API does not debounce.

// finishBotPresence maps a run outcome to idle (ok / user stop) or error.
func (s *Server) finishBotPresence(userID, conversationID, agentID string, err error) {
	if err == nil || isCancelErr(err) {
		s.publishBotPresence(userID, conversationID, agentID, "idle")
		return
	}
	s.publishBotPresence(userID, conversationID, agentID, "error")
}

// POST /internal/bot-presence — runtime / tools can push Bot busy state.
func (s *Server) handleInternalBotPresence(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ConversationID string `json:"conversation_id"`
		AgentID        string `json:"agent_id"`
		Status         string `json:"status"`
		UserID         string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	status := strings.TrimSpace(body.Status)
	if _, ok := allowedBotPresence[status]; !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status"})
		return
	}
	convID := strings.TrimSpace(body.ConversationID)
	agentID := strings.TrimSpace(body.AgentID)
	uid := strings.TrimSpace(body.UserID)
	if agentID == "" && uid != "" {
		if resolved, err := s.db.ResolveAgentID(uid, ""); err == nil {
			agentID = resolved
		}
	}
	if agentID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent_id required"})
		return
	}
	if convID == "" {
		if uid == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "conversation_id or user_id required"})
			return
		}
		c, err := s.db.FindConversationForUserAgent(uid, agentID)
		if err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation not found"})
			return
		}
		convID = c.ID
	}
	// Without user_id only conversation SSE is notified (no chat WS fan-out).
	s.publishBotPresence(uid, convID, agentID, status)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":              true,
		"conversation_id": convID,
		"agent_id":        agentID,
		"status":          status,
	})
}
