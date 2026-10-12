package db

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestEnsureAlwaysOnSkillsParity(t *testing.T) {
	_ = os.Setenv("OPEN_BOT_ROOT", "/Users/tangxin/Workprojects/open-bot")
	ResetCuratedBuiltinCacheForTest()
	if !IsAlwaysOnBuiltin("find-skills") {
		t.Fatal("need manifest")
	}
	got := EnsureAlwaysOnSkills([]string{"grill-me"})
	if !containsName(got, "find-skills") {
		t.Fatalf("EnsureAlwaysOnSkills: %v", got)
	}
}

// Old bots may have an allowlist that predates find-skills (no agent_skills row for it).
// always_on must still mount it via ListEnabledSkillNamesForAgent.
func TestLiveAlwaysOnMissingAgentSkillRow(t *testing.T) {
	_ = os.Setenv("OPEN_BOT_ROOT", "/Users/tangxin/Workprojects/open-bot")
	ResetCuratedBuiltinCacheForTest()
	d, err := Open("")
	if err != nil {
		t.Skip(err)
	}
	defer d.SQL.Close()

	u, err := d.CreateUser(fmt.Sprintf("ao_edge_%d", time.Now().UnixNano()), "x")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	t.Cleanup(func() {
		_, _ = d.SQL.Exec(`DELETE FROM agent_skills WHERE agent_id IN (SELECT id FROM agents WHERE user_id=$1)`, u.ID)
		_, _ = d.SQL.Exec(`DELETE FROM agents WHERE user_id=$1`, u.ID)
		_, _ = d.SQL.Exec(`DELETE FROM user_skills WHERE user_id=$1`, u.ID)
		_, _ = d.SQL.Exec(`DELETE FROM users WHERE id=$1`, u.ID)
	})

	a, err := d.CreateAgent(u.ID, "edge-bot", "", "")
	if err != nil {
		t.Fatal(err)
	}
	// Materialize a pre-curated allowlist: grill-me only (no find-skills row).
	if _, err := d.SQL.Exec(`DELETE FROM agent_skills WHERE agent_id=$1`, a.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SQL.Exec(`INSERT INTO agent_skills (agent_id, skill_name, enabled) VALUES ($1,'grill-me',TRUE)`, a.ID); err != nil {
		t.Fatal(err)
	}
	// Ensure account catalog includes find-skills as always-on.
	if _, err := d.ListUserSkills(u.ID); err != nil {
		t.Fatal(err)
	}

	enabled, err := d.ListEnabledSkillNamesForAgent(u.ID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !containsName(enabled, "find-skills") {
		t.Fatalf("always_on must mount find-skills when agent_skills row is missing; got %v", enabled)
	}

	list, err := d.ListAgentSkills(u.ID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range list {
		if s.Name == "find-skills" {
			if !s.Enabled || !s.AlwaysOn {
				t.Fatalf("ListAgentSkills should force-enable always_on: %+v", s)
			}
			return
		}
	}
	t.Fatal("find-skills missing from ListAgentSkills")
}
