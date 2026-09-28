package db

import (
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"
)

// AgentWithOwner is an agent row plus owner username for admin listing.
type AgentWithOwner struct {
	Agent
	OwnerUsername string `json:"owner_username"`
}

func scanAgentRow(scan func(dest ...any) error) (*Agent, error) {
	var a Agent
	var uid string
	if err := scan(&a.ID, &uid, &a.Name, &a.Description, &a.SystemPrompt, &a.IsBuiltin, &a.ComputerMode, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return nil, err
	}
	a.UserID = uid
	return &a, nil
}

const agentSelectCols = `id, COALESCE(user_id,''), name, description, system_prompt, is_builtin, COALESCE(computer_mode,'team'), created_at, updated_at`

// ListAgentsByOrg returns all agents owned by users in the given org.
func (d *DB) ListAgentsByOrg(orgID string) ([]AgentWithOwner, error) {
	rows, err := d.SQL.Query(`
SELECT a.id, COALESCE(a.user_id,''), a.name, a.description, a.system_prompt, a.is_builtin,
       COALESCE(a.computer_mode,'team'), a.created_at, a.updated_at, COALESCE(u.username, '')
FROM agents a
JOIN users u ON u.id = a.user_id
WHERE u.org_id = $1 AND LOWER(u.username) <> LOWER($2)
ORDER BY a.created_at DESC
`, orgID, A2ASystemUsername)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]AgentWithOwner, 0)
	for rows.Next() {
		var a Agent
		var uid, owner string
		if err := rows.Scan(&a.ID, &uid, &a.Name, &a.Description, &a.SystemPrompt, &a.IsBuiltin, &a.ComputerMode, &a.CreatedAt, &a.UpdatedAt, &owner); err != nil {
			return nil, err
		}
		a.UserID = uid
		out = append(out, AgentWithOwner{Agent: a, OwnerUsername: owner})
	}
	return out, rows.Err()
}

// GetAgentByID loads an agent by id without user ownership filter.
func (d *DB) GetAgentByID(id string) (*Agent, error) {
	row := d.SQL.QueryRow(`SELECT `+agentSelectCols+` FROM agents WHERE id = $1`, id)
	a, err := scanAgentRow(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return a, nil
}

// GetAgentInOrg returns the agent if its owner belongs to orgID.
func (d *DB) GetAgentInOrg(id, orgID string) (*AgentWithOwner, error) {
	row := d.SQL.QueryRow(`
SELECT a.id, COALESCE(a.user_id,''), a.name, a.description, a.system_prompt, a.is_builtin,
       COALESCE(a.computer_mode,'team'), a.created_at, a.updated_at, COALESCE(u.username, '')
FROM agents a
JOIN users u ON u.id = a.user_id
WHERE a.id = $1 AND u.org_id = $2
`, id, orgID)
	var a Agent
	var uid, owner string
	if err := row.Scan(&a.ID, &uid, &a.Name, &a.Description, &a.SystemPrompt, &a.IsBuiltin, &a.ComputerMode, &a.CreatedAt, &a.UpdatedAt, &owner); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	a.UserID = uid
	return &AgentWithOwner{Agent: a, OwnerUsername: owner}, nil
}

// CreateAgentFull creates an agent for userID with optional computer_mode (team|private).
func (d *DB) CreateAgentFull(userID, name, description, systemPrompt, computerMode string) (*Agent, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("name required")
	}
	mode := strings.TrimSpace(computerMode)
	if mode != "private" {
		mode = "team"
	}
	now := Now()
	a := &Agent{
		ID:           uuid.NewString(),
		UserID:       userID,
		Name:         name,
		Description:  strings.TrimSpace(description),
		SystemPrompt: strings.TrimSpace(systemPrompt),
		IsBuiltin:    false,
		ComputerMode: mode,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	_, err := d.SQL.Exec(`
INSERT INTO agents (id, user_id, name, description, system_prompt, is_builtin, computer_mode, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,FALSE,$6,$7,$8)
`, a.ID, a.UserID, a.Name, a.Description, a.SystemPrompt, a.ComputerMode, a.CreatedAt, a.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return a, nil
}

// UpdateAgentAdmin updates mutable fields for an agent identified by id (admin path).
func (d *DB) UpdateAgentAdmin(id, name, description, systemPrompt string, computerMode *string) (*Agent, error) {
	a, err := d.GetAgentByID(id)
	if err != nil {
		return nil, err
	}
	if a.IsBuiltin {
		return nil, errors.New("builtin agent is read-only")
	}
	if n := strings.TrimSpace(name); n != "" {
		a.Name = n
	}
	a.Description = strings.TrimSpace(description)
	a.SystemPrompt = strings.TrimSpace(systemPrompt)
	if computerMode != nil {
		mode := strings.TrimSpace(*computerMode)
		if mode != "private" {
			mode = "team"
		}
		a.ComputerMode = mode
	}
	a.UpdatedAt = Now()
	_, err = d.SQL.Exec(`
UPDATE agents SET name=$1, description=$2, system_prompt=$3, computer_mode=$4, updated_at=$5
WHERE id=$6 AND is_builtin = FALSE
`, a.Name, a.Description, a.SystemPrompt, a.ComputerMode, a.UpdatedAt, id)
	if err != nil {
		return nil, err
	}
	return a, nil
}

// DeleteAgentByID deletes a non-builtin agent by id (admin path).
func (d *DB) DeleteAgentByID(id string) error {
	res, err := d.SQL.Exec(`DELETE FROM agents WHERE id = $1 AND is_builtin = FALSE`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
