package db

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Channel struct {
	ID             string    `json:"id"`
	UserID         string    `json:"user_id"`
	Name           string    `json:"name"`
	CreatedAt      time.Time `json:"created_at"`
	Members        []string  `json:"members,omitempty"`
	ConversationID string    `json:"conversation_id,omitempty"`
}

type AgentBusMessage struct {
	ID          string     `json:"id"`
	UserID      string     `json:"user_id"`
	FromAgentID string     `json:"from_agent_id"`
	ToAgentID   *string    `json:"to_agent_id,omitempty"`
	ChannelID   *string    `json:"channel_id,omitempty"`
	Priority    bool       `json:"priority"`
	Body        string     `json:"body"`
	ReplyToID   *string    `json:"reply_to_id,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	ReadAt      *time.Time `json:"read_at,omitempty"`
}

func (d *DB) CreateChannel(userID, name string) (*Channel, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("name required")
	}
	c := &Channel{
		ID:        uuid.NewString(),
		UserID:    userID,
		Name:      name,
		CreatedAt: Now(),
		Members:   []string{},
	}
	_, err := d.SQL.Exec(
		`INSERT INTO channels (id, user_id, name, created_at) VALUES ($1,$2,$3,$4)`,
		c.ID, c.UserID, c.Name, c.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (d *DB) ListChannels(userID string) ([]*Channel, error) {
	rows, err := d.SQL.Query(
		`SELECT id, user_id, name, created_at FROM channels WHERE user_id = $1 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Channel
	for rows.Next() {
		var c Channel
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.CreatedAt); err != nil {
			return nil, err
		}
		members, _ := d.ListChannelMembers(c.ID)
		c.Members = members
		out = append(out, &c)
	}
	return out, rows.Err()
}

