package db

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestInsertAndListMemoryRecalls(t *testing.T) {
	d, err := Open("")
	if err != nil {
		t.Skip(err)
	}
	defer d.Close()

	org, err := d.EnsureDefaultOrg()
	if err != nil {
		t.Fatal(err)
	}
	orgID := org.ID
	userID := uuid.NewString()
	convID := uuid.NewString()
	agentID := uuid.NewString()
	username := "recall-" + userID[:8]
	if _, err := d.SQL.Exec(
		`INSERT INTO users (id, username, password_hash, org_id, role) VALUES ($1, $2, 'x', $3, 'member')`,
		userID, username, orgID,
	); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = d.SQL.Exec(`DELETE FROM memory_recalls WHERE user_id = $1`, userID)
		_, _ = d.SQL.Exec(`DELETE FROM agents WHERE id = $1`, agentID)
		_, _ = d.SQL.Exec(`DELETE FROM users WHERE id = $1`, userID)
	})
	if err := d.migrateMemoryRecalls(); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SQL.Exec(
		`INSERT INTO agents (id, user_id, name, system_prompt) VALUES ($1, $2, 'RecallBot', '')`,
		agentID, userID,
	); err != nil {
		t.Fatal(err)
	}

	score := 0.91
	rec, err := d.InsertMemoryRecall(
		orgID, userID, agentID, convID, "msg-1", "run-1", "lf-trace-1", "chat", "dm",
		2, 1,
		[]MemoryRecallItem{
			{Source: "explicit", Scope: "bot", Tier: "profile", Content: "likes tea", Snippet: "[bot] [profile] likes tea", MemoryID: "m1"},
			{Source: "mem0", Scope: "user", Content: "lives in shanghai", Snippet: "[user] [mem0] lives in shanghai", Score: &score},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if rec == nil || rec.ID == "" || rec.ItemCount != 2 {
		t.Fatalf("insert = %+v", rec)
	}

	list, err := d.ListOrgMemoryRecalls(orgID, MemoryRecallFilter{
		UserID:         userID,
		ConversationID: convID,
		Limit:          10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) < 1 || list[0].ID != rec.ID {
		t.Fatalf("list = %+v", list)
	}
	if list[0].Username != username {
		t.Fatalf("username = %q", list[0].Username)
	}
	got, err := d.GetOrgMemoryRecall(orgID, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 2 || got.Items[0].Source != "explicit" {
		t.Fatalf("get items = %+v", got.Items)
	}
	if got.RunID != "run-1" || got.LangfuseTraceID != "lf-trace-1" {
		t.Fatalf("ids run=%q lf=%q", got.RunID, got.LangfuseTraceID)
	}
	byRun, err := d.ListOrgMemoryRecalls(orgID, MemoryRecallFilter{RunID: "run-1", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(byRun) < 1 || byRun[0].ID != rec.ID {
		t.Fatalf("by run = %+v", byRun)
	}
	byLF, err := d.ListOrgMemoryRecalls(orgID, MemoryRecallFilter{LangfuseTraceID: "lf-trace-1", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(byLF) < 1 || byLF[0].ID != rec.ID {
		t.Fatalf("by lf = %+v", byLF)
	}

	// time filter excludes future window start
	future := time.Now().Add(time.Hour)
	empty, err := d.ListOrgMemoryRecalls(orgID, MemoryRecallFilter{
		UserID: userID,
		From:   future,
		Limit:  10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected empty future filter, got %+v", empty)
	}
}
