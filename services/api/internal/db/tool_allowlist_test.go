package db

import (
	"fmt"
	"testing"
	"time"
)

func TestMatchUserToolAllowlist(t *testing.T) {
	entries := []UserToolAllowlistEntry{
		{ToolGroup: "shell", Tool: ""},
		{ToolGroup: "file_write", Tool: "host_write", AgentID: "bot-1"},
	}
	if !MatchUserToolAllowlist(entries, "host_shell", "shell", "") {
		t.Fatal("group match")
	}
	if !MatchUserToolAllowlist(entries, "host_write", "file_write", "bot-1") {
		t.Fatal("agent-scoped match")
	}
	if MatchUserToolAllowlist(entries, "host_write", "file_write", "") {
		t.Fatal("agent-scoped must not match empty agent")
	}
	if MatchUserToolAllowlist(entries, "host_write", "file_write", "bot-2") {
		t.Fatal("wrong agent")
	}
	if MatchUserToolAllowlist(nil, "host_shell", "shell", "") {
		t.Fatal("empty list")
	}
}

func TestUserToolAllowlistCRUD(t *testing.T) {
	d, err := Open("")
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	if err := d.migrateUserToolAllowlist(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	u, err := d.CreateUser(fmt.Sprintf("allowlist_%d", time.Now().UnixNano()), "x")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	e1, err := d.UpsertUserToolAllowlist(u.ID, "shell", "host_shell", "")
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if e1.ToolGroup != "shell" || e1.Tool != "host_shell" {
		t.Fatalf("entry: %+v", e1)
	}
	// idempotent upsert same key
	e2, err := d.UpsertUserToolAllowlist(u.ID, "shell", "host_shell", "")
	if err != nil {
		t.Fatalf("upsert2: %v", err)
	}
	if e2.ID != e1.ID {
		t.Fatalf("upsert should keep same row id: %s vs %s", e1.ID, e2.ID)
	}
	list, err := d.ListUserToolAllowlist(u.ID)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: n=%d err=%v", len(list), err)
	}
	if err := d.DeleteUserToolAllowlist(u.ID, e1.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	list, err = d.ListUserToolAllowlist(u.ID)
	if err != nil || len(list) != 0 {
		t.Fatalf("after delete: n=%d err=%v", len(list), err)
	}
	if err := d.DeleteUserToolAllowlist(u.ID, e1.ID); err != ErrNotFound {
		t.Fatalf("second delete want ErrNotFound got %v", err)
	}
}
