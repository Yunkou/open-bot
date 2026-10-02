package db

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPurgeUserDataKeepsAccountAndWipesBusiness(t *testing.T) {
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
	username := "purge-" + userID[:8]
	email := username + "@example.com"
	hash := "hash-keep-me"
	if _, err := d.SQL.Exec(`
INSERT INTO users (id, username, password_hash, org_id, role, email, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
`, userID, username, hash, org.ID, RoleMember, email, Now()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = d.SQL.Exec(`DELETE FROM users WHERE id = $1`, userID)
	})

	agentID := uuid.NewString()
	convID := uuid.NewString()
	channelID := uuid.NewString()
	routineID := uuid.NewString()
	memID := uuid.NewString()
	taskID := uuid.NewString()
	machineID := uuid.NewString()
	secretID := uuid.NewString()
	mcpID := uuid.NewString()
	a2aID := uuid.NewString()
	inviteID := uuid.NewString()

	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := d.SQL.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}

	mustExec(`INSERT INTO agents (id, user_id, name, description, system_prompt, is_builtin, created_at, updated_at)
VALUES ($1,$2,'bot','','',FALSE,$3,$3)`, agentID, userID, Now())
	// Soft-deleted agent must also be hard-purged.
	softAgent := uuid.NewString()
	mustExec(`INSERT INTO agents (id, user_id, name, description, system_prompt, is_builtin, created_at, updated_at, deleted_at)
VALUES ($1,$2,'gone','','',FALSE,$3,$3,$3)`, softAgent, userID, Now())
	mustExec(`INSERT INTO agent_skills (agent_id, skill_name, enabled) VALUES ($1,'demo',TRUE)`, agentID)
	mustExec(`INSERT INTO conversations (id, user_id, agent_id, title) VALUES ($1,$2,$3,'t')`, convID, userID, agentID)
	mustExec(`INSERT INTO messages (id, conversation_id, role, content) VALUES ($1,$2,'user','hi')`, uuid.NewString(), convID)
	mustExec(`INSERT INTO conversation_tasks (id, user_id, conversation_id, agent_id, goal, status)
VALUES ($1,$2,$3,$4,'g','queued')`, uuid.NewString(), userID, convID, agentID)
	mustExec(`INSERT INTO channels (id, user_id, name) VALUES ($1,$2,'ch')`, channelID, userID)
	mustExec(`INSERT INTO channel_members (channel_id, agent_id) VALUES ($1,$2)`, channelID, agentID)
	mustExec(`INSERT INTO agent_messages (id, user_id, from_agent_id, body) VALUES ($1,$2,$3,'ping')`, uuid.NewString(), userID, agentID)
	mustExec(`INSERT INTO memories (id, user_id, tier, content) VALUES ($1,$2,'note','fact')`, memID, userID)
	mustExec(`INSERT INTO memory_recalls (id, org_id, user_id, agent_id, source, scene)
VALUES ($1,$2,$3,$4,'chat','test')`, uuid.NewString(), org.ID, userID, agentID)
	mustExec(`INSERT INTO usage_runs (org_id, user_id, agent_id, conversation_id, day, prompt_tokens, completion_tokens, total_tokens, source)
VALUES ($1,$2,$3,$4,CURRENT_DATE,1,2,3,'chat')`, org.ID, userID, agentID, convID)
	mustExec(`INSERT INTO routines (id, user_id, name, prompt, schedule_cron, enabled)
VALUES ($1,$2,'r','p','0 9 * * *',TRUE)`, routineID, userID)
	mustExec(`INSERT INTO routine_runs (id, routine_id, status) VALUES ($1,$2,'pending')`, uuid.NewString(), routineID)
	mustExec(`INSERT INTO user_machines (id, user_id, machine_key, label) VALUES ($1,$2,'k','Mac')`, machineID, userID)
	mustExec(`INSERT INTO bot_secrets (id, user_id, agent_id, name, ciphertext) VALUES ($1,$2,$3,'tok','x')`, secretID, userID, agentID)
	mustExec(`INSERT INTO bot_secret_requests (id, user_id, agent_id, name, status) VALUES ($1,$2,$3,'tok','pending')`, uuid.NewString(), userID, agentID)
	mustExec(`INSERT INTO mcp_servers (id, user_id, name) VALUES ($1,$2,'mcp')`, mcpID, userID)
	mustExec(`INSERT INTO sandboxes (id, user_id, status) VALUES ($1,$2,'stopped')`, uuid.NewString(), userID)
	mustExec(`INSERT INTO inbound_hooks (id, user_id, provider, token) VALUES ($1,$2,'gh',$3)`, uuid.NewString(), userID, "tok-"+userID[:8])
	mustExec(`INSERT INTO llm_connections (id, user_id, name) VALUES ($1,$2,'llm')`, uuid.NewString(), userID)
	mustExec(`INSERT INTO user_skills (user_id, skill_name, enabled) VALUES ($1,'demo',TRUE)`, userID)
	mustExec(`INSERT INTO user_skill_files (user_id, skill_name, description, body_markdown) VALUES ($1,'custom','','# x')`, userID)
	mustExec(`INSERT INTO user_skill_package_files (user_id, skill_name, path, content) VALUES ($1,'custom','SKILL.md','# x')`, userID)
	mustExec(`INSERT INTO a2a_tasks (id, user_id, agent_id, state) VALUES ($1,$2,$3,'submitted')`, a2aID, userID, agentID)
	mustExec(`INSERT INTO a2a_push_configs (id, task_id, url) VALUES ($1,$2,'http://example')`, uuid.NewString(), a2aID)
	mustExec(`INSERT INTO org_invites (id, org_id, username_or_email, role, invited_by, status)
VALUES ($1,$2,'invitee@example.com','member',$3,'pending')`, inviteID, org.ID, userID)
	_ = taskID
	_ = time.Now()

	res, err := d.PurgeUserData(userID)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.AgentIDs) < 2 {
		t.Fatalf("expected soft+live agents in result, got %#v", res.AgentIDs)
	}

	u, err := d.GetUserByID(userID)
	if err != nil {
		t.Fatal(err)
	}
	if u.Username != username || u.Email != email || u.Role != RoleMember || u.OrgID != org.ID {
		t.Fatalf("account fields changed: %+v", u)
	}
	var keptHash string
	if err := d.SQL.QueryRow(`SELECT password_hash FROM users WHERE id = $1`, userID).Scan(&keptHash); err != nil {
		t.Fatal(err)
	}
	if keptHash != hash {
		t.Fatalf("password_hash changed: %q", keptHash)
	}

	assertZero := func(table, q string, args ...any) {
		t.Helper()
		var n int
		if err := d.SQL.QueryRow(q, args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", table, err)
		}
		if n != 0 {
			t.Fatalf("%s still has %d rows", table, n)
		}
	}
	assertZero("agents", `SELECT COUNT(*) FROM agents WHERE user_id = $1`, userID)
	assertZero("conversations", `SELECT COUNT(*) FROM conversations WHERE user_id = $1`, userID)
	assertZero("messages", `SELECT COUNT(*) FROM messages WHERE conversation_id = $1`, convID)
	assertZero("memories", `SELECT COUNT(*) FROM memories WHERE user_id = $1`, userID)
	assertZero("memory_recalls", `SELECT COUNT(*) FROM memory_recalls WHERE user_id = $1`, userID)
	assertZero("usage_runs", `SELECT COUNT(*) FROM usage_runs WHERE user_id = $1`, userID)
	assertZero("routines", `SELECT COUNT(*) FROM routines WHERE user_id = $1`, userID)
	assertZero("routine_runs", `SELECT COUNT(*) FROM routine_runs WHERE routine_id = $1`, routineID)
	assertZero("channels", `SELECT COUNT(*) FROM channels WHERE user_id = $1`, userID)
	assertZero("agent_messages", `SELECT COUNT(*) FROM agent_messages WHERE user_id = $1`, userID)
	assertZero("user_machines", `SELECT COUNT(*) FROM user_machines WHERE user_id = $1`, userID)
	assertZero("bot_secrets", `SELECT COUNT(*) FROM bot_secrets WHERE user_id = $1`, userID)
	assertZero("mcp_servers", `SELECT COUNT(*) FROM mcp_servers WHERE user_id = $1`, userID)
	assertZero("sandboxes", `SELECT COUNT(*) FROM sandboxes WHERE user_id = $1`, userID)
	assertZero("llm_connections", `SELECT COUNT(*) FROM llm_connections WHERE user_id = $1`, userID)
	assertZero("user_skills", `SELECT COUNT(*) FROM user_skills WHERE user_id = $1`, userID)
	assertZero("user_skill_files", `SELECT COUNT(*) FROM user_skill_files WHERE user_id = $1`, userID)
	assertZero("user_skill_package_files", `SELECT COUNT(*) FROM user_skill_package_files WHERE user_id = $1`, userID)
	assertZero("a2a_tasks", `SELECT COUNT(*) FROM a2a_tasks WHERE user_id = $1`, userID)
	assertZero("a2a_push", `SELECT COUNT(*) FROM a2a_push_configs WHERE task_id = $1`, a2aID)
	assertZero("agent_skills", `SELECT COUNT(*) FROM agent_skills WHERE agent_id = $1`, agentID)
	assertZero("org_invites", `SELECT COUNT(*) FROM org_invites WHERE id = $1`, inviteID)

	// Soft-deleted user cannot be purged (account already gone from product view).
	other := uuid.NewString()
	mustExec(`INSERT INTO users (id, username, password_hash, org_id, role, deleted_at)
VALUES ($1,$2,'x',$3,'member',$4)`, other, "purge-soft-"+other[:8], org.ID, Now())
	t.Cleanup(func() { _, _ = d.SQL.Exec(`DELETE FROM users WHERE id = $1`, other) })
	if _, err := d.PurgeUserData(other); err == nil {
		t.Fatal("expected ErrNotFound for soft-deleted user")
	}
}

func TestPurgeUserDataRejectsMissing(t *testing.T) {
	d, err := Open("")
	if err != nil {
		t.Skip(err)
	}
	defer d.Close()
	if _, err := d.PurgeUserData(uuid.NewString()); err == nil {
		t.Fatal("expected not found")
	}
}
