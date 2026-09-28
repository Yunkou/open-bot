package db

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Agent struct {
	ID           string     `json:"id"`
	UserID       string     `json:"user_id,omitempty"`
	Name         string     `json:"name"`
	Description  string     `json:"description"`
	SystemPrompt string     `json:"system_prompt"`
	IsBuiltin    bool       `json:"is_builtin"`
	ComputerMode string     `json:"computer_mode"` // team|private
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
}

// PurgeBuiltinAgents removes seeded built-in assistants (open-bot / general / any is_builtin).
// Called at API start so existing DBs drop builtins; new installs never re-seed them.
// soft-delete: hard purge — startup cleanup of deprecated rows, not a product delete.
func (d *DB) PurgeBuiltinAgents() error {
	_, err := d.SQL.Exec(`DELETE FROM agents WHERE is_builtin = TRUE`)
	return err
}

// FirstAgentID returns the user's oldest agent id, or an error if none exist.
func (d *DB) FirstAgentID(userID string) (string, error) {
	row := d.SQL.QueryRow(
		`SELECT id FROM agents WHERE user_id = $1 AND deleted_at IS NULL ORDER BY created_at ASC LIMIT 1`,
		userID,
	)
	var id string
	if err := row.Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", errors.New("no agents; create an agent first")
		}
		return "", err
	}
	return id, nil
}

// ResolveAgentID returns agentID when non-empty; otherwise the user's first agent.
func (d *DB) ResolveAgentID(userID, agentID string) (string, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID != "" {
		return agentID, nil
	}
	return d.FirstAgentID(userID)
}

func (d *DB) ListAgents(userID string) ([]*Agent, error) {
	rows, err := d.SQL.Query(
		`SELECT id, COALESCE(user_id,''), name, description, system_prompt, is_builtin, COALESCE(computer_mode,'team'), created_at, updated_at
		 FROM agents
		 WHERE user_id = $1 AND deleted_at IS NULL
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
		if err := rows.Scan(&a.ID, &uid, &a.Name, &a.Description, &a.SystemPrompt, &a.IsBuiltin, &a.ComputerMode, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		a.UserID = uid
		out = append(out, &a)
	}
	return out, rows.Err()
}

func (d *DB) GetAgent(userID, id string) (*Agent, error) {
	row := d.SQL.QueryRow(
		`SELECT id, COALESCE(user_id,''), name, description, system_prompt, is_builtin, COALESCE(computer_mode,'team'), created_at, updated_at
		 FROM agents WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		id, userID,
	)
	var a Agent
	var uid string
	if err := row.Scan(&a.ID, &uid, &a.Name, &a.Description, &a.SystemPrompt, &a.IsBuiltin, &a.ComputerMode, &a.CreatedAt, &a.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	a.UserID = uid
	return &a, nil
}

func (d *DB) CreateAgent(userID, name, description, systemPrompt string) (*Agent, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("name required")
	}
	now := Now()
	a := &Agent{
		ID:           uuid.NewString(),
		UserID:       userID,
		Name:         name,
		Description:  strings.TrimSpace(description),
		SystemPrompt: strings.TrimSpace(systemPrompt),
		IsBuiltin:    false,
		ComputerMode: "team",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	_, err := d.SQL.Exec(
		`INSERT INTO agents (id, user_id, name, description, system_prompt, is_builtin, computer_mode, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,FALSE,$6,$7,$8)`,
		a.ID, a.UserID, a.Name, a.Description, a.SystemPrompt, a.ComputerMode, a.CreatedAt, a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (d *DB) UpdateAgent(userID, id, name, description, systemPrompt string) (*Agent, error) {
	a, err := d.GetAgent(userID, id)
	if err != nil {
		return nil, err
	}
	if a.IsBuiltin {
		return nil, errors.New("builtin agent is read-only")
	}
	if a.UserID != userID {
		return nil, ErrNotFound
	}
	if n := strings.TrimSpace(name); n != "" {
		a.Name = n
	}
	a.Description = strings.TrimSpace(description)
	a.SystemPrompt = strings.TrimSpace(systemPrompt)
	a.UpdatedAt = Now()
	_, err = d.SQL.Exec(
		`UPDATE agents SET name=$1, description=$2, system_prompt=$3, updated_at=$4 WHERE id=$5 AND user_id=$6 AND deleted_at IS NULL`,
		a.Name, a.Description, a.SystemPrompt, a.UpdatedAt, id, userID,
	)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (d *DB) SetAgentComputerMode(userID, id, mode string) (*Agent, error) {
	mode = strings.TrimSpace(mode)
	if mode != "private" {
		mode = "team"
	}
	a, err := d.GetAgent(userID, id)
	if err != nil {
		return nil, err
	}
	a.ComputerMode = mode
	a.UpdatedAt = Now()
	_, err = d.SQL.Exec(
		`UPDATE agents SET computer_mode=$1, updated_at=$2 WHERE id=$3 AND user_id=$4 AND deleted_at IS NULL`,
		mode, a.UpdatedAt, id, userID,
	)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (d *DB) DeleteAgent(userID, id string) error {
	res, err := d.SQL.Exec(
		`UPDATE agents SET deleted_at = $3, updated_at = $3
		 WHERE id = $1 AND user_id = $2 AND is_builtin = FALSE AND deleted_at IS NULL`,
		id, userID, Now(),
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
