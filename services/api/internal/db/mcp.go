package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type MCPServer struct {
	ID        string            `json:"id"`
	UserID    string            `json:"user_id"`
	Name      string            `json:"name"`
	Transport string            `json:"transport"` // stdio | sse | http
	Command   string            `json:"command"`
	Args      []string          `json:"args"`
	URL       string            `json:"url"`
	Env       map[string]string `json:"env"`
	Enabled   bool              `json:"enabled"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

type MCPServerPublic struct {
	ID        string            `json:"id"`
	UserID    string            `json:"user_id"`
	Name      string            `json:"name"`
	Transport string            `json:"transport"`
	Command   string            `json:"command"`
	Args      []string          `json:"args"`
	URL       string            `json:"url"`
	Env       map[string]string `json:"env"`
	Enabled   bool              `json:"enabled"`
	CreatedAt string            `json:"created_at"`
	UpdatedAt string            `json:"updated_at"`
}

func (s *MCPServer) Public() MCPServerPublic {
	args := s.Args
	if args == nil {
		args = []string{}
	}
	env := s.Env
	if env == nil {
		env = map[string]string{}
	}
	return MCPServerPublic{
		ID:        s.ID,
		UserID:    s.UserID,
		Name:      s.Name,
		Transport: s.Transport,
		Command:   s.Command,
		Args:      args,
		URL:       s.URL,
		Env:       env,
		Enabled:   s.Enabled,
		CreatedAt: FormatTime(s.CreatedAt),
		UpdatedAt: FormatTime(s.UpdatedAt),
	}
}

func encodeJSONDefault(v any, fallback string) string {
	if v == nil {
		return fallback
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fallback
	}
	return string(b)
}

func decodeStringSlice(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return []string{}
	}
	if out == nil {
		return []string{}
	}
	return out
}

func decodeStringMap(raw string) map[string]string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]string{}
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return map[string]string{}
	}
	if out == nil {
		return map[string]string{}
	}
	return out
}

func normalizeTransport(t string) (string, error) {
	t = strings.ToLower(strings.TrimSpace(t))
	switch t {
	case "", "stdio":
		return "stdio", nil
	case "sse", "http":
		return t, nil
	default:
		return "", errors.New("transport must be stdio, sse, or http")
	}
}

func scanMCPServer(row interface{ Scan(dest ...any) error }) (*MCPServer, error) {
	var s MCPServer
	var argsRaw, envRaw string
	if err := row.Scan(
		&s.ID, &s.UserID, &s.Name, &s.Transport, &s.Command, &argsRaw, &s.URL, &envRaw,
		&s.Enabled, &s.CreatedAt, &s.UpdatedAt,
	); err != nil {
		return nil, err
	}
	s.Args = decodeStringSlice(argsRaw)
	s.Env = decodeStringMap(envRaw)
	return &s, nil
}

func (d *DB) ListMCPServers(userID string) ([]*MCPServer, error) {
	rows, err := d.SQL.Query(
		`SELECT id, user_id, name, transport, command, args_json, url, env_json, enabled, created_at, updated_at
		 FROM mcp_servers WHERE user_id = $1 ORDER BY created_at ASC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*MCPServer
	for rows.Next() {
		s, err := scanMCPServer(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (d *DB) GetMCPServer(userID, id string) (*MCPServer, error) {
	row := d.SQL.QueryRow(
		`SELECT id, user_id, name, transport, command, args_json, url, env_json, enabled, created_at, updated_at
		 FROM mcp_servers WHERE id = $1 AND user_id = $2`,
		id, userID,
	)
	s, err := scanMCPServer(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return s, nil
}

func (d *DB) CreateMCPServer(userID, name, transport, command, url string, args []string, env map[string]string, enabled bool) (*MCPServer, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("name required")
	}
	tr, err := normalizeTransport(transport)
	if err != nil {
		return nil, err
	}
	if args == nil {
		args = []string{}
	}
	if env == nil {
		env = map[string]string{}
	}
	now := Now()
	s := &MCPServer{
		ID:        uuid.NewString(),
		UserID:    userID,
		Name:      name,
		Transport: tr,
		Command:   strings.TrimSpace(command),
		Args:      args,
		URL:       strings.TrimSpace(url),
		Env:       env,
		Enabled:   enabled,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if tr == "stdio" && s.Command == "" {
		return nil, errors.New("command required for stdio transport")
	}
	if (tr == "sse" || tr == "http") && s.URL == "" {
		return nil, errors.New("url required for sse/http transport")
	}
	_, err = d.SQL.Exec(
		`INSERT INTO mcp_servers (id, user_id, name, transport, command, args_json, url, env_json, enabled, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		s.ID, s.UserID, s.Name, s.Transport, s.Command,
		encodeJSONDefault(s.Args, "[]"), s.URL, encodeJSONDefault(s.Env, "{}"),
		s.Enabled, s.CreatedAt, s.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return s, nil
}

type MCPUpdate struct {
	Name      *string
	Transport *string
	Command   *string
	Args      *[]string
	URL       *string
	Env       *map[string]string
	Enabled   *bool
}

func (d *DB) UpdateMCPServer(userID, id string, upd MCPUpdate) (*MCPServer, error) {
	s, err := d.GetMCPServer(userID, id)
	if err != nil {
		return nil, err
	}
	if upd.Name != nil {
		n := strings.TrimSpace(*upd.Name)
		if n == "" {
			return nil, errors.New("name required")
		}
		s.Name = n
	}
	if upd.Transport != nil {
		tr, err := normalizeTransport(*upd.Transport)
		if err != nil {
			return nil, err
		}
		s.Transport = tr
	}
	if upd.Command != nil {
		s.Command = strings.TrimSpace(*upd.Command)
	}
	if upd.Args != nil {
		s.Args = *upd.Args
		if s.Args == nil {
			s.Args = []string{}
		}
	}
	if upd.URL != nil {
		s.URL = strings.TrimSpace(*upd.URL)
	}
	if upd.Env != nil {
		s.Env = *upd.Env
		if s.Env == nil {
			s.Env = map[string]string{}
		}
	}
	if upd.Enabled != nil {
		s.Enabled = *upd.Enabled
	}
	if s.Transport == "stdio" && s.Command == "" {
		return nil, errors.New("command required for stdio transport")
	}
	if (s.Transport == "sse" || s.Transport == "http") && s.URL == "" {
		return nil, errors.New("url required for sse/http transport")
	}
	s.UpdatedAt = Now()
	_, err = d.SQL.Exec(
		`UPDATE mcp_servers SET name=$1, transport=$2, command=$3, args_json=$4, url=$5, env_json=$6, enabled=$7, updated_at=$8
		 WHERE id=$9 AND user_id=$10`,
		s.Name, s.Transport, s.Command, encodeJSONDefault(s.Args, "[]"), s.URL,
		encodeJSONDefault(s.Env, "{}"), s.Enabled, s.UpdatedAt, id, userID,
	)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (d *DB) DeleteMCPServer(userID, id string) error {
	res, err := d.SQL.Exec(`DELETE FROM mcp_servers WHERE id = $1 AND user_id = $2`, id, userID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
