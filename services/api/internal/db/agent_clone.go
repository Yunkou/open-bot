package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// CloneAgentOptions controls what a bot copy takes from its source.
//
// Always forked (independent copy on the new agent_id):
//   - name (default "<原名> 副本"), description, system_prompt (persona), computer_mode
//   - per-bot skill allowlist (agent_skills rows; none → new bot also inherits all account-enabled skills)
//
// Account-level, therefore naturally shared (nothing to copy):
//   - user-scope memory, account skill library / custom skills, LLM connections,
//     MCP servers, registered machines, user settings, team-mode workspace
//
// Optional extras (callers set defaults: Web / clone_agent default CopyMemory true;
// FollowUp always forces CopyMemory in httpserver):
//   - CopyMemory: explicit bot-scope memories (memories.scope='bot') are duplicated onto the new agent.
//     Mem0 auto-memories tagged with the old agent are NOT copied.
//   - CopyRoutines: routines bound to the source are duplicated **paused** (enabled=false) so cron/
//     event triggers never fire twice; pinned conversation is re-pointed to the new bot's thread.
//
// Never copied: conversations / messages / threads, channel (group) memberships, agent_pair memories,
// bot secrets (per-bot credentials must be re-granted), private-mode workspace files, background tasks,
// usage stats, agent-bus messages, avatar_shape/color (freshly assigned under the same per-user lock).
type CloneAgentOptions struct {
	Name         string
	Description  *string
	SystemPrompt *string
	ComputerMode string
	CopyMemory   bool
	CopyRoutines bool
}

type CloneAgentResult struct {
	Agent          *Agent   `json:"agent"`
	SourceID       string   `json:"source_agent_id"`
	SkillsCopied   int      `json:"skills_copied"`
	SkillsInherit  bool     `json:"skills_inherit_account"`
	MemoriesCopied int      `json:"memories_copied"`
	RoutinesCopied int      `json:"routines_copied"`
	RoutineNames   []string `json:"routine_names,omitempty"`
	ConversationID string   `json:"conversation_id,omitempty"`
}

// DefaultCloneName returns "<name> 副本", then "<name> 副本 2", … avoiding live names of this user.
func DefaultCloneName(base string, existing []string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "助手"
	}
	taken := map[string]bool{}
	for _, n := range existing {
		taken[strings.TrimSpace(n)] = true
	}
	cand := base + " 副本"
	if !taken[cand] {
		return cand
	}
	for i := 2; i < 1000; i++ {
		c := fmt.Sprintf("%s 副本 %d", base, i)
		if !taken[c] {
			return c
		}
	}
	return cand + " " + uuid.NewString()[:4]
}

