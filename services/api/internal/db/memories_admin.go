package db

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// MemoryAdminFilter narrows org-scoped memory and compaction listings.
type MemoryAdminFilter struct {
	Scope       string
	UserID      string
	AgentID     string
	ChannelID   string
	PeerAgentID string
	Tier        string
	Limit       int
}

// AdminMemory is one explicit memory row for the admin console.
type AdminMemory struct {
	ID            string
	UserID        string
	Username      string
	Scope         string
	AgentID       string
	AgentName     string
	ChannelID     string
	ChannelName   string
	PeerAgentID   string
	PeerAgentName string
	Tier          string
	Content       string
	Tags          string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// AdminCompaction is one conversation summary (messages.role = summary).
type AdminCompaction struct {
	ID             string
	Content        string
	CreatedAt      time.Time
	ConversationID string
	Title          string
	UserID         string
	Username       string
	AgentID        string
	AgentName      string
	ChannelID      string
	ChannelName    string
	Scope          string
}

// AdminChannel is a group chat in an org, for memory filters.
type AdminChannel struct {
	ID          string
	UserID      string
	Username    string
	Name        string
	CreatedAt   time.Time
	MemberIDs   string
	MemberNames string
}

// NormalizeAgentPair orders two bot ids so A-B and B-A match the same memory row.
func NormalizeAgentPair(a, b string) (string, string) {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" || a <= b {
		return a, b
	}
	return b, a
}

func clampAdminLimit(n, def, max int) int {
	if n <= 0 {
		return def
	}
	if n > max {
		return max
	}
	return n
}

