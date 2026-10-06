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
	AvatarShape  string     `json:"avatar_shape"`
	AvatarColor  string     `json:"avatar_color"`
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
		`SELECT id, COALESCE(user_id,''), name, description, system_prompt, is_builtin, COALESCE(computer_mode,'team'),
		        COALESCE(avatar_shape,''), COALESCE(avatar_color,''), created_at, updated_at
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
		if err := rows.Scan(&a.ID, &uid, &a.Name, &a.Description, &a.SystemPrompt, &a.IsBuiltin, &a.ComputerMode, &a.AvatarShape, &a.AvatarColor, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		a.UserID = uid
		out = append(out, &a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Backfill avatar for rows created before avatar columns existed,
	// avoiding shape+color pairs already used by this user.
	used := make(map[string]struct{})
	for _, a := range out {
		if a.AvatarShape != "" && a.AvatarColor != "" {
			used[a.AvatarShape+"|"+a.AvatarColor] = struct{}{}
		}
	}
	for _, a := range out {
		if a.AvatarShape == "" || a.AvatarColor == "" {
			s, c := AssignAvatarAvoiding(a.ID, used)
			a.AvatarShape, a.AvatarColor = s, c
			used[s+"|"+c] = struct{}{}
			_, _ = d.SQL.Exec(`UPDATE agents SET avatar_shape=$1, avatar_color=$2 WHERE id=$3`, s, c, a.ID)
		}
	}
	return out, nil
}

func (d *DB) GetAgent(userID, id string) (*Agent, error) {
	row := d.SQL.QueryRow(
		`SELECT id, COALESCE(user_id,''), name, description, system_prompt, is_builtin, COALESCE(computer_mode,'team'),
		        COALESCE(avatar_shape,''), COALESCE(avatar_color,''), created_at, updated_at
		 FROM agents WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		id, userID,
	)
	var a Agent
	var uid string
	if err := row.Scan(&a.ID, &uid, &a.Name, &a.Description, &a.SystemPrompt, &a.IsBuiltin, &a.ComputerMode, &a.AvatarShape, &a.AvatarColor, &a.CreatedAt, &a.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	a.UserID = uid
	if a.AvatarShape == "" || a.AvatarColor == "" {
		s, c := AssignAvatarFromID(a.ID)
		a.AvatarShape, a.AvatarColor = s, c
		_, _ = d.SQL.Exec(`UPDATE agents SET avatar_shape=$1, avatar_color=$2 WHERE id=$3`, s, c, a.ID)
	}
	return &a, nil
}

func (d *DB) CreateAgent(userID, name, description, systemPrompt string) (*Agent, error) {
	return d.CreateAgentWithAvatar(userID, name, description, systemPrompt, "", "")
}

// CreateAgentWithAvatar creates an agent with optional whitelisted avatar_shape / avatar_color.
// Empty values are auto-assigned (agent_id hash, avoiding the user's used shape+color pairs).
func (d *DB) CreateAgentWithAvatar(userID, name, description, systemPrompt, shape, color string) (*Agent, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("name required")
	}
	shape = strings.TrimSpace(shape)
	if shape != "" && !IsAllowedAvatarShape(shape) {
		return nil, ErrAvatarNotAllowed("avatar_shape not allowed")
	}
	if c := strings.TrimSpace(color); c != "" {
		color = NormalizeAvatarColor(c)
		if color == "" {
			return nil, ErrAvatarNotAllowed("avatar_color not allowed")
		}
	} else {
		color = ""
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
	used := map[string]struct{}{}
	if existing, err := d.ListAgents(userID); err == nil {
		for _, e := range existing {
			if e.AvatarShape != "" && e.AvatarColor != "" {
				used[e.AvatarShape+"|"+e.AvatarColor] = struct{}{}
			}
		}
	}
	a.AvatarShape, a.AvatarColor = AssignAvatarAvoiding(a.ID, used)
	if shape != "" {
		a.AvatarShape = shape
	}
	if color != "" {
		a.AvatarColor = color
	}
	_, err := d.SQL.Exec(
		`INSERT INTO agents (id, user_id, name, description, system_prompt, is_builtin, computer_mode, avatar_shape, avatar_color, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,FALSE,$6,$7,$8,$9,$10)`,
		a.ID, a.UserID, a.Name, a.Description, a.SystemPrompt, a.ComputerMode, a.AvatarShape, a.AvatarColor, a.CreatedAt, a.UpdatedAt,
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

// UpdateAgentAvatar sets avatar_shape / avatar_color (whitelisted). Empty values keep current.
func (d *DB) UpdateAgentAvatar(userID, id, shape, color string) (*Agent, error) {
	shape = strings.TrimSpace(shape)
	colorRaw := strings.TrimSpace(color)
	if shape != "" && !IsAllowedAvatarShape(shape) {
		return nil, ErrAvatarNotAllowed("avatar_shape not allowed")
	}
	color = ""
	if colorRaw != "" {
		color = NormalizeAvatarColor(colorRaw)
		if color == "" {
			return nil, ErrAvatarNotAllowed("avatar_color not allowed")
		}
	}
	a, err := d.GetAgent(userID, id)
	if err != nil {
		return nil, err
	}
	if shape == "" && color == "" {
		return a, nil
	}
	if shape != "" {
		a.AvatarShape = shape
	}
	if color != "" {
		a.AvatarColor = color
	}
	a.UpdatedAt = Now()
	_, err = d.SQL.Exec(
		`UPDATE agents SET avatar_shape=$1, avatar_color=$2, updated_at=$3 WHERE id=$4 AND user_id=$5 AND deleted_at IS NULL`,
		a.AvatarShape, a.AvatarColor, a.UpdatedAt, id, userID,
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
