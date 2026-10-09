package db

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           string     `json:"id"`
	Username     string     `json:"username"`
	PasswordHash string     `json:"-"`
	OrgID        string     `json:"org_id,omitempty"`
	Role         string     `json:"role,omitempty"`
	Email        string     `json:"email,omitempty"`
	CasdoorSub   string     `json:"-"`
	CreatedAt    time.Time  `json:"created_at"`
	DeletedAt    *time.Time `json:"deleted_at,omitempty"`
}

var ErrUserExists = errors.New("username already exists")
var ErrNotFound = errors.New("not found")

func scanUser(row interface{ Scan(dest ...any) error }) (*User, error) {
	var u User
	var orgID, email, casdoor sql.NullString
	var deletedAt sql.NullTime
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &orgID, &u.Role, &email, &casdoor, &u.CreatedAt, &deletedAt); err != nil {
		return nil, err
	}
	if orgID.Valid {
		u.OrgID = orgID.String
	}
	if email.Valid {
		u.Email = email.String
	}
	if casdoor.Valid {
		u.CasdoorSub = casdoor.String
	}
	if deletedAt.Valid {
		t := deletedAt.Time
		u.DeletedAt = &t
	}
	if u.Role == "" {
		u.Role = RoleMember
	}
	return &u, nil
}

const userSelectCols = `id, username, password_hash, org_id, COALESCE(role, 'member'), COALESCE(email, ''), casdoor_sub, created_at, deleted_at`

func (d *DB) CreateUser(username, passwordHash string) (*User, error) {
	return d.CreateUserFull(username, passwordHash, "", "", "")
}

func (d *DB) CreateUserFull(username, passwordHash, email, casdoorSub, role string) (*User, error) {
	username = strings.TrimSpace(username)
	if username == "" || passwordHash == "" {
		return nil, errors.New("username and password required")
	}
	u := &User{
		ID:           uuid.NewString(),
		Username:     username,
		PasswordHash: passwordHash,
		Email:        strings.TrimSpace(email),
		CasdoorSub:   strings.TrimSpace(casdoorSub),
		Role:         NormalizeRole(role),
		CreatedAt:    Now(),
	}
	_, err := d.SQL.Exec(
		`INSERT INTO users (id, username, password_hash, email, casdoor_sub, role, created_at)
VALUES ($1, $2, $3, $4, NULLIF($5, ''), $6, $7)`,
		u.ID, u.Username, u.PasswordHash, u.Email, u.CasdoorSub, u.Role, u.CreatedAt,
	)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") ||
			strings.Contains(err.Error(), "duplicate key") {
			return nil, ErrUserExists
		}
		return nil, err
	}
	if username != A2ASystemUsername {
		if err := d.AssignNewUserToOrg(u); err != nil {
			return nil, err
		}
	}
	return u, nil
}

func (d *DB) GetUserByUsername(username string) (*User, error) {
	row := d.SQL.QueryRow(
		`SELECT `+userSelectCols+` FROM users WHERE LOWER(username) = LOWER($1) AND deleted_at IS NULL`,
		strings.TrimSpace(username),
	)
	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return u, nil
}

func (d *DB) GetUserByID(id string) (*User, error) {
	row := d.SQL.QueryRow(
		`SELECT `+userSelectCols+` FROM users WHERE id = $1 AND deleted_at IS NULL`,
		id,
	)
	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return u, nil
}

// GetUserByIDIncludingDeleted loads a user even when soft-deleted (admin paths only).
// soft-delete: include deleted
func (d *DB) GetUserByIDIncludingDeleted(id string) (*User, error) {
	row := d.SQL.QueryRow(`SELECT `+userSelectCols+` FROM users WHERE id = $1`, id)
	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return u, nil
}

func (d *DB) GetUserByCasdoorSub(sub string) (*User, error) {
	sub = strings.TrimSpace(sub)
	if sub == "" {
		return nil, ErrNotFound
	}
	row := d.SQL.QueryRow(
		`SELECT `+userSelectCols+` FROM users WHERE casdoor_sub = $1 AND deleted_at IS NULL`,
		sub,
	)
	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return u, nil
}

func (d *DB) LinkCasdoorSub(userID, sub, email string) error {
	_, err := d.SQL.Exec(`
UPDATE users SET casdoor_sub = NULLIF($2, ''),
  email = CASE WHEN $3 <> '' THEN $3 ELSE email END
WHERE id = $1 AND deleted_at IS NULL
`, userID, strings.TrimSpace(sub), strings.TrimSpace(email))
	return err
}

const A2ASystemUsername = "__a2a__"

// EnsureA2ASystemUser creates/returns the internal user used by the A2A adapter.
func (d *DB) EnsureA2ASystemUser(passwordHash string) (*User, error) {
	if u, err := d.GetUserByUsername(A2ASystemUsername); err == nil {
		return u, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	u, err := d.CreateUser(A2ASystemUsername, passwordHash)
	if err != nil {
		if errors.Is(err, ErrUserExists) {
			return d.GetUserByUsername(A2ASystemUsername)
		}
		return nil, err
	}
	return u, nil
}

func (u *User) PublicMap() map[string]any {
	if u == nil {
		return nil
	}
	m := map[string]any{
		"id":         u.ID,
		"username":   u.Username,
		"email":      u.Email,
		"org_id":     u.OrgID,
		"role":       NormalizeRole(u.Role),
		"created_at": u.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if u.DeletedAt != nil {
		m["deleted_at"] = u.DeletedAt.UTC().Format(time.RFC3339Nano)
	}
	return m
}

// LookupUsernames returns id→username for the given ids (includes soft-deleted).
// soft-delete: include deleted
func (d *DB) LookupUsernames(ids []string) map[string]string {
	out := map[string]string{}
	clean := uniqueNonEmpty(ids)
	if d == nil || d.SQL == nil || len(clean) == 0 {
		return out
	}
	q, args := buildIDInQuery(`SELECT id, username FROM users WHERE id IN (`, clean)
	rows, err := d.SQL.Query(q, args...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			continue
		}
		out[id] = name
	}
	return out
}

func uniqueNonEmpty(ids []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func buildIDInQuery(prefix string, ids []string) (string, []any) {
	args := make([]any, len(ids))
	b := strings.Builder{}
	b.WriteString(prefix)
	for i, id := range ids {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('$')
		b.WriteString(strconv.Itoa(i + 1))
		args[i] = id
	}
	b.WriteByte(')')
	return b.String(), args
}
