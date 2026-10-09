package db

import (
	"testing"

	"github.com/google/uuid"
)

func TestDefaultCloneName(t *testing.T) {
	if got := DefaultCloneName("小助", nil); got != "小助 副本" {
		t.Fatalf("got %q", got)
	}
	if got := DefaultCloneName("小助", []string{"小助", "小助 副本"}); got != "小助 副本 2" {
		t.Fatalf("got %q", got)
	}
	if got := DefaultCloneName("小助", []string{"小助 副本", "小助 副本 2"}); got != "小助 副本 3" {
		t.Fatalf("got %q", got)
	}
	if got := DefaultCloneName("  ", nil); got != "助手 副本" {
		t.Fatalf("got %q", got)
	}
}

func TestCloneAgentCopiesPersonaSkillsAndOptIns(t *testing.T) {
	d, err := Open("")
	if err != nil {
		t.Skip(err)
	}
	defer d.Close()
	org, err := d.EnsureDefaultOrg()
	if err != nil {
		t.Fatal(err)
	}
	mk := func(prefix string) string {
		id := uuid.NewString()
		if _, err := d.SQL.Exec(`INSERT INTO users (id, username, password_hash, org_id, role, created_at)
VALUES ($1,$2,'x',$3,$4,$5)`, id, prefix+id[:8], org.ID, RoleMember, Now()); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = d.SQL.Exec(`DELETE FROM agent_skills WHERE agent_id IN (SELECT id FROM agents WHERE user_id=$1)`, id)
			_, _ = d.SQL.Exec(`DELETE FROM users WHERE id=$1`, id)
		})
		return id
	}
	uid := mk("clone-")
	other := mk("clone-other-")

	src, err := d.CreateAgent(uid, "写手", "负责写作", "你是写手")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.SetAgentComputerMode(uid, src.ID, "private"); err != nil {
		t.Fatal(err)
	}
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := d.SQL.Exec(q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	mustExec(`INSERT INTO agent_skills (agent_id, skill_name, enabled) VALUES ($1,'a',TRUE),($1,'b',FALSE)`, src.ID)
	mustExec(`INSERT INTO memories (id, user_id, tier, content, scope, agent_id) VALUES ($1,$2,'note','bot fact','bot',$3)`, uuid.NewString(), uid, src.ID)
	mustExec(`INSERT INTO memories (id, user_id, tier, content, scope, agent_id) VALUES ($1,$2,'note','user fact','user','')`, uuid.NewString(), uid)
	srcConv, err := d.GetOrCreatePrimaryConversation(uid, src.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.AddMessage(srcConv.ID, "user", "hello"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.CreateRoutine(uid, "日报", "写日报", "0 9 * * *", true, src.ID, "Asia/Shanghai", srcConv.ID, "[]", 2, false); err != nil {
		t.Fatal(err)
	}

	// Other tenant/user cannot clone it.
	if _, err := d.CloneAgent(other, src.ID, CloneAgentOptions{}); err != ErrNotFound {
		t.Fatalf("cross-user clone err=%v want ErrNotFound", err)
	}

	// Plain clone: persona + skills, no memory / routines / history.
	res, err := d.CloneAgent(uid, src.ID, CloneAgentOptions{})
	if err != nil {
		t.Fatal(err)
	}
	a := res.Agent
	if a.ID == src.ID || a.Name != "写手 副本" || a.SystemPrompt != "你是写手" || a.Description != "负责写作" || a.ComputerMode != "private" {
		t.Fatalf("bad clone %+v", a)
	}
	if res.SkillsCopied != 2 || res.SkillsInherit {
		t.Fatalf("skills %+v", res)
	}
	var en bool
	if err := d.SQL.QueryRow(`SELECT enabled FROM agent_skills WHERE agent_id=$1 AND skill_name='b'`, a.ID).Scan(&en); err != nil || en {
		t.Fatalf("skill b enabled=%v err=%v", en, err)
	}
	var n int
	_ = d.SQL.QueryRow(`SELECT COUNT(*) FROM memories WHERE agent_id=$1`, a.ID).Scan(&n)
	if n != 0 || res.MemoriesCopied != 0 {
		t.Fatalf("memories copied without opt-in: %d", n)
	}
	_ = d.SQL.QueryRow(`SELECT COUNT(*) FROM routines WHERE agent_id=$1`, a.ID).Scan(&n)
	if n != 0 {
		t.Fatalf("routines copied without opt-in: %d", n)
	}
	if res.ConversationID == "" || res.ConversationID == srcConv.ID {
		t.Fatalf("conversation %q", res.ConversationID)
	}
	msgs, _ := d.ListMessages(uid, res.ConversationID)
	if len(msgs) != 0 {
		t.Fatalf("history copied: %d", len(msgs))
	}

	// Opt-in clone with overrides.
	sp := "你是周报写手"
	res2, err := d.CloneAgent(uid, src.ID, CloneAgentOptions{Name: "周报", SystemPrompt: &sp, CopyMemory: true, CopyRoutines: true})
	if err != nil {
		t.Fatal(err)
	}
	if res2.Agent.Name != "周报" || res2.Agent.SystemPrompt != sp {
		t.Fatalf("overrides %+v", res2.Agent)
	}
	if res2.MemoriesCopied != 1 {
		t.Fatalf("memories copied %d", res2.MemoriesCopied)
	}
	var content, scope string
	if err := d.SQL.QueryRow(`SELECT content, scope FROM memories WHERE agent_id=$1`, res2.Agent.ID).Scan(&content, &scope); err != nil || content != "bot fact" || scope != "bot" {
		t.Fatalf("memory %q %q %v", content, scope, err)
	}
	if res2.RoutinesCopied != 1 {
		t.Fatalf("routines copied %d", res2.RoutinesCopied)
	}
	var enabled bool
	var convID string
	if err := d.SQL.QueryRow(`SELECT enabled, conversation_id FROM routines WHERE agent_id=$1`, res2.Agent.ID).Scan(&enabled, &convID); err != nil {
		t.Fatal(err)
	}
	if enabled || convID != res2.ConversationID {
		t.Fatalf("routine enabled=%v conv=%q want paused + %q", enabled, convID, res2.ConversationID)
	}
	// Source untouched.
	if s2, err := d.GetAgent(uid, src.ID); err != nil || s2.Name != "写手" {
		t.Fatalf("source changed %+v %v", s2, err)
	}
}

func TestCloneAgentWithoutAllowlistInheritsAccountSkills(t *testing.T) {
	d, err := Open("")
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
VALUES ($1,$2,'x',$3,$4,$5)`, uid, "clone-inh-"+uid[:8], org.ID, RoleMember, Now()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = d.SQL.Exec(`DELETE FROM users WHERE id=$1`, uid) })
	src, err := d.CreateAgent(uid, "通用", "", "")
	if err != nil {
		t.Fatal(err)
	}
	res, err := d.CloneAgent(uid, src.ID, CloneAgentOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.SkillsInherit || res.SkillsCopied != 0 {
		t.Fatalf("%+v", res)
	}
}
