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
	ID          string    `json:"id"`
	UserID      string    `json:"user_id"`
	MachineKey  string    `json:"machine_key"`
	Label       string    `json:"label"`
	Platform    string    `json:"platform"`
	OS          string    `json:"os"`
	Arch        string    `json:"arch"`
	App         string    `json:"app"`
	AppVersion  string    `json:"app_version"`
	Status      string    `json:"status"` // online | offline
	LastSeen    time.Time `json:"last_seen"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	FileOpCount int64     `json:"file_op_count"`
	// ExecPolicy: allow | ask | deny — whether the bot may run host ops on this machine.
	// Auto-review still applies when allow/ask.
	ExecPolicy string `json:"exec_policy"`
	// DeviceType: desktop | mobile. Phones are login-only (not hosts). Default desktop.
	DeviceType string `json:"device_type"`
	// HostEligible is computed: device_type == desktop. Not stored.
	HostEligible bool `json:"host_eligible"`
	// Connected is set by the API from the live exec socket, not stored.
	Connected bool `json:"connected"`
}

type MachineRegisterInput struct {
	MachineKey string
	Label      string
	Platform   string
	OS         string
	Arch       string
	App        string
	AppVersion string
	DeviceType string // optional; empty → InferDeviceType(platform, os, app)
}

func (m *Machine) ApplyOnlineStatus(now time.Time) {
	if m.Status == "online" && now.Sub(m.LastSeen) > MachineOfflineAfter {
		m.Status = "offline"
	}
}

func scanMachine(sc interface{ Scan(dest ...any) error }) (Machine, error) {
	var m Machine
	err := sc.Scan(
		&m.ID, &m.UserID, &m.MachineKey, &m.Label, &m.Platform, &m.OS, &m.Arch,
		&m.App, &m.AppVersion, &m.Status, &m.LastSeen, &m.CreatedAt, &m.UpdatedAt,
		&m.FileOpCount, &m.ExecPolicy, &m.DeviceType,
	)
	if err == nil {
		m.ExecPolicy = NormalizeMachineExecPolicy(m.ExecPolicy)
		m.ApplyDeviceFields()
	}
	return m, err
}

func (d *DB) ListMachines(userID string) ([]Machine, error) {
	rows, err := d.SQL.Query(
		`SELECT id, user_id, machine_key, label, platform, os, arch, app, app_version,
		        status, last_seen, created_at, updated_at, file_op_count,
		        COALESCE(exec_policy, 'allow'), COALESCE(device_type, 'desktop')
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
		m, err := scanMachine(rows)
		if err != nil {
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
		        status, last_seen, created_at, updated_at, file_op_count,
		        COALESCE(exec_policy, 'allow'), COALESCE(device_type, 'desktop')
		 FROM user_machines WHERE user_id = $1 AND id = $2`,
		userID, id,
	)
	m, err := scanMachine(row)
	if err != nil {
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
	deviceType := ResolveDeviceType(in.DeviceType, platform, in.OS, in.App)
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
		// Keep user-renamed label; only refresh platform metadata + online status + device_type.
		_, err = d.SQL.Exec(
			`UPDATE user_machines SET
			   platform = $1, os = $2, arch = $3, app = $4, app_version = $5,
			   device_type = $6,
			   status = 'online', last_seen = $7, updated_at = $7
			 WHERE id = $8 AND user_id = $9`,
			platform,
			strings.TrimSpace(in.OS),
			strings.TrimSpace(in.Arch),
			strings.TrimSpace(in.App),
			strings.TrimSpace(in.AppVersion),
			deviceType,
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
		DeviceType: deviceType,
		Status:     "online",
		LastSeen:   now,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	m.ApplyDeviceFields()
	_, err = d.SQL.Exec(
		`INSERT INTO user_machines (
		   id, user_id, machine_key, label, platform, os, arch, app, app_version,
		   status, last_seen, created_at, updated_at, device_type
		 ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		m.ID, m.UserID, m.MachineKey, m.Label, m.Platform, m.OS, m.Arch, m.App, m.AppVersion,
		m.Status, m.LastSeen, m.CreatedAt, m.UpdatedAt, m.DeviceType,
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

func (d *DB) IncrementMachineFileOps(userID, id string) (*Machine, error) {
	now := Now()
	res, err := d.SQL.Exec(
		`UPDATE user_machines SET file_op_count = file_op_count + 1, updated_at = $1
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

func (d *DB) UsualWorkMachine(userID string) (*Machine, error) {
	list, err := d.ListMachines(userID)
	if err != nil {
		return nil, err
	}
	var best *Machine
	for i := range list {
		m := &list[i]
		if !IsHostEligible(*m) {
			continue
		}
		if m.FileOpCount <= 0 {
			continue
		}
		if best == nil || m.FileOpCount > best.FileOpCount {
			best = m
		}
	}
	return best, nil
}

func (d *DB) UpdateMachineLabel(userID, id, label string) (*Machine, error) {
	return d.UpdateMachine(userID, id, &label, nil)
}

// UpdateMachine patches label and/or exec_policy. At least one field required.
func (d *DB) UpdateMachine(userID, id string, label, execPolicy *string) (*Machine, error) {
	if label == nil && execPolicy == nil {
		return nil, errors.New("label or exec_policy required")
	}
	var name *string
	if label != nil {
		n := strings.TrimSpace(*label)
		if n == "" {
			return nil, errors.New("label required")
		}
		if len([]rune(n)) > 64 {
			return nil, errors.New("label too long")
		}
		name = &n
	}
	var policy *string
	if execPolicy != nil {
		p := NormalizeMachineExecPolicy(*execPolicy)
		policy = &p
	}
	now := Now()
	res, err := d.SQL.Exec(
		`UPDATE user_machines SET
		   label = COALESCE($1, label),
		   exec_policy = COALESCE($2, exec_policy),
		   updated_at = $3
		 WHERE id = $4 AND user_id = $5`,
		name, policy, now, id, userID,
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
	if err := d.ClearAgentMachineIDForMachine(userID, id); err != nil {
		return err
	}
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
