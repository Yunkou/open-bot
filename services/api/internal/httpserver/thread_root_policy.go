package httpserver

import "strings"

// resolvePersistedThreadRootID decides messages.thread_root_id for a user send.
//
// Product rule (PM):
//   - Mainline sends — including explicit「回复」with reply_to_id only — leave
//     thread_root_id empty (Grok-style quote stays on the main timeline).
//   - Only an explicit client thread_root_id (sidebar thread post) is persisted.
//
// replyToID is accepted for documentation/tests; it must never invent a root.
func resolvePersistedThreadRootID(clientThreadRootID, replyToID string) string {
	_ = replyToID // reply_to alone never assigns thread membership
	return strings.TrimSpace(clientThreadRootID)
}
