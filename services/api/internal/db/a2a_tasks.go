package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// A2A task lifecycle states (A2A TaskState subset).
const (
	A2AStateSubmitted = "submitted"
	A2AStateWorking   = "working"
	A2AStateCompleted = "completed"
	A2AStateFailed    = "failed"
	A2AStateCanceled  = "canceled"
)

func NormalizeA2AState(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case A2AStateSubmitted, "task_state_submitted":
		return A2AStateSubmitted
	case A2AStateWorking, "task_state_working":
		return A2AStateWorking
	case A2AStateCompleted, "task_state_completed":
		return A2AStateCompleted
	case A2AStateFailed, "task_state_failed":
		return A2AStateFailed
	case A2AStateCanceled, "cancelled", "task_state_canceled", "task_state_cancelled":
		return A2AStateCanceled
	default:
		return s
	}
}

func A2AStateTerminal(s string) bool {
	switch NormalizeA2AState(s) {
	case A2AStateCompleted, A2AStateFailed, A2AStateCanceled:
		return true
	default:
		return false
	}
}

type A2ATask struct {
	ID             string
	ContextID      string
	UserID         string
	AgentID        string
	State          string
	InputText      string
	ResultText     string
	ErrorMessage   string
	ArtifactsJSON  string
	HistoryJSON    string
	MetadataJSON   string
	PushURL        string
	PushToken      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	CompletedAt    *time.Time
}

func (d *DB) migrateA2ATasks() error {
	_, err := d.SQL.Exec(`
CREATE TABLE IF NOT EXISTS a2a_tasks (
  id TEXT PRIMARY KEY,
  context_id TEXT NOT NULL DEFAULT '',
  user_id TEXT NOT NULL REFERENCES users(id),
  agent_id TEXT NOT NULL DEFAULT '',
  state TEXT NOT NULL DEFAULT 'submitted',
  input_text TEXT NOT NULL DEFAULT '',
  result_text TEXT NOT NULL DEFAULT '',
  error_message TEXT NOT NULL DEFAULT '',
  artifacts_json TEXT NOT NULL DEFAULT '[]',
  history_json TEXT NOT NULL DEFAULT '[]',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  push_url TEXT NOT NULL DEFAULT '',
  push_token TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  completed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_a2a_tasks_context ON a2a_tasks(context_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_a2a_tasks_state ON a2a_tasks(state, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_a2a_tasks_user ON a2a_tasks(user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS a2a_push_configs (
  id TEXT PRIMARY KEY,
  task_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  url TEXT NOT NULL,
  token TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_a2a_push_task ON a2a_push_configs(task_id) WHERE task_id <> '';
CREATE INDEX IF NOT EXISTS idx_a2a_push_agent ON a2a_push_configs(agent_id) WHERE agent_id <> '';
`)
	return err
}

