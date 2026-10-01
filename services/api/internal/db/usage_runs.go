package db

import (
	"strings"
	"time"
)

type UsageRunDay struct {
	Day               string `json:"day"`
	OrgID             string `json:"org_id,omitempty"`
	UserID            string `json:"user_id,omitempty"`
	Username          string `json:"username,omitempty"`
	AgentID           string `json:"agent_id,omitempty"`
	AgentName         string `json:"agent_name,omitempty"`
	RunCount          int64  `json:"run_count"`
	PromptTokens      int64  `json:"prompt_tokens"`
	CompletionTokens  int64  `json:"completion_tokens"`
	TotalTokens       int64  `json:"total_tokens"`
}

type OrgUsageDetailed struct {
	OrgID             string         `json:"org_id"`
	MemberCount       int            `json:"member_count"`
	ConversationCount int            `json:"conversation_count"`
	MessageCount      int            `json:"message_count"`
	AgentCount        int            `json:"agent_count"`
	RunCount          int64          `json:"run_count"`
	PromptTokens      int64          `json:"prompt_tokens"`
	CompletionTokens  int64          `json:"completion_tokens"`
	TotalTokens       int64          `json:"total_tokens"`
	ByDay             []UsageRunDay  `json:"by_day,omitempty"`
	ByUser            []UsageRunDay  `json:"by_user,omitempty"`
	ByBot             []UsageRunDay  `json:"by_bot,omitempty"`
}

type PlatformOrgSummary struct {
	OrgID             string `json:"org_id"`
	Slug              string `json:"slug"`
	Name              string `json:"name"`
	MemberCount       int    `json:"member_count"`
	ConversationCount int    `json:"conversation_count"`
	MessageCount      int    `json:"message_count"`
	AgentCount        int    `json:"agent_count"`
	RunCount          int64  `json:"run_count"`
	PromptTokens      int64  `json:"prompt_tokens"`
	CompletionTokens  int64  `json:"completion_tokens"`
	TotalTokens       int64  `json:"total_tokens"`
	CreatedAt         string `json:"created_at"`
}

func (d *DB) migrateUsageRuns() error {
	_, err := d.SQL.Exec(`
CREATE TABLE IF NOT EXISTS usage_runs (
  id BIGSERIAL PRIMARY KEY,
  org_id TEXT NOT NULL DEFAULT '',
  user_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  conversation_id TEXT NOT NULL DEFAULT '',
  day DATE NOT NULL DEFAULT (CURRENT_DATE),
  prompt_tokens BIGINT NOT NULL DEFAULT 0,
  completion_tokens BIGINT NOT NULL DEFAULT 0,
  total_tokens BIGINT NOT NULL DEFAULT 0,
  source TEXT NOT NULL DEFAULT 'chat',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_usage_runs_org_day ON usage_runs(org_id, day DESC);
CREATE INDEX IF NOT EXISTS idx_usage_runs_user_day ON usage_runs(user_id, day DESC);
CREATE INDEX IF NOT EXISTS idx_usage_runs_agent_day ON usage_runs(agent_id, day DESC);
`)
	return err
}

// RecordUsageRun appends one completed chat/runtime run. Tokens may be 0 when unknown.
func (d *DB) RecordUsageRun(orgID, userID, agentID, conversationID, source string, promptTokens, completionTokens, totalTokens int64) error {
	orgID = strings.TrimSpace(orgID)
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil
	}
	if orgID == "" {
		if u, err := d.GetUserByID(userID); err == nil {
			orgID = u.OrgID
		}
	}
	agentID = strings.TrimSpace(agentID)
	conversationID = strings.TrimSpace(conversationID)
	source = strings.TrimSpace(source)
	if source == "" {
		source = "chat"
	}
	if totalTokens <= 0 && (promptTokens > 0 || completionTokens > 0) {
		totalTokens = promptTokens + completionTokens
	}
	day := time.Now().In(time.Local).Format("2006-01-02")
	_, err := d.SQL.Exec(`
INSERT INTO usage_runs (
  org_id, user_id, agent_id, conversation_id, day,
  prompt_tokens, completion_tokens, total_tokens, source, created_at
) VALUES ($1,$2,$3,$4,$5::date,$6,$7,$8,$9,$10)
`, orgID, userID, agentID, conversationID, day,
		promptTokens, completionTokens, totalTokens, source, Now())
	return err
}

