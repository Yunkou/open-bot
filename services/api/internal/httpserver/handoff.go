package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/tangxin/open-bot/services/api/internal/db"
)

type handoffNoteBody struct {
	UserID          string `json:"user_id"`
	ConversationID  string `json:"conversation_id"`
	ThreadRootID    string `json:"thread_root_id"`
	FromBot         string `json:"from_bot"`
	ToBot           string `json:"to_bot"`
	Purpose         string `json:"purpose"`
	Status          string `json:"status"`
	AgentMessageID  string `json:"agent_message_id"`
	ViewerAgentID   string `json:"viewer_agent_id"` // resolve conversation if conversation_id empty
	SkipIfProjected bool   `json:"skip_if_projected"`
}

var allowedHandoffStatus = map[string]struct{}{
	"running":           {},
	"done":              {},
	"failed":            {},
	"rejected":          {},
	"awaiting_approval": {},
}

// POST /internal/handoff-notes — projection layer insert (idempotent on agent_message_id).
func (s *Server) handleInternalHandoffNote(w http.ResponseWriter, r *http.Request) {
	var body handoffNoteBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	uid := strings.TrimSpace(body.UserID)
	if uid == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "user_id required"})
		return
	}
	status := strings.TrimSpace(body.Status)
	if _, ok := allowedHandoffStatus[status]; !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status"})
		return
	}
	agentMsgID := strings.TrimSpace(body.AgentMessageID)
	if agentMsgID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent_message_id required"})
		return
	}
	if body.SkipIfProjected {
		exists, err := s.db.HasHandoffForAgentMessage(agentMsgID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if exists {
			writeJSON(w, http.StatusOK, map[string]any{"skipped": true, "reason": "already_projected"})
			return
		}
	}

	convID := strings.TrimSpace(body.ConversationID)
	if convID == "" {
		viewer := strings.TrimSpace(body.ViewerAgentID)
		if viewer == "" {
			viewer = strings.TrimSpace(body.FromBot)
		}
		c, err := s.db.FindConversationForUserAgent(uid, viewer)
		if err != nil {
			if errors.Is(err, db.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		convID = c.ID
	} else {
		if _, err := s.db.GetConversation(uid, convID); err != nil {
			if errors.Is(err, db.ErrNotFound) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "conversation not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}

	hp := db.HandoffPayload{
		FromBot:        body.FromBot,
		ToBot:          body.ToBot,
		Purpose:        body.Purpose,
		Status:         status,
		AgentMessageID: agentMsgID,
	}
	msg, created, err := s.db.AddHandoffNote(convID, body.ThreadRootID, hp)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	// If note already existed, allow status refresh when different.
	if !created && msg.Handoff != nil && msg.Handoff.Status != status {
		updated, uerr := s.db.UpdateHandoffStatus(agentMsgID, status)
		if uerr == nil && updated != nil {
			msg = updated
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"created":         created,
		"conversation_id": convID,
		"message":         msg,
	})
}
