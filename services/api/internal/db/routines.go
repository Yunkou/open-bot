package db

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Routine struct {
	ID           string      `json:"id"`
	UserID       string      `json:"user_id"`
	Name         string      `json:"name"`
	Prompt       string      `json:"prompt"`
	ScheduleCron string      `json:"schedule_cron"`
	Enabled      bool        `json:"enabled"`
	AgentID      string      `json:"agent_id"`
	LastRunAt    *time.Time  `json:"last_run_at,omitempty"`
	NextRunAt    *time.Time  `json:"next_run_at,omitempty"`
	LastError    string      `json:"last_error,omitempty"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
	LastRun      *RoutineRun `json:"last_run,omitempty"`
}

type RoutineRun struct {
	ID         string    `json:"id"`
	RoutineID  string    `json:"routine_id"`
	Status     string    `json:"status"`
	ResultText string    `json:"result_text"`
	CreatedAt  time.Time `json:"created_at"`
}

const routineSelect = `SELECT id, user_id, name, prompt, schedule_cron, enabled,
		 COALESCE(agent_id,'open-bot'), last_run_at, next_run_at, COALESCE(last_error,''), created_at, updated_at
		 FROM routines`

func (d *DB) CreateRoutine(userID, name, prompt, scheduleCron string, enabled bool) (*Routine, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("name required")
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil, errors.New("prompt required")
	}
	scheduleCron = strings.TrimSpace(scheduleCron)
	if scheduleCron == "" {
		scheduleCron = "0 9 * * *"
	}
	agentID, aerr := d.FirstAgentID(userID)
	if aerr != nil {
		return nil, aerr
	}
	now := Now()
	r := &Routine{
		ID:           uuid.NewString(),
		UserID:       userID,
		Name:         name,
		Prompt:       prompt,
		ScheduleCron: scheduleCron,
		Enabled:      enabled,
		AgentID:      agentID,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	_, err := d.SQL.Exec(
		`INSERT INTO routines (id, user_id, name, prompt, schedule_cron, enabled, agent_id, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		r.ID, r.UserID, r.Name, r.Prompt, r.ScheduleCron, r.Enabled, r.AgentID, r.CreatedAt, r.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func scanRoutine(row interface{ Scan(dest ...any) error }) (*Routine, error) {
	var r Routine
	var last, next sql.NullTime
	if err := row.Scan(
		&r.ID, &r.UserID, &r.Name, &r.Prompt, &r.ScheduleCron, &r.Enabled,
		&r.AgentID, &last, &next, &r.LastError, &r.CreatedAt, &r.UpdatedAt,
	); err != nil {
		return nil, err
	}
	// leave empty; callers should ResolveAgentID
	if last.Valid {
		t := last.Time
		r.LastRunAt = &t
	}
	if next.Valid {
		t := next.Time
		r.NextRunAt = &t
	}
	return &r, nil
}

func (d *DB) ListRoutines(userID string) ([]*Routine, error) {
	rows, err := d.SQL.Query(routineSelect+` WHERE user_id = $1 ORDER BY created_at ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Routine
	for rows.Next() {
		r, err := scanRoutine(rows)
		if err != nil {
			return nil, err
		}
		if lr, lerr := d.LatestRoutineRun(r.ID); lerr == nil {
			r.LastRun = lr
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) GetRoutine(userID, id string) (*Routine, error) {
	row := d.SQL.QueryRow(routineSelect+` WHERE id = $1 AND user_id = $2`, id, userID)
	r, err := scanRoutine(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if lr, lerr := d.LatestRoutineRun(r.ID); lerr == nil {
		r.LastRun = lr
	}
	return r, nil
}

func (d *DB) GetRoutineByID(id string) (*Routine, error) {
	row := d.SQL.QueryRow(routineSelect+` WHERE id = $1`, id)
	r, err := scanRoutine(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return r, nil
}

type RoutineUpdate struct {
	Name         *string
	Prompt       *string
	ScheduleCron *string
	Enabled      *bool
	AgentID      *string
}

func (d *DB) UpdateRoutine(userID, id string, upd RoutineUpdate) (*Routine, error) {
	r, err := d.GetRoutine(userID, id)
	if err != nil {
		return nil, err
	}
	if upd.Name != nil {
		n := strings.TrimSpace(*upd.Name)
		if n == "" {
			return nil, errors.New("name required")
		}
		r.Name = n
	}
	if upd.Prompt != nil {
		p := strings.TrimSpace(*upd.Prompt)
		if p == "" {
			return nil, errors.New("prompt required")
		}
		r.Prompt = p
	}
	if upd.ScheduleCron != nil {
		c := strings.TrimSpace(*upd.ScheduleCron)
		if c == "" {
			return nil, errors.New("schedule_cron required")
		}
		r.ScheduleCron = c
	}
	if upd.Enabled != nil {
		r.Enabled = *upd.Enabled
	}
	if upd.AgentID != nil {
		a := strings.TrimSpace(*upd.AgentID)
		if a == "" {
			var aerr error
			a, aerr = d.FirstAgentID(userID)
			if aerr != nil {
				return nil, aerr
			}
		}
		r.AgentID = a
	}
	r.UpdatedAt = Now()
	_, err = d.SQL.Exec(
		`UPDATE routines SET name=$1, prompt=$2, schedule_cron=$3, enabled=$4, agent_id=$5, updated_at=$6
		 WHERE id=$7 AND user_id=$8`,
		r.Name, r.Prompt, r.ScheduleCron, r.Enabled, r.AgentID, r.UpdatedAt, id, userID,
	)
	if err != nil {
		return nil, err
	}
	return d.GetRoutine(userID, id)
}

func (d *DB) DeleteRoutine(userID, id string) error {
	res, err := d.SQL.Exec(`DELETE FROM routines WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (d *DB) ListEnabledRoutines() ([]*Routine, error) {
	rows, err := d.SQL.Query(routineSelect + ` WHERE enabled = TRUE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Routine
	for rows.Next() {
		r, err := scanRoutine(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) ClaimDueRoutines() ([]*Routine, error) {
	return d.ListEnabledRoutines()
}

func (d *DB) MarkRoutineRun(routineID string, at time.Time) error {
	_, err := d.SQL.Exec(
		`UPDATE routines SET last_run_at = $1, updated_at = $1, last_error = '' WHERE id = $2`,
		at, routineID,
	)
	return err
}

func (d *DB) MarkRoutineSchedule(routineID string, lastRun, nextRun *time.Time, lastErr string) error {
	_, err := d.SQL.Exec(
		`UPDATE routines SET last_run_at = $1, next_run_at = $2, last_error = $3, updated_at = NOW() WHERE id = $4`,
		lastRun, nextRun, lastErr, routineID,
	)
	return err
}

func (d *DB) CreateRoutineRun(routineID, status, resultText string) (*RoutineRun, error) {
	if status == "" {
		status = "pending"
	}
	run := &RoutineRun{
		ID:         uuid.NewString(),
		RoutineID:  routineID,
		Status:     status,
		ResultText: resultText,
		CreatedAt:  Now(),
	}
	_, err := d.SQL.Exec(
		`INSERT INTO routine_runs (id, routine_id, status, result_text, created_at)
		 VALUES ($1,$2,$3,$4,$5)`,
		run.ID, run.RoutineID, run.Status, run.ResultText, run.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return run, nil
}

func (d *DB) UpdateRoutineRun(id, status, resultText string) error {
	_, err := d.SQL.Exec(
		`UPDATE routine_runs SET status = $1, result_text = $2 WHERE id = $3`,
		status, resultText, id,
	)
	return err
}

func (d *DB) LatestRoutineRun(routineID string) (*RoutineRun, error) {
	row := d.SQL.QueryRow(
		`SELECT id, routine_id, status, result_text, created_at
		 FROM routine_runs WHERE routine_id = $1
		 ORDER BY created_at DESC LIMIT 1`,
		routineID,
	)
	var run RoutineRun
	if err := row.Scan(&run.ID, &run.RoutineID, &run.Status, &run.ResultText, &run.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &run, nil
}

func (d *DB) ListRoutineRuns(userID, routineID string, limit int) ([]*RoutineRun, error) {
	if _, err := d.GetRoutine(userID, routineID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := d.SQL.Query(
		`SELECT id, routine_id, status, result_text, created_at
		 FROM routine_runs WHERE routine_id = $1
		 ORDER BY created_at DESC LIMIT $2`,
		routineID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*RoutineRun
	for rows.Next() {
		var run RoutineRun
		if err := rows.Scan(&run.ID, &run.RoutineID, &run.Status, &run.ResultText, &run.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &run)
	}
	return out, rows.Err()
}
