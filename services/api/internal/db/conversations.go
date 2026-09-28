package db

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Conversation struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	AgentID   string    `json:"agent_id"`
	Title     string    `json:"title"`
	ChannelID string    `json:"channel_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  []Message `json:"messages,omitempty"`
}

type Message struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id,omitempty"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	AgentID        string    `json:"agent_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// AgentThreadPreview is sidebar metadata for one bot's primary thread.
type AgentThreadPreview struct {
	ConversationID string    `json:"conversation_id,omitempty"`
	LastMessage    string    `json:"last_message,omitempty"`
	UpdatedAt      time.Time `json:"updated_at,omitempty"`
}

func (d *DB) CreateConversation(userID, agentID, title string) (*Conversation, error) {
	resolved, err := d.ResolveAgentID(userID, agentID)
	if err != nil {
		return nil, err
	}
	agentID = resolved
	title = strings.TrimSpace(title)
	if title == "" {
		title = "新对话"
	}
	now := Now()
	c := &Conversation{
		ID:        uuid.NewString(),
		UserID:    userID,
		AgentID:   agentID,
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
		Messages:  []Message{},
	}
	_, err = d.SQL.Exec(
		`INSERT INTO conversations (id, user_id, agent_id, title, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6)`,
		c.ID, c.UserID, c.AgentID, c.Title, c.CreatedAt, c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (d *DB) ListConversations(userID string, limit int) ([]*Conversation, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := d.SQL.Query(
		`SELECT id, user_id, agent_id, title, COALESCE(channel_id,''), created_at, updated_at
		 FROM conversations WHERE user_id = $1 AND (channel_id IS NULL OR channel_id = '')
		 ORDER BY updated_at DESC LIMIT $2`,
		userID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Conversation
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.ID, &c.UserID, &c.AgentID, &c.Title, &c.ChannelID, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}

func (d *DB) GetConversation(userID, id string) (*Conversation, error) {
	row := d.SQL.QueryRow(
		`SELECT id, user_id, agent_id, title, COALESCE(channel_id,''), created_at, updated_at
		 FROM conversations WHERE id = $1 AND user_id = $2`,
		id, userID,
	)
	var c Conversation
	if err := row.Scan(&c.ID, &c.UserID, &c.AgentID, &c.Title, &c.ChannelID, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

func (d *DB) EnsureConversation(userID, id, agentID, title string) (*Conversation, error) {
	c, err := d.GetConversation(userID, id)
	if err == nil {
		return c, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	resolved, rerr := d.ResolveAgentID(userID, agentID)
	if rerr != nil {
		return nil, rerr
	}
	agentID = resolved
	title = strings.TrimSpace(title)
	if title == "" {
		title = "会话 " + id
	}
	now := Now()
	c = &Conversation{
		ID:        id,
		UserID:    userID,
		AgentID:   agentID,
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err = d.SQL.Exec(
		`INSERT INTO conversations (id, user_id, agent_id, title, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 ON CONFLICT (id) DO NOTHING`,
		c.ID, c.UserID, c.AgentID, c.Title, c.CreatedAt, c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return d.GetConversation(userID, id)
}

func (d *DB) TouchConversation(userID, id string) error {
	_, err := d.SQL.Exec(
		`UPDATE conversations SET updated_at = $1 WHERE id = $2 AND user_id = $3`,
		Now(), id, userID,
	)
	return err
}

func (d *DB) AddMessage(conversationID, role, content string) (*Message, error) {
	return d.AddMessageWithAgentAt(conversationID, role, content, "", Now())
}

func (d *DB) AddMessageWithAgent(conversationID, role, content, agentID string) (*Message, error) {
	return d.AddMessageWithAgentAt(conversationID, role, content, agentID, Now())
}

func (d *DB) AddMessageAt(conversationID, role, content string, at time.Time) (*Message, error) {
	return d.AddMessageWithAgentAt(conversationID, role, content, "", at)
}

func (d *DB) AddMessageWithAgentAt(conversationID, role, content, agentID string, at time.Time) (*Message, error) {
	if at.IsZero() {
		at = Now()
	}
	m := &Message{
		ID:             uuid.NewString(),
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
		AgentID:        strings.TrimSpace(agentID),
		CreatedAt:      at,
	}
	tx, err := d.SQL.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(
		`INSERT INTO messages (id, conversation_id, role, content, agent_id, created_at) VALUES ($1,$2,$3,$4,$5,$6)`,
		m.ID, m.ConversationID, m.Role, m.Content, m.AgentID, m.CreatedAt,
	); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(
		`UPDATE conversations SET updated_at = $1 WHERE id = $2`,
		m.CreatedAt, conversationID,
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return m, nil
}

func (d *DB) ListMessages(userID, conversationID string) ([]Message, error) {
	// ownership check
	if _, err := d.GetConversation(userID, conversationID); err != nil {
		return nil, err
	}
	rows, err := d.SQL.Query(
		`SELECT id, conversation_id, role, content, COALESCE(agent_id,''), created_at
		 FROM messages WHERE conversation_id = $1 ORDER BY created_at ASC`,
		conversationID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.AgentID, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (d *DB) DeleteConversation(userID, id string) error {
	res, err := d.SQL.Exec(`DELETE FROM conversations WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// CountUserMessages counts user-role messages in a conversation (no ownership check).
func (d *DB) CountUserMessages(conversationID string) (int, error) {
	var n int
	err := d.SQL.QueryRow(
		`SELECT COUNT(*) FROM messages WHERE conversation_id = $1 AND role = 'user'`,
		conversationID,
	).Scan(&n)
	return n, err
}

func (d *DB) UpdateConversationTitle(userID, id, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("title required")
	}
	res, err := d.SQL.Exec(
		`UPDATE conversations SET title = $1, updated_at = $2 WHERE id = $3 AND user_id = $4`,
		title, Now(), id, userID,
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

// EnsureChannelConversation returns the conversation bound to a channel (1:1), creating if needed.
func (d *DB) EnsureChannelConversation(userID, channelID, agentID, title string) (*Conversation, error) {
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return nil, errors.New("channel_id required")
	}
	row := d.SQL.QueryRow(
		`SELECT id, user_id, agent_id, title, COALESCE(channel_id,''), created_at, updated_at
		 FROM conversations WHERE user_id = $1 AND channel_id = $2`,
		userID, channelID,
	)
	var c Conversation
	err := row.Scan(&c.ID, &c.UserID, &c.AgentID, &c.Title, &c.ChannelID, &c.CreatedAt, &c.UpdatedAt)
	if err == nil {
		// Keep agent_id fresh when first member changes.
		agentID = strings.TrimSpace(agentID)
		if agentID != "" && agentID != c.AgentID {
			_, _ = d.SQL.Exec(`UPDATE conversations SET agent_id = $1, updated_at = $2 WHERE id = $3`, agentID, Now(), c.ID)
			c.AgentID = agentID
		}
		return &c, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	resolved, rerr := d.ResolveAgentID(userID, agentID)
	if rerr != nil {
		return nil, rerr
	}
	agentID = resolved
	title = strings.TrimSpace(title)
	if title == "" {
		title = "群聊"
	}
	now := Now()
	c = Conversation{
		ID:        uuid.NewString(),
		UserID:    userID,
		AgentID:   agentID,
		Title:     title,
		ChannelID: channelID,
		CreatedAt: now,
		UpdatedAt: now,
		Messages:  []Message{},
	}
	_, err = d.SQL.Exec(
		`INSERT INTO conversations (id, user_id, agent_id, title, channel_id, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		c.ID, c.UserID, c.AgentID, c.Title, c.ChannelID, c.CreatedAt, c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// GetConversationByChannel looks up the linked conversation without creating.
func (d *DB) GetConversationByChannel(userID, channelID string) (*Conversation, error) {
	row := d.SQL.QueryRow(
		`SELECT id, user_id, agent_id, title, COALESCE(channel_id,''), created_at, updated_at
		 FROM conversations WHERE user_id = $1 AND channel_id = $2`,
		userID, channelID,
	)
	var c Conversation
	if err := row.Scan(&c.ID, &c.UserID, &c.AgentID, &c.Title, &c.ChannelID, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &c, nil
}

// GetOrCreatePrimaryConversation returns the latest non-channel conversation for (user, agent), creating one if missing.
func (d *DB) GetOrCreatePrimaryConversation(userID, agentID string) (*Conversation, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil, errors.New("agent id required")
	}
	row := d.SQL.QueryRow(
		`SELECT id, user_id, agent_id, title, COALESCE(channel_id,''), created_at, updated_at
		 FROM conversations
		 WHERE user_id = $1 AND agent_id = $2 AND (channel_id IS NULL OR channel_id = '')
		 ORDER BY updated_at DESC
		 LIMIT 1`,
		userID, agentID,
	)
	var c Conversation
	err := row.Scan(&c.ID, &c.UserID, &c.AgentID, &c.Title, &c.ChannelID, &c.CreatedAt, &c.UpdatedAt)
	if err == nil {
		return &c, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	title := "与助手的对话"
	return d.CreateConversation(userID, agentID, title)
}

// GetAgentThreadPreview returns last message snippet for the primary thread of an agent.
func (d *DB) GetAgentThreadPreview(userID, agentID string) (*AgentThreadPreview, error) {
	agentID = strings.TrimSpace(agentID)
	row := d.SQL.QueryRow(
		`SELECT c.id, c.updated_at,
		        COALESCE((
		          SELECT CASE
		            WHEN length(m.content) > 80 THEN substr(m.content, 1, 80) || '…'
		            ELSE m.content
		          END
		          FROM messages m
		          WHERE m.conversation_id = c.id AND m.role IN ('user','assistant')
		          ORDER BY m.created_at DESC
		          LIMIT 1
		        ), '')
		 FROM conversations c
		 WHERE c.user_id = $1 AND c.agent_id = $2 AND (c.channel_id IS NULL OR c.channel_id = '')
		 ORDER BY c.updated_at DESC
		 LIMIT 1`,
		userID, agentID,
	)
	var p AgentThreadPreview
	if err := row.Scan(&p.ConversationID, &p.UpdatedAt, &p.LastMessage); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return &AgentThreadPreview{}, nil
		}
		return nil, err
	}
	return &p, nil
}
