package db

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Agent struct {
	ID            string `json:"id"`
	UserID        string `json:"user_id,omitempty"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	SystemPrompt  string `json:"system_prompt"`
	IsBuiltin     bool   `json:"is_builtin"`
	ComputerMode  string `json:"computer_mode"` // team|private
	AvatarShape   string `json:"avatar_shape"`
	AvatarColor   string `json:"avatar_color"`
	AvatarUserSet bool   `json:"avatar_user_set"`
	// MachineID binds this Bot to a user_machines.id (host/runtime exec channel).
	// Empty = unbound = offline for green-dot online.
	MachineID string `json:"machine_id,omitempty"`
	// MachineIDSource records who wrote MachineID: "" (unbound or legacy unknown),
	// "migrate" (one-time backfill), "user" (CREATE/PATCH). Server-side only —
	// clients never send it. Corrective migrations may only clear "migrate".
	MachineIDSource string     `json:"machine_id_source,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	DeletedAt       *time.Time `json:"deleted_at,omitempty"`
	// Online is computed by the API (bound machine Connected + MachineOfflineAfter); not stored.
	Online bool `json:"online"`
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
		        COALESCE(avatar_shape,''), COALESCE(avatar_color,''), COALESCE(avatar_user_set,FALSE),
		        COALESCE(machine_id,''), COALESCE(machine_id_source,''), created_at, updated_at
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
		if err := rows.Scan(&a.ID, &uid, &a.Name, &a.Description, &a.SystemPrompt, &a.IsBuiltin, &a.ComputerMode, &a.AvatarShape, &a.AvatarColor, &a.AvatarUserSet, &a.MachineID, &a.MachineIDSource, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		a.UserID = uid
		// Read-only canonicalize for response; never write back on list.
		if canon := CanonicalAvatarShape(a.AvatarShape); canon != "" {
			a.AvatarShape = canon
		}
		out = append(out, &a)
	}
	return out, rows.Err()
}