func (d *DB) GetChannel(userID, id string) (*Channel, error) {
	row := d.SQL.QueryRow(
		`SELECT id, user_id, name, created_at FROM channels WHERE id = $1 AND user_id = $2`,
		id, userID,
	)
	var c Channel
	if err := row.Scan(&c.ID, &c.UserID, &c.Name, &c.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	members, _ := d.ListChannelMembers(c.ID)
	c.Members = members
	return &c, nil
}

func (d *DB) DeleteChannel(userID, id string) error {
	if _, err := d.GetChannel(userID, id); err != nil {
		return err
	}

	tx, err := d.SQL.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(
		`DELETE FROM conversations WHERE channel_id = $1 AND user_id = $2`,
		id, userID,
	); err != nil {
		return err
	}
	res, err := tx.Exec(
		`DELETE FROM channels WHERE id = $1 AND user_id = $2`,
		id, userID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return tx.Commit()
}

func (d *DB) ListChannelMembers(channelID string) ([]string, error) {
	rows, err := d.SQL.Query(
		`SELECT agent_id FROM channel_members WHERE channel_id = $1 ORDER BY agent_id`,
		channelID,
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
		out = append(out, id)
	}
	return out, rows.Err()
}

func (d *DB) AddChannelMember(userID, channelID, agentID string) error {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return errors.New("agent_id required")
	}
	if _, err := d.GetChannel(userID, channelID); err != nil {
		return err
	}
	if _, err := d.GetAgent(userID, agentID); err != nil {
		return err
	}
	_, err := d.SQL.Exec(
		`INSERT INTO channel_members (channel_id, agent_id) VALUES ($1,$2)
		 ON CONFLICT (channel_id, agent_id) DO NOTHING`,
		channelID, agentID,
	)
	return err
}

func (d *DB) PostAgentMessage(userID, fromAgentID string, toAgentID, channelID *string, priority bool, body string, replyToID *string) (*AgentBusMessage, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, errors.New("body required")
	}
	resolved, rerr := d.ResolveAgentID(userID, fromAgentID)
	if rerr != nil {
		return nil, rerr
	}
	fromAgentID = resolved
	if _, err := d.GetAgent(userID, fromAgentID); err != nil {
		return nil, errors.New("unknown from_agent_id")
	}

	var toPtr, chPtr, replyPtr *string
	if toAgentID != nil {
		v := strings.TrimSpace(*toAgentID)
		if v != "" {
			if _, err := d.GetAgent(userID, v); err != nil {
				return nil, errors.New("unknown to_agent_id")
			}
			toPtr = &v
		}
	}
	if channelID != nil {
		v := strings.TrimSpace(*channelID)
		if v != "" {
			if _, err := d.GetChannel(userID, v); err != nil {
				return nil, err
			}
			chPtr = &v
		}
	}
	if replyToID != nil {
		v := strings.TrimSpace(*replyToID)
		if v != "" {
			replyPtr = &v
		}
	}
	if toPtr == nil && chPtr == nil {
		return nil, errors.New("to_agent_id or channel_id required")
	}

	m := &AgentBusMessage{
		ID:          uuid.NewString(),
		UserID:      userID,
		FromAgentID: fromAgentID,
		ToAgentID:   toPtr,
		ChannelID:   chPtr,
		Priority:    priority,
		Body:        body,
		ReplyToID:   replyPtr,
		CreatedAt:   Now(),
	}
	_, err := d.SQL.Exec(
		`INSERT INTO agent_messages
		 (id, user_id, from_agent_id, to_agent_id, channel_id, priority, body, reply_to_id, created_at, read_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,NULL)`,
		m.ID, m.UserID, m.FromAgentID, m.ToAgentID, m.ChannelID, m.Priority, m.Body, m.ReplyToID, m.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (d *DB) ListAgentInbox(userID string, unreadOnly bool, agentID string, limit int) ([]*AgentBusMessage, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	agentID = strings.TrimSpace(agentID)

	query := `
SELECT id, user_id, from_agent_id, to_agent_id, channel_id, priority, body, reply_to_id, created_at, read_at
FROM agent_messages
WHERE user_id = $1`
	args := []any{userID}
	argN := 2

	if unreadOnly {
		query += ` AND read_at IS NULL`
	}
	if agentID != "" {
		query += ` AND (to_agent_id = $` + strconv.Itoa(argN) + ` OR channel_id IN (
		  SELECT channel_id FROM channel_members WHERE agent_id = $` + strconv.Itoa(argN) + `
		))`
		args = append(args, agentID)
		argN++
	}
	query += ` ORDER BY priority DESC, created_at DESC LIMIT $` + strconv.Itoa(argN)
	args = append(args, limit)

	rows, err := d.SQL.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*AgentBusMessage
	for rows.Next() {
		var m AgentBusMessage
		var toID, chID, replyID sql.NullString
		var readAt sql.NullTime
		if err := rows.Scan(
			&m.ID, &m.UserID, &m.FromAgentID, &toID, &chID,
			&m.Priority, &m.Body, &replyID, &m.CreatedAt, &readAt,
		); err != nil {
			return nil, err
		}
		if toID.Valid {
			v := toID.String
			m.ToAgentID = &v
		}
		if chID.Valid {
			v := chID.String
			m.ChannelID = &v
		}
		if replyID.Valid {
			v := replyID.String
			m.ReplyToID = &v
		}
		if readAt.Valid {
			t := readAt.Time
			m.ReadAt = &t
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

func (d *DB) HasReplyTo(userID, messageID string) (bool, error) {
	var exists bool
	err := d.SQL.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM agent_messages WHERE user_id = $1 AND reply_to_id = $2)`,
		userID, messageID,
	).Scan(&exists)
	return exists, err
}

func (d *DB) MarkAgentMessageRead(userID, id string) error {
	res, err := d.SQL.Exec(
		`UPDATE agent_messages SET read_at = $1 WHERE id = $2 AND user_id = $3 AND read_at IS NULL`,
		Now(), id, userID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		// already read or missing — treat missing as not found
		var exists bool
		_ = d.SQL.QueryRow(
			`SELECT EXISTS(SELECT 1 FROM agent_messages WHERE id = $1 AND user_id = $2)`,
			id, userID,
		).Scan(&exists)
		if !exists {
			return ErrNotFound
		}
	}
	return nil
}
