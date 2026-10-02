package httpserver

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tangxin/open-bot/services/api/internal/db"
)

type recallPersistContext struct {
	UserID         string
	AgentID        string
	ConversationID string
	MessageID      string
	Source         string
}

func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	default:
		return ""
	}
}

func asInt(v any) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	case json.Number:
		n, _ := t.Int64()
		return int(n)
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(t))
		return n
	default:
		return 0
	}
}

func parseRecallItems(raw any) []db.MemoryRecallItem {
	arr, ok := raw.([]any)
	if !ok {
		// already typed via json into []map
		b, err := json.Marshal(raw)
		if err != nil {
			return nil
		}
		var items []db.MemoryRecallItem
		if json.Unmarshal(b, &items) != nil {
			return nil
		}
		return items
	}
	out := make([]db.MemoryRecallItem, 0, len(arr))
	for _, el := range arr {
		m, ok := el.(map[string]any)
		if !ok {
			continue
		}
		item := db.MemoryRecallItem{
			Source:      asString(m["source"]),
			Scope:       asString(m["scope"]),
			Tier:        asString(m["tier"]),
			Content:     asString(m["content"]),
			Snippet:     asString(m["snippet"]),
			MemoryID:    asString(m["memory_id"]),
			AgentID:     asString(m["agent_id"]),
			ChannelID:   asString(m["channel_id"]),
			PeerAgentID: asString(m["peer_agent_id"]),
		}
		if scoreRaw, exists := m["score"]; exists && scoreRaw != nil {
			switch t := scoreRaw.(type) {
			case float64:
				item.Score = &t
			case json.Number:
				if f, err := t.Float64(); err == nil {
					item.Score = &f
				}
			}
		}
		out = append(out, item)
	}
	return out
}

// persistMemoryRecallFromMeta extracts memory_recall from a runtime meta payload,
// stores it, and strips the heavy field so clients only see counts.
func (s *Server) persistMemoryRecallFromMeta(payload map[string]any, ctx recallPersistContext) {
	if payload == nil {
		return
	}
	raw, ok := payload["memory_recall"]
	if !ok || raw == nil {
		return
	}
	// Always strip before forwarding SSE to clients, even if persistence fails.
	delete(payload, "memory_recall")
	if s == nil || s.db == nil {
		return
	}

	block, ok := raw.(map[string]any)
	if !ok {
		b, err := json.Marshal(raw)
		if err != nil {
			return
		}
		block = map[string]any{}
		if json.Unmarshal(b, &block) != nil {
			return
		}
	}
	items := parseRecallItems(block["items"])
	explicit := asInt(block["explicit_count"])
	mem0 := asInt(block["mem0_count"])
	if explicit == 0 {
		explicit = asInt(payload["memory_recalled"])
	}
	if mem0 == 0 {
		mem0 = asInt(payload["mem0_recalled"])
	}
	userID := strings.TrimSpace(ctx.UserID)
	if userID == "" {
		userID = asString(payload["user_id"])
	}
	agentID := strings.TrimSpace(ctx.AgentID)
	if agentID == "" {
		agentID = asString(payload["agent_id"])
	}
	convID := strings.TrimSpace(ctx.ConversationID)
	if convID == "" {
		convID = asString(payload["conversation_id"])
	}
	source := strings.TrimSpace(ctx.Source)
	if source == "" {
		source = "chat"
	}
	_, _ = s.db.InsertMemoryRecall(
		"",
		userID,
		agentID,
		convID,
		ctx.MessageID,
		asString(block["run_id"]),
		asString(block["langfuse_trace_id"]),
		source,
		asString(block["scene"]),
		explicit,
		mem0,
		items,
	)
}

