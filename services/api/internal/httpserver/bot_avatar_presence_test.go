package httpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tangxin/open-bot/services/api/internal/db"
)

func TestBotAvatarCreateListPatchAndPresence(t *testing.T) {
	d, err := db.Open("")
	if err != nil {
		t.Skip(err)
	}
	defer d.Close()
	org, err := d.EnsureDefaultOrg()
	if err != nil {
		t.Fatal(err)
	}
	uid := uuid.NewString()
	if _, err := d.SQL.Exec(`INSERT INTO users (id, username, password_hash, org_id, role, created_at)
VALUES ($1,$2,'x',$3,$4,$5)`, uid, "avatar-http-"+uid[:8], org.ID, db.RoleMember, db.Now()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = d.SQL.Exec(`DELETE FROM users WHERE id=$1`, uid) })
	s := &Server{db: d, events: newChatHub(), convEvents: newConversationEventHub()}

	do := func(h http.HandlerFunc, method, path, id string, body any) (*httptest.ResponseRecorder, map[string]any) {
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		req := httptest.NewRequest(method, path, &buf)
		if id != "" {
			req.SetPathValue("id", id)
		}
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, uid))
		rec := httptest.NewRecorder()
		h(rec, req)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec, out
	}

	// 1. Create with avatar in body (no PATCH fallback).
	rec, out := do(s.handleCreateAgent, http.MethodPost, "/v1/agents", "", map[string]any{
		"name": "形象Bot", "avatar_shape": "hex", "avatar_color": "#9B5DE5",
	})
	if rec.Code != http.StatusCreated || out["avatar_shape"] != "hex" || out["avatar_color"] != "#9b5de5" {
		t.Fatalf("create code=%d out=%v", rec.Code, out)
	}
	agentID, _ := out["id"].(string)
	rec, _ = do(s.handleCreateAgent, http.MethodPost, "/v1/agents", "", map[string]any{"name": "bad", "avatar_shape": "star"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("create with bad shape code=%d", rec.Code)
	}
	// Create without avatar -> auto-assigned within whitelist.
	_, out = do(s.handleCreateAgent, http.MethodPost, "/v1/agents", "", map[string]any{"name": "自动Bot"})
	if sh, _ := out["avatar_shape"].(string); !db.IsAllowedAvatarShape(sh) {
		t.Fatalf("auto shape=%v", out["avatar_shape"])
	}

	// 2. List returns avatar_shape / avatar_color.
	_, out = do(s.handleListAgents, http.MethodGet, "/v1/agents", "", nil)
	found := false
	for _, raw := range out["agents"].([]any) {
		a := raw.(map[string]any)
		if a["id"] == agentID {
			found = a["avatar_shape"] == "hex" && a["avatar_color"] == "#9b5de5"
		}
		if a["avatar_shape"] == "" || a["avatar_color"] == "" {
			t.Fatalf("list missing avatar: %v", a)
		}
	}
	if !found {
		t.Fatalf("created agent avatar not in list: %v", out)
	}

	// 3. PATCH avatar-only keeps name; whitelist enforced.
	rec, out = do(s.handlePatchAgent, http.MethodPatch, "/v1/agents/"+agentID, agentID, map[string]any{
		"avatar_shape": "diamond", "avatar_color": "#118ab2",
	})
	if rec.Code != http.StatusOK || out["avatar_shape"] != "diamond" || out["avatar_color"] != "#118ab2" || out["name"] != "形象Bot" {
		t.Fatalf("patch code=%d out=%v", rec.Code, out)
	}
	rec, _ = do(s.handlePatchAgent, http.MethodPatch, "/v1/agents/"+agentID, agentID, map[string]any{"avatar_color": "#123456"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("patch bad color code=%d", rec.Code)
	}

	// 4. bot_presence: internal endpoint validates status and fans out on conversation SSE hub.
	conv, err := d.GetOrCreatePrimaryConversation(uid, agentID)
	if err != nil {
		t.Fatal(err)
	}
	ch := s.convEvents.subscribe(conv.ID)
	defer s.convEvents.unsubscribe(conv.ID, ch)
	for _, st := range []string{"working", "awaiting_approval", "error", "idle"} {
		rec, _ = do(s.handleInternalBotPresence, http.MethodPost, "/internal/bot-presence", "", map[string]any{
			"conversation_id": conv.ID, "agent_id": agentID, "user_id": uid, "status": st,
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("presence %s code=%d", st, rec.Code)
		}
		select {
		case payload := <-ch:
			var evt map[string]any
			_ = json.Unmarshal(payload, &evt)
			if evt["type"] != "bot_presence" || evt["status"] != st || evt["agent_id"] != agentID {
				t.Fatalf("evt=%v", evt)
			}
		case <-time.After(time.Second):
			t.Fatalf("no bot_presence for %s", st)
		}
	}
	rec, _ = do(s.handleInternalBotPresence, http.MethodPost, "/internal/bot-presence", "", map[string]any{
		"conversation_id": conv.ID, "agent_id": agentID, "status": "sleeping",
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid presence code=%d", rec.Code)
	}
}
