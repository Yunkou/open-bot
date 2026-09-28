package db

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// OfflineAfter is how long after last_seen a machine is considered offline.
const MachineOfflineAfter = 90 * time.Second

type Machine struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	MachineKey string    `json:"machine_key"`
	Label      string    `json:"label"`
	Platform   string    `json:"platform"`
	OS         string    `json:"os"`
	Arch       string    `json:"arch"`
	App        string    `json:"app"`
	AppVersion string    `json:"app_version"`
	Status     string    `json:"status"` // online | offline
	LastSeen   time.Time `json:"last_seen"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type MachineRegisterInput struct {
	MachineKey string
	Label      string
	Platform   string
	OS         string
	Arch       string
	App        string
	AppVersion string
}

func (m *Machine) ApplyOnlineStatus(now time.Time) {
	if m.Status == "online" && now.Sub(m.LastSeen) > MachineOfflineAfter {
		m.Status = "offline"
	}
}

func (d *DB) ListMachines(userID string) ([]Machine, error) {
	rows, err := d.SQL.Query(
		`SELECT id, user_id, machine_key, label, platform, os, arch, app, app_version,
		        status, last_seen, created_at, updated_at
		 FROM user_machines WHERE user_id = $1 ORDER BY last_seen DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	now := Now()
	out := make([]Machine, 0)
	for rows.Next() {
		var m Machine
		if err := rows.Scan(
			&m.ID, &m.UserID, &m.MachineKey, &m.Label, &m.Platform, &m.OS, &m.Arch,
			&m.App, &m.AppVersion, &m.Status, &m.LastSeen, &m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, err
		}
		m.ApplyOnlineStatus(now)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (d *DB) GetMachine(userID, id string) (*Machine, error) {
	row := d.SQL.QueryRow(
		`SELECT id, user_id, machine_key, label, platform, os, arch, app, app_version,
		        status, last_seen, created_at, updated_at
		 FROM user_machines WHERE user_id = $1 AND id = $2`,
		userID, id,
	)
	var m Machine
	if err := row.Scan(
		&m.ID, &m.UserID, &m.MachineKey, &m.Label, &m.Platform, &m.OS, &m.Arch,
		&m.App, &m.AppVersion, &m.Status, &m.LastSeen, &m.CreatedAt, &m.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	m.ApplyOnlineStatus(Now())
	return &m, nil
}

func (d *DB) RegisterMachine(userID string, in MachineRegisterInput) (*Machine, error) {
	key := strings.TrimSpace(in.MachineKey)
	if key == "" {
		return nil, errors.New("machine_key required")
	}
	label := strings.TrimSpace(in.Label)
	if label == "" {
		label = defaultMachineLabel(in.Platform, in.App)
	}
	platform := strings.TrimSpace(in.Platform)
	if platform == "" {
		platform = "unknown"
	}
	now := Now()
	// Upsert by (user_id, machine_key)
	var existingID string
	err := d.SQL.QueryRow(
		`SELECT id FROM user_machines WHERE user_id = $1 AND machine_key = $2`,
		userID, key,
	).Scan(&existingID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if existingID != "" {
		_, err = d.SQL.Exec(
			`UPDATE user_machines SET
			   label = $1, platform = $2, os = $3, arch = $4, app = $5, app_version = $6,
			   status = 'online', last_seen = $7, updated_at = $7
			 WHERE id = $8 AND user_id = $9`,
			label,
			platform,
			strings.TrimSpace(in.OS),
			strings.TrimSpace(in.Arch),
			strings.TrimSpace(in.App),
			strings.TrimSpace(in.AppVersion),
			now,
			existingID,
			userID,
		)
		if err != nil {
			return nil, err
		}
		return d.GetMachine(userID, existingID)
	}
	m := &Machine{
		ID:         uuid.NewString(),
		UserID:     userID,
		MachineKey: key,
		Label:      label,
		Platform:   platform,
		OS:         strings.TrimSpace(in.OS),
		Arch:       strings.TrimSpace(in.Arch),
		App:        strings.TrimSpace(in.App),
		AppVersion: strings.TrimSpace(in.AppVersion),
		Status:     "online",
		LastSeen:   now,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	_, err = d.SQL.Exec(
		`INSERT INTO user_machines (
		   id, user_id, machine_key, label, platform, os, arch, app, app_version,
		   status, last_seen, created_at, updated_at
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		m.ID, m.UserID, m.MachineKey, m.Label, m.Platform, m.OS, m.Arch, m.App, m.AppVersion,
		m.Status, m.LastSeen, m.CreatedAt, m.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (d *DB) HeartbeatMachine(userID, id string) (*Machine, error) {
	now := Now()
	res, err := d.SQL.Exec(
		`UPDATE user_machines SET status = 'online', last_seen = $1, updated_at = $1
		 WHERE id = $2 AND user_id = $3`,
		now, id, userID,
	)
	if err != nil {
		return nil, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return nil, ErrNotFound
	}
	return d.GetMachine(userID, id)
}

func (d *DB) DeleteMachine(userID, id string) error {
	res, err := d.SQL.Exec(
		`DELETE FROM user_machines WHERE id = $1 AND user_id = $2`,
		id, userID,
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

func defaultMachineLabel(platform, app string) string {
	p := strings.TrimSpace(platform)
	switch p {
	case "macos":
		return "我的 Mac"
	case "windows":
		return "我的 Windows 电脑"
	case "linux":
		return "我的 Linux 电脑"
	case "ios":
		return "我的 iPhone/iPad"
	case "android":
		return "我的 Android 设备"
	case "web":
		return "浏览器会话"
	}
	if strings.TrimSpace(app) != "" {
		return "本机 (" + app + ")"
	}
	return "本机"
}
