package db

import (
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

type MessageFeedback struct {
	ID             string    `json:"id"`
	UserID         string    `json:"user_id,omitempty"`
	MessageID      string    `json:"message_id"`
	AgentID        string    `json:"agent_id"`
	ConversationID string    `json:"conversation_id"`
	Polarity       string    `json:"polarity"` // positive|negative
	Reasons        []string  `json:"reasons"`
	Note           string    `json:"note,omitempty"`
	Source         string    `json:"source"` // feedback_menu|reaction_followup
	CreatedAt      time.Time `json:"created_at"`
}

type BotLesson struct {
	ID          string     `json:"id"`
	UserID      string     `json:"user_id,omitempty"`
	AgentID     string     `json:"agent_id"`
	FeedbackID  string     `json:"feedback_id,omitempty"`
	Title       string     `json:"title"`
	Body        string     `json:"body"`
	Tags        []string   `json:"tags"`
	Status      string     `json:"status"` // pending|active|ignored
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ConfirmedAt *time.Time `json:"confirmed_at,omitempty"`
}

func (d *DB) CreateMessageFeedback(userID string, f MessageFeedback) (*MessageFeedback, error) {
	f.Polarity = strings.TrimSpace(f.Polarity)
	if f.Polarity != "positive" && f.Polarity != "negative" {
		return nil, errors.New("polarity must be positive or negative")
	}
	f.Source = strings.TrimSpace(f.Source)
	if f.Source == "" {
		f.Source = "feedback_menu"
	}
	if f.Source != "feedback_menu" && f.Source != "reaction_followup" {
		return nil, errors.New("source must be feedback_menu or reaction_followup")
	}
	f.MessageID = strings.TrimSpace(f.MessageID)
	f.AgentID = strings.TrimSpace(f.AgentID)
	f.ConversationID = strings.TrimSpace(f.ConversationID)
	f.Note = strings.TrimSpace(f.Note)
	if utf8.RuneCountInString(f.Note) > 500 {
		return nil, errors.New("note too long")
	}
	if f.MessageID == "" || f.AgentID == "" || f.ConversationID == "" {
		return nil, errors.New("message_id, agent_id, conversation_id required")
	}
	// ownership: conversation belongs to user; message in conversation
	if _, err := d.GetConversation(userID, f.ConversationID); err != nil {
		return nil, err
	}
	ref, err := d.GetMessageOwned(userID, f.MessageID)
	if err != nil {
		return nil, err
	}
	if ref.ConversationID != f.ConversationID {
		return nil, errors.New("message not in conversation")
	}
	if f.Reasons == nil {
		f.Reasons = []string{}
	}
	now := Now()
	f.ID = uuid.NewString()
	f.UserID = userID
	f.CreatedAt = now
	_, err = d.SQL.Exec(
		`INSERT INTO message_feedbacks
		 (id, user_id, message_id, agent_id, conversation_id, polarity, reasons_json, note, source, created_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		f.ID, userID, f.MessageID, f.AgentID, f.ConversationID, f.Polarity, encodeJSONDefault(f.Reasons, "[]"), f.Note, f.Source, now,
	)
	if err != nil {
		return nil, err
	}
	return &f, nil
}

func (d *DB) ListMessageFeedbacks(userID, agentID string, limit int) ([]*MessageFeedback, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	agentID = strings.TrimSpace(agentID)
	rows, err := d.SQL.Query(
		`SELECT id, user_id, message_id, agent_id, conversation_id, polarity, reasons_json, note, source, created_at
		 FROM message_feedbacks WHERE user_id=$1 AND agent_id=$2
		 ORDER BY created_at DESC LIMIT $3`,
		userID, agentID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*MessageFeedback
	for rows.Next() {
		var f MessageFeedback
		var reasonsRaw string
		if err := rows.Scan(&f.ID, &f.UserID, &f.MessageID, &f.AgentID, &f.ConversationID, &f.Polarity, &reasonsRaw, &f.Note, &f.Source, &f.CreatedAt); err != nil {
			return nil, err
		}
		f.Reasons = decodeStringSlice(reasonsRaw)
		out = append(out, &f)
	}
	return out, rows.Err()
}

func (d *DB) CreateBotLesson(userID string, lesson BotLesson) (*BotLesson, error) {
	lesson.AgentID = strings.TrimSpace(lesson.AgentID)
	lesson.Title = strings.TrimSpace(lesson.Title)
	lesson.Body = strings.TrimSpace(lesson.Body)
	lesson.FeedbackID = strings.TrimSpace(lesson.FeedbackID)
	if lesson.AgentID == "" || lesson.Title == "" || lesson.Body == "" {
		return nil, errors.New("agent_id, title, body required")
	}
	if utf8.RuneCountInString(lesson.Title) > 40 {
		return nil, errors.New("title too long")
	}
	if utf8.RuneCountInString(lesson.Body) > 500 {
		return nil, errors.New("body too long")
	}
	if _, err := d.GetAgent(userID, lesson.AgentID); err != nil {
		return nil, err
	}
	status := strings.TrimSpace(lesson.Status)
	if status == "" {
		status = "pending"
	}
	if status != "pending" && status != "active" && status != "ignored" {
		return nil, errors.New("invalid status")
	}
	if lesson.Tags == nil {
		lesson.Tags = []string{}
	}
	now := Now()
	lesson.ID = uuid.NewString()
	lesson.UserID = userID
	lesson.Status = status
	lesson.CreatedAt = now
	lesson.UpdatedAt = now
	var feedbackArg any
	if lesson.FeedbackID != "" {
		feedbackArg = lesson.FeedbackID
	}
	var confirmed any
	if status == "active" {
		lesson.ConfirmedAt = &now
		confirmed = now
	}
	_, err := d.SQL.Exec(
		`INSERT INTO bot_lessons
		 (id, user_id, agent_id, feedback_id, title, body, tags_json, status, created_at, updated_at, confirmed_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		lesson.ID, userID, lesson.AgentID, feedbackArg, lesson.Title, lesson.Body, encodeJSONDefault(lesson.Tags, "[]"), status, now, now, confirmed,
	)
	if err != nil {
		return nil, err
	}
	return &lesson, nil
}

func (d *DB) ListBotLessons(userID, agentID, status string, limit int) ([]*BotLesson, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	agentID = strings.TrimSpace(agentID)
	status = strings.TrimSpace(status)
	var rows *sql.Rows
	var err error
	if status == "" {
		rows, err = d.SQL.Query(
			`SELECT id, user_id, agent_id, COALESCE(feedback_id,''), title, body, tags_json, status, created_at, updated_at, confirmed_at
			 FROM bot_lessons WHERE user_id=$1 AND agent_id=$2
			 ORDER BY updated_at DESC LIMIT $3`,
			userID, agentID, limit,
		)
	} else {
		rows, err = d.SQL.Query(
			`SELECT id, user_id, agent_id, COALESCE(feedback_id,''), title, body, tags_json, status, created_at, updated_at, confirmed_at
			 FROM bot_lessons WHERE user_id=$1 AND agent_id=$2 AND status=$3
			 ORDER BY updated_at DESC LIMIT $4`,
			userID, agentID, status, limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanBotLessons(rows)
}

func (d *DB) ListActiveBotLessons(userID, agentID string) ([]*BotLesson, error) {
	return d.ListBotLessons(userID, agentID, "active", 200)
}

func scanBotLessons(rows *sql.Rows) ([]*BotLesson, error) {
	var out []*BotLesson
	for rows.Next() {
		var l BotLesson
		var tagsRaw string
		var confirmed sql.NullTime
		if err := rows.Scan(&l.ID, &l.UserID, &l.AgentID, &l.FeedbackID, &l.Title, &l.Body, &tagsRaw, &l.Status, &l.CreatedAt, &l.UpdatedAt, &confirmed); err != nil {
			return nil, err
		}
		l.Tags = decodeStringSlice(tagsRaw)
		if confirmed.Valid {
			t := confirmed.Time.UTC()
			l.ConfirmedAt = &t
		}
		out = append(out, &l)
	}
	return out, rows.Err()
}

func (d *DB) GetBotLesson(userID, id string) (*BotLesson, error) {
	row := d.SQL.QueryRow(
		`SELECT id, user_id, agent_id, COALESCE(feedback_id,''), title, body, tags_json, status, created_at, updated_at, confirmed_at
		 FROM bot_lessons WHERE id=$1 AND user_id=$2`,
		id, userID,
	)
	var l BotLesson
	var tagsRaw string
	var confirmed sql.NullTime
	if err := row.Scan(&l.ID, &l.UserID, &l.AgentID, &l.FeedbackID, &l.Title, &l.Body, &tagsRaw, &l.Status, &l.CreatedAt, &l.UpdatedAt, &confirmed); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	l.Tags = decodeStringSlice(tagsRaw)
	if confirmed.Valid {
		t := confirmed.Time.UTC()
		l.ConfirmedAt = &t
	}
	return &l, nil
}

type BotLessonPatch struct {
	Title  *string
	Body   *string
	Tags   *[]string
	Status *string
}

func (d *DB) UpdateBotLesson(userID, id string, patch BotLessonPatch) (*BotLesson, error) {
	l, err := d.GetBotLesson(userID, id)
	if err != nil {
		return nil, err
	}
	if patch.Title != nil {
		title := strings.TrimSpace(*patch.Title)
		if title == "" || utf8.RuneCountInString(title) > 40 {
			return nil, errors.New("invalid title")
		}
		l.Title = title
	}
	if patch.Body != nil {
		body := strings.TrimSpace(*patch.Body)
		if body == "" || utf8.RuneCountInString(body) > 500 {
			return nil, errors.New("invalid body")
		}
		l.Body = body
	}
	if patch.Tags != nil {
		l.Tags = *patch.Tags
		if l.Tags == nil {
			l.Tags = []string{}
		}
	}
	now := Now()
	if patch.Status != nil {
		st := strings.TrimSpace(*patch.Status)
		if st != "pending" && st != "active" && st != "ignored" {
			return nil, errors.New("invalid status")
		}
		l.Status = st
		if st == "active" {
			l.ConfirmedAt = &now
		}
		if st == "pending" || st == "ignored" {
			l.ConfirmedAt = nil
		}
	}
	l.UpdatedAt = now
	var confirmed any
	if l.ConfirmedAt != nil {
		confirmed = *l.ConfirmedAt
	}
	_, err = d.SQL.Exec(
		`UPDATE bot_lessons SET title=$1, body=$2, tags_json=$3, status=$4, updated_at=$5, confirmed_at=$6
		 WHERE id=$7 AND user_id=$8`,
		l.Title, l.Body, encodeJSONDefault(l.Tags, "[]"), l.Status, l.UpdatedAt, confirmed, id, userID,
	)
	if err != nil {
		return nil, err
	}
	return l, nil
}

func (d *DB) DeleteBotLesson(userID, id string) error {
	res, err := d.SQL.Exec(`DELETE FROM bot_lessons WHERE id=$1 AND user_id=$2`, id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
