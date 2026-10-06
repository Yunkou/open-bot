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

func TestMachineHostOnline(t *testing.T) {
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		m    db.Machine
		want bool
	}{
		{
			name: "connected_fresh",
			m:    db.Machine{Connected: true, LastSeen: now.Add(-30 * time.Second)},
			want: true,
		},
		{
			name: "connected_at_boundary",
			m:    db.Machine{Connected: true, LastSeen: now.Add(-db.MachineOfflineAfter)},
			want: true,
		},
		{
			name: "connected_stale",
			m:    db.Machine{Connected: true, LastSeen: now.Add(-db.MachineOfflineAfter - time.Second)},
			want: false,
		},
		{
			name: "disconnected_fresh",
			m:    db.Machine{Connected: false, LastSeen: now},
			want: false,
		},
		{
			name: "connected_zero_last_seen",
			m:    db.Machine{Connected: true},
			want: false,
		},
	}
	if db.MachineOfflineAfter != 90*time.Second {
		t.Fatalf("MachineOfflineAfter=%v want 90s", db.MachineOfflineAfter)
	}
	for _, tc := range cases {
		if got := machineHostOnline(tc.m, now); got != tc.want {
			t.Fatalf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestPublishBotOnlineEventShape(t *testing.T) {
	s := &Server{
		events:     newChatHub(),
		convEvents: newConversationEventHub(),
	}
	convID := "conv-bot-online-1"
	agentID := "agent-1"
	ch := s.convEvents.subscribe(convID)
	defer s.convEvents.unsubscribe(convID, ch)

	client := &chatClient{userID: "user-1", hub: s.events, send: make(chan []byte, 4)}
	s.events.register(client)
	defer s.events.unregister(client)

	s.publishBotOnline("user-1", agentID, true)

	select {
	case raw := <-client.send:
		var evt map[string]any
		if err := json.Unmarshal(raw, &evt); err != nil {
			t.Fatal(err)
		}
		if evt["type"] != "bot_online" || evt["agent_id"] != agentID || evt["online"] != true {
			t.Fatalf("chat ws evt=%v", evt)
		}
		if _, ok := evt["updated_at"].(string); !ok || evt["updated_at"] == "" {
			t.Fatalf("missing updated_at: %v", evt)
		}
	case <-time.After(time.Second):
		t.Fatal("no chat ws bot_online")
	}

	payload, _ := json.Marshal(map[string]any{
		"type": "bot_online", "conversation_id": convID, "agent_id": agentID, "online": false,
	})
	s.convEvents.Publish(convID, payload)
	select {
	case raw := <-ch:
		var evt map[string]any
		if err := json.Unmarshal(raw, &evt); err != nil {
			t.Fatal(err)
		}
		if evt["type"] != "bot_online" || evt["agent_id"] != agentID || evt["online"] != false {
			t.Fatalf("sse evt=%v", evt)
		}
	case <-time.After(time.Second):
		t.Fatal("no conv sse bot_online")
	}
}

func TestListAgentsOnlinePerBotBinding(t *testing.T) {
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
VALUES ($1,$2,'x',$3,$4,$5)`, uid, "online-http-"+uid[:8], org.ID, db.RoleMember, db.Now()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = d.SQL.Exec(`DELETE FROM users WHERE id=$1`, uid) })

	s := &Server{db: d, events: newChatHub(), convEvents: newConversationEventHub(), hosts: newHostHub()}
	a1, err := d.CreateAgentWithAvatar(uid, "BotA", "", "", "cloud", "#118ab2")
	if err != nil {
		t.Fatal(err)
	}
	a2, err := d.CreateAgentWithAvatar(uid, "BotB", "", "", "drop", "#9b5de5")
	if err != nil {
		t.Fatal(err)
	}

	m1, err := d.RegisterMachine(uid, db.MachineRegisterInput{
		MachineKey: "k1-" + uid[:8], Label: "Mac", Platform: "macos",
	})
	if err != nil {
		t.Fatal(err)
	}
	m2, err := d.RegisterMachine(uid, db.MachineRegisterInput{
		MachineKey: "k2-" + uid[:8], Label: "Linux", Platform: "linux",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.SetAgentMachineID(uid, a1.ID, m1.ID); err != nil {
		t.Fatal(err)
	}
	// a2 unbound => offline even if some user machine is connected

	s.hosts.mu.Lock()
	s.hosts.sessions[hostKey(uid, m1.ID)] = &hostSession{userID: uid, machineID: m1.ID}
	s.hosts.sessions[hostKey(uid, m2.ID)] = &hostSession{userID: uid, machineID: m2.ID}
	s.hosts.mu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/v1/agents", nil)
	req = req.WithContext(context.WithValue(req.Context(), userIDKey, uid))
	rec := httptest.NewRecorder()
	s.handleListAgents(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	mids := map[string]string{}
	for _, raw := range out["agents"].([]any) {
		item := raw.(map[string]any)
		id, _ := item["id"].(string)
		on, _ := item["online"].(bool)
		got[id] = on
		if mid, ok := item["machine_id"].(string); ok {
			mids[id] = mid
		}
	}
	if !got[a1.ID] {
		t.Fatalf("bound connected bot want online: %v", got)
	}
	if got[a2.ID] {
		t.Fatalf("unbound bot must be offline: %v", got)
	}
	if mids[a1.ID] != m1.ID {
		t.Fatalf("a1 machine_id=%q want %q", mids[a1.ID], m1.ID)
	}
	if mids[a2.ID] != "" {
		t.Fatalf("a2 machine_id should be empty, got %q", mids[a2.ID])
	}

	// Bind a2 to m2, then disconnect only m1 => a1 offline, a2 online.
	if _, err := d.SetAgentMachineID(uid, a2.ID, m2.ID); err != nil {
		t.Fatal(err)
	}
	s.hosts.mu.Lock()
	delete(s.hosts.sessions, hostKey(uid, m1.ID))
	s.hosts.mu.Unlock()

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v1/agents", nil)
	req = req.WithContext(context.WithValue(req.Context(), userIDKey, uid))
	s.handleListAgents(rec, req)
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	got = map[string]bool{}
	for _, raw := range out["agents"].([]any) {
		item := raw.(map[string]any)
		got[item["id"].(string)] = item["online"].(bool)
	}
	if got[a1.ID] {
		t.Fatalf("a1 should be offline after its channel drop: %v", got)
	}
	if !got[a2.ID] {
		t.Fatalf("a2 should stay online on m2: %v", got)
	}

	// PATCH bind on create path: create unbound stays offline.
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(map[string]any{"name": "另一个"})
	req = httptest.NewRequest(http.MethodPost, "/v1/agents", &buf)
	req = req.WithContext(context.WithValue(req.Context(), userIDKey, uid))
	rec = httptest.NewRecorder()
	s.handleCreateAgent(rec, req)
	var created map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created["online"] != false {
		t.Fatalf("create unbound online=%v", created["online"])
	}
}

func TestBotOnlineFanoutOnlyBoundAgent(t *testing.T) {
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
VALUES ($1,$2,'x',$3,$4,$5)`, uid, "online-sse-"+uid[:8], org.ID, db.RoleMember, db.Now()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = d.SQL.Exec(`DELETE FROM users WHERE id=$1`, uid) })

	s := &Server{db: d, events: newChatHub(), convEvents: newConversationEventHub(), hosts: newHostHub()}
	s.hosts.onChange = func(userID, machineID string, _ bool) {
		s.publishAgentsOnlineForMachine(userID, machineID)
	}
	aBound, err := d.CreateAgentWithAvatar(uid, "BoundBot", "", "", "drop", "#9b5de5")
	if err != nil {
		t.Fatal(err)
	}
	aOther, err := d.CreateAgentWithAvatar(uid, "OtherBot", "", "", "cloud", "#118ab2")
	if err != nil {
		t.Fatal(err)
	}
	convBound, err := d.GetOrCreatePrimaryConversation(uid, aBound.ID)
	if err != nil {
		t.Fatal(err)
	}
	convOther, err := d.GetOrCreatePrimaryConversation(uid, aOther.ID)
	if err != nil {
		t.Fatal(err)
	}
	m, err := d.RegisterMachine(uid, db.MachineRegisterInput{
		MachineKey: "sse-" + uid[:8], Label: "Mac", Platform: "macos",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.SetAgentMachineID(uid, aBound.ID, m.ID); err != nil {
		t.Fatal(err)
	}
	// aOther intentionally unbound / different — must not light up.

	chBound := s.convEvents.subscribe(convBound.ID)
	defer s.convEvents.unsubscribe(convBound.ID, chBound)
	chOther := s.convEvents.subscribe(convOther.ID)
	defer s.convEvents.unsubscribe(convOther.ID, chOther)

	sess := &hostSession{userID: uid, machineID: m.ID, pending: map[string]chan map[string]any{}}
	s.hosts.register(sess)

	select {
	case raw := <-chBound:
		var evt map[string]any
		_ = json.Unmarshal(raw, &evt)
		if evt["type"] != "bot_online" || evt["agent_id"] != aBound.ID || evt["online"] != true {
			t.Fatalf("connect evt=%v", evt)
		}
		if evt["conversation_id"] != convBound.ID {
			t.Fatalf("conversation_id=%v", evt["conversation_id"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no bot_online on connect for bound agent")
	}

	select {
	case raw := <-chOther:
		t.Fatalf("unbound agent must not receive bot_online, got %s", raw)
	case <-time.After(200 * time.Millisecond):
	}

	s.hosts.unregister(sess)
	select {
	case raw := <-chBound:
		var evt map[string]any
		_ = json.Unmarshal(raw, &evt)
		if evt["type"] != "bot_online" || evt["online"] != false {
			t.Fatalf("disconnect evt=%v", evt)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no bot_online on disconnect")
	}
}
