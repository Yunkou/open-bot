package httpserver

import (
	"errors"
	"net/http"

	"github.com/tangxin/open-bot/services/api/internal/db"
)

func (s *Server) toolAllowlistOrEmpty(uid string) []db.UserToolAllowlistEntry {
	if s == nil || s.db == nil {
		return nil
	}
	entries, err := s.db.ListUserToolAllowlist(uid)
	if err != nil {
		return nil
	}
	return entries
}

// GET /v1/me/tool-allowlist — list Always-allow entries for 设置·审核.
func (s *Server) handleListToolAllowlist(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	entries, err := s.db.ListUserToolAllowlist(uid)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if entries == nil {
		entries = []db.UserToolAllowlistEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": entries})
}

// DELETE /v1/me/tool-allowlist/{id} — revoke one Always-allow entry.
func (s *Server) handleDeleteToolAllowlist(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	id := r.PathValue("id")
	if err := s.db.DeleteUserToolAllowlist(uid, id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": id})
}

// attachToolAllowlist stamps user Always-allow entries into the runtime payload
// so agent-runtime can skip waiting_approval for matching tools (same pattern
// as client timezone). Runtime must still respect hard-deny / exec_policy=ask.
func attachToolAllowlist(payload map[string]any, entries []db.UserToolAllowlistEntry) {
	if payload == nil {
		return
	}
	if entries == nil {
		entries = []db.UserToolAllowlistEntry{}
	}
	slim := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		slim = append(slim, map[string]any{
			"id":         e.ID,
			"tool_group": e.ToolGroup,
			"tool":       e.Tool,
			"agent_id":   e.AgentID,
		})
	}
	payload["tool_allowlist"] = slim
}