func memoryRecallToJSON(item db.MemoryRecall) map[string]any {
	items := make([]map[string]any, 0, len(item.Items))
	for _, it := range item.Items {
		row := map[string]any{
			"source":         it.Source,
			"scope":          it.Scope,
			"tier":           it.Tier,
			"content":        it.Content,
			"snippet":        it.Snippet,
			"memory_id":      it.MemoryID,
			"agent_id":       it.AgentID,
			"channel_id":     it.ChannelID,
			"peer_agent_id":  it.PeerAgentID,
		}
		if it.Score != nil {
			row["score"] = *it.Score
		}
		items = append(items, row)
	}
	return map[string]any{
		"id":              item.ID,
		"org_id":          item.OrgID,
		"user_id":         item.UserID,
		"username":        item.Username,
		"agent_id":        item.AgentID,
		"agent_name":      item.AgentName,
		"conversation_id": item.ConversationID,
		"message_id":         item.MessageID,
		"run_id":             item.RunID,
		"langfuse_trace_id":  item.LangfuseTraceID,
		"source":             item.Source,
		"scene":           item.Scene,
		"explicit_count":  item.ExplicitCount,
		"mem0_count":      item.Mem0Count,
		"item_count":      item.ItemCount,
		"items":           items,
		"created_at":      item.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func (s *Server) handleAdminListMemoryRecalls(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.loadAuthUser(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	f := db.MemoryRecallFilter{
		UserID:          strings.TrimSpace(q.Get("user_id")),
		ConversationID:  strings.TrimSpace(q.Get("conversation_id")),
		AgentID:         strings.TrimSpace(q.Get("agent_id")),
		RunID:           strings.TrimSpace(q.Get("run_id")),
		LangfuseTraceID: strings.TrimSpace(q.Get("langfuse_trace_id")),
		Limit:           limit,
	}
	if raw := strings.TrimSpace(q.Get("from")); raw != "" {
		if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			f.From = t
		} else if t, err := time.Parse(time.RFC3339, raw); err == nil {
			f.From = t
		} else {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "from 需为 RFC3339"})
			return
		}
	}
	if raw := strings.TrimSpace(q.Get("to")); raw != "" {
		if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
			f.To = t
		} else if t, err := time.Parse(time.RFC3339, raw); err == nil {
			f.To = t
		} else {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "to 需为 RFC3339"})
			return
		}
	}
	if f.UserID != "" {
		u, err := s.db.GetUserByID(f.UserID)
		if err != nil || u.OrgID != admin.OrgID {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "目标用户不在本组织"})
			return
		}
	}
	if f.AgentID != "" {
		if _, err := s.db.GetAgentInOrg(f.AgentID, admin.OrgID); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "目标 Bot 不在本组织"})
			return
		}
	}
	list, err := s.db.ListOrgMemoryRecalls(admin.OrgID, f)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		out = append(out, memoryRecallToJSON(item))
	}
	writeJSON(w, http.StatusOK, map[string]any{"recalls": out})
}

func (s *Server) handleAdminGetMemoryRecall(w http.ResponseWriter, r *http.Request) {
	admin, ok := s.loadAuthUser(w, r)
	if !ok {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
		return
	}
	item, err := s.db.GetOrgMemoryRecall(admin.OrgID, id)
	if err != nil {
		if err == db.ErrNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"recall": memoryRecallToJSON(*item)})
}

func (s *Server) handleInternalRecordMemoryRecall(w http.ResponseWriter, r *http.Request) {
	var body struct {
		UserID           string                `json:"user_id"`
		AgentID          string                `json:"agent_id"`
		ConversationID   string                `json:"conversation_id"`
		MessageID        string                `json:"message_id"`
		RunID            string                `json:"run_id"`
		LangfuseTraceID  string                `json:"langfuse_trace_id"`
		Source           string                `json:"source"`
		Scene            string                `json:"scene"`
		ExplicitCount    int                   `json:"explicit_count"`
		Mem0Count        int                   `json:"mem0_count"`
		Items            []db.MemoryRecallItem `json:"items"`
		MemoryRecall     map[string]any        `json:"memory_recall"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	items := body.Items
	scene := body.Scene
	explicit := body.ExplicitCount
	mem0 := body.Mem0Count
	runID := body.RunID
	lfTraceID := body.LangfuseTraceID
	if body.MemoryRecall != nil {
		if len(items) == 0 {
			items = parseRecallItems(body.MemoryRecall["items"])
		}
		if scene == "" {
			scene = asString(body.MemoryRecall["scene"])
		}
		if explicit == 0 {
			explicit = asInt(body.MemoryRecall["explicit_count"])
		}
		if mem0 == 0 {
			mem0 = asInt(body.MemoryRecall["mem0_count"])
		}
		if runID == "" {
			runID = asString(body.MemoryRecall["run_id"])
		}
		if lfTraceID == "" {
			lfTraceID = asString(body.MemoryRecall["langfuse_trace_id"])
		}
	}
	rec, err := s.db.InsertMemoryRecall(
		"",
		body.UserID,
		body.AgentID,
		body.ConversationID,
		body.MessageID,
		runID,
		lfTraceID,
		body.Source,
		scene,
		explicit,
		mem0,
		items,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if rec == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "skipped": true})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": rec.ID})
}
