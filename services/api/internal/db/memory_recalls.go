package db

import (
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	maxRecallItems        = 32
	maxRecallSnippetChars = 500
	maxRecallItemsJSON    = 48 * 1024
)

// MemoryRecallFilter narrows org-scoped per-run recall listings.
type MemoryRecallFilter struct {
	UserID           string
	ConversationID   string
	AgentID          string
	RunID            string
	LangfuseTraceID  string
	From             time.Time
	To               time.Time
	Limit            int
}

// MemoryRecallItem is one recalled snippet persisted for admin review.
type MemoryRecallItem struct {
	Source      string   `json:"source"`
	Scope       string   `json:"scope,omitempty"`
	Tier        string   `json:"tier,omitempty"`
	Content     string   `json:"content,omitempty"`
	Snippet     string   `json:"snippet,omitempty"`
	MemoryID    string   `json:"memory_id,omitempty"`
	AgentID     string   `json:"agent_id,omitempty"`
	ChannelID   string   `json:"channel_id,omitempty"`
	PeerAgentID string   `json:"peer_agent_id,omitempty"`
	Score       *float64 `json:"score,omitempty"`
}

// MemoryRecall is one chat/runtime turn's recall payload.
type MemoryRecall struct {
	ID              string
	OrgID           string
	UserID          string
	Username        string
	AgentID         string
	AgentName       string
	ConversationID  string
	MessageID       string
	RunID           string
	LangfuseTraceID string
	Source          string
	Scene           string
	ExplicitCount   int
	Mem0Count       int
	ItemCount       int
	Items           []MemoryRecallItem
	CreatedAt       time.Time
}

func (d *DB) migrateMemoryRecalls() error {
	_, err := d.SQL.Exec(`
CREATE TABLE IF NOT EXISTS memory_recalls (
  id TEXT PRIMARY KEY,
  org_id TEXT NOT NULL DEFAULT '',
  user_id TEXT NOT NULL DEFAULT '',
  agent_id TEXT NOT NULL DEFAULT '',
  conversation_id TEXT NOT NULL DEFAULT '',
  message_id TEXT NOT NULL DEFAULT '',
  run_id TEXT NOT NULL DEFAULT '',
  langfuse_trace_id TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT 'chat',
  scene TEXT NOT NULL DEFAULT '',
  explicit_count INT NOT NULL DEFAULT 0,
  mem0_count INT NOT NULL DEFAULT 0,
  item_count INT NOT NULL DEFAULT 0,
  items_json JSONB NOT NULL DEFAULT '[]'::jsonb,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE memory_recalls ADD COLUMN IF NOT EXISTS langfuse_trace_id TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_memory_recalls_org_created
  ON memory_recalls(org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_memory_recalls_user_created
  ON memory_recalls(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_memory_recalls_conv_created
  ON memory_recalls(conversation_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_memory_recalls_run
  ON memory_recalls(run_id) WHERE run_id <> '';
CREATE INDEX IF NOT EXISTS idx_memory_recalls_lf_trace
  ON memory_recalls(langfuse_trace_id) WHERE langfuse_trace_id <> '';
`)
	return err
}

func clipRecallText(s string, limit int) string {
	s = strings.TrimSpace(s)
	if limit <= 0 || len(s) <= limit {
		return s
	}
	if limit == 1 {
		return "…"
	}
	return s[:limit-1] + "…"
}

