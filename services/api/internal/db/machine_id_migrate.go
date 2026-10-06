package db

import (
	"fmt"
	"strings"
)

// agents.machine_id_source values. Server-side only; clients never send it.
const (
	// MachineIDSourceNone: unbound, or legacy value written before the source
	// column existed (provenance unknown → treated as user-owned, never cleared).
	MachineIDSourceNone = ""
	// MachineIDSourceMigrate: written by the one-time backfill. Only this value
	// may be cleared by a corrective migration.
	MachineIDSourceMigrate = "migrate"
	// MachineIDSourceUser: written by CREATE/PATCH (SetAgentMachineID).
	MachineIDSourceUser = "user"
)

// Migration markers (app_migrations):
//
//	agent_machine_id_backfill_v1 — old pack: single-machine OR unique-highest
//	  file_op_count. Bound ALL unbound bots of a multi-machine user to one host.
//	  Wrote no source mark.
//	agent_machine_id_backfill_v2 — single-machine only; now stamps
//	  machine_id_source='migrate' on the rows it fills.
//	agent_machine_id_backfill_v2_clear_multi — RETIRED. Old pack cleared every
//	  binding of a >1-machine user whose bots all shared one machine_id. That
//	  signature also matches an intentional "all bots on this Mac" user pick, so
//	  it is no longer run. Marker name kept only for history / tests.
//	agent_machine_id_source_v3 — source-only corrective: for users with >1
//	  machines, clear machine_id only where machine_id_source='migrate'. Rows with
//	  source 'user' or '' (legacy unknown) are never touched.
const (
	agentMachineIDBackfillV1Marker = "agent_machine_id_backfill_v1"
	agentMachineIDBackfillV2Marker = "agent_machine_id_backfill_v2"
	agentMachineIDClearMultiMarker = "agent_machine_id_backfill_v2_clear_multi" // retired; not run
	agentMachineIDSourceV3Marker   = "agent_machine_id_source_v3"
)

// migrateAgentMachineID runs after machine_id + machine_id_source columns exist.
//
//  1. v2 forward fill (once): users with exactly one machine → bind empty agents
//     to it, source='migrate'. 0 or >1 machines → leave empty.
//  2. v3 corrective (once): users with >1 machines → clear rows whose
//     source='migrate'. Never clears 'user' or legacy ”.
//
// computer_mode, Connected, last_seen alone and file_op_count are NOT used.
func (d *DB) migrateAgentMachineID() error {
	if _, err := d.SQL.Exec(`
CREATE TABLE IF NOT EXISTS app_migrations (
  name       TEXT PRIMARY KEY,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`); err != nil {
		return err
	}
	if first, err := d.claimMigration(agentMachineIDBackfillV2Marker); err != nil {
		return err
	} else if first {
		if err := d.backfillAgentMachineID(nil); err != nil {
			return err
		}
	}
	if first, err := d.claimMigration(agentMachineIDSourceV3Marker); err != nil {
		return err
	} else if first {
		if err := d.clearMigrateSourcedMultiMachine(nil); err != nil {
			return err
		}
	}
	return nil
}

// claimMigration inserts the marker; true if this call applied it first.
func (d *DB) claimMigration(name string) (bool, error) {
	res, err := d.SQL.Exec(
		`INSERT INTO app_migrations (name) VALUES ($1) ON CONFLICT (name) DO NOTHING`,
		name,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// clearMigrateSourcedMultiMachine clears bindings written by migrate for users
// that have >1 registered machines. User picks (source='user') — including an
// intentional "every bot on one machine" — and legacy unmarked rows (source=”)
// are never cleared; no heuristic tries to guess provenance.
//
// onlyUsers nil = all users (migration). Non-nil scopes to those user ids
// (tests must not mutate other users on a shared develop DB).
func (d *DB) clearMigrateSourcedMultiMachine(onlyUsers []string) error {
	q := `
UPDATE agents a
   SET machine_id='', machine_id_source='', updated_at=$1
 WHERE a.deleted_at IS NULL
   AND a.machine_id_source = $2
   AND (SELECT COUNT(*) FROM user_machines m WHERE m.user_id = a.user_id) > 1`
	args := []any{Now(), MachineIDSourceMigrate}
	if onlyUsers != nil {
		if len(onlyUsers) == 0 {
			return nil
		}
		q += ` AND a.user_id = ANY($3)`
		args = append(args, onlyUsers)
	}
	if _, err := d.SQL.Exec(q, args...); err != nil {
		return fmt.Errorf("clear migrate-sourced machine_id: %w", err)
	}
	return nil
}

// backfillAgentMachineID fills empty machine_id where the user has exactly one
// machine and stamps machine_id_source='migrate'. onlyUsers nil = all users;
// non-nil scopes (tests).
func (d *DB) backfillAgentMachineID(onlyUsers []string) error {
	rows, err := d.SQL.Query(`
SELECT DISTINCT user_id
FROM agents
WHERE deleted_at IS NULL
  AND COALESCE(user_id,'') <> ''
  AND COALESCE(machine_id,'') = ''
ORDER BY user_id
`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var userIDs []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return err
		}
		uid = strings.TrimSpace(uid)
		if uid != "" {
			userIDs = append(userIDs, uid)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if onlyUsers != nil {
		keep := make(map[string]bool, len(onlyUsers))
		for _, u := range onlyUsers {
			keep[u] = true
		}
		filtered := userIDs[:0]
		for _, u := range userIDs {
			if keep[u] {
				filtered = append(filtered, u)
			}
		}
		userIDs = filtered
	}
	now := Now()
	for _, uid := range userIDs {
		machines, err := d.ListMachines(uid)
		if err != nil {
			return fmt.Errorf("list machines for %s: %w", uid, err)
		}
		mid := resolveUniqueBackfillMachineID(machines)
		if mid == "" {
			continue
		}
		if _, err := d.SQL.Exec(
			`UPDATE agents SET machine_id=$1, machine_id_source=$2, updated_at=$3
			 WHERE user_id=$4 AND deleted_at IS NULL AND COALESCE(machine_id,'') = ''`,
			mid, MachineIDSourceMigrate, now, uid,
		); err != nil {
			return fmt.Errorf("backfill machine_id for %s: %w", uid, err)
		}
	}
	return nil
}

// resolveUniqueBackfillMachineID picks a host only when unambiguous.
//
// Heuristic (prefer empty over wrong):
//  1. Exactly one registered machine for the user → that id.
//  2. Otherwise "" (leave unbound; user picks via PATCH).
//
// Does not use file_op_count (machine-level, not Bot×machine), computer_mode,
// last_seen alone, or Connected (unavailable at migrate).
func resolveUniqueBackfillMachineID(machines []Machine) string {
	if len(machines) != 1 {
		return ""
	}
	return strings.TrimSpace(machines[0].ID)
}

// cloneMachineIDSource copies the source's provenance as-is; an empty binding
// always has empty source.
func cloneMachineIDSource(machineID, source string) string {
	if strings.TrimSpace(machineID) == "" {
		return MachineIDSourceNone
	}
	return strings.TrimSpace(source)
}
