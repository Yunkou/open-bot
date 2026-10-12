package db

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestLiveReplaceAgentSkillsAlwaysKeepsFindSkills(t *testing.T) {
	_ = os.Setenv("OPEN_BOT_ROOT", "/Users/tangxin/Workprojects/open-bot")
	ResetCuratedBuiltinCacheForTest()
	d, err := Open("")
	if err != nil {
		t.Skip(err)
	}
	defer d.SQL.Close()

	u, err := d.CreateUser(fmt.Sprintf("ao_rep_%d", time.Now().UnixNano()), "x")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = d.SQL.Exec(`DELETE FROM agent_skills WHERE agent_id IN (SELECT id FROM agents WHERE user_id=$1)`, u.ID)
		_, _ = d.SQL.Exec(`DELETE FROM agents WHERE user_id=$1`, u.ID)
		_, _ = d.SQL.Exec(`DELETE FROM user_skills WHERE user_id=$1`, u.ID)
		_, _ = d.SQL.Exec(`DELETE FROM users WHERE id=$1`, u.ID)
	})
	if _, err := d.ListUserSkills(u.ID); err != nil {
		t.Fatal(err)
	}
	a, err := d.CreateAgent(u.ID, "replace-bot", "", "")
	if err != nil {
		t.Fatal(err)
	}
	// Explicitly replace with empty / without find-skills
	if err := d.ReplaceAgentSkills(u.ID, a.ID, []string{}); err != nil {
		t.Fatal(err)
	}
	var en bool
	if err := d.SQL.QueryRow(`SELECT enabled FROM agent_skills WHERE agent_id=$1 AND skill_name='find-skills'`, a.ID).Scan(&en); err != nil {
		t.Fatalf("find-skills row missing after empty replace: %v", err)
	}
	if !en {
		t.Fatal("find-skills must be enabled=true after empty replace")
	}
	enabled, err := d.ListEnabledSkillNamesForAgent(u.ID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !containsName(enabled, "find-skills") {
		t.Fatalf("ListEnabled missing find-skills after empty replace: %v", enabled)
	}

	// Replace with only grill-me
	if err := d.ReplaceAgentSkills(u.ID, a.ID, []string{"grill-me"}); err != nil {
		t.Fatal(err)
	}
	if err := d.SQL.QueryRow(`SELECT enabled FROM agent_skills WHERE agent_id=$1 AND skill_name='find-skills'`, a.ID).Scan(&en); err != nil || !en {
		t.Fatalf("find-skills after grill-me-only replace: en=%v err=%v", en, err)
	}
}

func TestLiveSeedNewAgentDefaultSkillsIdempotent(t *testing.T) {
	_ = os.Setenv("OPEN_BOT_ROOT", "/Users/tangxin/Workprojects/open-bot")
	ResetCuratedBuiltinCacheForTest()
	d, err := Open("")
	if err != nil {
		t.Skip(err)
	}
	defer d.SQL.Close()
	u, err := d.CreateUser(fmt.Sprintf("ao_seed_%d", time.Now().UnixNano()), "x")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = d.SQL.Exec(`DELETE FROM agent_skills WHERE agent_id IN (SELECT id FROM agents WHERE user_id=$1)`, u.ID)
		_, _ = d.SQL.Exec(`DELETE FROM agents WHERE user_id=$1`, u.ID)
		_, _ = d.SQL.Exec(`DELETE FROM user_skills WHERE user_id=$1`, u.ID)
		_, _ = d.SQL.Exec(`DELETE FROM users WHERE id=$1`, u.ID)
	})
	a, err := d.CreateAgent(u.ID, "seed-bot", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := d.SeedNewAgentDefaultSkills(u.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	var n1 int
	_ = d.SQL.QueryRow(`SELECT COUNT(*) FROM agent_skills WHERE agent_id=$1 AND enabled`, a.ID).Scan(&n1)
	// Second seed must no-op (not rewrite)
	if err := d.SeedNewAgentDefaultSkills(u.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	var n2, fs int
	_ = d.SQL.QueryRow(`SELECT COUNT(*) FROM agent_skills WHERE agent_id=$1 AND enabled`, a.ID).Scan(&n2)
	_ = d.SQL.QueryRow(`SELECT COUNT(*) FROM agent_skills WHERE agent_id=$1 AND skill_name='find-skills' AND enabled`, a.ID).Scan(&fs)
	if n1 != n2 {
		t.Fatalf("seed not idempotent: %d vs %d", n1, n2)
	}
	if fs != 1 {
		t.Fatal("find-skills not enabled after seed")
	}
	if n1 < 10 {
		t.Fatalf("expected curated batch sized seed, got %d enabled", n1)
	}
}
