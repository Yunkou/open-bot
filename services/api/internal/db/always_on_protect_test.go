package db

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestSetGlobalSkillEnabledBlockedForAlwaysOn(t *testing.T) {
	_ = os.Setenv("OPEN_BOT_ROOT", "/Users/tangxin/Workprojects/open-bot")
	ResetCuratedBuiltinCacheForTest()
	d, err := Open("")
	if err != nil {
		t.Skip(err)
	}
	defer d.SQL.Close()

	// Ensure find-skills exists and is enabled before the negative test.
	_, _ = d.SetGlobalSkillEnabled("find-skills", true)

	_, err = d.SetGlobalSkillEnabled("find-skills", false)
	if err != ErrSkillDisableBlocked {
		t.Fatalf("SetGlobalSkillEnabled(false) got %v, want ErrSkillDisableBlocked", err)
	}
	var en bool
	if err := d.SQL.QueryRow(`SELECT enabled FROM global_skills WHERE name='find-skills'`).Scan(&en); err != nil {
		t.Fatal(err)
	}
	if !en {
		t.Fatal("find-skills must remain enabled after blocked disable")
	}
}

func TestDeleteGlobalSkillBlockedForAlwaysOn(t *testing.T) {
	_ = os.Setenv("OPEN_BOT_ROOT", "/Users/tangxin/Workprojects/open-bot")
	ResetCuratedBuiltinCacheForTest()
	d, err := Open("")
	if err != nil {
		t.Skip(err)
	}
	defer d.SQL.Close()

	err = d.DeleteGlobalSkill("find-skills")
	if err != ErrSkillDisableBlocked {
		t.Fatalf("DeleteGlobalSkill got %v, want ErrSkillDisableBlocked", err)
	}
	var n int
	if err := d.SQL.QueryRow(`SELECT COUNT(*) FROM global_skills WHERE name='find-skills'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("find-skills must still exist: n=%d err=%v", n, err)
	}
}

func TestUpsertGlobalSkillCannotDisableAlwaysOn(t *testing.T) {
	_ = os.Setenv("OPEN_BOT_ROOT", "/Users/tangxin/Workprojects/open-bot")
	ResetCuratedBuiltinCacheForTest()
	d, err := Open("")
	if err != nil {
		t.Skip(err)
	}
	defer d.SQL.Close()
	_, err = d.UpsertGlobalSkill("find-skills", "x", "body", false)
	if err != ErrSkillDisableBlocked {
		t.Fatalf("UpsertGlobalSkill(enabled=false) got %v", err)
	}
}

func TestSeedGlobalSkillsFromDiskReenablesAlwaysOn(t *testing.T) {
	_ = os.Setenv("OPEN_BOT_ROOT", "/Users/tangxin/Workprojects/open-bot")
	ResetCuratedBuiltinCacheForTest()
	d, err := Open("")
	if err != nil {
		t.Skip(err)
	}
	defer d.SQL.Close()

	// Bypass the blocked setter to simulate a stale disabled row.
	if _, err := d.SQL.Exec(`UPDATE global_skills SET enabled=FALSE WHERE name='find-skills'`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = d.SQL.Exec(`UPDATE global_skills SET enabled=TRUE WHERE name='find-skills'`)
	})
	if err := d.SeedGlobalSkillsFromDisk(); err != nil {
		t.Fatal(err)
	}
	var en bool
	if err := d.SQL.QueryRow(`SELECT enabled FROM global_skills WHERE name='find-skills'`).Scan(&en); err != nil {
		t.Fatal(err)
	}
	if !en {
		t.Fatal("SeedGlobalSkillsFromDisk must re-enable always_on find-skills")
	}
}

func TestLiveReplaceAndSeedKeepFindSkills(t *testing.T) {
	_ = os.Setenv("OPEN_BOT_ROOT", "/Users/tangxin/Workprojects/open-bot")
	ResetCuratedBuiltinCacheForTest()
	d, err := Open("")
	if err != nil {
		t.Skip(err)
	}
	defer d.SQL.Close()

	u, err := d.CreateUser(fmt.Sprintf("ao_keep_%d", time.Now().UnixNano()), "x")
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
	a, err := d.CreateAgent(u.ID, "keep-bot", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := d.SeedNewAgentDefaultSkills(u.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := d.ReplaceAgentSkills(u.ID, a.ID, []string{"grill-me"}); err != nil {
		t.Fatal(err)
	}
	enabled, err := d.ListEnabledSkillNamesForAgent(u.ID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !containsName(enabled, "find-skills") {
		t.Fatalf("missing find-skills after replace: %v", enabled)
	}
	_, err = d.SetAgentSkillEnabled(u.ID, a.ID, "find-skills", false)
	if err != ErrSkillDisableBlocked {
		t.Fatalf("agent disable got %v", err)
	}
}

func containsName(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}
