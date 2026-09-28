package db

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	TaskQueued    = "queued"
	TaskRunning   = "running"
	TaskDone      = "done"
	TaskFailed    = "failed"
	TaskCancelled = "cancelled"
)

// ConversationTask is one piece of work the bot finishes after the chat turn ends.
type ConversationTask struct {
	ID              string
	UserID          string
	ConversationID  string
	AgentID         string
	Goal            string
	Status          string
	Attempt         int
	LastError       string
	LeaseUntil      sql.NullTime
	SourceMessageID string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	StartedAt       sql.NullTime
	FinishedAt      sql.NullTime
}

const taskSelectCols = `id, user_id, conversation_id, agent_id, goal, status, attempt, last_error,
  lease_until, source_message_id, created_at, updated_at, started_at, finished_at`

func scanTask(row interface{ Scan(dest ...any) error }) (*ConversationTask, error) {
	var t ConversationTask
	err := row.Scan(
		&t.ID, &t.UserID, &t.ConversationID, &t.AgentID, &t.Goal, &t.Status, &t.Attempt, &t.LastError,
		&t.LeaseUntil, &t.SourceMessageID, &t.CreatedAt, &t.UpdatedAt, &t.StartedAt, &t.FinishedAt,
	)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// EnqueueConversationTask inserts a queued task. The same goal already queued or
// running on this conversation is returned instead of a duplicate.
func (d *DB) EnqueueConversationTask(userID, conversationID, agentID, goal, sourceMessageID string) (*ConversationTask, error) {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return nil, errors.New("goal required")
	}
	existing, err := d.findOpenTaskByGoal(conversationID, goal)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	now := Now()
	id := uuid.NewString()
	row := d.SQL.QueryRow(
		`INSERT INTO conversation_tasks
		   (id, user_id, conversation_id, agent_id, goal, status, source_message_id, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8)
		 RETURNING `+taskSelectCols,
		id, userID, conversationID, strings.TrimSpace(agentID), goal, TaskQueued, strings.TrimSpace(sourceMessageID), now,
	)
	return scanTask(row)
}

func (d *DB) findOpenTaskByGoal(conversationID, goal string) (*ConversationTask, error) {
	row := d.SQL.QueryRow(
		`SELECT `+taskSelectCols+`
		 FROM conversation_tasks
		 WHERE conversation_id = $1 AND goal = $2 AND status IN ($3, $4)
		 ORDER BY created_at
		 LIMIT 1`,
		conversationID, goal, TaskQueued, TaskRunning,
	)
	t, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return t, err
}

// OpenConversationTask returns the running task, or the oldest queued one.
func (d *DB) OpenConversationTask(conversationID string) (*ConversationTask, error) {
	row := d.SQL.QueryRow(
		`SELECT `+taskSelectCols+`
		 FROM conversation_tasks
		 WHERE conversation_id = $1 AND status IN ($2, $3)
		 ORDER BY CASE status WHEN 'running' THEN 0 ELSE 1 END, created_at
		 LIMIT 1`,
		conversationID, TaskRunning, TaskQueued,
	)
	t, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return t, err
}

func (d *DB) ConversationHasOpenTask(conversationID string) (bool, error) {
	var exists bool
	err := d.SQL.QueryRow(
		`SELECT EXISTS(
		   SELECT 1 FROM conversation_tasks
		   WHERE conversation_id = $1 AND status IN ($2, $3)
		 )`,
		conversationID, TaskQueued, TaskRunning,
	).Scan(&exists)
	return exists, err
}

// ActiveTaskConversationIDs lists conversations with queued or running tasks for a user.
func (d *DB) ActiveTaskConversationIDs(userID string) (map[string]struct{}, error) {
	rows, err := d.SQL.Query(
		`SELECT DISTINCT conversation_id FROM conversation_tasks
		 WHERE user_id = $1 AND status IN ($2, $3)`,
		userID, TaskQueued, TaskRunning,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]struct{}{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = struct{}{}
	}
	return out, rows.Err()
}