func (d *DB) GetOrgUsageDetailed(orgID string, days int) (*OrgUsageDetailed, error) {
	if days <= 0 || days > 366 {
		days = 30
	}
	base, err := d.GetOrgUsage(orgID)
	if err != nil {
		return nil, err
	}
	out := &OrgUsageDetailed{
		OrgID:             base.OrgID,
		MemberCount:       base.MemberCount,
		ConversationCount: base.ConversationCount,
		MessageCount:      base.MessageCount,
		AgentCount:        base.AgentCount,
	}
	_ = d.SQL.QueryRow(`
SELECT COALESCE(SUM(1),0), COALESCE(SUM(prompt_tokens),0),
       COALESCE(SUM(completion_tokens),0), COALESCE(SUM(total_tokens),0)
FROM usage_runs WHERE org_id = $1 AND day >= (CURRENT_DATE - ($2::int || ' days')::interval)
`, orgID, days).Scan(&out.RunCount, &out.PromptTokens, &out.CompletionTokens, &out.TotalTokens)

	out.ByDay, _ = d.queryUsageGrouped(`
SELECT to_char(day, 'YYYY-MM-DD'), '', '', '', '',
       COUNT(*), COALESCE(SUM(prompt_tokens),0), COALESCE(SUM(completion_tokens),0), COALESCE(SUM(total_tokens),0)
FROM usage_runs WHERE org_id = $1 AND day >= (CURRENT_DATE - ($2::int || ' days')::interval)
GROUP BY day ORDER BY day DESC
`, orgID, days)

	out.ByUser, _ = d.queryUsageGrouped(`
SELECT '', r.user_id, COALESCE(u.username, ''), '', '',
       COUNT(*), COALESCE(SUM(r.prompt_tokens),0), COALESCE(SUM(r.completion_tokens),0), COALESCE(SUM(r.total_tokens),0)
FROM usage_runs r
LEFT JOIN users u ON u.id = r.user_id
WHERE r.org_id = $1 AND r.day >= (CURRENT_DATE - ($2::int || ' days')::interval)
GROUP BY r.user_id, u.username
ORDER BY COUNT(*) DESC
LIMIT 50
`, orgID, days)

	out.ByBot, _ = d.queryUsageGrouped(`
SELECT '', '', '', r.agent_id, COALESCE(a.name, r.agent_id),
       COUNT(*), COALESCE(SUM(r.prompt_tokens),0), COALESCE(SUM(r.completion_tokens),0), COALESCE(SUM(r.total_tokens),0)
FROM usage_runs r
LEFT JOIN agents a ON a.id = r.agent_id AND a.deleted_at IS NULL
WHERE r.org_id = $1 AND r.day >= (CURRENT_DATE - ($2::int || ' days')::interval)
GROUP BY r.agent_id, a.name
ORDER BY COUNT(*) DESC
LIMIT 50
`, orgID, days)

	return out, nil
}

func (d *DB) queryUsageGrouped(q string, orgID string, days int) ([]UsageRunDay, error) {
	rows, err := d.SQL.Query(q, orgID, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UsageRunDay
	for rows.Next() {
		var r UsageRunDay
		if err := rows.Scan(
			&r.Day, &r.UserID, &r.Username, &r.AgentID, &r.AgentName,
			&r.RunCount, &r.PromptTokens, &r.CompletionTokens, &r.TotalTokens,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) ListOrgs() ([]Org, error) {
	rows, err := d.SQL.Query(`SELECT id, slug, name, created_at FROM orgs ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Org
	for rows.Next() {
		var o Org
		if err := rows.Scan(&o.ID, &o.Slug, &o.Name, &o.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (d *DB) PlatformOrgSummaries(days int) ([]PlatformOrgSummary, error) {
	if days <= 0 || days > 366 {
		days = 30
	}
	orgs, err := d.ListOrgs()
	if err != nil {
		return nil, err
	}
	out := make([]PlatformOrgSummary, 0, len(orgs))
	for _, o := range orgs {
		u, _ := d.GetOrgUsage(o.ID)
		s := PlatformOrgSummary{
			OrgID:     o.ID,
			Slug:      o.Slug,
			Name:      o.Name,
			CreatedAt: o.CreatedAt.UTC().Format(time.RFC3339),
		}
		if u != nil {
			s.MemberCount = u.MemberCount
			s.ConversationCount = u.ConversationCount
			s.MessageCount = u.MessageCount
			s.AgentCount = u.AgentCount
		}
		_ = d.SQL.QueryRow(`
SELECT COALESCE(SUM(1),0), COALESCE(SUM(prompt_tokens),0),
       COALESCE(SUM(completion_tokens),0), COALESCE(SUM(total_tokens),0)
FROM usage_runs WHERE org_id = $1 AND day >= (CURRENT_DATE - ($2::int || ' days')::interval)
`, o.ID, days).Scan(&s.RunCount, &s.PromptTokens, &s.CompletionTokens, &s.TotalTokens)
		out = append(out, s)
	}
	return out, nil
}
