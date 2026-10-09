package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// AutoReviewRuleAction values for user NL rules.
const (
	AutoReviewAskFirst  = "ask_first"
	AutoReviewAutoAllow = "auto_allow"
)

// AutoReviewRule is a per-user natural-language Auto-review preference.
// Matching is simple keyword/intent against tool op + command preview + reason —
// not the chat LLM granting itself permission.
type AutoReviewRule struct {
	ID     string `json:"id"`
	When   string `json:"when"`   // "当 bot 想要:" NL text
	Action string `json:"action"` // ask_first | auto_allow
}

// UserSettings holds per-user Bot preferences (timezone + Auto-review).
type UserSettings struct {
	UserID            string           `json:"user_id"`
	Timezone          string           `json:"timezone"` // IANA; empty = auto-detect on client
	AutoReviewEnabled bool             `json:"auto_review_enabled"`
	AutoReviewRules   []AutoReviewRule `json:"auto_review_rules"`
	UpdatedAt         time.Time        `json:"updated_at"`
}

func DefaultUserSettings(userID string) UserSettings {
	return UserSettings{
		UserID:            userID,
		Timezone:          "",
		AutoReviewEnabled: true,
		AutoReviewRules:   []AutoReviewRule{},
		UpdatedAt:         Now(),
	}
}

func (d *DB) migrateUserSettings() error {
	_, err := d.SQL.Exec(`
CREATE TABLE IF NOT EXISTS user_settings (
  user_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  timezone TEXT NOT NULL DEFAULT '',
  auto_review_enabled BOOLEAN NOT NULL DEFAULT TRUE,
  auto_review_rules_json TEXT NOT NULL DEFAULT '[]',
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE user_machines ADD COLUMN IF NOT EXISTS exec_policy TEXT NOT NULL DEFAULT 'allow';
`)
	return err
}

func (d *DB) GetUserSettings(userID string) (UserSettings, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return UserSettings{}, errors.New("user_id required")
	}
	row := d.SQL.QueryRow(`
SELECT user_id, timezone, auto_review_enabled, auto_review_rules_json, updated_at
FROM user_settings WHERE user_id = $1`, userID)
	var s UserSettings
	var rulesRaw string
	err := row.Scan(&s.UserID, &s.Timezone, &s.AutoReviewEnabled, &rulesRaw, &s.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return DefaultUserSettings(userID), nil
		}
		return UserSettings{}, err
	}
	s.AutoReviewRules = parseAutoReviewRules(rulesRaw)
	return s, nil
}

type UserSettingsPatch struct {
	Timezone          *string           `json:"timezone"`
	AutoReviewEnabled *bool             `json:"auto_review_enabled"`
	AutoReviewRules   *[]AutoReviewRule `json:"auto_review_rules"`
}

func (d *DB) UpsertUserSettings(userID string, patch UserSettingsPatch) (UserSettings, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return UserSettings{}, errors.New("user_id required")
	}
	cur, err := d.GetUserSettings(userID)
	if err != nil {
		return UserSettings{}, err
	}
	if patch.Timezone != nil {
		cur.Timezone = strings.TrimSpace(*patch.Timezone)
		if len([]rune(cur.Timezone)) > 64 {
			return UserSettings{}, errors.New("timezone too long")
		}
	}
	if patch.AutoReviewEnabled != nil {
		cur.AutoReviewEnabled = *patch.AutoReviewEnabled
	}
	if patch.AutoReviewRules != nil {
		normalized, nerr := normalizeAutoReviewRules(*patch.AutoReviewRules)
		if nerr != nil {
			return UserSettings{}, nerr
		}
		cur.AutoReviewRules = normalized
	}
	cur.UserID = userID
	cur.UpdatedAt = Now()
	raw, err := json.Marshal(cur.AutoReviewRules)
	if err != nil {
		return UserSettings{}, err
	}
	_, err = d.SQL.Exec(`
INSERT INTO user_settings (user_id, timezone, auto_review_enabled, auto_review_rules_json, updated_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (user_id) DO UPDATE SET
  timezone = EXCLUDED.timezone,
  auto_review_enabled = EXCLUDED.auto_review_enabled,
  auto_review_rules_json = EXCLUDED.auto_review_rules_json,
  updated_at = EXCLUDED.updated_at`,
		userID, cur.Timezone, cur.AutoReviewEnabled, string(raw), cur.UpdatedAt,
	)
	if err != nil {
		return UserSettings{}, err
	}
	return cur, nil
}

func parseAutoReviewRules(raw string) []AutoReviewRule {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return []AutoReviewRule{}
	}
	var rules []AutoReviewRule
	if err := json.Unmarshal([]byte(raw), &rules); err != nil {
		return []AutoReviewRule{}
	}
	out, _ := normalizeAutoReviewRules(rules)
	return out
}

func normalizeAutoReviewRules(in []AutoReviewRule) ([]AutoReviewRule, error) {
	if len(in) > 50 {
		return nil, errors.New("too many auto-review rules (max 50)")
	}
	out := make([]AutoReviewRule, 0, len(in))
	for _, r := range in {
		when := strings.TrimSpace(r.When)
		if when == "" {
			continue
		}
		if len([]rune(when)) > 200 {
			return nil, errors.New("rule when text too long")
		}
		action := strings.TrimSpace(r.Action)
		if action != AutoReviewAskFirst && action != AutoReviewAutoAllow {
			return nil, errors.New("rule action must be ask_first or auto_allow")
		}
		id := strings.TrimSpace(r.ID)
		if id == "" {
			id = uuid.NewString()
		}
		out = append(out, AutoReviewRule{ID: id, When: when, Action: action})
	}
	if out == nil {
		out = []AutoReviewRule{}
	}
	return out, nil
}

// MachineExecPolicy values for per-machine host permission.
const (
	MachineExecAllow = "allow" // 始终允许：不弹确认卡；硬拒绝仍失败；先询问仍确认
	MachineExecAsk   = "ask"   // 每次询问
	MachineExecDeny  = "deny"  // 不允许
)

func NormalizeMachineExecPolicy(v string) string {
	switch strings.TrimSpace(strings.ToLower(v)) {
	case MachineExecAsk:
		return MachineExecAsk
	case MachineExecDeny:
		return MachineExecDeny
	default:
		return MachineExecAllow
	}
}
