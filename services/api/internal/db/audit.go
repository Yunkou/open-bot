package db

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
)

type AuditLog struct {
	ID          string    `json:"id"`
	OrgID       string    `json:"org_id"`
	ActorUserID string    `json:"actor_user_id"`
	Action      string    `json:"action"`
	TargetType  string    `json:"target_type"`
	TargetID    string    `json:"target_id"`
	MetaJSON    string    `json:"meta_json"`
	CreatedAt   time.Time `json:"created_at"`
}

type AuditLogPublic struct {
	ID            string `json:"id"`
	OrgID         string `json:"org_id"`
	ActorUserID   string `json:"actor_user_id"`
	ActorUsername string `json:"actor_username,omitempty"`
	Action        string `json:"action"`
	TargetType    string `json:"target_type"`
	TargetID      string `json:"target_id"`
	MetaJSON      string `json:"meta_json"`
	CreatedAt     string `json:"created_at"`
}

func (d *DB) migrateAuditLogs() error {
	_, err := d.SQL.Exec(`
CREATE TABLE IF NOT EXISTS audit_logs (
  id TEXT PRIMARY KEY,
  org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  actor_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
  action TEXT NOT NULL,
  target_type TEXT NOT NULL DEFAULT '',
  target_id TEXT NOT NULL DEFAULT '',
  meta_json TEXT NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_org_created
  ON audit_logs(org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action
  ON audit_logs(org_id, action);
`)
	return err
}

func (d *DB) InsertAuditLog(orgID, actorUserID, action, targetType, targetID, metaJSON string) error {
	orgID = strings.TrimSpace(orgID)
	action = strings.TrimSpace(action)
	if orgID == "" || action == "" {
		return nil
	}
	if strings.TrimSpace(metaJSON) == "" {
		metaJSON = "{}"
	}
	if !json.Valid([]byte(metaJSON)) {
		metaJSON = "{}"
	}
	_, err := d.SQL.Exec(`
INSERT INTO audit_logs (id, org_id, actor_user_id, action, target_type, target_id, meta_json, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
`, uuid.NewString(), orgID, nullIfEmpty(actorUserID), action,
		strings.TrimSpace(targetType), strings.TrimSpace(targetID), metaJSON, Now())
	return err
}

func (d *DB) ListAuditLogs(orgID string, limit int) ([]AuditLogPublic, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := d.SQL.Query(`
SELECT a.id, a.org_id, COALESCE(a.actor_user_id, ''), COALESCE(u.username, ''),
       a.action, a.target_type, a.target_id, a.meta_json, a.created_at
FROM audit_logs a
LEFT JOIN users u ON u.id = a.actor_user_id
WHERE a.org_id = $1
ORDER BY a.created_at DESC
LIMIT $2
`, orgID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AuditLogPublic, 0)
	for rows.Next() {
		var a AuditLogPublic
		var created time.Time
		if err := rows.Scan(
			&a.ID, &a.OrgID, &a.ActorUserID, &a.ActorUsername,
			&a.Action, &a.TargetType, &a.TargetID, &a.MetaJSON, &created,
		); err != nil {
			return nil, err
		}
		a.CreatedAt = FormatTime(created)
		out = append(out, a)
	}
	return out, rows.Err()
}
