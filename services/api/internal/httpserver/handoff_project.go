package httpserver

import (
	"log"
	"strings"
	"unicode/utf8"

	"github.com/tangxin/open-bot/services/api/internal/db"
)

const handoffPurposeMaxRunes = 120

// shouldProjectHandoff: visible Bot↔Bot handoffs only (direct to_agent).
// Channel-only / no-target chatter stays on the bus (default invisible).
func shouldProjectHandoff(msg *db.AgentBusMessage) bool {
	if msg == nil {
		return false
	}
	if msg.ToAgentID == nil || strings.TrimSpace(*msg.ToAgentID) == "" {
		return false
	}
	// Reply on the bus (priority wake echo) is not a new user-facing handoff.
	if msg.ReplyToID != nil && strings.TrimSpace(*msg.ReplyToID) != "" {
		return false
	}
	return true
}

func purposeFromBody(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return "协作交接"
	}
	if utf8.RuneCountInString(body) <= handoffPurposeMaxRunes {
		return body
	}
	runes := []rune(body)
	return string(runes[:handoffPurposeMaxRunes]) + "…"
}

// projectHandoffAfterBusWrite inserts a timeline handoff note after agent-bus success.
// Idempotent on agent_message_id; skips if that id already appears on the user timeline (@ dedup).
func (s *Server) projectHandoffAfterBusWrite(userID string, msg *db.AgentBusMessage) {
	if !shouldProjectHandoff(msg) {
		return
	}
	to := strings.TrimSpace(*msg.ToAgentID)
	status := "running"
	if !msg.Priority {
		// Non-priority direct handoff: still user-visible when it changes task routing;
		// mark done immediately (no wake lifecycle).
		status = "done"
	}
	s.writeHandoffProjection(userID, msg, to, status, true)
}

// projectHandoffStatus updates / inserts projection after priority wake completes or fails.
func (s *Server) projectHandoffStatus(userID string, msg *db.AgentBusMessage, status string) {
	if msg == nil || msg.ToAgentID == nil {
		return
	}
	to := strings.TrimSpace(*msg.ToAgentID)
	if to == "" {
		return
	}
	s.writeHandoffProjection(userID, msg, to, status, false)
}

func (s *Server) writeHandoffProjection(userID string, msg *db.AgentBusMessage, toBot, status string, skipIfProjected bool) {
	if s == nil || s.db == nil || msg == nil {
		return
	}
	agentMsgID := strings.TrimSpace(msg.ID)
	if agentMsgID == "" {
		return
	}
	if skipIfProjected {
		exists, err := s.db.HasHandoffForAgentMessage(agentMsgID)
		if err != nil {
			log.Printf("handoff project exists check msg=%s: %v", agentMsgID, err)
			return
		}
		if exists {
			// Already on timeline (e.g. @ path) — do not double-project.
			return
		}
	}

	viewer := strings.TrimSpace(msg.FromAgentID)
	if viewer == "" {
		viewer = "open-bot"
	}
	conv, err := s.db.FindConversationForUserAgent(userID, viewer)
	if err != nil {
		log.Printf("handoff project find conv user=%s viewer=%s: %v", userID, viewer, err)
		return
	}

	hp := db.HandoffPayload{
		FromBot:        viewer,
		ToBot:          toBot,
		Purpose:        purposeFromBody(msg.Body),
		Status:         status,
		AgentMessageID: agentMsgID,
	}
	note, created, err := s.db.AddHandoffNote(conv.ID, "", hp)
	if err != nil {
		log.Printf("handoff project insert msg=%s: %v", agentMsgID, err)
		return
	}
	if !created && note != nil && note.Handoff != nil && note.Handoff.Status != status {
		if _, uerr := s.db.UpdateHandoffStatus(agentMsgID, status); uerr != nil {
			log.Printf("handoff project status update msg=%s: %v", agentMsgID, uerr)
		}
	}
}