func (d *DB) CreateA2ATask(userID, agentID, contextID, inputText string) (*A2ATask, error) {
	now := Now()
	t := &A2ATask{
		ID:            uuid.NewString(),
		ContextID:     strings.TrimSpace(contextID),
		UserID:        userID,
		AgentID:       strings.TrimSpace(agentID),
		State:         A2AStateSubmitted,
		InputText:     inputText,
		ArtifactsJSON: "[]",
		HistoryJSON:   "[]",
		MetadataJSON:  "{}",
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	if t.ContextID == "" {
		t.ContextID = "a2a-" + t.ID
	}
	_, err := d.SQL.Exec(`
INSERT INTO a2a_tasks (
  id, context_id, user_id, agent_id, state, input_text,
  artifacts_json, history_json, metadata_json, created_at, updated_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
`, t.ID, t.ContextID, t.UserID, t.AgentID, t.State, t.InputText,
		t.ArtifactsJSON, t.HistoryJSON, t.MetadataJSON, t.CreatedAt, t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return t, nil
}

func scanA2ATask(row interface{ Scan(dest ...any) error }) (*A2ATask, error) {
	t := &A2ATask{}
	var completed sql.NullTime
	err := row.Scan(
		&t.ID, &t.ContextID, &t.UserID, &t.AgentID, &t.State,
		&t.InputText, &t.ResultText, &t.ErrorMessage,
		&t.ArtifactsJSON, &t.HistoryJSON, &t.MetadataJSON,
		&t.PushURL, &t.PushToken,
		&t.CreatedAt, &t.UpdatedAt, &completed,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if completed.Valid {
		t.CompletedAt = &completed.Time
	}
	return t, nil
}

const a2aTaskCols = `
id, context_id, user_id, agent_id, state,
input_text, result_text, error_message,
artifacts_json, history_json, metadata_json,
push_url, push_token, created_at, updated_at, completed_at`

func (d *DB) GetA2ATask(id string) (*A2ATask, error) {
	row := d.SQL.QueryRow(`SELECT `+a2aTaskCols+` FROM a2a_tasks WHERE id = $1`, strings.TrimSpace(id))
	return scanA2ATask(row)
}

func (d *DB) UpdateA2ATaskState(id, state, resultText, errorMessage, artifactsJSON, historyJSON string) (*A2ATask, error) {
	state = NormalizeA2AState(state)
	now := Now()
	var completed any
	if A2AStateTerminal(state) {
		completed = now
	} else {
		completed = nil
	}
	if artifactsJSON == "" {
		artifactsJSON = "[]"
	}
	if historyJSON == "" {
		historyJSON = "[]"
	}
	res, err := d.SQL.Exec(`
UPDATE a2a_tasks SET
  state = $2,
  result_text = COALESCE(NULLIF($3, ''), result_text),
  error_message = COALESCE($4, error_message),
  artifacts_json = CASE WHEN $5 = '' OR $5 = '[]' THEN artifacts_json ELSE $5 END,
  history_json = CASE WHEN $6 = '' OR $6 = '[]' THEN history_json ELSE $6 END,
  updated_at = $7,
  completed_at = COALESCE($8::timestamptz, completed_at)
WHERE id = $1
`, id, state, resultText, errorMessage, artifactsJSON, historyJSON, now, completed)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, ErrNotFound
	}
	return d.GetA2ATask(id)
}

func (d *DB) SetA2ATaskPush(taskID, url, token string) error {
	_, err := d.SQL.Exec(`
UPDATE a2a_tasks SET push_url = $2, push_token = $3, updated_at = $4 WHERE id = $1
`, taskID, strings.TrimSpace(url), strings.TrimSpace(token), Now())
	return err
}

type A2APushConfig struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id,omitempty"`
	AgentID   string    `json:"agent_id,omitempty"`
	URL       string    `json:"url"`
	Token     string    `json:"token,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

func (d *DB) UpsertA2APushConfig(taskID, agentID, url, token string) (*A2APushConfig, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return nil, errors.New("push url required")
	}
	cfg := &A2APushConfig{
		ID:        uuid.NewString(),
		TaskID:    strings.TrimSpace(taskID),
		AgentID:   strings.TrimSpace(agentID),
		URL:       url,
		Token:     strings.TrimSpace(token),
		CreatedAt: Now(),
	}
	_, err := d.SQL.Exec(`
INSERT INTO a2a_push_configs (id, task_id, agent_id, url, token, created_at)
VALUES ($1,$2,$3,$4,$5,$6)
`, cfg.ID, cfg.TaskID, cfg.AgentID, cfg.URL, cfg.Token, cfg.CreatedAt)
	if err != nil {
		return nil, err
	}
	if cfg.TaskID != "" {
		_ = d.SetA2ATaskPush(cfg.TaskID, cfg.URL, cfg.Token)
	}
	return cfg, nil
}

func (d *DB) ListA2APushConfigs(taskID, agentID string) ([]A2APushConfig, error) {
	rows, err := d.SQL.Query(`
SELECT id, task_id, agent_id, url, token, created_at FROM a2a_push_configs
WHERE ($1 <> '' AND task_id = $1) OR ($2 <> '' AND agent_id = $2)
ORDER BY created_at DESC
`, strings.TrimSpace(taskID), strings.TrimSpace(agentID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []A2APushConfig
	for rows.Next() {
		var c A2APushConfig
		if err := rows.Scan(&c.ID, &c.TaskID, &c.AgentID, &c.URL, &c.Token, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// A2ATaskPublicMap builds an A2A-shaped Task object for JSON-RPC results.
func A2ATaskPublicMap(t *A2ATask) map[string]any {
	if t == nil {
		return nil
	}
	var artifacts any = []any{}
	_ = json.Unmarshal([]byte(t.ArtifactsJSON), &artifacts)
	var history any = []any{}
	_ = json.Unmarshal([]byte(t.HistoryJSON), &history)
	status := map[string]any{
		"state":     t.State,
		"timestamp": t.UpdatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
	}
	if t.ErrorMessage != "" && t.State == A2AStateFailed {
		status["message"] = map[string]any{
			"role":  "agent",
			"parts": []map[string]any{{"kind": "text", "text": t.ErrorMessage}},
		}
	}
	out := map[string]any{
		"id":        t.ID,
		"contextId": t.ContextID,
		"status":    status,
		"artifacts": artifacts,
		"history":   history,
		"kind":      "task",
		"agent_id":  t.AgentID,
	}
	if t.ResultText != "" {
		out["result_text"] = t.ResultText
	}
	return out
}