func normalizeRecallItems(raw []MemoryRecallItem) []MemoryRecallItem {
	if len(raw) == 0 {
		return []MemoryRecallItem{}
	}
	out := make([]MemoryRecallItem, 0, len(raw))
	for _, item := range raw {
		src := strings.TrimSpace(item.Source)
		if src == "" {
			src = "explicit"
		}
		if src != "explicit" && src != "mem0" {
			src = "explicit"
		}
		content := clipRecallText(item.Content, maxRecallSnippetChars)
		snippet := clipRecallText(item.Snippet, maxRecallSnippetChars)
		if content == "" && snippet == "" {
			continue
		}
		out = append(out, MemoryRecallItem{
			Source:      src,
			Scope:       strings.TrimSpace(item.Scope),
			Tier:        strings.TrimSpace(item.Tier),
			Content:     content,
			Snippet:     snippet,
			MemoryID:    strings.TrimSpace(item.MemoryID),
			AgentID:     strings.TrimSpace(item.AgentID),
			ChannelID:   strings.TrimSpace(item.ChannelID),
			PeerAgentID: strings.TrimSpace(item.PeerAgentID),
			Score:       item.Score,
		})
		if len(out) >= maxRecallItems {
			break
		}
	}
	return out
}

// InsertMemoryRecall stores one per-run recall payload. Empty user_id is a no-op.
func (d *DB) InsertMemoryRecall(
	orgID, userID, agentID, conversationID, messageID, runID, langfuseTraceID, source, scene string,
	explicitCount, mem0Count int,
	items []MemoryRecallItem,
) (*MemoryRecall, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, nil
	}
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		if u, err := d.GetUserByID(userID); err == nil {
			orgID = u.OrgID
		}
	}
	agentID = strings.TrimSpace(agentID)
	conversationID = strings.TrimSpace(conversationID)
	messageID = strings.TrimSpace(messageID)
	runID = strings.TrimSpace(runID)
	langfuseTraceID = strings.TrimSpace(langfuseTraceID)
	source = strings.TrimSpace(source)
	if source == "" {
		source = "chat"
	}
	scene = strings.TrimSpace(scene)
	items = normalizeRecallItems(items)
	if explicitCount < 0 {
		explicitCount = 0
	}
	if mem0Count < 0 {
		mem0Count = 0
	}
	itemCount := len(items)
	raw, err := json.Marshal(items)
	if err != nil {
		return nil, err
	}
	if len(raw) > maxRecallItemsJSON {
		// Drop trailing items until under cap.
		for len(items) > 1 && len(raw) > maxRecallItemsJSON {
			items = items[:len(items)-1]
			raw, err = json.Marshal(items)
			if err != nil {
				return nil, err
			}
		}
		itemCount = len(items)
	}
	id := uuid.NewString()
	now := Now()
	_, err = d.SQL.Exec(`
INSERT INTO memory_recalls (
  id, org_id, user_id, agent_id, conversation_id, message_id, run_id, langfuse_trace_id, source, scene,
  explicit_count, mem0_count, item_count, items_json, created_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14::jsonb,$15)
`, id, orgID, userID, agentID, conversationID, messageID, runID, langfuseTraceID, source, scene,
		explicitCount, mem0Count, itemCount, string(raw), now)
	if err != nil {
		return nil, err
	}
	return &MemoryRecall{
		ID:             id,
		OrgID:          orgID,
		UserID:         userID,
		AgentID:        agentID,
		ConversationID:  conversationID,
		MessageID:       messageID,
		RunID:           runID,
		LangfuseTraceID: langfuseTraceID,
		Source:          source,
		Scene:          scene,
		ExplicitCount:  explicitCount,
		Mem0Count:      mem0Count,
		ItemCount:      itemCount,
		Items:          items,
		CreatedAt:      now,
	}, nil
}

func scanMemoryRecall(scanner interface {
	Scan(dest ...any) error
}, withNames bool) (*MemoryRecall, error) {
	var (
		r           MemoryRecall
		itemsRaw    []byte
		username    sql.NullString
		agentName   sql.NullString
	)
	dest := []any{
		&r.ID, &r.OrgID, &r.UserID, &r.AgentID, &r.ConversationID, &r.MessageID, &r.RunID, &r.LangfuseTraceID,
		&r.Source, &r.Scene, &r.ExplicitCount, &r.Mem0Count, &r.ItemCount, &itemsRaw, &r.CreatedAt,
	}
	if withNames {
		dest = append(dest, &username, &agentName)
	}
	if err := scanner.Scan(dest...); err != nil {
		return nil, err
	}
	if withNames {
		r.Username = username.String
		r.AgentName = agentName.String
	}
	r.Items = []MemoryRecallItem{}
	if len(itemsRaw) > 0 {
		_ = json.Unmarshal(itemsRaw, &r.Items)
	}
	return &r, nil
}

