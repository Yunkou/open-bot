package db

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type BotSecretMeta struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	AgentID   string    `json:"agent_id"`
	Name      string    `json:"name"`
	Origin    string    `json:"origin"`
	AuthType  string    `json:"auth_type"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type BotSecret struct {
	BotSecretMeta
	Ciphertext string `json:"-"`
}

type BotSecretRequest struct {
	ID             string     `json:"id"`
	UserID         string     `json:"user_id"`
	AgentID        string     `json:"agent_id"`
	ConversationID string     `json:"conversation_id"`
	Name           string     `json:"name"`
	Origin         string     `json:"origin"`
	AuthType       string     `json:"auth_type"`
	Reason         string     `json:"reason"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
}

func (d *DB) CreateBotSecret(userID, agentID, name, origin, authType, ciphertext string) (*BotSecretMeta, error) {
	userID = strings.TrimSpace(userID)
	name = strings.TrimSpace(name)
	if userID == "" || name == "" {
		return nil, errors.New("user_id and name required")
	}
	if strings.TrimSpace(ciphertext) == "" {
		return nil, errors.New("ciphertext required")
	}
	agentID = strings.TrimSpace(agentID)
	origin = strings.TrimSpace(origin)
	authType = strings.TrimSpace(authType)
	if authType == "" {
		authType = "bearer"
	}
	now := Now()
	id := uuid.NewString()
	_, err := d.SQL.Exec(
		`INSERT INTO bot_secrets (id, user_id, agent_id, name, origin, auth_type, ciphertext, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		 ON CONFLICT (user_id, agent_id, name) DO UPDATE SET
		   origin=EXCLUDED.origin, auth_type=EXCLUDED.auth_type, ciphertext=EXCLUDED.ciphertext, updated_at=EXCLUDED.updated_at
		 RETURNING id`,
		id, userID, agentID, name, origin, authType, ciphertext, now, now,
	)
	if err != nil {
		return nil, err
	}
	// Re-read to get actual id on upsert
	row := d.SQL.QueryRow(
		`SELECT id, user_id, agent_id, name, origin, auth_type, created_at, updated_at
		 FROM bot_secrets WHERE user_id=$1 AND agent_id=$2 AND name=$3`,
		userID, agentID, name,
	)
	var m BotSecretMeta
	if err := row.Scan(&m.ID, &m.UserID, &m.AgentID, &m.Name, &m.Origin, &m.AuthType, &m.CreatedAt, &m.UpdatedAt); err != nil {
		return nil, err
	}
	return &m, nil
}

func (d *DB) ListBotSecrets(userID, agentID string) ([]*BotSecretMeta, error) {
	userID = strings.TrimSpace(userID)
	q := `SELECT id, user_id, agent_id, name, origin, auth_type, created_at, updated_at FROM bot_secrets WHERE user_id=$1`
	args := []any{userID}
	if strings.TrimSpace(agentID) != "" {
		q += ` AND agent_id=$2`
		args = append(args, strings.TrimSpace(agentID))
	}
	q += ` ORDER BY name ASC`
	rows, err := d.SQL.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*BotSecretMeta
	for rows.Next() {
		var m BotSecretMeta
		if err := rows.Scan(&m.ID, &m.UserID, &m.AgentID, &m.Name, &m.Origin, &m.AuthType, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

func (d *DB) DeleteBotSecret(userID, id string) error {
	res, err := d.SQL.Exec(`DELETE FROM bot_secrets WHERE id=$1 AND user_id=$2`, id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (d *DB) GetBotSecretDecrypt(userID, agentID, name string) (*BotSecret, error) {
	row := d.SQL.QueryRow(
		`SELECT id, user_id, agent_id, name, origin, auth_type, ciphertext, created_at, updated_at
		 FROM bot_secrets WHERE user_id=$1 AND agent_id=$2 AND name=$3`,
		userID, strings.TrimSpace(agentID), strings.TrimSpace(name),
	)
	var s BotSecret
	if err := row.Scan(&s.ID, &s.UserID, &s.AgentID, &s.Name, &s.Origin, &s.AuthType, &s.Ciphertext, &s.CreatedAt, &s.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &s, nil
}

func (d *DB) GetBotSecretByID(userID, id string) (*BotSecret, error) {
	row := d.SQL.QueryRow(
		`SELECT id, user_id, agent_id, name, origin, auth_type, ciphertext, created_at, updated_at
		 FROM bot_secrets WHERE id=$1 AND user_id=$2`,
		id, userID,
	)
	var s BotSecret
	if err := row.Scan(&s.ID, &s.UserID, &s.AgentID, &s.Name, &s.Origin, &s.AuthType, &s.Ciphertext, &s.CreatedAt, &s.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &s, nil
}

func (d *DB) CreateSecretRequest(userID, agentID, convID, name, origin, authType, reason string) (*BotSecretRequest, error) {
	now := Now()
	r := &BotSecretRequest{
		ID:             uuid.NewString(),
		UserID:         userID,
		AgentID:        strings.TrimSpace(agentID),
		ConversationID: strings.TrimSpace(convID),
		Name:           strings.TrimSpace(name),
		Origin:         strings.TrimSpace(origin),
		AuthType:       strings.TrimSpace(authType),
		Reason:         strings.TrimSpace(reason),
		Status:         "pending",
		CreatedAt:      now,
	}
	if r.AuthType == "" {
		r.AuthType = "bearer"
	}
	_, err := d.SQL.Exec(
		`INSERT INTO bot_secret_requests (id, user_id, agent_id, conversation_id, name, origin, auth_type, reason, status, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		r.ID, r.UserID, r.AgentID, r.ConversationID, r.Name, r.Origin, r.AuthType, r.Reason, r.Status, r.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (d *DB) ListPendingSecretRequests(userID string) ([]*BotSecretRequest, error) {
	rows, err := d.SQL.Query(
		`SELECT id, user_id, agent_id, conversation_id, name, origin, auth_type, reason, status, created_at, resolved_at
		 FROM bot_secret_requests WHERE user_id=$1 AND status='pending' ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*BotSecretRequest
	for rows.Next() {
		var r BotSecretRequest
		var resolved sql.NullTime
		if err := rows.Scan(&r.ID, &r.UserID, &r.AgentID, &r.ConversationID, &r.Name, &r.Origin, &r.AuthType, &r.Reason, &r.Status, &r.CreatedAt, &resolved); err != nil {
			return nil, err
		}
		if resolved.Valid {
			t := resolved.Time
			r.ResolvedAt = &t
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}

func (d *DB) ResolveSecretRequest(userID, id, status string) error {
	now := Now()
	res, err := d.SQL.Exec(
		`UPDATE bot_secret_requests SET status=$3, resolved_at=$4 WHERE id=$1 AND user_id=$2 AND status='pending'`,
		id, userID, status, now,
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