func (d *DB) GetAgent(userID, id string) (*Agent, error) {
	row := d.SQL.QueryRow(
		`SELECT id, COALESCE(user_id,''), name, description, system_prompt, is_builtin, COALESCE(computer_mode,'team'),
		        COALESCE(avatar_shape,''), COALESCE(avatar_color,''), COALESCE(avatar_user_set,FALSE),
		        COALESCE(machine_id,''), COALESCE(machine_id_source,''), created_at, updated_at
		 FROM agents WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		id, userID,
	)
	var a Agent
	var uid string
	if err := row.Scan(&a.ID, &uid, &a.Name, &a.Description, &a.SystemPrompt, &a.IsBuiltin, &a.ComputerMode, &a.AvatarShape, &a.AvatarColor, &a.AvatarUserSet, &a.MachineID, &a.MachineIDSource, &a.CreatedAt, &a.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	a.UserID = uid
	if canon := CanonicalAvatarShape(a.AvatarShape); canon != "" {
		a.AvatarShape = canon
	}
	return &a, nil
}

func (d *DB) CreateAgent(userID, name, description, systemPrompt string) (*Agent, error) {
	return d.CreateAgentWithAvatar(userID, name, description, systemPrompt, "", "")
}

// CreateAgentWithAvatar creates an agent with optional whitelisted avatar_shape / avatar_color.
// Empty values are auto-assigned (agent_id hash, avoiding the user's used shape+color pairs).
// Any client-provided shape or color marks avatar_user_set=true.
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
	userSet := shape != "" || color != ""
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
	tx, err := d.SQL.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	s, c, err := assignAvatarUnderLock(tx, userID, a.ID, shape, color)
	if err != nil {
		return nil, err
	}
	a.AvatarShape, a.AvatarColor, a.AvatarUserSet = s, c, userSet

	if _, err := tx.Exec(
		`INSERT INTO agents (id, user_id, name, description, system_prompt, is_builtin, computer_mode, avatar_shape, avatar_color, avatar_user_set, machine_id, machine_id_source, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,FALSE,$6,$7,$8,$9,$10,$11,$12,$13)`,
		a.ID, a.UserID, a.Name, a.Description, a.SystemPrompt, a.ComputerMode, a.AvatarShape, a.AvatarColor, a.AvatarUserSet, a.MachineID, a.MachineIDSource, a.CreatedAt, a.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return a, nil
}

// assignAvatarUnderLock serializes auto-assignment per user (pg_advisory_xact_lock)
// so Create / Clone / admin create share one lock and never pick the same free pair.
// When both shape and color are non-empty, no lock is taken.
func assignAvatarUnderLock(tx *sql.Tx, userID, agentID, shape, color string) (string, string, error) {
	shape = strings.TrimSpace(shape)
	color = strings.TrimSpace(color)
	if shape != "" && color != "" {
		return shape, color, nil
	}
	if _, err := tx.Exec(`SELECT pg_advisory_xact_lock($1)`, AvatarAssignLockKey(userID)); err != nil {
		return "", "", err
	}
	used, err := usedAvatarPairsTx(tx, userID)
	if err != nil {
		return "", "", err
	}
	s, c := AssignAvatarAvoiding(agentID, used)
	if shape != "" {
		s = shape
	}
	if color != "" {
		c = color
	}
	return s, c, nil
}

// usedAvatarPairsTx returns shape|color pairs of the user's live agents (inside tx).
func usedAvatarPairsTx(tx *sql.Tx, userID string) (map[string]struct{}, error) {
	rows, err := tx.Query(
		`SELECT COALESCE(avatar_shape,''), COALESCE(avatar_color,'')
		 FROM agents WHERE user_id = $1 AND deleted_at IS NULL`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	used := map[string]struct{}{}
	for rows.Next() {
		var s, c string
		if err := rows.Scan(&s, &c); err != nil {
			return nil, err
		}
		if canon := CanonicalAvatarShape(s); canon != "" {
			s = canon
		}
		if s != "" && c != "" {
			used[s+"|"+c] = struct{}{}
		}
	}
	return used, rows.Err()
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
// Always marks avatar_user_set=true (manual choice; migrate will not reassign).
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
	a.AvatarUserSet = true
	a.UpdatedAt = Now()
	_, err = d.SQL.Exec(
		`UPDATE agents SET avatar_shape=$1, avatar_color=$2, avatar_user_set=TRUE, updated_at=$3 WHERE id=$4 AND user_id=$5 AND deleted_at IS NULL`,
		a.AvatarShape, a.AvatarColor, a.UpdatedAt, id, userID,
	)
	if err != nil {
		return nil, err
	}
	return a, nil
}

// SetAgentMachineID binds (or clears) the Bot's host/runtime exec channel.
// machineID empty clears the binding. Non-empty must be a user_machines.id owned by userID.
// This is the user path (CREATE/PATCH): it stamps machine_id_source='user' when
// set, ” when cleared. Only migrate writes 'migrate'; clients never send source.
func (d *DB) SetAgentMachineID(userID, id, machineID string) (*Agent, error) {
	userID = strings.TrimSpace(userID)
	id = strings.TrimSpace(id)
	machineID = strings.TrimSpace(machineID)
	if userID == "" || id == "" {
		return nil, errors.New("user_id and agent id required")
	}
	if machineID != "" {
		m, err := d.GetMachine(userID, machineID)
		if err != nil {
			return nil, err
		}
		if m == nil || !IsHostEligible(*m) {
			return nil, ErrMobileNotHost
		}
	}
	a, err := d.GetAgent(userID, id)
	if err != nil {
		return nil, err
	}
	src := MachineIDSourceUser
	if machineID == "" {
		src = MachineIDSourceNone
	}
	a.MachineID = machineID
	a.MachineIDSource = src
	a.UpdatedAt = Now()
	_, err = d.SQL.Exec(
		`UPDATE agents SET machine_id=$1, machine_id_source=$2, updated_at=$3 WHERE id=$4 AND user_id=$5 AND deleted_at IS NULL`,
		machineID, src, a.UpdatedAt, id, userID,
	)
	if err != nil {
		return nil, err
	}
	return a, nil
}

// ListAgentsByMachineID returns live agents bound to machineID.
func (d *DB) ListAgentsByMachineID(userID, machineID string) ([]*Agent, error) {
	userID = strings.TrimSpace(userID)
	machineID = strings.TrimSpace(machineID)
	if userID == "" || machineID == "" {
		return nil, nil
	}
	rows, err := d.SQL.Query(
		`SELECT id, COALESCE(user_id,''), name, description, system_prompt, is_builtin, COALESCE(computer_mode,'team'),
		        COALESCE(avatar_shape,''), COALESCE(avatar_color,''), COALESCE(avatar_user_set,FALSE),
		        COALESCE(machine_id,''), COALESCE(machine_id_source,''), created_at, updated_at
		 FROM agents
		 WHERE user_id = $1 AND deleted_at IS NULL AND machine_id = $2
		 ORDER BY created_at ASC`,
		userID, machineID,
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

// ClearAgentMachineIDForMachine clears bindings when a machine is deleted.
func (d *DB) ClearAgentMachineIDForMachine(userID, machineID string) error {
	userID = strings.TrimSpace(userID)
	machineID = strings.TrimSpace(machineID)
	if userID == "" || machineID == "" {
		return nil
	}
	_, err := d.SQL.Exec(
		`UPDATE agents SET machine_id='', machine_id_source='', updated_at=$3
		 WHERE user_id=$1 AND machine_id=$2 AND deleted_at IS NULL`,
		userID, machineID, Now(),
	)
	return err
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