func clampRecallLimit(n int) int {
	if n <= 0 {
		return 50
	}
	if n > 200 {
		return 200
	}
	return n
}

// ListOrgMemoryRecalls returns recent per-run recalls for an org.
func (d *DB) ListOrgMemoryRecalls(orgID string, f MemoryRecallFilter) ([]MemoryRecall, error) {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return nil, nil
	}
	limit := clampRecallLimit(f.Limit)
	args := []any{orgID}
	q := `
SELECT r.id, r.org_id, r.user_id, r.agent_id, r.conversation_id, r.message_id, r.run_id, r.langfuse_trace_id,
       r.source, r.scene, r.explicit_count, r.mem0_count, r.item_count, r.items_json, r.created_at,
       COALESCE(u.username, ''), COALESCE(a.name, r.agent_id)
FROM memory_recalls r
LEFT JOIN users u ON u.id = r.user_id
LEFT JOIN agents a ON a.id = r.agent_id AND a.deleted_at IS NULL
WHERE r.org_id = $1`
	if uid := strings.TrimSpace(f.UserID); uid != "" {
		args = append(args, uid)
		q += ` AND r.user_id = $` + strconv.Itoa(len(args))
	}
	if cid := strings.TrimSpace(f.ConversationID); cid != "" {
		args = append(args, cid)
		q += ` AND r.conversation_id = $` + strconv.Itoa(len(args))
	}
	if aid := strings.TrimSpace(f.AgentID); aid != "" {
		args = append(args, aid)
		q += ` AND r.agent_id = $` + strconv.Itoa(len(args))
	}
	if rid := strings.TrimSpace(f.RunID); rid != "" {
		args = append(args, rid)
		q += ` AND r.run_id = $` + strconv.Itoa(len(args))
	}
	if tid := strings.TrimSpace(f.LangfuseTraceID); tid != "" {
		args = append(args, tid)
		q += ` AND r.langfuse_trace_id = $` + strconv.Itoa(len(args))
	}
	if !f.From.IsZero() {
		args = append(args, f.From.UTC())
		q += ` AND r.created_at >= $` + strconv.Itoa(len(args))
	}
	if !f.To.IsZero() {
		args = append(args, f.To.UTC())
		q += ` AND r.created_at <= $` + strconv.Itoa(len(args))
	}
	args = append(args, limit)
	q += ` ORDER BY r.created_at DESC LIMIT $` + strconv.Itoa(len(args))

	rows, err := d.SQL.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MemoryRecall
	for rows.Next() {
		item, err := scanMemoryRecall(rows, true)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

// GetOrgMemoryRecall returns one recall if it belongs to the org.
func (d *DB) GetOrgMemoryRecall(orgID, id string) (*MemoryRecall, error) {
	orgID = strings.TrimSpace(orgID)
	id = strings.TrimSpace(id)
	if orgID == "" || id == "" {
		return nil, ErrNotFound
	}
	row := d.SQL.QueryRow(`
SELECT r.id, r.org_id, r.user_id, r.agent_id, r.conversation_id, r.message_id, r.run_id, r.langfuse_trace_id,
       r.source, r.scene, r.explicit_count, r.mem0_count, r.item_count, r.items_json, r.created_at,
       COALESCE(u.username, ''), COALESCE(a.name, r.agent_id)
FROM memory_recalls r
LEFT JOIN users u ON u.id = r.user_id
LEFT JOIN agents a ON a.id = r.agent_id AND a.deleted_at IS NULL
WHERE r.org_id = $1 AND r.id = $2
`, orgID, id)
	item, err := scanMemoryRecall(row, true)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return item, nil
}

