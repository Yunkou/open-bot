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

func TestBackfillAgentMachineID_IdempotentAndConservative(t *testing.T) {
	d, err := Open("")
	if err != nil {
		t.Skip(err)
	}
	defer d.Close()

	org, err := d.EnsureDefaultOrg()
	if err != nil {
		t.Fatal(err)
	}

	newUser := func(prefix string) string {
		uid := uuid.NewString()
		if _, err := d.SQL.Exec(
			`INSERT INTO users (id, username, password_hash, org_id, role) VALUES ($1, $2, 'x', $3, 'member')`,
			uid, prefix+"-"+uid[:8], org.ID,
		); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = d.SQL.Exec(`DELETE FROM agents WHERE user_id = $1`, uid)
			_, _ = d.SQL.Exec(`DELETE FROM user_machines WHERE user_id = $1`, uid)
			_, _ = d.SQL.Exec(`DELETE FROM users WHERE id = $1`, uid)
		})
		return uid
	}

	reg := func(uid, key, label string) *Machine {
		m, err := d.RegisterMachine(uid, MachineRegisterInput{
			MachineKey: key,
			Label:      label,
			Platform:   "macos",
			App:        "desktop",
		})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}

	machineIDOf := func(agentID string) string {
		var mid string
		if err := d.SQL.QueryRow(`SELECT COALESCE(machine_id,'') FROM agents WHERE id=$1`, agentID).Scan(&mid); err != nil {
			t.Fatal(err)
		}
		return mid
	}

	// --- user A: exactly one machine → all unbound agents bind to it
	ua := newUser("mid-one")
	ma := reg(ua, "key-a", "Mac A")
	a1, err := d.CreateAgent(ua, "bot-a1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	a2, err := d.CreateAgent(ua, "bot-a2", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if machineIDOf(a1.ID) != "" || machineIDOf(a2.ID) != "" {
		t.Fatal("new agents should start unbound")
	}

	// --- user B: two machines, unique usual via file_op_count → must stay empty
	ub := newUser("mid-usual")
	mb1 := reg(ub, "key-b1", "Mac B1")
	mb2 := reg(ub, "key-b2", "Mac B2")
	if _, err := d.IncrementMachineFileOps(ub, mb2.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.IncrementMachineFileOps(ub, mb2.ID); err != nil {
		t.Fatal(err)
	}
	b1, err := d.CreateAgent(ub, "bot-b1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	_ = mb1

	// --- user C: two machines, no ops → stay empty
	uc := newUser("mid-ambig")
	_ = reg(uc, "key-c1", "Mac C1")
	_ = reg(uc, "key-c2", "Mac C2")
	c1, err := d.CreateAgent(uc, "bot-c1", "", "")
	if err != nil {
		t.Fatal(err)
	}

	// --- user D: two machines, tied ops → stay empty; pre-bound agent untouched
	ud := newUser("mid-tie")
	md1 := reg(ud, "key-d1", "Mac D1")
	md2 := reg(ud, "key-d2", "Mac D2")
	if _, err := d.IncrementMachineFileOps(ud, md1.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := d.IncrementMachineFileOps(ud, md2.ID); err != nil {
		t.Fatal(err)
	}
	dBound, err := d.CreateAgent(ud, "bot-d-bound", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.SetAgentMachineID(ud, dBound.ID, md1.ID); err != nil {
		t.Fatal(err)
	}
	dEmpty, err := d.CreateAgent(ud, "bot-d-empty", "", "")
	if err != nil {
		t.Fatal(err)
	}

	// Force re-run of v2 backfill (Open may already have applied the marker).
	if _, err := d.SQL.Exec(`DELETE FROM app_migrations WHERE name=$1`, agentMachineIDBackfillV2Marker); err != nil {
		t.Fatal(err)
	}
	if err := d.migrateAgentMachineID(); err != nil {
		t.Fatal(err)
	}

	if got := machineIDOf(a1.ID); got != ma.ID {
		t.Fatalf("user A bot1: got %q want %q", got, ma.ID)
	}
	if got := machineIDOf(a2.ID); got != ma.ID {
		t.Fatalf("user A bot2: got %q want %q", got, ma.ID)
	}
	if got := machineIDOf(b1.ID); got != "" {
		t.Fatalf("user B multi-machine must stay empty (file_op usual ignored), got %q", got)
	}
	if got := machineIDOf(c1.ID); got != "" {
		t.Fatalf("user C ambiguous must stay empty, got %q", got)
	}
	if got := machineIDOf(dBound.ID); got != md1.ID {
		t.Fatalf("pre-bound must stay %q, got %q", md1.ID, got)
	}
	if got := machineIDOf(dEmpty.ID); got != "" {
		t.Fatalf("tie/multi must stay empty, got %q", got)
	}

	// Idempotent: second migrate is no-op (v2 marker present); clearing one agent stays empty.
	if _, err := d.SQL.Exec(`UPDATE agents SET machine_id='' WHERE id=$1`, a1.ID); err != nil {
		t.Fatal(err)
	}
	if err := d.migrateAgentMachineID(); err != nil {
		t.Fatal(err)
	}
	if got := machineIDOf(a1.ID); got != "" {
		t.Fatalf("after v2 marker applied, must not re-bind intentionally empty; got %q", got)
	}
}

func TestClearMultiMachineBulkBackfill(t *testing.T) {
	d, err := Open("")
	if err != nil {
		t.Skip(err)
	}
	defer d.Close()

	org, err := d.EnsureDefaultOrg()
	if err != nil {
		t.Fatal(err)
	}

	newUser := func(prefix string) string {
		uid := uuid.NewString()
		if _, err := d.SQL.Exec(
			`INSERT INTO users (id, username, password_hash, org_id, role) VALUES ($1, $2, 'x', $3, 'member')`,
			uid, prefix+"-"+uid[:8], org.ID,
		); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = d.SQL.Exec(`DELETE FROM agents WHERE user_id = $1`, uid)
			_, _ = d.SQL.Exec(`DELETE FROM user_machines WHERE user_id = $1`, uid)
			_, _ = d.SQL.Exec(`DELETE FROM users WHERE id = $1`, uid)
		})
		return uid
	}

	reg := func(uid, key, label string) *Machine {
		m, err := d.RegisterMachine(uid, MachineRegisterInput{
			MachineKey: key,
			Label:      label,
			Platform:   "macos",
			App:        "desktop",
		})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}

	machineIDOf := func(agentID string) string {
		var mid string
		if err := d.SQL.QueryRow(`SELECT COALESCE(machine_id,'') FROM agents WHERE id=$1`, agentID).Scan(&mid); err != nil {
			t.Fatal(err)
		}
		return mid
	}

	setMid := func(agentID, mid string) {
		if _, err := d.SQL.Exec(`UPDATE agents SET machine_id=$1 WHERE id=$2`, mid, agentID); err != nil {
			t.Fatal(err)
		}
	}

	// Simulate v1 already applied so corrective path runs.
	if _, err := d.SQL.Exec(
		`INSERT INTO app_migrations (name) VALUES ($1) ON CONFLICT (name) DO NOTHING`,
		agentMachineIDBackfillV1Marker,
	); err != nil {
		t.Fatal(err)
	}
	// Reset clear_multi + v2 so migrate re-runs both.
	if _, err := d.SQL.Exec(
		`DELETE FROM app_migrations WHERE name IN ($1, $2)`,
		agentMachineIDClearMultiMarker, agentMachineIDBackfillV2Marker,
	); err != nil {
		t.Fatal(err)
	}

	// --- user E: >1 machines, all agents same non-empty mid (v1 bulk signature) → clear
	ue := newUser("mid-clear")
	me1 := reg(ue, "key-e1", "Mac E1")
	_ = reg(ue, "key-e2", "Mac E2")
	e1, err := d.CreateAgent(ue, "bot-e1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	e2, err := d.CreateAgent(ue, "bot-e2", "", "")
	if err != nil {
		t.Fatal(err)
	}
	setMid(e1.ID, me1.ID)
	setMid(e2.ID, me1.ID)

	// --- user F: >1 machines, mixed machine_ids → leave alone
	uf := newUser("mid-mixed")
	mf1 := reg(uf, "key-f1", "Mac F1")
	mf2 := reg(uf, "key-f2", "Mac F2")
	f1, err := d.CreateAgent(uf, "bot-f1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	f2, err := d.CreateAgent(uf, "bot-f2", "", "")
	if err != nil {
		t.Fatal(err)
	}
	setMid(f1.ID, mf1.ID)
	setMid(f2.ID, mf2.ID)

	// --- user G: exactly one machine, all same mid → do NOT clear (correct single-machine bind)
	ug := newUser("mid-keep-one")
	mg := reg(ug, "key-g", "Mac G")
	g1, err := d.CreateAgent(ug, "bot-g1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	setMid(g1.ID, mg.ID)

	if err := d.migrateAgentMachineID(); err != nil {
		t.Fatal(err)
	}

	if got := machineIDOf(e1.ID); got != "" {
		t.Fatalf("user E bulk signature must clear, got %q", got)
	}
	if got := machineIDOf(e2.ID); got != "" {
		t.Fatalf("user E bot2 must clear, got %q", got)
	}
	if got := machineIDOf(f1.ID); got != mf1.ID {
		t.Fatalf("user F mixed must keep %q, got %q", mf1.ID, got)
	}
	if got := machineIDOf(f2.ID); got != mf2.ID {
		t.Fatalf("user F mixed must keep %q, got %q", mf2.ID, got)
	}
	if got := machineIDOf(g1.ID); got != mg.ID {
		t.Fatalf("user G single-machine must keep %q, got %q", mg.ID, got)
	}

	// Idempotent clear marker: re-bind E to same mid, second migrate must not clear again.
	setMid(e1.ID, me1.ID)
	setMid(e2.ID, me1.ID)
	if err := d.migrateAgentMachineID(); err != nil {
		t.Fatal(err)
	}
	if got := machineIDOf(e1.ID); got != me1.ID {
		t.Fatalf("after clear_multi marker, must not re-clear; got %q", got)
	}
}
