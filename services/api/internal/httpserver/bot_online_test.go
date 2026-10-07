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
	// a2 unbound => 任一在线 (any connected host lights it)

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
	if !got[a2.ID] {
		t.Fatalf("unbound bot must be online when any host is up: %v", got)
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

	// Create unbound while m2 still connected => 任一在线 → online.
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(map[string]any{"name": "另一个"})
	req = httptest.NewRequest(http.MethodPost, "/v1/agents", &buf)
	req = req.WithContext(context.WithValue(req.Context(), userIDKey, uid))
	rec = httptest.NewRecorder()
	s.handleCreateAgent(rec, req)
	var created map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &created)
	if created["online"] != true {
		t.Fatalf("create unbound with live host online=%v", created["online"])
	}
}

func TestBotOnlineFanoutBoundAndUnbound(t *testing.T) {
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
	// aOther unbound → 任一在线: also lights when this host connects.

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
		var evt map[string]any
		_ = json.Unmarshal(raw, &evt)
		if evt["type"] != "bot_online" || evt["agent_id"] != aOther.ID || evt["online"] != true {
			t.Fatalf("unbound connect evt=%v", evt)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no bot_online on connect for unbound agent")
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

// Acceptance: host socket still Connected but heartbeat stops → after
// MachineOfflineAfter the sweep pushes bot_online offline without any
// ListAgents refresh; heartbeat resume while Connected → online again.
func TestBotOnlineStaleHeartbeatSweepPublishes(t *testing.T) {
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
VALUES ($1,$2,'x',$3,$4,$5)`, uid, "online-stale-"+uid[:8], org.ID, db.RoleMember, db.Now()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = d.SQL.Exec(`DELETE FROM agents WHERE user_id=$1`, uid)
		_, _ = d.SQL.Exec(`DELETE FROM user_machines WHERE user_id=$1`, uid)
		_, _ = d.SQL.Exec(`DELETE FROM users WHERE id=$1`, uid)
	})

	s := &Server{db: d, events: newChatHub(), convEvents: newConversationEventHub(), hosts: newHostHub()}
	aStale, err := d.CreateAgentWithAvatar(uid, "StaleBot", "", "", "drop", "#9b5de5")
	if err != nil {
		t.Fatal(err)
	}
	aFresh, err := d.CreateAgentWithAvatar(uid, "FreshBot", "", "", "cloud", "#118ab2")
	if err != nil {
		t.Fatal(err)
	}
	mStale, err := d.RegisterMachine(uid, db.MachineRegisterInput{MachineKey: "stale-" + uid[:8], Label: "Frozen Mac", Platform: "macos"})
	if err != nil {
		t.Fatal(err)
	}
	mFresh, err := d.RegisterMachine(uid, db.MachineRegisterInput{MachineKey: "fresh-" + uid[:8], Label: "Live Mac", Platform: "macos"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.SetAgentMachineID(uid, aStale.ID, mStale.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SetAgentMachineID(uid, aFresh.ID, mFresh.ID); err != nil {
		t.Fatal(err)
	}
	convStale, err := d.GetOrCreatePrimaryConversation(uid, aStale.ID)
	if err != nil {
		t.Fatal(err)
	}
	convFresh, err := d.GetOrCreatePrimaryConversation(uid, aFresh.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Both sockets Connected; no onChange hook (simulates an API restart where
	// the client only learned "online" from ListAgents).
	s.hosts.mu.Lock()
	s.hosts.sessions[hostKey(uid, mStale.ID)] = &hostSession{userID: uid, machineID: mStale.ID}
	s.hosts.sessions[hostKey(uid, mFresh.ID)] = &hostSession{userID: uid, machineID: mFresh.ID}
	s.hosts.mu.Unlock()

	req := httptest.NewRequest(http.MethodGet, "/v1/agents", nil)
	req = req.WithContext(context.WithValue(req.Context(), userIDKey, uid))
	rec := httptest.NewRecorder()
	s.handleListAgents(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list code=%d", rec.Code)
	}

	chStale := s.convEvents.subscribe(convStale.ID)
	defer s.convEvents.unsubscribe(convStale.ID, chStale)
	chFresh := s.convEvents.subscribe(convFresh.ID)
	defer s.convEvents.unsubscribe(convFresh.ID, chFresh)

	expect := func(ch chan []byte, want bool, label string) {
		t.Helper()
		select {
		case raw := <-ch:
			var evt map[string]any
			_ = json.Unmarshal(raw, &evt)
			if evt["type"] != "bot_online" || evt["agent_id"] != aStale.ID || evt["online"] != want {
				t.Fatalf("%s: evt=%v", label, evt)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s: no bot_online", label)
		}
	}
	expectNone := func(ch chan []byte, label string) {
		t.Helper()
		select {
		case raw := <-ch:
			t.Fatalf("%s: unexpected %s", label, raw)
		case <-time.After(150 * time.Millisecond):
		}
	}

	// Fresh sweep: nothing flipped.
	s.sweepBotOnline()
	expectNone(chStale, "fresh sweep stale-bot")
	expectNone(chFresh, "fresh sweep fresh-bot")

	// Heartbeat stops: last_seen ages past 90s; socket still Connected.
	old := db.Now().Add(-(db.MachineOfflineAfter + 30*time.Second))
	if _, err := d.SQL.Exec(`UPDATE user_machines SET last_seen=$1 WHERE id=$2`, old, mStale.ID); err != nil {
		t.Fatal(err)
	}
	if !s.hosts.Connected(uid, mStale.ID) {
		t.Fatal("precondition: socket still connected")
	}
	s.sweepBotOnline()
	expect(chStale, false, "stale → offline push")
	expectNone(chFresh, "other machine's bot untouched")

	// Second sweep: no duplicate.
	s.sweepBotOnline()
	expectNone(chStale, "no duplicate offline")

	// Heartbeat resumes while still Connected → online push.
	if _, err := d.HeartbeatMachine(uid, mStale.ID); err != nil {
		t.Fatal(err)
	}
	s.sweepBotOnline()
	expect(chStale, true, "heartbeat resume → online push")
}

// Unseen agent (never served/published) on a Connected-but-stale socket still
// gets an offline push from the sweep.
func TestBotOnlineStaleSweepUnseenAgent(t *testing.T) {
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
VALUES ($1,$2,'x',$3,$4,$5)`, uid, "online-unseen-"+uid[:8], org.ID, db.RoleMember, db.Now()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = d.SQL.Exec(`DELETE FROM agents WHERE user_id=$1`, uid)
		_, _ = d.SQL.Exec(`DELETE FROM user_machines WHERE user_id=$1`, uid)
		_, _ = d.SQL.Exec(`DELETE FROM users WHERE id=$1`, uid)
	})
	s := &Server{db: d, events: newChatHub(), convEvents: newConversationEventHub(), hosts: newHostHub()}
	a, err := d.CreateAgentWithAvatar(uid, "UnseenBot", "", "", "drop", "#9b5de5")
	if err != nil {
		t.Fatal(err)
	}
	m, err := d.RegisterMachine(uid, db.MachineRegisterInput{MachineKey: "unseen-" + uid[:8], Label: "Mac", Platform: "macos"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.SetAgentMachineID(uid, a.ID, m.ID); err != nil {
		t.Fatal(err)
	}
	conv, err := d.GetOrCreatePrimaryConversation(uid, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.SQL.Exec(`UPDATE user_machines SET last_seen=$1 WHERE id=$2`,
		db.Now().Add(-(db.MachineOfflineAfter + time.Minute)), m.ID); err != nil {
		t.Fatal(err)
	}
	s.hosts.mu.Lock()
	s.hosts.sessions[hostKey(uid, m.ID)] = &hostSession{userID: uid, machineID: m.ID}
	s.hosts.mu.Unlock()
	ch := s.convEvents.subscribe(conv.ID)
	defer s.convEvents.unsubscribe(conv.ID, ch)

	s.sweepBotOnline()
	select {
	case raw := <-ch:
		var evt map[string]any
		_ = json.Unmarshal(raw, &evt)
		if evt["agent_id"] != a.ID || evt["online"] != false {
			t.Fatalf("evt=%v", evt)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no offline push for unseen stale agent")
	}
	s.sweepBotOnline()
	select {
	case raw := <-ch:
		t.Fatalf("duplicate %s", raw)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestAgentHostOnlineSessionOverridesPreferred(t *testing.T) {
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
VALUES ($1,$2,'x',$3,$4,$5)`, uid, "online-sess-"+uid[:8], org.ID, db.RoleMember, db.Now()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = d.SQL.Exec(`DELETE FROM users WHERE id=$1`, uid) })

	s := &Server{db: d, events: newChatHub(), convEvents: newConversationEventHub(), hosts: newHostHub()}
	a, err := d.CreateAgentWithAvatar(uid, "SessBot", "", "", "drop", "#9b5de5")
	if err != nil {
		t.Fatal(err)
	}
	mPref, err := d.RegisterMachine(uid, db.MachineRegisterInput{
		MachineKey: "pref-" + uid[:8], Label: "Pref", Platform: "macos",
	})
	if err != nil {
		t.Fatal(err)
	}
	mSess, err := d.RegisterMachine(uid, db.MachineRegisterInput{
		MachineKey: "sess-" + uid[:8], Label: "Sess", Platform: "linux",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.SetAgentMachineID(uid, a.ID, mPref.ID); err != nil {
		t.Fatal(err)
	}
	a, _ = d.GetAgent(uid, a.ID)

	// Prefer online, session host offline → session wins (must not 误亮).
	s.hosts.mu.Lock()
	s.hosts.sessions[hostKey(uid, mPref.ID)] = &hostSession{userID: uid, machineID: mPref.ID}
	s.hosts.mu.Unlock()
	if !s.agentHostOnline(uid, a) {
		t.Fatal("preferred host up → ListAgents online")
	}
	if s.agentHostOnlineSession(uid, a, mSess.ID) {
		t.Fatal("session host down → conversation online must be false")
	}

	// Session host up → online even if we only check session id.
	s.hosts.mu.Lock()
	s.hosts.sessions[hostKey(uid, mSess.ID)] = &hostSession{userID: uid, machineID: mSess.ID}
	delete(s.hosts.sessions, hostKey(uid, mPref.ID))
	s.hosts.mu.Unlock()
	if !s.agentHostOnlineSession(uid, a, mSess.ID) {
		t.Fatal("session host up → online")
	}
	if s.agentHostOnline(uid, a) {
		t.Fatal("preferred down, no session arg → ListAgents offline")
	}
}
