package db

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	OrgID        string    `json:"org_id,omitempty"`
	Role         string    `json:"role,omitempty"`
	Email        string    `json:"email,omitempty"`
	CasdoorSub   string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

var ErrUserExists = errors.New("username already exists")
var ErrNotFound = errors.New("not found")

func scanUser(row interface{ Scan(dest ...any) error }) (*User, error) {
	var u User
	var orgID, email, casdoor sql.NullString
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &orgID, &u.Role, &email, &casdoor, &u.CreatedAt); err != nil {
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
	if u.Role == "" {
		u.Role = RoleMember
	}
	return &u, nil
}

const userSelectCols = `id, username, password_hash, org_id, COALESCE(role, 'member'), COALESCE(email, ''), casdoor_sub, created_at`

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
		`SELECT `+userSelectCols+` FROM users WHERE LOWER(username) = LOWER($1)`,
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
		`SELECT `+userSelectCols+` FROM users WHERE id = $1`,
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

func (d *DB) GetUserByCasdoorSub(sub string) (*User, error) {
	sub = strings.TrimSpace(sub)
	if sub == "" {
		return nil, ErrNotFound
	}
	row := d.SQL.QueryRow(
		`SELECT `+userSelectCols+` FROM users WHERE casdoor_sub = $1`,
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
WHERE id = $1
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
	return map[string]any{
		"id":         u.ID,
		"username":   u.Username,
		"email":      u.Email,
		"org_id":     u.OrgID,
		"role":       NormalizeRole(u.Role),
		"created_at": u.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
}
