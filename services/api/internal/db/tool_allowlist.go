package db

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// UserToolAllowlistEntry is a persisted "Always allow" preference for a tool
// family (tool_group) and/or exact tool name. Distinct from NL auto_review_rules.
type UserToolAllowlistEntry struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	ToolGroup string    `json:"tool_group"` // e.g. shell | git | file_write | file_delete | <tool>
	Tool      string    `json:"tool"`       // optional exact tool, e.g. host_shell
	AgentID   string    `json:"agent_id"`   // empty = all agents (MVP)
	CreatedAt time.Time `json:"created_at"`
}

func (d *DB) migrateUserToolAllowlist() error {
	_, err := d.SQL.Exec(`
CREATE TABLE IF NOT EXISTS user_tool_allowlist (
  id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  tool_group TEXT NOT NULL DEFAULT '',
  tool TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS user_tool_allowlist_uniq
  ON user_tool_allowlist (user_id, tool_group, tool, agent_id);
CREATE INDEX IF NOT EXISTS user_tool_allowlist_user_idx
  ON user_tool_allowlist (user_id);
`)
	return err
}

// UpsertUserToolAllowlist inserts or refreshes an Always-allow entry.
// MVP key: user_id + tool_group + tool + agent_id (agent_id usually empty).
func (d *DB) UpsertUserToolAllowlist(userID, toolGroup, tool, agentID string) (UserToolAllowlistEntry, error) {
	userID = strings.TrimSpace(userID)
	toolGroup = strings.TrimSpace(toolGroup)
	tool = strings.TrimSpace(tool)
	agentID = strings.TrimSpace(agentID)
	if userID == "" {
		return UserToolAllowlistEntry{}, errors.New("user_id required")
	}
	if toolGroup == "" && tool == "" {
		return UserToolAllowlistEntry{}, errors.New("tool_group or tool required")
	}
	if toolGroup == "" {
		toolGroup = tool
	}
	if len([]rune(toolGroup)) > 64 || len([]rune(tool)) > 128 || len([]rune(agentID)) > 64 {
		return UserToolAllowlistEntry{}, errors.New("tool_group/tool/agent_id too long")
	}
	now := Now()
	id := uuid.NewString()
	_, err := d.SQL.Exec(`
INSERT INTO user_tool_allowlist (id, user_id, tool_group, tool, agent_id, created_at)
VALUES ($1, $2, $3, $4, $5, $6)
ON CONFLICT (user_id, tool_group, tool, agent_id) DO UPDATE SET
  created_at = EXCLUDED.created_at`,
		id, userID, toolGroup, tool, agentID, now)
	if err != nil {
		return UserToolAllowlistEntry{}, err
	}
	row := d.SQL.QueryRow(`
SELECT id, user_id, tool_group, tool, agent_id, created_at
FROM user_tool_allowlist
WHERE user_id = $1 AND tool_group = $2 AND tool = $3 AND agent_id = $4`,
		userID, toolGroup, tool, agentID)
	var e UserToolAllowlistEntry
	if err := row.Scan(&e.ID, &e.UserID, &e.ToolGroup, &e.Tool, &e.AgentID, &e.CreatedAt); err != nil {
		return UserToolAllowlistEntry{}, err
	}
	return e, nil
}

func (d *DB) ListUserToolAllowlist(userID string) ([]UserToolAllowlistEntry, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, errors.New("user_id required")
	}
	rows, err := d.SQL.Query(`
SELECT id, user_id, tool_group, tool, agent_id, created_at
FROM user_tool_allowlist
WHERE user_id = $1
ORDER BY created_at DESC, tool_group ASC, tool ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]UserToolAllowlistEntry, 0)
	for rows.Next() {
		var e UserToolAllowlistEntry
		if err := rows.Scan(&e.ID, &e.UserID, &e.ToolGroup, &e.Tool, &e.AgentID, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (d *DB) DeleteUserToolAllowlist(userID, entryID string) error {
	userID = strings.TrimSpace(userID)
	entryID = strings.TrimSpace(entryID)
	if userID == "" || entryID == "" {
		return errors.New("user_id and id required")
	}
	res, err := d.SQL.Exec(`
DELETE FROM user_tool_allowlist WHERE id = $1 AND user_id = $2`, entryID, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// MatchUserToolAllowlist reports whether any entry matches tool/tool_group
// (and optional agentID). Empty agent on entry matches all agents.
func MatchUserToolAllowlist(entries []UserToolAllowlistEntry, tool, toolGroup, agentID string) bool {
	tool = strings.TrimSpace(tool)
	toolGroup = strings.TrimSpace(toolGroup)
	agentID = strings.TrimSpace(agentID)
	if tool == "" && toolGroup == "" {
		return false
	}
	for _, e := range entries {
		if strings.TrimSpace(e.AgentID) != "" {
			if agentID == "" || e.AgentID != agentID {
				continue
			}
		}
		eg := strings.TrimSpace(e.ToolGroup)
		et := strings.TrimSpace(e.Tool)
		if et != "" && tool != "" && strings.EqualFold(et, tool) {
			return true
		}
		if eg != "" && toolGroup != "" && strings.EqualFold(eg, toolGroup) {
			return true
		}
		// Entry stored as tool-only with tool_group==tool (Upsert default).
		if eg != "" && tool != "" && strings.EqualFold(eg, tool) {
			return true
		}
	}
	return false
}
