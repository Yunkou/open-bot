package httpserver

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tangxin/open-bot/services/api/internal/db"
)

func (s *Server) handleAdminListMemories(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.loadAuthUser(w, r)
	if !ok {
		return
	}
	f, ok := s.memoryAdminFilter(w, r, admin.OrgID)
	if !ok {
		return
	}
	list, err := s.db.ListOrgMemories(admin.OrgID, f)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		out = append(out, map[string]any{
			"id":              item.ID,
			"user_id":         item.UserID,
			"username":        item.Username,
			"scope":           item.Scope,
			"agent_id":        item.AgentID,
			"agent_name":      item.AgentName,
			"channel_id":      item.ChannelID,
			"channel_name":    item.ChannelName,
			"peer_agent_id":   item.PeerAgentID,
			"peer_agent_name": item.PeerAgentName,
			"tier":            item.Tier,
			"content":         item.Content,
			"tags":            splitTags(item.Tags),
			"created_at":      item.CreatedAt.UTC().Format(time.RFC3339Nano),
			"updated_at":      item.UpdatedAt.UTC().Format(time.RFC3339Nano),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"memories": out})
}

func (s *Server) handleAdminListAutoMemories(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.loadAuthUser(w, r)
	if !ok {
		return
	}
	f, ok := s.memoryAdminFilter(w, r, admin.OrgID)
	if !ok {
		return
	}
	if strings.TrimSpace(f.UserID) == "" {
		writeJSON(w, http.StatusOK, map[string]any{
			"enabled":  false,
			"memories": []any{},
			"reason":   "请选择用户",
		})
		return
	}
	q := r.URL.Query()
	q.Set("user_id", f.UserID)
	q.Set("limit", strconv.Itoa(f.Limit))
	if f.Scope != "" {
		q.Set("scope", f.Scope)
	}
	if f.Scope == "agent_pair" {
		q.Set("agent_id", f.AgentID)
		q.Set("peer_agent_id", f.PeerAgentID)
	}
	s.proxyJSON(w, http.MethodGet, "/v1/memories/auto", q, nil, "")
}

func (s *Server) handleAdminListCompactions(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.loadAuthUser(w, r)
	if !ok {
		return
	}
	f, ok := s.memoryAdminFilter(w, r, admin.OrgID)
	if !ok {
		return
	}
	if f.Scope == "agent_pair" {
		writeJSON(w, http.StatusOK, map[string]any{
			"compactions": []any{},
			"note":        "Bot 之间的总线消息不产生压缩摘要。压缩只按用户-Bot 会话和群组会话归类。",
		})
		return
	}
	if f.Scope == "user" {
		f.Scope = ""
	}
	list, err := s.db.ListOrgCompactions(admin.OrgID, f)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		out = append(out, map[string]any{
			"id":              item.ID,
			"content":         item.Content,
			"created_at":      item.CreatedAt.UTC().Format(time.RFC3339Nano),
			"conversation_id": item.ConversationID,
			"title":           item.Title,
			"user_id":         item.UserID,
			"username":        item.Username,
			"agent_id":        item.AgentID,
			"agent_name":      item.AgentName,
			"channel_id":      item.ChannelID,
			"channel_name":    item.ChannelName,
			"scope":           item.Scope,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"compactions": out})
}

func (s *Server) handleAdminListChannels(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.loadAuthUser(w, r)
	if !ok {
		return
	}
	list, err := s.db.ListOrgChannels(admin.OrgID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		out = append(out, map[string]any{
			"id":           item.ID,
			"user_id":      item.UserID,
			"username":     item.Username,
			"name":         item.Name,
			"created_at":   item.CreatedAt.UTC().Format(time.RFC3339Nano),
			"member_ids":   splitTags(item.MemberIDs),
			"member_names": item.MemberNames,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"channels": out})
}

func (s *Server) memoryAdminFilter(w http.ResponseWriter, r *http.Request, orgID string) (db.MemoryAdminFilter, bool) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	f := db.MemoryAdminFilter{
		Scope:       strings.TrimSpace(q.Get("scope")),
		UserID:      strings.TrimSpace(q.Get("user_id")),
		AgentID:     strings.TrimSpace(q.Get("agent_id")),
		ChannelID:   strings.TrimSpace(q.Get("channel_id")),
		PeerAgentID: strings.TrimSpace(q.Get("peer_agent_id")),
		Tier:        strings.TrimSpace(q.Get("tier")),
		Limit:       limit,
	}
	if f.Limit <= 0 {
		f.Limit = 100
	}
	switch f.Scope {
	case "", "user", "bot", "channel", "agent_pair":
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "未知的 scope"})
		return f, false
	}
	if f.UserID != "" {
		u, err := s.db.GetUserByID(f.UserID)
		if err != nil || u.OrgID != orgID {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "目标用户不在本组织"})
			return f, false
		}
	}
	if f.AgentID != "" {
		if _, err := s.db.GetAgentInOrg(f.AgentID, orgID); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "目标 Bot 不在本组织"})
			return f, false
		}
	}
	if f.PeerAgentID != "" {
		if _, err := s.db.GetAgentInOrg(f.PeerAgentID, orgID); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "对端 Bot 不在本组织"})
			return f, false
		}
	}
	if f.ChannelID != "" {
		ok, err := s.db.ChannelInOrg(f.ChannelID, orgID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return f, false
		}
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "目标群组不在本组织"})
			return f, false
		}
	}
	if f.Scope == "agent_pair" {
		f.AgentID, f.PeerAgentID = db.NormalizeAgentPair(f.AgentID, f.PeerAgentID)
	}
	return f, true
}

func splitTags(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
