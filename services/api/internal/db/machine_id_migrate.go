package db

import (
	"fmt"
	"strings"
)

// Migration markers (app_migrations):
//
//   agent_machine_id_backfill_v1 — old pack: single-machine OR unique-highest
//     file_op_count (machine-level usual host). That bound ALL unbound bots of a
//     multi-machine user to the same host and failed acceptance.
//   agent_machine_id_backfill_v2 — this pack: single-machine only.
//   agent_machine_id_backfill_v2_clear_multi — corrective when v1 already ran:
//     for users with >1 machines whose live agents all share one non-empty
//     machine_id (bulk-backfill signature), clear those bindings. Mixed or empty
//     → leave alone (user already differentiated or nothing to fix).
const (
	agentMachineIDBackfillV1Marker     = "agent_machine_id_backfill_v1"
	agentMachineIDBackfillV2Marker     = "agent_machine_id_backfill_v2"
	agentMachineIDClearMultiMarker     = "agent_machine_id_backfill_v2_clear_multi"
)

// migrateAgentMachineID runs after the machine_id column exists.
//
// Forward fill (v2): for each user with unbound agents, if they have exactly one
// user_machines row, bind empty machine_id to it. Multi-machine / zero → leave empty.
//
// computer_mode is sandbox layout only and is NOT used.
// Live hostHub Connected is unavailable at migrate time.
// file_op_count is machine-level (not Bot×machine) and is NOT used.
func (d *DB) migrateAgentMachineID() error {
	if _, err := d.SQL.Exec(`
CREATE TABLE IF NOT EXISTS app_migrations (
  name       TEXT PRIMARY KEY,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`); err != nil {
		return err
	}
	if err := d.maybeClearMultiMachineBulkBackfill(); err != nil {
		return err
	}
	tx, err := d.SQL.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(
		`INSERT INTO app_migrations (name) VALUES ($1) ON CONFLICT (name) DO NOTHING`,
		agentMachineIDBackfillV2Marker,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil // v2 already applied
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return d.backfillAgentMachineID()
}

// maybeClearMultiMachineBulkBackfill undoes the v1 multi-machine bulk bind when
// the bad-backfill signature is still present. Idempotent via clear_multi marker.
// Runs only if v1 was applied (otherwise there is nothing to correct).
func (d *DB) maybeClearMultiMachineBulkBackfill() error {
	var v1 int
	if err := d.SQL.QueryRow(
		`SELECT COUNT(*) FROM app_migrations WHERE name=$1`,
		agentMachineIDBackfillV1Marker,
	).Scan(&v1); err != nil {
		return err
	}
	if v1 == 0 {
		return nil
	}
	tx, err := d.SQL.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(
		`INSERT INTO app_migrations (name) VALUES ($1) ON CONFLICT (name) DO NOTHING`,
		agentMachineIDClearMultiMarker,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil // already corrected
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return d.clearMultiMachineBulkBackfill()
}

// clearMultiMachineBulkBackfill clears agent.machine_id for users who look like
// they received the v1 bulk bind: >1 registered machines AND every live agent
// shares the same non-empty machine_id. Mixed bindings are left untouched.
func (d *DB) clearMultiMachineBulkBackfill() error {
	rows, err := d.SQL.Query(`
SELECT a.user_id
FROM agents a
WHERE a.deleted_at IS NULL
  AND COALESCE(a.user_id,'') <> ''
GROUP BY a.user_id
HAVING COUNT(*) FILTER (WHERE COALESCE(a.machine_id,'') <> '') > 0
   AND COUNT(DISTINCT NULLIF(TRIM(a.machine_id), '')) = 1
   AND COUNT(*) FILTER (WHERE COALESCE(TRIM(a.machine_id),'') = '') = 0
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
	now := Now()
	for _, uid := range userIDs {
		machines, err := d.ListMachines(uid)
		if err != nil {
			return fmt.Errorf("list machines for clear %s: %w", uid, err)
		}
		if len(machines) <= 1 {
			continue // only correct multi-machine bulk bind
		}
		if _, err := d.SQL.Exec(
			`UPDATE agents SET machine_id='', updated_at=$1
			 WHERE user_id=$2 AND deleted_at IS NULL AND COALESCE(machine_id,'') <> ''`,
			now, uid,
		); err != nil {
			return fmt.Errorf("clear multi-machine bulk machine_id for %s: %w", uid, err)
		}
	}
	return nil
}

// backfillAgentMachineID fills empty machine_id where the user has exactly one machine.
func (d *DB) backfillAgentMachineID() error {
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
			`UPDATE agents SET machine_id=$1, updated_at=$2
			 WHERE user_id=$3 AND deleted_at IS NULL AND COALESCE(machine_id,'') = ''`,
			mid, now, uid,
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
// Does not use file_op_count (machine-level, not Bot×machine — would bind every
// unbound bot of a multi-machine user to the same host), computer_mode,
// last_seen alone, or Connected (unavailable at migrate).
func resolveUniqueBackfillMachineID(machines []Machine) string {
	if len(machines) != 1 {
		return ""
	}
	return strings.TrimSpace(machines[0].ID)
}
