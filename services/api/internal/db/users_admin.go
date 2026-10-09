package db

import (
	"errors"
	"strings"
)

// CountPlatformAdmins returns how many live non-system platform_admin users exist.
func (d *DB) CountPlatformAdmins() (int, error) {
	var n int
	err := d.SQL.QueryRow(`
SELECT COUNT(*) FROM users
WHERE role = $1 AND LOWER(username) <> LOWER($2) AND deleted_at IS NULL
`, RolePlatformAdmin, A2ASystemUsername).Scan(&n)
	return n, err
}

// UpdateUserEmail sets email for a user (empty clears to "").
func (d *DB) UpdateUserEmail(userID, email string) error {
	res, err := d.SQL.Exec(`
UPDATE users SET email = $2
WHERE id = $1 AND LOWER(username) <> LOWER($3) AND deleted_at IS NULL
`, userID, strings.TrimSpace(email), A2ASystemUsername)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetUserPasswordHash updates password_hash only (never returned in JSON).
func (d *DB) SetUserPasswordHash(userID, passwordHash string) error {
	passwordHash = strings.TrimSpace(passwordHash)
	if passwordHash == "" {
		return errors.New("password hash required")
	}
	res, err := d.SQL.Exec(`
UPDATE users SET password_hash = $2
WHERE id = $1 AND LOWER(username) <> LOWER($3) AND deleted_at IS NULL
`, userID, passwordHash, A2ASystemUsername)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SoftDeleteUser marks a user and all of its agents as deleted, and disables the
// user's routines, in one transaction. Rows stay in the database for audit.
// Returns the ids of the cascaded agents so the caller can record them.
// Admin HTTP delete calls PurgeUserData first (while the account is live); after
// a full purge this typically only sets users.deleted_at (agents already gone).
// Caller must enforce safeguards (self, last platform_admin, __a2a__, same org).
func (d *DB) SoftDeleteUser(userID string) ([]string, error) {
	tx, err := d.SQL.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	now := Now()

	rows, err := tx.Query(`
UPDATE agents SET deleted_at = $2, updated_at = $2
WHERE user_id = $1 AND deleted_at IS NULL
RETURNING id
`, userID, now)
	if err != nil {
		return nil, err
	}
	agentIDs := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		agentIDs = append(agentIDs, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	if _, err := tx.Exec(`
UPDATE routines SET enabled = FALSE, updated_at = $2
WHERE user_id = $1 AND enabled = TRUE
`, userID, now); err != nil {
		return nil, err
	}

	if _, err := tx.Exec(`
UPDATE conversation_tasks
SET status = 'cancelled', updated_at = $2, finished_at = $2, lease_until = NULL
WHERE user_id = $1 AND status IN ('queued', 'running')
`, userID, now); err != nil {
		return nil, err
	}

	res, err := tx.Exec(`
UPDATE users SET deleted_at = $2
WHERE id = $1 AND deleted_at IS NULL AND LOWER(username) <> LOWER($3)
`, userID, now, A2ASystemUsername)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return agentIDs, nil
}
