package db

import (
	"database/sql"
	"errors"
	"strings"
)

// migrateConversationLastMachineID stores the desktop client machine that last
// sent a user message in this conversation (session host for green-dot online).
func (d *DB) migrateConversationLastMachineID() error {
	_, err := d.SQL.Exec(`ALTER TABLE conversations ADD COLUMN IF NOT EXISTS last_machine_id TEXT NOT NULL DEFAULT ''`)
	return err
}

// SetConversationLastMachineID records the session host for online / host routing display.
func (d *DB) SetConversationLastMachineID(userID, conversationID, machineID string) error {
	userID = strings.TrimSpace(userID)
	conversationID = strings.TrimSpace(conversationID)
	machineID = strings.TrimSpace(machineID)
	if userID == "" || conversationID == "" {
		return nil
	}
	_, err := d.SQL.Exec(
		`UPDATE conversations SET last_machine_id=$1, updated_at=$2 WHERE id=$3 AND user_id=$4`,
		machineID, Now(), conversationID, userID,
	)
	return err
}

// ConversationLastMachineID returns the session host id (may be empty).
func (d *DB) ConversationLastMachineID(userID, conversationID string) (string, error) {
	userID = strings.TrimSpace(userID)
	conversationID = strings.TrimSpace(conversationID)
	if userID == "" || conversationID == "" {
		return "", nil
	}
	var mid string
	err := d.SQL.QueryRow(
		`SELECT COALESCE(last_machine_id,'') FROM conversations WHERE id=$1 AND user_id=$2`,
		conversationID, userID,
	).Scan(&mid)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrNotFound
		}
		return "", err
	}
	return strings.TrimSpace(mid), nil
}

// ListAgentIDsByConversationLastMachine returns agent_ids of conversations whose
// session host is machineID (for bot_online fan-out when that host flips).
func (d *DB) ListAgentIDsByConversationLastMachine(userID, machineID string) ([]string, error) {
	userID = strings.TrimSpace(userID)
	machineID = strings.TrimSpace(machineID)
	if userID == "" || machineID == "" {
		return nil, nil
	}
	rows, err := d.SQL.Query(
		`SELECT DISTINCT agent_id FROM conversations
		 WHERE user_id=$1 AND COALESCE(last_machine_id,'')=$2 AND COALESCE(agent_id,'')<>''`,
		userID, machineID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		id = strings.TrimSpace(id)
		if id != "" {
			out = append(out, id)
		}
	}
	return out, rows.Err()
}

// ListAgentsWithEmptyMachineID returns agents that rely on「任一在线」for green-dot.
func (d *DB) ListAgentsWithEmptyMachineID(userID string) ([]*Agent, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, nil
	}
	rows, err := d.SQL.Query(
		`SELECT id, COALESCE(user_id,''), name, description, system_prompt, is_builtin, COALESCE(computer_mode,'team'),
		        COALESCE(avatar_shape,''), COALESCE(avatar_color,''), COALESCE(avatar_user_set,FALSE),
		        COALESCE(machine_id,''), COALESCE(machine_id_source,''), created_at, updated_at
		 FROM agents
		 WHERE user_id = $1 AND deleted_at IS NULL AND COALESCE(machine_id,'') = ''
		 ORDER BY created_at ASC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Agent
	for rows.Next() {
		var a Agent
		var uid string
		if err := rows.Scan(&a.ID, &uid, &a.Name, &a.Description, &a.SystemPrompt, &a.IsBuiltin, &a.ComputerMode, &a.AvatarShape, &a.AvatarColor, &a.AvatarUserSet, &a.MachineID, &a.MachineIDSource, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		a.UserID = uid
		if canon := CanonicalAvatarShape(a.AvatarShape); canon != "" {
			a.AvatarShape = canon
		}
		out = append(out, &a)
	}
	return out, rows.Err()
}