// ListOrgMemories returns explicit memories owned by live users in orgID.
func (d *DB) ListOrgMemories(orgID string, f MemoryAdminFilter) ([]AdminMemory, error) {
	f.Limit = clampAdminLimit(f.Limit, 100, 200)
	if f.Scope == "agent_pair" {
		f.AgentID, f.PeerAgentID = NormalizeAgentPair(f.AgentID, f.PeerAgentID)
	}
	conds := []string{
		"u.org_id = $1",
		"u.deleted_at IS NULL",
		"LOWER(u.username) <> LOWER($2)",
	}
	args := []any{orgID, A2ASystemUsername}
	add := func(column, val string) {
		val = strings.TrimSpace(val)
		if val == "" {
			return
		}
		args = append(args, val)
		conds = append(conds, column+" = $"+itoa(len(args)))
	}
	if s := strings.TrimSpace(f.Scope); s != "" {
		args = append(args, s)
		conds = append(conds, "m.scope = $"+itoa(len(args)))
	}
	add("m.user_id", f.UserID)
	add("m.agent_id", f.AgentID)
	add("m.channel_id", f.ChannelID)
	add("m.peer_agent_id", f.PeerAgentID)
	add("m.tier", f.Tier)
	args = append(args, f.Limit)
	q := `
SELECT m.id, m.user_id, COALESCE(u.username, ''), COALESCE(m.scope, 'user'),
       COALESCE(m.agent_id, ''), COALESCE(ag.name, ''),
       COALESCE(m.channel_id, ''), COALESCE(ch.name, ''),
       COALESCE(m.peer_agent_id, ''), COALESCE(peer.name, ''),
       m.tier, m.content, COALESCE(array_to_string(m.tags, ','), ''),
       m.created_at, m.updated_at
FROM memories m
JOIN users u ON u.id = m.user_id
LEFT JOIN agents ag ON ag.id = m.agent_id AND ag.user_id = m.user_id AND ag.deleted_at IS NULL
LEFT JOIN channels ch ON ch.id = m.channel_id AND ch.user_id = m.user_id
LEFT JOIN agents peer ON peer.id = m.peer_agent_id AND peer.user_id = m.user_id AND peer.deleted_at IS NULL
WHERE ` + strings.Join(conds, " AND ") + `
ORDER BY m.updated_at DESC
LIMIT $` + itoa(len(args))
	rows, err := d.SQL.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AdminMemory, 0)
	for rows.Next() {
		var item AdminMemory
		if err := rows.Scan(
			&item.ID, &item.UserID, &item.Username, &item.Scope,
			&item.AgentID, &item.AgentName,
			&item.ChannelID, &item.ChannelName,
			&item.PeerAgentID, &item.PeerAgentName,
			&item.Tier, &item.Content, &item.Tags,
			&item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// ListOrgCompactions returns summary messages for live users in orgID.
// Scope "bot" is a user-bot thread (no channel). Scope "channel" is a group thread.
func (d *DB) ListOrgCompactions(orgID string, f MemoryAdminFilter) ([]AdminCompaction, error) {
	f.Limit = clampAdminLimit(f.Limit, 100, 200)
	conds := []string{
		"msg.role = 'summary'",
		"u.org_id = $1",
		"u.deleted_at IS NULL",
		"LOWER(u.username) <> LOWER($2)",
	}
	args := []any{orgID, A2ASystemUsername}
	switch strings.TrimSpace(f.Scope) {
	case "bot":
		conds = append(conds, "(c.channel_id IS NULL OR c.channel_id = '')")
	case "channel":
		conds = append(conds, "c.channel_id IS NOT NULL AND c.channel_id <> ''")
	}
	add := func(cond, val string) {
		if strings.TrimSpace(val) == "" {
			return
		}
		args = append(args, strings.TrimSpace(val))
		conds = append(conds, cond+" $"+itoa(len(args)))
	}
	add("c.user_id =", f.UserID)
	add("c.agent_id =", f.AgentID)
	add("c.channel_id =", f.ChannelID)
	args = append(args, f.Limit)
	q := `
SELECT msg.id, msg.content, msg.created_at,
       c.id, c.title, c.user_id, COALESCE(u.username, ''),
       c.agent_id, COALESCE(ag.name, ''),
       COALESCE(c.channel_id, ''), COALESCE(ch.name, '')
FROM messages msg
JOIN conversations c ON c.id = msg.conversation_id
JOIN users u ON u.id = c.user_id
LEFT JOIN agents ag ON ag.id = c.agent_id AND ag.deleted_at IS NULL
LEFT JOIN channels ch ON ch.id = c.channel_id
WHERE ` + strings.Join(conds, " AND ") + `
ORDER BY msg.created_at DESC
LIMIT $` + itoa(len(args))
	rows, err := d.SQL.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AdminCompaction, 0)
	for rows.Next() {
		var item AdminCompaction
		if err := rows.Scan(
			&item.ID, &item.Content, &item.CreatedAt,
			&item.ConversationID, &item.Title, &item.UserID, &item.Username,
			&item.AgentID, &item.AgentName,
			&item.ChannelID, &item.ChannelName,
		); err != nil {
			return nil, err
		}
		if item.ChannelID != "" {
			item.Scope = "channel"
		} else {
			item.Scope = "bot"
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// ListOrgChannels returns group chats owned by live users in orgID.
func (d *DB) ListOrgChannels(orgID string) ([]AdminChannel, error) {
	rows, err := d.SQL.Query(`
SELECT c.id, c.user_id, COALESCE(u.username, ''), c.name, c.created_at,
       COALESCE(string_agg(cm.agent_id, ',' ORDER BY cm.agent_id), ''),
       COALESCE(string_agg(ag.name, '、' ORDER BY cm.agent_id), '')
FROM channels c
JOIN users u ON u.id = c.user_id
LEFT JOIN channel_members cm ON cm.channel_id = c.id
LEFT JOIN agents ag ON ag.id = cm.agent_id AND ag.deleted_at IS NULL
WHERE u.org_id = $1 AND u.deleted_at IS NULL
  AND LOWER(u.username) <> LOWER($2)
GROUP BY c.id, c.user_id, u.username, c.name, c.created_at
ORDER BY c.created_at DESC
`, orgID, A2ASystemUsername)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AdminChannel, 0)
	for rows.Next() {
		var item AdminChannel
		if err := rows.Scan(&item.ID, &item.UserID, &item.Username, &item.Name, &item.CreatedAt, &item.MemberIDs, &item.MemberNames); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// ChannelInOrg reports whether the channel belongs to a live user in orgID.
func (d *DB) ChannelInOrg(id, orgID string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, nil
	}
	var one int
	err := d.SQL.QueryRow(`
SELECT 1 FROM channels c
JOIN users u ON u.id = c.user_id
WHERE c.id = $1 AND u.org_id = $2 AND u.deleted_at IS NULL
`, id, orgID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
