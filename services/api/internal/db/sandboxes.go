package db

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Sandbox statuses for phase-1 computer MVP.
const (
	SandboxStatusCreating = "creating"
	SandboxStatusRunning  = "running"
	SandboxStatusStopped  = "stopped"
	SandboxStatusError    = "error"
)

type Sandbox struct {
	ID             string    `json:"id"`
	UserID         string    `json:"user_id"`
	ContainerID    string    `json:"container_id"`
	Status         string    `json:"status"`
	Image          string    `json:"image"`
	WorkdirHost    string    `json:"workdir_host"`
	ComputerMode   string    `json:"computer_mode"` // team|private (user default)
	DesktopPort    int       `json:"desktop_port,omitempty"`
	DesktopToken   string    `json:"desktop_token,omitempty"`
	CheckpointPath string    `json:"checkpoint_path,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	LastError      string    `json:"last_error,omitempty"`
}

func scanSandbox(row interface{ Scan(dest ...any) error }) (*Sandbox, error) {
	var s Sandbox
	if err := row.Scan(
		&s.ID, &s.UserID, &s.ContainerID, &s.Status, &s.Image, &s.WorkdirHost,
		&s.ComputerMode, &s.DesktopPort, &s.DesktopToken, &s.CheckpointPath,
		&s.CreatedAt, &s.UpdatedAt, &s.LastError,
	); err != nil {
		return nil, err
	}
	if s.ComputerMode == "" {
		s.ComputerMode = "team"
	}
	return &s, nil
}

const sandboxSelect = `SELECT id, user_id, container_id, status, image, workdir_host,
		 COALESCE(computer_mode,'team'), COALESCE(desktop_port,0), COALESCE(desktop_token,''),
		 COALESCE(checkpoint_path,''), created_at, updated_at, last_error
		 FROM sandboxes`

// GetOrCreateSandbox returns the user's sandbox row, inserting a stopped stub if missing.
func (d *DB) GetOrCreateSandbox(userID, defaultImage string) (*Sandbox, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, errors.New("user_id required")
	}
	row := d.SQL.QueryRow(sandboxSelect+` WHERE user_id = $1`, userID)
	s, err := scanSandbox(row)
	if err == nil {
		return s, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	now := Now()
	img := strings.TrimSpace(defaultImage)
	s = &Sandbox{
		ID:           uuid.NewString(),
		UserID:       userID,
		ContainerID:  "",
		Status:       SandboxStatusStopped,
		Image:        img,
		WorkdirHost:  "",
		ComputerMode: "team",
		CreatedAt:    now,
		UpdatedAt:    now,
		LastError:    "",
	}
	_, err = d.SQL.Exec(
		`INSERT INTO sandboxes (id, user_id, container_id, status, image, workdir_host, computer_mode, created_at, updated_at, last_error)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		 ON CONFLICT (user_id) DO NOTHING`,
		s.ID, s.UserID, s.ContainerID, s.Status, s.Image, s.WorkdirHost, s.ComputerMode, s.CreatedAt, s.UpdatedAt, s.LastError,
	)
	if err != nil {
		return nil, err
	}
	row = d.SQL.QueryRow(sandboxSelect+` WHERE user_id = $1`, userID)
	return scanSandbox(row)
}

func (d *DB) GetSandboxByUser(userID string) (*Sandbox, error) {
	row := d.SQL.QueryRow(sandboxSelect+` WHERE user_id = $1`, userID)
	s, err := scanSandbox(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return s, nil
}

func (d *DB) UpdateSandbox(s *Sandbox) error {
	if s == nil || s.ID == "" {
		return errors.New("sandbox required")
	}
	if s.ComputerMode == "" {
		s.ComputerMode = "team"
	}
	s.UpdatedAt = Now()
	res, err := d.SQL.Exec(
		`UPDATE sandboxes SET container_id=$2, status=$3, image=$4, workdir_host=$5, updated_at=$6, last_error=$7,
		 computer_mode=$8, desktop_port=$9, desktop_token=$10, checkpoint_path=$11
		 WHERE id=$1`,
		s.ID, s.ContainerID, s.Status, s.Image, s.WorkdirHost, s.UpdatedAt, s.LastError,
		s.ComputerMode, s.DesktopPort, s.DesktopToken, s.CheckpointPath,
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
