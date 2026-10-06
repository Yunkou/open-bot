package db

import (
	"database/sql"
	"errors"
	"strings"
)

// AllowedReactionEmojis is the P0 whitelist shared with the web client.
var AllowedReactionEmojis = []string{"👍", "❤️", "😂", "🎉", "👀", "🙏", "✅", "❌", "👎"}

var allowedReactionSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(AllowedReactionEmojis))
	for _, e := range AllowedReactionEmojis {
		m[e] = struct{}{}
	}
	return m
}()

var ErrInvalidEmoji = errors.New("emoji not allowed")

// ReactionSummary is the aggregated shape returned on messages and SSE.
type ReactionSummary struct {
	Emoji string `json:"emoji"`
	Count int    `json:"count"`
	Me    bool   `json:"me"`
}

// MessageRef is a message owned by the requesting user (via conversation).
type MessageRef struct {
	ID             string
	ConversationID string
	UserID         string
}

func IsAllowedReactionEmoji(emoji string) bool {
	_, ok := allowedReactionSet[emoji]
	return ok
}

// GetMessageOwned returns a message if it belongs to a conversation owned by userID.
func (d *DB) GetMessageOwned(userID, messageID string) (*MessageRef, error) {
	row := d.SQL.QueryRow(
		`SELECT m.id, m.conversation_id, c.user_id
		 FROM messages m
		 JOIN conversations c ON c.id = m.conversation_id
		 WHERE m.id = $1 AND c.user_id = $2`,
		messageID, userID,
	)
	var ref MessageRef
	if err := row.Scan(&ref.ID, &ref.ConversationID, &ref.UserID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &ref, nil
}

// ToggleReaction adds the emoji if missing, otherwise removes it.
func (d *DB) ToggleReaction(userID, messageID, emoji string) (action string, count int, me bool, err error) {
	emoji = strings.TrimSpace(emoji)
	if !IsAllowedReactionEmoji(emoji) {
		return "", 0, false, ErrInvalidEmoji
	}
	var exists bool
	if err := d.SQL.QueryRow(
		`SELECT EXISTS(
			SELECT 1 FROM message_reactions
			WHERE message_id = $1 AND user_id = $2 AND emoji = $3
		)`,
		messageID, userID, emoji,
	).Scan(&exists); err != nil {
		return "", 0, false, err
	}
	if exists {
		if _, err := d.SQL.Exec(
			`DELETE FROM message_reactions
			 WHERE message_id = $1 AND user_id = $2 AND emoji = $3`,
			messageID, userID, emoji,
		); err != nil {
			return "", 0, false, err
		}
		action = "remove"
		me = false
	} else {
		if _, err := d.SQL.Exec(
			`INSERT INTO message_reactions (message_id, user_id, emoji, created_at)
			 VALUES ($1,$2,$3,$4)
			 ON CONFLICT (message_id, user_id, emoji) DO NOTHING`,
			messageID, userID, emoji, Now(),
		); err != nil {
			return "", 0, false, err
		}
		action = "add"
		me = true
	}
	count, err = d.CountReaction(messageID, emoji)
	if err != nil {
		return "", 0, false, err
	}
	return action, count, me, nil
}

// RemoveReaction explicitly deletes one reaction row.
func (d *DB) RemoveReaction(userID, messageID, emoji string) (removed bool, count int, err error) {
	emoji = strings.TrimSpace(emoji)
	if !IsAllowedReactionEmoji(emoji) {
		return false, 0, ErrInvalidEmoji
	}
	res, err := d.SQL.Exec(
		`DELETE FROM message_reactions
		 WHERE message_id = $1 AND user_id = $2 AND emoji = $3`,
		messageID, userID, emoji,
	)
	if err != nil {
		return false, 0, err
	}
	n, _ := res.RowsAffected()
	count, err = d.CountReaction(messageID, emoji)
	if err != nil {
		return false, 0, err
	}
	return n > 0, count, nil
}

func (d *DB) CountReaction(messageID, emoji string) (int, error) {
	var n int
	err := d.SQL.QueryRow(
		`SELECT COUNT(*) FROM message_reactions WHERE message_id = $1 AND emoji = $2`,
		messageID, emoji,
	).Scan(&n)
	return n, err
}

// ListReactionSummariesForConversation aggregates reactions for all messages in a conversation.
func (d *DB) ListReactionSummariesForConversation(userID, conversationID string) (map[string][]ReactionSummary, error) {
	out := make(map[string][]ReactionSummary)
	rows, err := d.SQL.Query(
		`SELECT r.message_id, r.emoji,
		        COUNT(*)::int AS cnt,
		        BOOL_OR(r.user_id = $1) AS me
		 FROM message_reactions r
		 JOIN messages m ON m.id = r.message_id
		 WHERE m.conversation_id = $2
		 GROUP BY r.message_id, r.emoji
		 ORDER BY r.message_id, MIN(r.created_at)`,
		userID, conversationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var mid, emoji string
		var cnt int
		var me bool
		if err := rows.Scan(&mid, &emoji, &cnt, &me); err != nil {
			return nil, err
		}
		out[mid] = append(out[mid], ReactionSummary{Emoji: emoji, Count: cnt, Me: me})
	}
	return out, rows.Err()
}
