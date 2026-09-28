package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
)

// newSSEEmitter writes SSE to the initiating client when still connected and
// always fans out to runHandle subscribers (for refresh / reconnect).
// Client disconnect never cancels the run — only handle.cancel does.
func newSSEEmitter(w http.ResponseWriter, flusher http.Flusher, clientCtx context.Context, handle *runHandle) func(event string, data any) {
	var clientGone atomic.Bool
	go func() {
		<-clientCtx.Done()
		clientGone.Store(true)
	}()
	return func(event string, data any) {
		if handle != nil {
			handle.Publish(event, data)
		}
		if clientGone.Load() || w == nil || flusher == nil {
			return
		}
		if clientCtx.Err() != nil {
			clientGone.Store(true)
			return
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, mustJSON(data)); err != nil {
			clientGone.Store(true)
			return
		}
		flusher.Flush()
	}
}

func (s *Server) handleConversationRunStatus(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	id := r.PathValue("id")
	if _, err := s.db.EnsureConversation(uid, id, "", "会话 "+id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"active": s.runs.isActive(id)})
}

// handleConversationEvents lets a client rejoin an in-flight run after refresh
// or reconnect. Disconnecting this stream does not cancel the run.
func (s *Server) handleConversationEvents(w http.ResponseWriter, r *http.Request) {
	uid := userIDFrom(r.Context())
	id := r.PathValue("id")
	if _, err := s.db.EnsureConversation(uid, id, "", "会话 "+id); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	writeFrame := func(event string, data any) bool {
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, mustJSON(data)); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	handle := s.runs.get(id)
	if handle == nil {
		_ = writeFrame("done", map[string]any{"ok": true, "active": false})
		return
	}
	sub, catchup, okSub := handle.Subscribe()
	if !okSub || sub == nil {
		_ = writeFrame("done", map[string]any{"ok": true, "active": false})
		return
	}
	defer handle.Unsubscribe(sub)

	for _, ev := range catchup {
		if !writeFrame(ev.Event, ev.Data) {
			return
		}
	}

	for {
		select {
		case <-r.Context().Done():
			// Subscriber left; run continues on the server.
			return
		case ev, open := <-sub.ch:
			if !open {
				// Run finished; if terminal done/cancelled was already published,
				// client saw it. Otherwise signal completion so UI can refresh.
				_ = writeFrame("done", map[string]any{"ok": true, "active": false})
				return
			}
			if !writeFrame(ev.Event, ev.Data) {
				return
			}
			if ev.Event == "done" {
				return
			}
		}
	}
}