func (d *DB) GetConversationTask(id string) (*ConversationTask, error) {
	row := d.SQL.QueryRow(`SELECT `+taskSelectCols+` FROM conversation_tasks WHERE id = $1`, id)
	t, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return t, err
}

// ClaimNextConversationTask marks the oldest queued task running when that
// conversation has no other running task. Returns nil when the queue is idle.
func (d *DB) ClaimNextConversationTask() (*ConversationTask, error) {
	row := d.SQL.QueryRow(`
WITH picked AS (
  SELECT id FROM conversation_tasks t
  WHERE t.status = $1
    AND NOT EXISTS (
      SELECT 1 FROM conversation_tasks r
      WHERE r.conversation_id = t.conversation_id AND r.status = $2
    )
  ORDER BY t.created_at
  FOR UPDATE SKIP LOCKED
  LIMIT 1
)
UPDATE conversation_tasks c
SET status = $2,
    started_at = COALESCE(c.started_at, NOW()),
    lease_until = NOW() + INTERVAL '2 minutes',
    updated_at = NOW()
FROM picked
WHERE c.id = picked.id
RETURNING c.id, c.user_id, c.conversation_id, c.agent_id, c.goal, c.status, c.attempt, c.last_error,
  c.lease_until, c.source_message_id, c.created_at, c.updated_at, c.started_at, c.finished_at
`, TaskQueued, TaskRunning)
	t, err := scanTask(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return t, err
}

func (d *DB) RenewConversationTaskLease(id string) error {
	_, err := d.SQL.Exec(
		`UPDATE conversation_tasks
		 SET lease_until = NOW() + INTERVAL '2 minutes', updated_at = NOW()
		 WHERE id = $1 AND status = $2`,
		id, TaskRunning,
	)
	return err
}

func (d *DB) SetConversationTaskAttempt(id string, attempt int) error {
	_, err := d.SQL.Exec(
		`UPDATE conversation_tasks SET attempt = $2, updated_at = NOW() WHERE id = $1 AND status = $3`,
		id, attempt, TaskRunning,
	)
	return err
}

// FinishConversationTask moves a running task to done or failed.
// Returns false when the row is no longer running (for example cancelled).
func (d *DB) FinishConversationTask(id, status, lastError string) (bool, error) {
	if status != TaskDone && status != TaskFailed {
		return false, errors.New("status must be done or failed")
	}
	res, err := d.SQL.Exec(
		`UPDATE conversation_tasks
		 SET status = $2, last_error = $3, finished_at = NOW(), updated_at = NOW(), lease_until = NULL
		 WHERE id = $1 AND status = $4`,
		id, status, lastError, TaskRunning,
	)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// CancelOpenConversationTasks cancels queued and running tasks for one conversation.
func (d *DB) CancelOpenConversationTasks(conversationID string) (userID string, n int, err error) {
	rows, err := d.SQL.Query(
		`UPDATE conversation_tasks
		 SET status = $2, finished_at = NOW(), updated_at = NOW(), lease_until = NULL
		 WHERE conversation_id = $1 AND status IN ($3, $4)
		 RETURNING user_id`,
		conversationID, TaskCancelled, TaskQueued, TaskRunning,
	)
	if err != nil {
		return "", 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return userID, n, err
		}
		userID = uid
		n++
	}
	return userID, n, rows.Err()
}

// RequeueExpiredConversationTasks returns running tasks whose lease was not renewed.
func (d *DB) RequeueExpiredConversationTasks() error {
	_, err := d.SQL.Exec(
		`UPDATE conversation_tasks
		 SET status = $1, lease_until = NULL, updated_at = NOW(),
		     last_error = CASE WHEN last_error = '' THEN 'lease expired' ELSE last_error END
		 WHERE status = $2 AND lease_until IS NOT NULL AND lease_until < NOW()`,
		TaskQueued, TaskRunning,
	)
	return err
}
