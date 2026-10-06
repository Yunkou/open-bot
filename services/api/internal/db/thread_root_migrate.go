package db

import (
	"fmt"
	"strings"
)

// Migration marker (app_migrations):
//
//	clear_auto_thread_root_v1 — one-time clear of thread_root_id values that the
//	  old handleSendMessage path wrote via ResolveThreadRoot(parent) on every
//	  explicit「回复」(and Bot/summary rows that inherited that root). Those
//	  "threads" were really mainline quotes: typically a single user row with
//	  reply_to_id == thread_root_id (= parent.ID when parent was mainline) plus
//	  Bot children sharing the same thread_root. Real sidebar threads that grew
//	  multiple user members are left alone.
//
// Wire into (*DB).migrate() before migrateVector (see NOTES / db.migrate-hook.diff):
//
//	if err := d.migrateClearAutoThreadRoot(); err != nil {
//		return err
//	}
const clearAutoThreadRootV1Marker = "clear_auto_thread_root_v1"

// clearAutoThreadRootSQL clears shallow auto-threads created by ResolveThreadRoot
// on mainline quotes.
//
// Conservative filter — CLEAR a (conversation_id, thread_root_id) group when:
//   - exactly one user message has that thread_root_id, AND
//   - that user message has reply_to_id = thread_root_id
//     (classic ResolveThreadRoot on a mainline parent: root := parent.ID)
//
// LEFT ALONE (may still hide from mainline until a future migrate / manual fix):
//   - Groups with 2+ user messages (continued sidebar conversation)
//   - Groups whose sole user has reply_to_id != thread_root_id
//     (joined an existing root by inheriting parent.ThreadRootID)
//   - Messages with empty/NULL thread_root_id
const clearAutoThreadRootSQL = `
WITH auto_roots AS (
  SELECT conversation_id, thread_root_id
  FROM messages
  WHERE thread_root_id IS NOT NULL
    AND thread_root_id <> ''
  GROUP BY conversation_id, thread_root_id
  HAVING COUNT(*) FILTER (WHERE role = 'user') = 1
     AND COUNT(*) FILTER (
           WHERE role = 'user'
             AND reply_to_id IS NOT NULL
             AND reply_to_id <> ''
             AND reply_to_id = thread_root_id
         ) = 1
)
UPDATE messages AS m
SET thread_root_id = NULL
FROM auto_roots AS a
WHERE m.conversation_id = a.conversation_id
  AND m.thread_root_id = a.thread_root_id
`

// migrateClearAutoThreadRoot clears historical auto thread_root rows once.
// Idempotent via app_migrations marker. Safe to re-run after claim: no-op.
func (d *DB) migrateClearAutoThreadRoot() error {
	if _, err := d.SQL.Exec(`
CREATE TABLE IF NOT EXISTS app_migrations (
  name       TEXT PRIMARY KEY,
  applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
)`); err != nil {
		return fmt.Errorf("app_migrations: %w", err)
	}
	first, err := d.claimThreadRootMigration(clearAutoThreadRootV1Marker)
	if err != nil {
		return err
	}
	if !first {
		return nil
	}
	if err := d.clearAutoThreadRoot(); err != nil {
		return err
	}
	return nil
}

// claimThreadRootMigration inserts the marker; true if this call applied it first.
// Named separately so this pack does not collide with claimMigration /
// claimReplyToMigration when those files are also present.
func (d *DB) claimThreadRootMigration(name string) (bool, error) {
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

func (d *DB) clearAutoThreadRoot() error {
	if _, err := d.SQL.Exec(clearAutoThreadRootSQL); err != nil {
		return fmt.Errorf("clear auto thread_root: %w", err)
	}
	return nil
}

// isShallowAutoThreadRoot is the in-memory predicate matching the SQL HAVING
// clause (for unit tests without a DB).
// userN = count of role=user in the group; quoteRootUserN = count of users with
// reply_to_id == thread_root_id.
func isShallowAutoThreadRoot(userN, quoteRootUserN int) bool {
	return userN == 1 && quoteRootUserN == 1
}

// userMessageLooksLikeAutoThreadSeed matches the classic ResolveThreadRoot
// mainline-parent case: thread_root_id == reply_to_id == parent.ID.
func userMessageLooksLikeAutoThreadSeed(replyToID, threadRootID string) bool {
	r := strings.TrimSpace(replyToID)
	t := strings.TrimSpace(threadRootID)
	return r != "" && t != "" && r == t
}
