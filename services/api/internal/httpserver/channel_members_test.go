package httpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/tangxin/open-bot/services/api/internal/db"
)

func TestRemoveChannelMemberHTTP(t *testing.T) {
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
VALUES ($1,$2,'x',$3,$4,$5)`, uid, "chmem-http-"+uid[:8], org.ID, db.RoleMember, db.Now()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = d.SQL.Exec(`DELETE FROM users WHERE id=$1`, uid) })

	s := &Server{db: d, events: newChatHub(), convEvents: newConversationEventHub()}

	lead, err := d.CreateAgent(uid, "lead", "", "")
	if err != nil {
		t.Fatal(err)
	}
	other, err := d.CreateAgent(uid, "other", "", "")
	if err != nil {
		t.Fatal(err)
	}
	ch, err := d.CreateChannel(uid, "HTTP群")
	if err != nil {
		t.Fatal(err)
	}
	_ = d.AddChannelMember(uid, ch.ID, lead.ID)
	_ = d.AddChannelMember(uid, ch.ID, other.ID)
	if _, err := d.EnsureChannelConversation(uid, ch.ID, lead.ID, ch.Name); err != nil {
		t.Fatal(err)
	}

	doDelete := func(channelID, agentID string) (*httptest.ResponseRecorder, map[string]any) {
		req := httptest.NewRequest(http.MethodDelete, "/v1/channels/"+channelID+"/members/"+agentID, nil)
		req.SetPathValue("id", channelID)
		req.SetPathValue("agentId", agentID)
		req = req.WithContext(context.WithValue(req.Context(), userIDKey, uid))
		rec := httptest.NewRecorder()
		s.handleRemoveChannelMember(rec, req)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec, out
	}

	// Success: remove non-lead → 200 channel with members.
	rec, out := doDelete(ch.ID, other.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("remove other code=%d body=%v", rec.Code, out)
	}
	members, _ := out["members"].([]any)
	if len(members) != 1 || members[0] != lead.ID {
		t.Fatalf("want only lead in members, got %v", out["members"])
	}

	// 400 last member.
	rec, out = doDelete(ch.ID, lead.ID)
	if rec.Code != http.StatusBadRequest || out["error"] != "群里至少保留一位成员" {
		t.Fatalf("last member code=%d err=%v", rec.Code, out["error"])
	}

	// Re-add other + try remove lead while 2 remain.
	_ = d.AddChannelMember(uid, ch.ID, other.ID)
	rec, out = doDelete(ch.ID, lead.ID)
	if rec.Code != http.StatusBadRequest || out["error"] != "不能移除主助理" {
		t.Fatalf("lead code=%d err=%v", rec.Code, out["error"])
	}

	// 404 missing member.
	rec, out = doDelete(ch.ID, uuid.NewString())
	if rec.Code != http.StatusNotFound || out["error"] != "not found" {
		t.Fatalf("missing member code=%d err=%v", rec.Code, out["error"])
	}

	// 404 missing channel.
	rec, out = doDelete(uuid.NewString(), other.ID)
	if rec.Code != http.StatusNotFound || out["error"] != "not found" {
		t.Fatalf("missing channel code=%d err=%v", rec.Code, out["error"])
	}
}
