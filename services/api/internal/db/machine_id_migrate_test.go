package db

import (
	"testing"

	"github.com/google/uuid"
)

func TestResolveUniqueBackfillMachineID(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   []Machine
		want string
	}{
		{"empty", nil, ""},
		{"one machine", []Machine{{ID: "m1", FileOpCount: 0}}, "m1"},
		{"one machine with ops", []Machine{{ID: "m1", FileOpCount: 3}}, "m1"},
		{"multi no ops", []Machine{{ID: "m1"}, {ID: "m2"}}, ""},
		// file_op_count must NOT decide multi-machine binds (machine-level, not Bot×machine)
		{"multi unique usual ignored", []Machine{{ID: "m1", FileOpCount: 1}, {ID: "m2", FileOpCount: 5}}, ""},
		{"multi tie ignored", []Machine{{ID: "m1", FileOpCount: 5}, {ID: "m2", FileOpCount: 5}}, ""},
		{"multi only one has ops ignored", []Machine{{ID: "phone", FileOpCount: 0}, {ID: "mac", FileOpCount: 2}}, ""},
		{"trim id", []Machine{{ID: "  m9  "}}, "m9"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := resolveUniqueBackfillMachineID(tc.in); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

// midHarness creates isolated users/machines/agents. All migrate helpers are
// called scoped to these users so the shared develop DB is never mutated.
type midHarness struct {
	t     *testing.T
	d     *DB
	org   string
	users []string
}

func newMidHarness(t *testing.T) *midHarness {
	d, err := Open("")
	if err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	org, err := d.EnsureDefaultOrg()
	if err != nil {
		t.Fatal(err)
	}
	return &midHarness{t: t, d: d, org: org.ID}
}

func (h *midHarness) user(prefix string) string {
	uid := uuid.NewString()
	if _, err := h.d.SQL.Exec(
		`INSERT INTO users (id, username, password_hash, org_id, role) VALUES ($1, $2, 'x', $3, 'member')`,
		uid, prefix+"-"+uid[:8], h.org,
	); err != nil {
		h.t.Fatal(err)
	}
	h.users = append(h.users, uid)
	h.t.Cleanup(func() {
		_, _ = h.d.SQL.Exec(`DELETE FROM agents WHERE user_id = $1`, uid)
		_, _ = h.d.SQL.Exec(`DELETE FROM user_machines WHERE user_id = $1`, uid)
		_, _ = h.d.SQL.Exec(`DELETE FROM users WHERE id = $1`, uid)
	})
	return uid
}

func (h *midHarness) machine(uid, key string) *Machine {
	m, err := h.d.RegisterMachine(uid, MachineRegisterInput{MachineKey: key, Label: key, Platform: "macos", App: "desktop"})
	if err != nil {
		h.t.Fatal(err)
	}
	return m
}

func (h *midHarness) agent(uid, name string) *Agent {
	a, err := h.d.CreateAgent(uid, name, "", "")
	if err != nil {
		h.t.Fatal(err)
	}
	return a
}

func (h *midHarness) get(agentID string) (mid, src string) {
	if err := h.d.SQL.QueryRow(
		`SELECT COALESCE(machine_id,''), COALESCE(machine_id_source,'') FROM agents WHERE id=$1`, agentID,
	).Scan(&mid, &src); err != nil {
		h.t.Fatal(err)
	}
	return
}

// raw writes machine_id/source bypassing SetAgentMachineID (simulates migrate or legacy rows).
func (h *midHarness) raw(agentID, mid, src string) {
	if _, err := h.d.SQL.Exec(`UPDATE agents SET machine_id=$1, machine_id_source=$2 WHERE id=$3`, mid, src, agentID); err != nil {
		h.t.Fatal(err)
	}
}

func (h *midHarness) want(label, agentID, wantMid, wantSrc string) {
	h.t.Helper()
	mid, src := h.get(agentID)
	if mid != wantMid || src != wantSrc {
		h.t.Fatalf("%s: got (%q,%q) want (%q,%q)", label, mid, src, wantMid, wantSrc)
	}
}

func TestBackfillAgentMachineID_StampsMigrateSource(t *testing.T) {
	h := newMidHarness(t)
	d := h.d

	// A: exactly one machine → unbound agents bind, source=migrate.
	ua := h.user("mid-one")
	ma := h.machine(ua, "key-a")
	a1 := h.agent(ua, "bot-a1")
	a2 := h.agent(ua, "bot-a2")
	h.want("new agent", a1.ID, "", "")

	// B: two machines, unique file_op usual → stay empty.
	ub := h.user("mid-usual")
	_ = h.machine(ub, "key-b1")
	mb2 := h.machine(ub, "key-b2")
	if _, err := d.IncrementMachineFileOps(ub, mb2.ID); err != nil {
		t.Fatal(err)
	}
	b1 := h.agent(ub, "bot-b1")

	// D: two machines; user-bound agent untouched (source stays user).
	ud := h.user("mid-userbound")
	md1 := h.machine(ud, "key-d1")
	_ = h.machine(ud, "key-d2")
	dBound := h.agent(ud, "bot-d-bound")
	if _, err := d.SetAgentMachineID(ud, dBound.ID, md1.ID); err != nil {
		t.Fatal(err)
	}
	dEmpty := h.agent(ud, "bot-d-empty")

	// E: one machine, one agent already user-bound → kept as user; sibling gets migrate.
	ue := h.user("mid-one-mixed")
	me := h.machine(ue, "key-e")
	eUser := h.agent(ue, "bot-e-user")
	if _, err := d.SetAgentMachineID(ue, eUser.ID, me.ID); err != nil {
		t.Fatal(err)
	}
	eEmpty := h.agent(ue, "bot-e-empty")

	if err := d.backfillAgentMachineID(h.users); err != nil {
		t.Fatal(err)
	}
	h.want("A bot1", a1.ID, ma.ID, MachineIDSourceMigrate)
	h.want("A bot2", a2.ID, ma.ID, MachineIDSourceMigrate)
	h.want("B multi", b1.ID, "", "")
	h.want("D user-bound", dBound.ID, md1.ID, MachineIDSourceUser)
	h.want("D empty", dEmpty.ID, "", "")
	h.want("E user", eUser.ID, me.ID, MachineIDSourceUser)
	h.want("E filled", eEmpty.ID, me.ID, MachineIDSourceMigrate)

	// Markers present after Open → migrate is a no-op; user unbind stays empty.
	if _, err := d.SetAgentMachineID(ua, a1.ID, ""); err != nil {
		t.Fatal(err)
	}
	if err := d.migrateAgentMachineID(); err != nil {
		t.Fatal(err)
	}
	h.want("A bot1 after unbind + migrate", a1.ID, "", "")
	h.want("A bot2 after migrate", a2.ID, ma.ID, MachineIDSourceMigrate)
}

func TestSetAgentMachineID_StampsUserSource(t *testing.T) {
	h := newMidHarness(t)
	d := h.d
	u := h.user("mid-set")
	m1 := h.machine(u, "key-s1")
	a := h.agent(u, "bot-s")
	h.raw(a.ID, m1.ID, MachineIDSourceMigrate)

	got, err := d.SetAgentMachineID(u, a.ID, m1.ID) // same id, user re-picks
	if err != nil {
		t.Fatal(err)
	}
	if got.MachineIDSource != MachineIDSourceUser {
		t.Fatalf("returned source %q", got.MachineIDSource)
	}
	h.want("set", a.ID, m1.ID, MachineIDSourceUser)
	if g, _ := d.GetAgent(u, a.ID); g == nil || g.MachineIDSource != MachineIDSourceUser {
		t.Fatalf("GetAgent source not scanned: %+v", g)
	}

	if _, err := d.SetAgentMachineID(u, a.ID, ""); err != nil {
		t.Fatal(err)
	}
	h.want("clear", a.ID, "", "")

	// Delete machine clears binding + source.
	if _, err := d.SetAgentMachineID(u, a.ID, m1.ID); err != nil {
		t.Fatal(err)
	}
	if err := d.ClearAgentMachineIDForMachine(u, m1.ID); err != nil {
		t.Fatal(err)
	}
	h.want("machine delete", a.ID, "", "")
}

func TestCloneAgent_CopiesMachineIDSource(t *testing.T) {
	h := newMidHarness(t)
	d := h.d
	u := h.user("mid-clone")
	m := h.machine(u, "key-c")
	for _, src := range []string{MachineIDSourceUser, MachineIDSourceMigrate, MachineIDSourceNone} {
		a := h.agent(u, "bot-src-"+src)
		h.raw(a.ID, m.ID, src)
		res, err := d.CloneAgent(u, a.ID, CloneAgentOptions{})
		if err != nil {
			t.Fatal(err)
		}
		h.want("clone of "+src, res.Agent.ID, m.ID, src)
	}
	unbound := h.agent(u, "bot-unbound")
	res, err := d.CloneAgent(u, unbound.ID, CloneAgentOptions{})
	if err != nil {
		t.Fatal(err)
	}
	h.want("clone of unbound", res.Agent.ID, "", "")
}

func TestClearMigrateSourcedMultiMachine(t *testing.T) {
	h := newMidHarness(t)
	d := h.d

	// U: >1 machines, user intentionally binds ALL bots to one machine → keep.
	uu := h.user("mid-user-all-one")
	mu := h.machine(uu, "key-u1")
	_ = h.machine(uu, "key-u2")
	u1 := h.agent(uu, "bot-u1")
	u2 := h.agent(uu, "bot-u2")
	for _, a := range []*Agent{u1, u2} {
		if _, err := d.SetAgentMachineID(uu, a.ID, mu.ID); err != nil {
			t.Fatal(err)
		}
	}

	// M: >1 machines, migrate-sourced binds → clear (mid + source).
	um := h.user("mid-migrate-multi")
	mm := h.machine(um, "key-m1")
	_ = h.machine(um, "key-m2")
	m1 := h.agent(um, "bot-m1")
	m2 := h.agent(um, "bot-m2")
	h.raw(m1.ID, mm.ID, MachineIDSourceMigrate)
	h.raw(m2.ID, mm.ID, MachineIDSourceUser) // same user, user pick → keep

	// L: >1 machines, legacy unmarked rows all same mid (v1 bulk OR user) → keep; no heuristic.
	ul := h.user("mid-legacy")
	ml := h.machine(ul, "key-l1")
	_ = h.machine(ul, "key-l2")
	l1 := h.agent(ul, "bot-l1")
	l2 := h.agent(ul, "bot-l2")
	h.raw(l1.ID, ml.ID, "")
	h.raw(l2.ID, ml.ID, "")

	// S: single machine, migrate-sourced → keep (correct backfill).
	us := h.user("mid-single")
	ms := h.machine(us, "key-s")
	s1 := h.agent(us, "bot-s1")
	h.raw(s1.ID, ms.ID, MachineIDSourceMigrate)

	if err := d.clearMigrateSourcedMultiMachine(h.users); err != nil {
		t.Fatal(err)
	}
	h.want("U user all-one bot1", u1.ID, mu.ID, MachineIDSourceUser)
	h.want("U user all-one bot2", u2.ID, mu.ID, MachineIDSourceUser)
	h.want("M migrate cleared", m1.ID, "", "")
	h.want("M user kept", m2.ID, mm.ID, MachineIDSourceUser)
	h.want("L legacy kept", l1.ID, ml.ID, "")
	h.want("L legacy kept 2", l2.ID, ml.ID, "")
	h.want("S single kept", s1.ID, ms.ID, MachineIDSourceMigrate)

	// Scoped call with empty scope is a no-op.
	h.raw(m1.ID, mm.ID, MachineIDSourceMigrate)
	if err := d.clearMigrateSourcedMultiMachine([]string{}); err != nil {
		t.Fatal(err)
	}
	h.want("empty scope noop", m1.ID, mm.ID, MachineIDSourceMigrate)

	// v3 marker present after Open → global migrate does not re-clear; retired
	// clear_multi never runs even with v1 marker history present.
	if err := d.migrateAgentMachineID(); err != nil {
		t.Fatal(err)
	}
	h.want("v3 idempotent", m1.ID, mm.ID, MachineIDSourceMigrate)
	h.want("U still kept", u1.ID, mu.ID, MachineIDSourceUser)
	h.want("L still kept", l1.ID, ml.ID, "")
}
