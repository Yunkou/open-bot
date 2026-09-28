package db

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	RolePlatformAdmin = "platform_admin"
	RoleOrgAdmin      = "org_admin"
	RoleMember        = "member"

	DefaultOrgSlug = "default"
	DefaultOrgName = "默认组织"
)

type Org struct {
	ID        string    `json:"id"`
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type OrgSettings struct {
	OrgID                 string    `json:"org_id"`
	LLMName               string    `json:"llm_name"`
	LLMBaseURL            string    `json:"llm_base_url"`
	LLMAPIKey             string    `json:"-"`
	LLMModel              string    `json:"llm_model"`
	LLMEnableTools        bool      `json:"llm_enable_tools"`
	LLMContextWindow      *int      `json:"llm_context_window,omitempty"`
	FeatureFlagsJSON      string    `json:"feature_flags_json"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type OrgSettingsPublic struct {
	OrgID            string `json:"org_id"`
	LLMName          string `json:"llm_name"`
	LLMBaseURL       string `json:"llm_base_url"`
	LLMModel         string `json:"llm_model"`
	LLMEnableTools   bool   `json:"llm_enable_tools"`
	LLMContextWindow *int   `json:"llm_context_window"`
	APIKeySet        bool   `json:"api_key_set"`
	APIKeyHint       string `json:"api_key_hint,omitempty"`
	FeatureFlagsJSON string `json:"feature_flags_json"`
	UpdatedAt        string `json:"updated_at"`
}

func (s *OrgSettings) Public() OrgSettingsPublic {
	hint := ""
	set := s.LLMAPIKey != ""
	if set {
		if len(s.LLMAPIKey) <= 4 {
			hint = "****"
		} else {
			hint = "****" + s.LLMAPIKey[len(s.LLMAPIKey)-4:]
		}
	}
	return OrgSettingsPublic{
		OrgID:            s.OrgID,
		LLMName:          s.LLMName,
		LLMBaseURL:       s.LLMBaseURL,
		LLMModel:         s.LLMModel,
		LLMEnableTools:   s.LLMEnableTools,
		LLMContextWindow: s.LLMContextWindow,
		APIKeySet:        set,
		APIKeyHint:       hint,
		FeatureFlagsJSON: s.FeatureFlagsJSON,
		UpdatedAt:        FormatTime(s.UpdatedAt),
	}
}

type OrgMember struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	OrgID     string    `json:"org_id"`
	CreatedAt time.Time `json:"created_at"`
}

type OrgInvite struct {
	ID              string    `json:"id"`
	OrgID           string    `json:"org_id"`
	UsernameOrEmail string    `json:"username_or_email"`
	Role            string    `json:"role"`
	InvitedBy       string    `json:"invited_by"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
}

type OrgUsage struct {
	OrgID            string `json:"org_id"`
	MemberCount      int    `json:"member_count"`
	ConversationCount int   `json:"conversation_count"`
	MessageCount     int    `json:"message_count"`
	AgentCount       int    `json:"agent_count"`
}

func NormalizeRole(role string) string {
	switch strings.TrimSpace(strings.ToLower(role)) {
	case RolePlatformAdmin:
		return RolePlatformAdmin
	case RoleOrgAdmin:
		return RoleOrgAdmin
	default:
		return RoleMember
	}
}

func RoleAtLeast(role string, min string) bool {
	rank := map[string]int{
		RoleMember:        1,
		RoleOrgAdmin:      2,
		RolePlatformAdmin: 3,
	}
	return rank[NormalizeRole(role)] >= rank[NormalizeRole(min)]
}

func (d *DB) migrateOrgs() error {
	_, err := d.SQL.Exec(`
CREATE TABLE IF NOT EXISTS orgs (
  id TEXT PRIMARY KEY,
  slug TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE users ADD COLUMN IF NOT EXISTS org_id TEXT REFERENCES orgs(id);
ALTER TABLE users ADD COLUMN IF NOT EXISTS role TEXT NOT NULL DEFAULT 'member';
ALTER TABLE users ADD COLUMN IF NOT EXISTS email TEXT NOT NULL DEFAULT '';
ALTER TABLE users ADD COLUMN IF NOT EXISTS casdoor_sub TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_users_casdoor_sub
  ON users(casdoor_sub) WHERE casdoor_sub IS NOT NULL AND casdoor_sub <> '';

CREATE TABLE IF NOT EXISTS org_settings (
  org_id TEXT PRIMARY KEY REFERENCES orgs(id) ON DELETE CASCADE,
  llm_name TEXT NOT NULL DEFAULT '',
  llm_base_url TEXT NOT NULL DEFAULT '',
  llm_api_key TEXT NOT NULL DEFAULT '',
  llm_model TEXT NOT NULL DEFAULT '',
  llm_enable_tools BOOLEAN NOT NULL DEFAULT FALSE,
  llm_context_window INT,
  feature_flags_json TEXT NOT NULL DEFAULT '{}',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS org_invites (
  id TEXT PRIMARY KEY,
  org_id TEXT NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  username_or_email TEXT NOT NULL,
  role TEXT NOT NULL DEFAULT 'member',
  invited_by TEXT REFERENCES users(id) ON DELETE SET NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_org_invites_org ON org_invites(org_id, status);
CREATE INDEX IF NOT EXISTS idx_org_invites_target ON org_invites(LOWER(username_or_email));
CREATE INDEX IF NOT EXISTS idx_users_org ON users(org_id);
`)
	if err != nil {
		return err
	}
	if _, err := d.EnsureDefaultOrg(); err != nil {
		return err
	}
	return d.backfillUsersOrg()
}

func (d *DB) EnsureDefaultOrg() (*Org, error) {
	if o, err := d.GetOrgBySlug(DefaultOrgSlug); err == nil {
		_ = d.EnsureOrgSettings(o.ID)
		return o, nil
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	o := &Org{
		ID:        uuid.NewString(),
		Slug:      DefaultOrgSlug,
		Name:      DefaultOrgName,
		CreatedAt: Now(),
	}
	_, err := d.SQL.Exec(
		`INSERT INTO orgs (id, slug, name, created_at) VALUES ($1, $2, $3, $4)`,
		o.ID, o.Slug, o.Name, o.CreatedAt,
	)
	if err != nil {
		if o2, err2 := d.GetOrgBySlug(DefaultOrgSlug); err2 == nil {
			_ = d.EnsureOrgSettings(o2.ID)
			return o2, nil
		}
		return nil, err
	}
	_ = d.EnsureOrgSettings(o.ID)
	return o, nil
}

func (d *DB) backfillUsersOrg() error {
	org, err := d.EnsureDefaultOrg()
	if err != nil {
		return err
	}
	_, err = d.SQL.Exec(`
UPDATE users SET org_id = $1
WHERE org_id IS NULL AND LOWER(username) <> LOWER($2)
`, org.ID, A2ASystemUsername)
	if err != nil {
		return err
	}
	// Do not auto-promote anyone to platform_admin. Existing roles are kept.
	// Ops admins are bootstrapped via BOOTSTRAP_ADMIN_* env (see BootstrapPlatformAdmin).
	_, err = d.SQL.Exec(`
UPDATE users SET role = $1
WHERE (role IS NULL OR role = '') AND LOWER(username) <> LOWER($2)
`, RoleMember, A2ASystemUsername)
	return err
}

func (d *DB) GetOrgByID(id string) (*Org, error) {
	row := d.SQL.QueryRow(`SELECT id, slug, name, created_at FROM orgs WHERE id = $1`, id)
	var o Org
	if err := row.Scan(&o.ID, &o.Slug, &o.Name, &o.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &o, nil
}

func (d *DB) GetOrgBySlug(slug string) (*Org, error) {
	row := d.SQL.QueryRow(`SELECT id, slug, name, created_at FROM orgs WHERE slug = $1`, strings.TrimSpace(slug))
	var o Org
	if err := row.Scan(&o.ID, &o.Slug, &o.Name, &o.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &o, nil
}

func (d *DB) EnsureOrgSettings(orgID string) error {
	_, err := d.SQL.Exec(`
INSERT INTO org_settings (org_id, updated_at) VALUES ($1, $2)
ON CONFLICT (org_id) DO NOTHING
`, orgID, Now())
	return err
}

func (d *DB) GetOrgSettings(orgID string) (*OrgSettings, error) {
	if err := d.EnsureOrgSettings(orgID); err != nil {
		return nil, err
	}
	row := d.SQL.QueryRow(`
SELECT org_id, llm_name, llm_base_url, llm_api_key, llm_model, llm_enable_tools,
       llm_context_window, feature_flags_json, updated_at
FROM org_settings WHERE org_id = $1
`, orgID)
	var s OrgSettings
	var cw sql.NullInt64
	if err := row.Scan(
		&s.OrgID, &s.LLMName, &s.LLMBaseURL, &s.LLMAPIKey, &s.LLMModel, &s.LLMEnableTools,
		&cw, &s.FeatureFlagsJSON, &s.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if cw.Valid && cw.Int64 > 0 {
		v := int(cw.Int64)
		s.LLMContextWindow = &v
	}
	return &s, nil
}

func (d *DB) UpsertOrgLLM(orgID, name, baseURL, apiKey, model string, enableTools bool, contextWindow *int, keepAPIKey bool) (*OrgSettings, error) {
	cur, err := d.GetOrgSettings(orgID)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "组织默认连接"
	}
	key := apiKey
	if keepAPIKey || strings.TrimSpace(apiKey) == "" {
		key = cur.LLMAPIKey
	}
	var cw any
	if contextWindow != nil && *contextWindow > 0 {
		cw = *contextWindow
	} else {
		cw = nil
	}
	now := Now()
	_, err = d.SQL.Exec(`
UPDATE org_settings SET
  llm_name = $2, llm_base_url = $3, llm_api_key = $4, llm_model = $5,
  llm_enable_tools = $6, llm_context_window = $7, updated_at = $8
WHERE org_id = $1
`, orgID, name, strings.TrimSpace(baseURL), key, strings.TrimSpace(model), enableTools, cw, now)
	if err != nil {
		return nil, err
	}
	return d.GetOrgSettings(orgID)
}

func (d *DB) SetOrgFeatureFlags(orgID, flagsJSON string) (*OrgSettings, error) {
	if strings.TrimSpace(flagsJSON) == "" {
		flagsJSON = "{}"
	}
	_, err := d.SQL.Exec(`
UPDATE org_settings SET feature_flags_json = $2, updated_at = $3 WHERE org_id = $1
`, orgID, flagsJSON, Now())
	if err != nil {
		return nil, err
	}
	return d.GetOrgSettings(orgID)
}

func (d *DB) ListOrgMembers(orgID string) ([]OrgMember, error) {
	rows, err := d.SQL.Query(`
SELECT id, username, COALESCE(email, ''), role, COALESCE(org_id, ''), created_at
FROM users
WHERE org_id = $1 AND LOWER(username) <> LOWER($2)
ORDER BY created_at ASC
`, orgID, A2ASystemUsername)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]OrgMember, 0)
	for rows.Next() {
		var m OrgMember
		if err := rows.Scan(&m.ID, &m.Username, &m.Email, &m.Role, &m.OrgID, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (d *DB) SetUserOrgRole(userID, orgID, role string) error {
	role = NormalizeRole(role)
	res, err := d.SQL.Exec(`
UPDATE users SET org_id = $2, role = $3 WHERE id = $1 AND LOWER(username) <> LOWER($4)
`, userID, orgID, role, A2ASystemUsername)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (d *DB) CreateOrgInvite(orgID, usernameOrEmail, role, invitedBy string) (*OrgInvite, error) {
	usernameOrEmail = strings.TrimSpace(usernameOrEmail)
	if usernameOrEmail == "" {
		return nil, errors.New("username_or_email required")
	}
	inv := &OrgInvite{
		ID:              uuid.NewString(),
		OrgID:           orgID,
		UsernameOrEmail: usernameOrEmail,
		Role:            NormalizeRole(role),
		InvitedBy:       invitedBy,
		Status:          "pending",
		CreatedAt:       Now(),
	}
	_, err := d.SQL.Exec(`
INSERT INTO org_invites (id, org_id, username_or_email, role, invited_by, status, created_at)
VALUES ($1, $2, $3, $4, $5, $6, $7)
`, inv.ID, inv.OrgID, inv.UsernameOrEmail, inv.Role, nullIfEmpty(inv.InvitedBy), inv.Status, inv.CreatedAt)
	if err != nil {
		return nil, err
	}
	return inv, nil
}

func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

func (d *DB) ListOrgInvites(orgID string) ([]OrgInvite, error) {
	rows, err := d.SQL.Query(`
SELECT id, org_id, username_or_email, role, COALESCE(invited_by, ''), status, created_at
FROM org_invites WHERE org_id = $1 AND status = 'pending'
ORDER BY created_at DESC
`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]OrgInvite, 0)
	for rows.Next() {
		var inv OrgInvite
		if err := rows.Scan(&inv.ID, &inv.OrgID, &inv.UsernameOrEmail, &inv.Role, &inv.InvitedBy, &inv.Status, &inv.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, inv)
	}
	return out, rows.Err()
}

// ApplyPendingInviteIfAny assigns org/role when a pending invite matches username or email.
func (d *DB) ApplyPendingInviteIfAny(u *User) error {
	if u == nil {
		return nil
	}
	row := d.SQL.QueryRow(`
SELECT id, org_id, role FROM org_invites
WHERE status = 'pending' AND (
  LOWER(username_or_email) = LOWER($1) OR
  ($2 <> '' AND LOWER(username_or_email) = LOWER($2))
)
ORDER BY created_at ASC
LIMIT 1
`, u.Username, u.Email)
	var invID, orgID, role string
	if err := row.Scan(&invID, &orgID, &role); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return err
	}
	if err := d.SetUserOrgRole(u.ID, orgID, role); err != nil {
		return err
	}
	_, err := d.SQL.Exec(`UPDATE org_invites SET status = 'accepted' WHERE id = $1`, invID)
	if err != nil {
		return err
	}
	u.OrgID = orgID
	u.Role = NormalizeRole(role)
	return nil
}

func (d *DB) AssignNewUserToOrg(u *User) error {
	if u == nil {
		return nil
	}
	if err := d.ApplyPendingInviteIfAny(u); err != nil {
		return err
	}
	if u.OrgID != "" {
		return nil
	}
	org, err := d.EnsureDefaultOrg()
	if err != nil {
		return err
	}
	// New registrants are always members (chat end-users). Ops admins come from
	// BOOTSTRAP_ADMIN_* or an explicit invite/role grant as org_admin/platform_admin.
	role := RoleMember
	if err := d.SetUserOrgRole(u.ID, org.ID, role); err != nil {
		return err
	}
	u.OrgID = org.ID
	u.Role = role
	return nil
}

// IsAdminRole reports whether role may access the business admin console.
func IsAdminRole(role string) bool {
	return RoleAtLeast(role, RoleOrgAdmin)
}

// HasPlatformAdmin reports whether any non-system platform_admin exists.
func (d *DB) HasPlatformAdmin() (bool, error) {
	var n int
	err := d.SQL.QueryRow(`
SELECT COUNT(*) FROM users
WHERE role = $1 AND LOWER(username) <> LOWER($2)
`, RolePlatformAdmin, A2ASystemUsername).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// BootstrapPlatformAdmin creates or promotes a platform_admin in the default org
// when none exists yet. passwordHash is required when creating a new user.
// Returns (user, created, error). No-op (nil, false, nil) when a platform_admin already exists.
func (d *DB) BootstrapPlatformAdmin(username, passwordHash, email string) (*User, bool, error) {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(email)
	if username == "" {
		return nil, false, errors.New("bootstrap admin username required")
	}
	has, err := d.HasPlatformAdmin()
	if err != nil {
		return nil, false, err
	}
	if has {
		return nil, false, nil
	}
	org, err := d.EnsureDefaultOrg()
	if err != nil {
		return nil, false, err
	}
	if existing, err := d.GetUserByUsername(username); err == nil {
		if err := d.SetUserOrgRole(existing.ID, org.ID, RolePlatformAdmin); err != nil {
			return nil, false, err
		}
		if email != "" {
			_ = d.LinkCasdoorSub(existing.ID, existing.CasdoorSub, email)
		}
		u, err := d.GetUserByID(existing.ID)
		return u, false, err
	} else if !errors.Is(err, ErrNotFound) {
		return nil, false, err
	}
	if strings.TrimSpace(passwordHash) == "" {
		return nil, false, errors.New("bootstrap admin password hash required")
	}
	u, err := d.CreateUserFull(username, passwordHash, email, "", RoleMember)
	if err != nil {
		return nil, false, err
	}
	if err := d.SetUserOrgRole(u.ID, org.ID, RolePlatformAdmin); err != nil {
		return nil, false, err
	}
	u.OrgID = org.ID
	u.Role = RolePlatformAdmin
	return u, true, nil
}

func (d *DB) GetOrgUsage(orgID string) (*OrgUsage, error) {
	u := &OrgUsage{OrgID: orgID}
	_ = d.SQL.QueryRow(`
SELECT COUNT(*) FROM users WHERE org_id = $1 AND LOWER(username) <> LOWER($2)
`, orgID, A2ASystemUsername).Scan(&u.MemberCount)
	_ = d.SQL.QueryRow(`
SELECT COUNT(*) FROM conversations c
JOIN users u ON u.id = c.user_id
WHERE u.org_id = $1
`, orgID).Scan(&u.ConversationCount)
	_ = d.SQL.QueryRow(`
SELECT COUNT(*) FROM messages m
JOIN conversations c ON c.id = m.conversation_id
JOIN users u ON u.id = c.user_id
WHERE u.org_id = $1
`, orgID).Scan(&u.MessageCount)
	_ = d.SQL.QueryRow(`
SELECT COUNT(*) FROM agents a
JOIN users u ON u.id = a.user_id
WHERE u.org_id = $1
`, orgID).Scan(&u.AgentCount)
	return u, nil
}
