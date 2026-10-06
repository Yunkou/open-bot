package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/tangxin/open-bot/services/api/internal/db"
)

func TestInternalCloneAgentAppendsPersonaAndRejectsForeignSource(t *testing.T) {
	d, err := db.Open("")
	if err != nil {
		t.Skip(err)
	}
	defer d.Close()
	org, err := d.EnsureDefaultOrg()
	if err != nil {
		t.Fatal(err)
	}
	mkUser := func() string {
		id := uuid.NewString()
		if _, err := d.SQL.Exec(`INSERT INTO users (id, username, password_hash, org_id, role, created_at)
VALUES ($1,$2,'x',$3,$4,$5)`, id, "clone-http-"+id[:8], org.ID, db.RoleMember, db.Now()); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = d.SQL.Exec(`DELETE FROM users WHERE id=$1`, id) })
		return id
	}
	uid := mkUser()
	other := mkUser()
	src, err := d.CreateAgent(uid, "研究员", "查资料", "你是研究员")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{db: d, events: newChatHub()}

	call := func(body map[string]any) (*httptest.ResponseRecorder, map[string]any) {
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/internal/agents/clone", bytes.NewReader(b))
		rec := httptest.NewRecorder()
		s.handleInternalCloneAgent(rec, req)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec, out
	}

	rec, _ := call(map[string]any{"user_id": other, "source_agent_id": src.ID})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign clone code=%d", rec.Code)
	}

	rec, out := call(map[string]any{
		"user_id":              uid,
		"source_agent_id":      src.ID,
		"name":                 "竞品研究员",
		"system_prompt_append": "只关注竞品动态。",
		"disable_skills":       []string{"no-such-skill-xyz"},
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	agent, _ := out["agent"].(map[string]any)
	if agent["name"] != "竞品研究员" || agent["system_prompt"] != "你是研究员\n\n只关注竞品动态。" {
		t.Fatalf("agent=%v", agent)
	}
	if out["skill_errors"] == nil {
		t.Fatalf("expected skill_errors for unknown skill: %v", out)
	}
	if out["conversation_id"] == "" {
		t.Fatalf("missing conversation_id: %v", out)
	}

	// follow_up forces bot memory copy even when copy_memory is false.
	memID := uuid.NewString()
	if _, err := d.SQL.Exec(
		`INSERT INTO memories (id, user_id, tier, content, scope, agent_id) VALUES ($1,$2,'note','prior fact','bot',$3)`,
		memID, uid, src.ID,
	); err != nil {
		t.Fatal(err)
	}
	rec, out = call(map[string]any{
		"user_id":         uid,
		"source_agent_id": src.ID,
		"name":            "带记忆副本",
		"copy_memory":     false,
		"follow_up":       "继续查竞品",
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("follow_up clone code=%d body=%s", rec.Code, rec.Body.String())
	}
	if out["memories_copied"] != float64(1) {
		t.Fatalf("expected memories_copied=1 got %v", out["memories_copied"])
	}
	if out["follow_up_status"] != "queued" && out["follow_up_task_id"] == nil {
		// task queue may or may not be fully wired in unit test; memory force is the contract
		t.Logf("follow_up fields: status=%v task=%v err=%v", out["follow_up_status"], out["follow_up_task_id"], out["follow_up_error"])
	}
	agent, _ = out["agent"].(map[string]any)
	newID, _ := agent["id"].(string)
	var n int
	if err := d.SQL.QueryRow(`SELECT COUNT(*) FROM memories WHERE agent_id=$1 AND content='prior fact'`, newID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("forced memory copy n=%d err=%v", n, err)
	}
}
