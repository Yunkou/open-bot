package db

import (
	"fmt"
	"strings"
)

// Migration marker (app_migrations):
//
//	clear_assistant_auto_reply_to_v1 — one-time clear of Bot assistant rows that
//	  auto-copied the triggering user message into reply_to_id (pre-fix path in
//	  handleSendMessage → saveAssistantThreaded(..., userMsg.ID, ...)). Those rows
//	  have empty thread_root_id. Real user「回复」always sets thread_root_id via
//	  ResolveThreadRoot (parent.ThreadRootID or parent.ID), so they are not touched.
//
// Wire into (*DB).migrate() before migrateVector (see NOTES / db.migrate-hook.diff):
//
//	if err := d.migrateClearAssistantAutoReplyTo(); err != nil {
//		return err
//	}
const clearAssistantAutoReplyToV1Marker = "clear_assistant_auto_reply_to_v1"

// clearAssistantAutoReplyToSQL clears auto-quotes on assistant rows.
// Matches: role=assistant, reply_to → user message, thread_root empty/NULL.
const clearAssistantAutoReplyToSQL = `
UPDATE messages AS a
SET reply_to_id = NULL
FROM messages AS u
WHERE a.role = 'assistant'
  AND a.reply_to_id IS NOT NULL
  AND a.reply_to_id <> ''
  AND (a.thread_root_id IS NULL OR a.thread_root_id = '')
  AND u.id = a.reply_to_id
  AND u.conversation_id = a.conversation_id
  AND u.role = 'user'
`

// migrateClearAssistantAutoReplyTo clears historical Bot auto-quotes once.
// Idempotent via app_migrations marker. Safe to re-run after claim: no-op.
func (d *DB) migrateClearAssistantAutoReplyTo() error {
	if _, err := d.SQL.Exec(`
CREATE TABLE IF NOT EXISTS app_migrations (
  name       TEXT PRIMARY KEY,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`); err != nil {
		return fmt.Errorf("app_migrations: %w", err)
	}
	first, err := d.claimReplyToMigration(clearAssistantAutoReplyToV1Marker)
	if err != nil {
		return err
	}
	if !first {
		return nil
	}
	if err := d.clearAssistantAutoReplyTo(); err != nil {
		return err
	}
	return nil
}

// claimReplyToMigration inserts the marker; true if this call applied it first.
// Named separately so this pack does not collide with claimMigration in
// machine_id_migrate.go when both are present.
func (d *DB) claimReplyToMigration(name string) (bool, error) {
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

// clearAssistantAutoReplyTo runs the UPDATE (exported to tests via package).
func (d *DB) clearAssistantAutoReplyTo() error {
	if _, err := d.SQL.Exec(clearAssistantAutoReplyToSQL); err != nil {
		return fmt.Errorf("clear assistant auto reply_to: %w", err)
	}
	return nil
}

// shouldClearAssistantAutoReplyTo is the in-memory predicate matching the SQL
// filter (for unit tests without a DB). parentRole is the role of the message
// referenced by replyToID.
func shouldClearAssistantAutoReplyTo(role, replyToID, threadRootID, parentRole string) bool {
	if strings.TrimSpace(role) != "assistant" {
		return false
	}
	if strings.TrimSpace(replyToID) == "" {
		return false
	}
	if strings.TrimSpace(threadRootID) != "" {
		return false
	}
	return strings.TrimSpace(parentRole) == "user"
}
