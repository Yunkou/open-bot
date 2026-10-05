package httpserver

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/tangxin/open-bot/services/api/internal/db"
)

const conversationSSEBuffer = 32

// conversationEventHub fans conversation-scoped SSE frames (e.g. reaction_updated)
// to GET /v1/conversations/{id}/events subscribers. Keyed by conversationID.
type conversationEventHub struct {
	mu      sync.RWMutex
	clients map[string]map[chan []byte]struct{}
}

func newConversationEventHub() *conversationEventHub {
	return &conversationEventHub{clients: make(map[string]map[chan []byte]struct{})}
}

func (h *conversationEventHub) subscribe(conversationID string) chan []byte {
	ch := make(chan []byte, conversationSSEBuffer)
	h.mu.Lock()
	defer h.mu.Unlock()
	set := h.clients[conversationID]
	if set == nil {
		set = make(map[chan []byte]struct{})
		h.clients[conversationID] = set
	}
	set[ch] = struct{}{}
	return ch
}

func (h *conversationEventHub) unsubscribe(conversationID string, ch chan []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	set, ok := h.clients[conversationID]
	if !ok {
		return
	}
	if _, exists := set[ch]; !exists {
		return
	}
	delete(set, ch)
	close(ch)
	if len(set) == 0 {
		delete(h.clients, conversationID)
	}
}

func (h *conversationEventHub) Publish(conversationID string, payload []byte) {
	if h == nil || conversationID == "" || len(payload) == 0 {
		return
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for ch := range h.clients[conversationID] {
		select {
		case ch <- payload:
		default:
			log.Printf("conversation sse drop slow client conv=%s", conversationID)
		}
	}
}

type reactionBody struct {
	Emoji string `json:"emoji"`
}

func (s *Server) handleToggleReaction(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	messageID := r.PathValue("id")
	var body reactionBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	ref, err := s.db.GetMessageOwned(uid, messageID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "message not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	action, count, me, err := s.db.ToggleReaction(uid, messageID, body.Emoji)
	if err != nil {
		if errors.Is(err, db.ErrInvalidEmoji) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "emoji not allowed"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	evt := map[string]any{
		"type":            "reaction_updated",
		"conversation_id": ref.ConversationID,
		"message_id":      messageID,
		"emoji":           strings.TrimSpace(body.Emoji),
		"count":           count,
		"me":              me,
		"action":          action,
	}
	s.publishReactionEvent(ref.UserID, ref.ConversationID, evt)
	writeJSON(w, http.StatusOK, evt)
}

func (s *Server) handleDeleteReaction(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	messageID := r.PathValue("id")
	emoji := strings.TrimSpace(r.URL.Query().Get("emoji"))
	if emoji == "" {
		var body reactionBody
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			emoji = strings.TrimSpace(body.Emoji)
		}
	}
	if emoji == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "emoji required"})
		return
	}
	ref, err := s.db.GetMessageOwned(uid, messageID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "message not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	_, count, err := s.db.RemoveReaction(uid, messageID, emoji)
	if err != nil {
		if errors.Is(err, db.ErrInvalidEmoji) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "emoji not allowed"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	evt := map[string]any{
		"type":            "reaction_updated",
		"conversation_id": ref.ConversationID,
		"message_id":      messageID,
		"emoji":           emoji,
		"count":           count,
		"me":              false,
		"action":          "remove",
	}
	s.publishReactionEvent(ref.UserID, ref.ConversationID, evt)
	writeJSON(w, http.StatusOK, evt)
}

// publishReactionEvent fans reaction_updated to conversation SSE (source of truth)
// and keeps chat WS publish for current web clients until they migrate.
func (s *Server) publishReactionEvent(userID, conversationID string, evt map[string]any) {
	if s.convEvents != nil && strings.TrimSpace(conversationID) != "" {
		if payload, err := json.Marshal(evt); err == nil {
			s.convEvents.Publish(conversationID, payload)
		}
	}
	if s.events != nil && strings.TrimSpace(userID) != "" {
		s.events.Publish(userID, evt)
	}
}
