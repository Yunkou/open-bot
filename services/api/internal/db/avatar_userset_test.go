package db

import (
	"testing"

	"github.com/google/uuid"
)

// avatar_user_set: auto-assign (create/clone/admin) → false; explicit create / PATCH → true.
// List/Get are read-only; only migrateAvatarV2 backfills empty avatars (avoiding taken pairs).
func TestAvatarUserSetAndReadOnlyListAndBackfill(t *testing.T) {
	d, err := Open("")
	if err != nil {
		t.Skip(err)
	}
	defer d.Close()

	org, err := d.EnsureDefaultOrg()
	if err != nil {
		t.Fatal(err)
	}
	userID := uuid.NewString()
	if _, err := d.SQL.Exec(
		`INSERT INTO users (id, username, password_hash, org_id, role) VALUES ($1, $2, 'x', $3, 'member')`,
		userID, "avatar-us-"+userID[:8], org.ID,
	); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = d.SQL.Exec(`DELETE FROM agent_skills WHERE agent_id IN (SELECT id FROM agents WHERE user_id = $1)`, userID)
		_, _ = d.SQL.Exec(`DELETE FROM agents WHERE user_id = $1`, userID)
		_, _ = d.SQL.Exec(`DELETE FROM users WHERE id = $1`, userID)
	})

	userSetOf := func(id string) bool {
		var v bool
		if err := d.SQL.QueryRow(`SELECT avatar_user_set FROM agents WHERE id=$1`, id).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}

	auto, err := d.CreateAgent(userID, "auto", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if auto.AvatarUserSet || userSetOf(auto.ID) || !IsAllowedAvatarShape(auto.AvatarShape) {
		t.Fatalf("auto create: user_set must be false, v2 shape; got %+v", auto)
	}
	explicit, err := d.CreateAgentWithAvatar(userID, "explicit", "", "", "petal", "#118ab2")
	if err != nil {
		t.Fatal(err)
	}
	if !explicit.AvatarUserSet || !userSetOf(explicit.ID) {
		t.Fatal("explicit create avatar: user_set must be true")
	}
	if _, err := d.CreateAgentWithAvatar(userID, "legacy-in", "", "", "circle", ""); err == nil {
		t.Fatal("legacy v1 shape id must be rejected on create (whitelist is v2 only)")
	}
	clone, err := d.CloneAgent(userID, explicit.ID, CloneAgentOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if clone.Agent.AvatarUserSet || userSetOf(clone.Agent.ID) {
		t.Fatal("clone: user_set must be false")
	}
	admin, err := d.CreateAgentFull(userID, "admin", "", "", "team")
	if err != nil {
		t.Fatal(err)
	}
	if admin.AvatarUserSet || userSetOf(admin.ID) {
		t.Fatal("admin create: user_set must be false")
	}
	patched, err := d.UpdateAgentAvatar(userID, auto.ID, "cloud", "")
	if err != nil {
		t.Fatal(err)
	}
	if !patched.AvatarUserSet || !userSetOf(auto.ID) {
		t.Fatal("PATCH avatar: user_set must be true")
	}

	// Legacy empty row + colliding user pick (same pair as explicit, user_set=true).
	emptyID, dupID := uuid.NewString(), uuid.NewString()
	if _, err := d.SQL.Exec(
		`INSERT INTO agents (id, user_id, name, description, system_prompt, computer_mode, avatar_shape, avatar_color, avatar_user_set, created_at, updated_at)
		 VALUES ($1,$2,'empty','','','team','','',FALSE,NOW(),NOW()),
		        ($3,$2,'dup','','','team','petal','#118ab2',TRUE,NOW(),NOW())`,
		emptyID, userID, dupID,
	); err != nil {
		t.Fatal(err)
	}

	// List + Get must not write.
	list, err := d.ListAgents(userID)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range list {
		if a.ID == emptyID && (a.AvatarShape != "" || a.AvatarColor != "") {
			t.Fatalf("list must not invent avatar: %+v", a)
		}
	}
	if _, err := d.GetAgent(userID, emptyID); err != nil {
		t.Fatal(err)
	}
	var s, c string
	if err := d.SQL.QueryRow(`SELECT avatar_shape, avatar_color FROM agents WHERE id=$1`, emptyID).Scan(&s, &c); err != nil {
		t.Fatal(err)
	}
	if s != "" || c != "" {
		t.Fatalf("list/get wrote avatar: %s|%s", s, c)
	}

	if err := d.migrateAvatarV2(); err != nil {
		t.Fatal(err)
	}
	rows, err := d.SQL.Query(`SELECT id, avatar_shape, avatar_color, avatar_user_set FROM agents WHERE user_id=$1 AND deleted_at IS NULL`, userID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	pairs := map[string][]string{}
	var emptyPair string
	for rows.Next() {
		var id, sh, co string
		var us bool
		if err := rows.Scan(&id, &sh, &co, &us); err != nil {
			t.Fatal(err)
		}
		if !IsAllowedAvatarShape(sh) || co == "" {
			t.Fatalf("agent %s has invalid avatar after migrate: %q %q", id, sh, co)
		}
		pairs[sh+"|"+co] = append(pairs[sh+"|"+co], id)
		switch id {
		case emptyID:
			emptyPair = sh + "|" + co
			if us {
				t.Fatal("backfilled avatar must stay user_set=false")
			}
		case dupID:
			if !us || sh != "petal" || co != "#118ab2" {
				t.Fatalf("user-set colliding avatar must be kept: %s|%s us=%v", sh, co, us)
			}
		case clone.Agent.ID, admin.ID:
			if us {
				t.Fatal("re-running migrate must not flip auto-assigned rows to user_set")
			}
		}
	}
	if len(pairs[emptyPair]) != 1 {
		t.Fatalf("backfill picked a taken pair %s: %v", emptyPair, pairs[emptyPair])
	}
	if len(pairs["petal|#118ab2"]) != 2 {
		t.Fatalf("expected kept collision on petal|#118ab2, got %v", pairs["petal|#118ab2"])
	}
}
