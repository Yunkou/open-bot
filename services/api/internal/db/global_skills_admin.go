package db

import (
	"fmt"
	"path/filepath"
	"strings"
)

// GlobalSkillListItem powers GET /v1/admin/skills (management list).
type GlobalSkillListItem struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Enabled     bool          `json:"enabled"`
	Source      string        `json:"source"` // builtin | custom
	ReadOnly    bool          `json:"read_only"`
	CreatedAt   string        `json:"created_at,omitempty"`
	UpdatedAt   string        `json:"updated_at,omitempty"`
	BotCount    int           `json:"bot_count"`
	Bots        []SkillBotRef `json:"bots"`
}

func diskBuiltinSkillNames() map[string]bool {
	out := map[string]bool{}
	disk, err := ListGlobalSkillsFromDisk()
	if err != nil {
		return out
	}
	for _, g := range disk {
		out[g.Name] = true
	}
	return out
}

// globalSkillBotUsage maps platform skill → org bots (not deleted) whose effective set includes it.
func (d *DB) globalSkillBotUsage(orgID string, skillNames []string) (map[string][]SkillBotRef, error) {
	out := map[string][]SkillBotRef{}
	if len(skillNames) == 0 {
		return out, nil
	}
	rows, err := d.SQL.Query(
		`SELECT a.id, a.name, a.user_id
		   FROM agents a
		   JOIN users u ON u.id = a.user_id
		  WHERE u.org_id = $1 AND a.deleted_at IS NULL
		  ORDER BY a.created_at ASC`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	type botRow struct {
		SkillBotRef
		UserID string
	}
	var bots []botRow
	for rows.Next() {
		var b botRow
		if err := rows.Scan(&b.ID, &b.Name, &b.UserID); err != nil {
			rows.Close()
			return nil, err
		}
		bots = append(bots, b)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(bots) == 0 {
		return out, nil
	}

	allow := map[string]map[string]bool{}
	rows, err = d.SQL.Query(
		`SELECT s.agent_id, s.skill_name, s.enabled
		   FROM agent_skills s
		   JOIN agents a ON a.id = s.agent_id
		   JOIN users u ON u.id = a.user_id
		  WHERE u.org_id = $1 AND a.deleted_at IS NULL`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var aid, n string
		var en bool
		if err := rows.Scan(&aid, &n, &en); err != nil {
			rows.Close()
			return nil, err
		}
		m := allow[aid]
		if m == nil {
			m = map[string]bool{}
			allow[aid] = m
		}
		m[n] = en
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	userDisabled := map[string]map[string]bool{}
	rows, err = d.SQL.Query(
		`SELECT us.user_id, us.skill_name
		   FROM user_skills us
		   JOIN users u ON u.id = us.user_id
		  WHERE u.org_id = $1 AND us.enabled = FALSE`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var uid, n string
		if err := rows.Scan(&uid, &n); err != nil {
			rows.Close()
			return nil, err
		}
		m := userDisabled[uid]
		if m == nil {
			m = map[string]bool{}
			userDisabled[uid] = m
		}
		m[n] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	nameSet := map[string]bool{}
	for _, n := range skillNames {
		nameSet[n] = true
	}

	for _, b := range bots {
		disabled := userDisabled[b.UserID]
		m, hasRows := allow[b.ID]
		for n := range nameSet {
			if disabled[n] {
				continue
			}
			if !hasRows {
				out[n] = append(out[n], b.SkillBotRef)
				continue
			}
			if en, ok := m[n]; ok && en {
				out[n] = append(out[n], b.SkillBotRef)
			}
		}
	}
	return out, nil
}

// ListGlobalSkillsDetailed returns platform skills with source / bot usage for the admin list.
func (d *DB) ListGlobalSkillsDetailed(orgID string, includeDisabled bool) ([]GlobalSkillListItem, error) {
	list, err := d.ListGlobalSkillsFromDB(includeDisabled)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(list))
	for _, sk := range list {
		names = append(names, sk.Name)
	}
	builtin := diskBuiltinSkillNames()
	usage, err := d.globalSkillBotUsage(orgID, names)
	if err != nil {
		return nil, err
	}
	out := make([]GlobalSkillListItem, 0, len(list))
	for _, sk := range list {
		it := GlobalSkillListItem{
			Name:        sk.Name,
			Description: sk.Description,
			Enabled:     sk.Enabled,
			Source:      SkillSourceCustom,
			ReadOnly:    false,
			CreatedAt:   sk.CreatedAt,
			UpdatedAt:   sk.UpdatedAt,
			Bots:        usage[sk.Name],
		}
		if builtin[sk.Name] {
			it.Source = SkillSourceBuiltin
		}
		if it.Bots == nil {
			it.Bots = []SkillBotRef{}
		}
		it.BotCount = len(it.Bots)
		out = append(out, it)
	}
	return out, nil
}

// SaveGlobalSkillPackage replaces all files of an existing platform skill (keeps enabled).
func (d *DB) SaveGlobalSkillPackage(name string, files []SkillFileRecord) (*GlobalSkillRecord, error) {
	name = strings.TrimSpace(strings.ToLower(name))
	if !ValidSkillName(name) {
		return nil, fmt.Errorf("invalid skill name")
	}
	existing, err := d.GetGlobalSkill(name)
	if err != nil {
		return nil, err
	}
	for _, f := range files {
		p := filepath.ToSlash(strings.TrimSpace(f.Path))
		if !ValidSkillRelPath(p) {
			return nil, fmt.Errorf("invalid path in package: %q", f.Path)
		}
	}
	if err := checkSkillFrontmatterSingleLine(files); err != nil {
		return nil, err
	}
	for _, f := range files {
		if filepath.ToSlash(strings.TrimSpace(f.Path)) == "SKILL.md" {
			if n, _ := parseSkillFrontmatter(f.Content, ""); strings.TrimSpace(n) == "" {
				return nil, fmt.Errorf("SKILL.md frontmatter name is required")
			}
		}
	}
	gotName, desc, normalized, err := ValidateSkillPackage(files, name, existing.Description)
	if err != nil {
		return nil, err
	}
	if gotName != name {
		return nil, fmt.Errorf("SKILL.md frontmatter name %q must equal %q (rename = create a new skill)", gotName, name)
	}
	var skillMD string
	for _, f := range files {
		if filepath.ToSlash(strings.TrimSpace(f.Path)) == "SKILL.md" {
			if n, dsc := parseSkillFrontmatter(f.Content, ""); strings.TrimSpace(n) == name && strings.TrimSpace(dsc) != "" {
				skillMD = f.Content
			}
		}
	}
	for i := range normalized {
		if normalized[i].Path == "SKILL.md" {
			if skillMD != "" {
				normalized[i].Content = skillMD
			} else {
				skillMD = normalized[i].Content
			}
		}
	}
	if desc == "" {
		desc = existing.Description
	}

	tx, err := d.SQL.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Exec(
		`UPDATE global_skills SET description = $2, body_markdown = $3, updated_at = NOW() WHERE name = $1`,
		name, desc, skillMD,
	)
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(`DELETE FROM global_skill_files WHERE skill_name = $1`, name)
	if err != nil {
		return nil, err
	}
	for _, f := range normalized {
		path := filepath.ToSlash(strings.TrimSpace(f.Path))
		if _, err := tx.Exec(
			`INSERT INTO global_skill_files (skill_name, path, content, updated_at)
			 VALUES ($1, $2, $3, NOW())
			 ON CONFLICT (skill_name, path) DO UPDATE SET content = EXCLUDED.content, updated_at = NOW()`,
			name, path, f.Content,
		); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return d.GetGlobalSkill(name)
}

// AdminListBotSkills returns the owner's agent skill toggles for a bot.
func (d *DB) AdminListBotSkills(ownerUserID, agentID string) ([]AgentSkill, error) {
	return d.ListAgentSkills(ownerUserID, agentID)
}

// AdminSetBotSkill toggles one skill on a bot.
func (d *DB) AdminSetBotSkill(ownerUserID, agentID, name string, enabled bool) (*AgentSkill, error) {
	return d.SetAgentSkillEnabled(ownerUserID, agentID, name, enabled)
}
