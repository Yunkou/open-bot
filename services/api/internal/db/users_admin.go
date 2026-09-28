package db

import (
	"errors"
	"strings"
)

// CountPlatformAdmins returns how many non-system platform_admin users exist.
func (d *DB) CountPlatformAdmins() (int, error) {
	var n int
	err := d.SQL.QueryRow(`
SELECT COUNT(*) FROM users
WHERE role = $1 AND LOWER(username) <> LOWER($2)
`, RolePlatformAdmin, A2ASystemUsername).Scan(&n)
	return n, err
}

// UpdateUserEmail sets email for a user (empty clears to "").
func (d *DB) UpdateUserEmail(userID, email string) error {
	res, err := d.SQL.Exec(`
UPDATE users SET email = $2 WHERE id = $1 AND LOWER(username) <> LOWER($3)
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
UPDATE users SET password_hash = $2 WHERE id = $1 AND LOWER(username) <> LOWER($3)
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

// DeleteUser hard-deletes a user. Related rows cascade via FK ON DELETE CASCADE
// (agents, conversations, llm_connections, …). audit_logs.actor_user_id and
// org_invites.invited_by use ON DELETE SET NULL.
// Caller must enforce safeguards (self, last platform_admin, __a2a__).
func (d *DB) DeleteUser(userID string) error {
	u, err := d.GetUserByID(userID)
	if err != nil {
		return err
	}
	if strings.EqualFold(u.Username, A2ASystemUsername) {
		return errors.New("cannot delete system user")
	}
	res, err := d.SQL.Exec(`DELETE FROM users WHERE id = $1 AND LOWER(username) <> LOWER($2)`, userID, A2ASystemUsername)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