// CloneAgent copies a bot owned by userID into a new bot owned by the same user
// (same org/tenant by construction). See CloneAgentOptions for shared vs forked fields.
func (d *DB) CloneAgent(userID, sourceID string, opts CloneAgentOptions) (*CloneAgentResult, error) {
	userID = strings.TrimSpace(userID)
	sourceID = strings.TrimSpace(sourceID)
	if userID == "" || sourceID == "" {
		return nil, errors.New("user_id and source agent id required")
	}
	src, err := d.GetAgent(userID, sourceID)
	if err != nil {
		return nil, err
	}

	name := strings.TrimSpace(opts.Name)
	if name == "" {
		list, lerr := d.ListAgents(userID)
		if lerr != nil {
			return nil, lerr
		}
		names := make([]string, 0, len(list))
		for _, a := range list {
			names = append(names, a.Name)
		}
		name = DefaultCloneName(src.Name, names)
	}
	if len([]rune(name)) > 80 {
		name = string([]rune(name)[:80])
	}
	desc := src.Description
	if opts.Description != nil {
		desc = strings.TrimSpace(*opts.Description)
	}
	sp := src.SystemPrompt
	if opts.SystemPrompt != nil {
		sp = strings.TrimSpace(*opts.SystemPrompt)
	}
	mode := strings.TrimSpace(opts.ComputerMode)
	if mode == "" {
		mode = src.ComputerMode
	}
	if mode != "private" {
		mode = "team"
	}

	now := Now()
	a := &Agent{
		ID:           uuid.NewString(),
		UserID:       userID,
		Name:         name,
		Description:  desc,
		SystemPrompt: sp,
		IsBuiltin:    false,
		ComputerMode: mode,
		MachineID:    strings.TrimSpace(src.MachineID),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	res := &CloneAgentResult{Agent: a, SourceID: src.ID}

	tx, err := d.SQL.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	shape, color, err := assignAvatarUnderLock(tx, userID, a.ID, "", "")
	if err != nil {
		return nil, err
	}
	a.AvatarShape, a.AvatarColor, a.AvatarUserSet = shape, color, false

	if _, err := tx.Exec(
		`INSERT INTO agents (id, user_id, name, description, system_prompt, is_builtin, computer_mode, avatar_shape, avatar_color, avatar_user_set, machine_id, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,FALSE,$6,$7,$8,FALSE,$9,$10,$11)`,
		a.ID, a.UserID, a.Name, a.Description, a.SystemPrompt, a.ComputerMode, a.AvatarShape, a.AvatarColor, a.MachineID, a.CreatedAt, a.UpdatedAt,
	); err != nil {
		return nil, err
	}

	// Skills allowlist: exact copy of rows (enabled + disabled) so effective set matches the source.
	r, err := tx.Exec(
		`INSERT INTO agent_skills (agent_id, skill_name, enabled)
		 SELECT $1, skill_name, enabled FROM agent_skills WHERE agent_id = $2
		 ON CONFLICT (agent_id, skill_name) DO NOTHING`,
		a.ID, src.ID,
	)
	if err != nil {
		return nil, err
	}
	if n, _ := r.RowsAffected(); n > 0 {
		res.SkillsCopied = int(n)
	} else {
		res.SkillsInherit = true
	}

	if opts.CopyMemory {
		n, err := cloneBotMemories(tx, d.memoriesHaveEmbedding(), userID, src.ID, a.ID)
		if err != nil {
			return nil, err
		}
		res.MemoriesCopied = n
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	// Primary thread for the new bot (empty history — chats are never copied).
	if conv, cerr := d.GetOrCreatePrimaryConversation(userID, a.ID); cerr == nil && conv != nil {
		res.ConversationID = conv.ID
	}

	if opts.CopyRoutines {
		names, rerr := d.cloneRoutinesPaused(userID, src.ID, a.ID, res.ConversationID)
		if rerr != nil {
			return res, fmt.Errorf("bot copied but routines failed: %w", rerr)
		}
		res.RoutinesCopied = len(names)
		res.RoutineNames = names
	}
	return res, nil
}

func (d *DB) memoriesHaveEmbedding() bool {
	var ok bool
	err := d.SQL.QueryRow(
		`SELECT EXISTS (SELECT 1 FROM information_schema.columns
		  WHERE table_name = 'memories' AND column_name = 'embedding')`,
	).Scan(&ok)
	return err == nil && ok
}

func cloneBotMemories(tx *sql.Tx, withEmbedding bool, userID, srcID, dstID string) (int, error) {
	rows, err := tx.Query(
		`SELECT id FROM memories WHERE user_id = $1 AND scope = 'bot' AND agent_id = $2 ORDER BY created_at ASC`,
		userID, srcID,
	)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	cols := `user_id, tier, content, tags, scope, channel_id, peer_agent_id, created_at`
	if withEmbedding {
		cols += `, embedding`
	}
	q := `INSERT INTO memories (id, agent_id, updated_at, ` + cols + `)
	      SELECT $1, $2, NOW(), ` + cols + ` FROM memories WHERE id = $3 AND user_id = $4`
	for _, id := range ids {
		if _, err := tx.Exec(q, uuid.NewString(), dstID, id, userID); err != nil {
			return 0, err
		}
	}
	return len(ids), nil
}

func (d *DB) cloneRoutinesPaused(userID, srcID, dstID, dstConvID string) ([]string, error) {
	list, err := d.ListRoutines(userID)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, rt := range list {
		if rt.AgentID != srcID {
			continue
		}
		convID := ""
		if strings.TrimSpace(rt.ConversationID) != "" {
			convID = dstConvID
		}
		created, err := d.CreateRoutine(
			userID, rt.Name, rt.Prompt, rt.ScheduleCron, false, dstID,
			rt.Timezone, convID, rt.TriggersJSON, rt.MaxRetries, rt.QuietUnchanged,
		)
		if err != nil {
			return names, err
		}
		names = append(names, created.Name)
	}
	return names, nil
}
