package db

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Skills admin (settings「技能」page): list with source / updated_at / bot usage,
// read any visible package (built-in read-only), whole-package save for custom skills.

const (
	SkillSourceBuiltin = "builtin"
	SkillSourceCustom  = "custom"
)

// SkillBotRef is a Bot (agent) of the same user whose effective skill set includes the skill.
type SkillBotRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SkillListItem extends UserSkill with the fields the settings list needs.
type SkillListItem struct {
	UserSkill
	Source    string        `json:"source"`               // builtin | custom
	ReadOnly  bool          `json:"read_only"`            // built-in skills are read-only for users
	UpdatedAt string        `json:"updated_at,omitempty"` // RFC3339 (FormatTime)
	BotCount  int           `json:"bot_count"`
	Bots      []SkillBotRef `json:"bots"`
}

// SkillPackageView is the editor payload: metadata + every package file.
type SkillPackageView struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Enabled     bool              `json:"enabled"`
	Custom      bool              `json:"custom"`
	Source      string            `json:"source"`
	ReadOnly    bool              `json:"read_only"`
	UpdatedAt   string            `json:"updated_at,omitempty"`
	FileCount   int               `json:"file_count"`
	Files       []SkillFileRecord `json:"files"`
}

func (d *DB) skillUpdatedAtMaps(userID string) (map[string]time.Time, map[string]time.Time, error) {
	global := map[string]time.Time{}
	custom := map[string]time.Time{}
	rows, err := d.SQL.Query(`SELECT name, updated_at FROM global_skills`)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var n string
		var t time.Time
		if err := rows.Scan(&n, &t); err != nil {
			rows.Close()
			return nil, nil, err
		}
		global[n] = t
	}
	rows.Close()
	rows, err = d.SQL.Query(`SELECT skill_name, updated_at FROM user_skill_files WHERE user_id = $1`, userID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var n string
		var t time.Time
		if err := rows.Scan(&n, &t); err != nil {
			return nil, nil, err
		}
		custom[n] = t
	}
	return global, custom, rows.Err()
}

// skillBotUsage maps skill name → bots (same user, not deleted) whose effective set includes it.
// Effective set mirrors ListEnabledSkillNamesForAgent: no agent_skills rows → all account-enabled.
func (d *DB) skillBotUsage(userID string, list []UserSkill) (map[string][]SkillBotRef, error) {
	rows, err := d.SQL.Query(
		`SELECT id, name FROM agents WHERE user_id = $1 AND deleted_at IS NULL ORDER BY created_at ASC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	var bots []SkillBotRef
	for rows.Next() {
		var b SkillBotRef
		if err := rows.Scan(&b.ID, &b.Name); err != nil {
			rows.Close()
			return nil, err
		}
		bots = append(bots, b)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := map[string][]SkillBotRef{}
	if len(bots) == 0 {
		return out, nil
	}
	// agent_id → (skill → enabled)
	allow := map[string]map[string]bool{}
	rows, err = d.SQL.Query(
		`SELECT s.agent_id, s.skill_name, s.enabled
		   FROM agent_skills s
		   JOIN agents a ON a.id = s.agent_id
		  WHERE a.user_id = $1 AND a.deleted_at IS NULL`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var aid, n string
		var en bool
		if err := rows.Scan(&aid, &n, &en); err != nil {
			return nil, err
		}
		m := allow[aid]
		if m == nil {
			m = map[string]bool{}
			allow[aid] = m
		}
		m[n] = en
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, s := range list {
		if !s.Enabled {
			continue
		}
		for _, b := range bots {
			m, has := allow[b.ID]
			if !has || m[s.Name] {
				out[s.Name] = append(out[s.Name], b)
			}
		}
	}
	return out, nil
}

// ListUserSkillsDetailed powers GET /v1/skills (settings list). Same rows as ListUserSkills.
func (d *DB) ListUserSkillsDetailed(userID string) ([]SkillListItem, error) {
	list, err := d.ListUserSkills(userID)
	if err != nil {
		return nil, err
	}
	gAt, cAt, err := d.skillUpdatedAtMaps(userID)
	if err != nil {
		return nil, err
	}
	usage, err := d.skillBotUsage(userID, list)
	if err != nil {
		return nil, err
	}
	out := make([]SkillListItem, 0, len(list))
	for _, s := range list {
		it := SkillListItem{UserSkill: s, Source: SkillSourceBuiltin, ReadOnly: true}
		if s.Custom {
			it.Source = SkillSourceCustom
			it.ReadOnly = false
			if t, ok := cAt[s.Name]; ok {
				it.UpdatedAt = FormatTime(t)
			}
		} else if t, ok := gAt[s.Name]; ok {
			it.UpdatedAt = FormatTime(t)
		}
		it.Bots = usage[s.Name]
		if it.Bots == nil {
			it.Bots = []SkillBotRef{}
		}
		it.BotCount = len(it.Bots)
		out = append(out, it)
	}
	return out, nil
}

// GetSkillPackageView returns a custom (editable) or built-in (read-only) skill package
// visible to userID. Custom lookup is scoped to the user's own rows.
func (d *DB) GetSkillPackageView(userID, name string) (*SkillPackageView, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrNotFound
	}
	meta, found, err := d.findSkillMeta(userID, name)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrNotFound
	}
	en := true
	_ = d.SQL.QueryRow(
		`SELECT enabled FROM user_skills WHERE user_id = $1 AND skill_name = $2`,
		userID, name,
	).Scan(&en)
	v := &SkillPackageView{Name: meta.Name, Description: meta.Description, Enabled: en, Custom: meta.Custom}
	if meta.Custom {
		v.Source = SkillSourceCustom
		files, err := d.ListUserSkillPackageFiles(userID, name)
		if err != nil {
			return nil, err
		}
		v.Files = files
		var t time.Time
		if err := d.SQL.QueryRow(
			`SELECT updated_at FROM user_skill_files WHERE user_id = $1 AND skill_name = $2`,
			userID, name,
		).Scan(&t); err == nil {
			v.UpdatedAt = FormatTime(t)
		}
	} else {
		v.Source = SkillSourceBuiltin
		v.ReadOnly = true
		g, err := d.GetGlobalSkill(name)
		switch {
		case err == nil:
			v.Files = g.Files
			v.UpdatedAt = g.UpdatedAt
		case errors.Is(err, ErrNotFound) && meta.Path != "":
			// Disk-only (not yet seeded) fallback.
			files, derr := ReadSkillPackageFromDisk(filepath.Dir(meta.Path))
			if derr != nil {
				return nil, derr
			}
			v.Files = files
		default:
			return nil, err
		}
	}
	if v.Files == nil {
		v.Files = []SkillFileRecord{}
	}
	sort.Slice(v.Files, func(i, j int) bool { return v.Files[i].Path < v.Files[j].Path })
	v.FileCount = len(v.Files)
	return v, nil
}

// CreateUserSkill creates a new custom skill; fails if the name exists (custom or built-in).
// files may be empty → SKILL.md template from name/description.
func (d *DB) CreateUserSkill(userID, name, description string, files []SkillFileRecord) (*SkillPackageView, error) {
	name = strings.TrimSpace(strings.ToLower(name))
	if !ValidSkillName(name) {
		return nil, fmt.Errorf("invalid skill name: use lowercase letters, digits, hyphens (e.g. my-skill)")
	}
	if _, found, err := d.findSkillMeta(userID, name); err != nil {
		return nil, err
	} else if found {
		return nil, fmt.Errorf("skill name %q already exists (conflicts)", name)
	}
	if strings.TrimSpace(description) == "" {
		description = "自定义技能：" + name
	}
	if len(files) == 0 {
		body := "# " + name + "\n\n## 何时使用\n\n- \n\n## 步骤\n\n1. \n"
		files = []SkillFileRecord{{Path: "SKILL.md", Content: BuildSkillMarkdown(name, description, body)}}
	}
	if err := checkSkillFrontmatterSingleLine(files); err != nil {
		return nil, err
	}
	gotName, _, _, err := ValidateSkillPackage(files, name, description)
	if err != nil {
		return nil, err
	}
	if gotName != name {
		return nil, fmt.Errorf("SKILL.md frontmatter name %q must equal %q", gotName, name)
	}
	if _, err := d.UploadUserSkillPackage(userID, files, name, description); err != nil {
		return nil, err
	}
	return d.GetSkillPackageView(userID, name)
}

// SaveUserSkillPackage replaces every file of an existing custom skill owned by userID
// (whole-package write-back from the editor). Frontmatter name must stay equal to name;
// account-level enabled flag is preserved.
func (d *DB) SaveUserSkillPackage(userID, name string, files []SkillFileRecord) (*SkillPackageView, error) {
	name = strings.TrimSpace(name)
	meta, found, err := d.findSkillMeta(userID, name)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrNotFound
	}
	if !meta.Custom {
		return nil, ErrSkillReadOnly
	}
	// Reject paths that the normalizer would silently rewrite (e.g. shared top-level dir).
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
	gotName, desc, normalized, err := ValidateSkillPackage(files, name, "")
	if err != nil {
		return nil, err
	}
	if gotName != name {
		return nil, fmt.Errorf("SKILL.md frontmatter name %q must equal %q (rename = create a new skill)", gotName, name)
	}
	// ValidateSkillPackage rebuilds SKILL.md frontmatter (name/description only). For editor
	// saves keep the author's exact text (extra frontmatter keys, formatting) when it is valid.
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
	tx, err := d.SQL.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.Exec(
		`UPDATE user_skill_files SET description = $3, body_markdown = $4, updated_at = NOW()
		  WHERE user_id = $1 AND skill_name = $2`,
		userID, name, desc, skillMD,
	)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	if _, err := tx.Exec(
		`DELETE FROM user_skill_package_files WHERE user_id = $1 AND skill_name = $2`,
		userID, name,
	); err != nil {
		return nil, err
	}
	for _, f := range normalized {
		if _, err := tx.Exec(
			`INSERT INTO user_skill_package_files (user_id, skill_name, path, content, updated_at)
			 VALUES ($1, $2, $3, $4, NOW())`,
			userID, name, f.Path, f.Content,
		); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return d.GetSkillPackageView(userID, name)
}

// ExportUserSkillZip zips a custom skill package owned by userID.
func (d *DB) ExportUserSkillZip(userID, name string) ([]byte, string, error) {
	v, err := d.GetSkillPackageView(userID, name)
	if err != nil {
		return nil, "", err
	}
	if !v.Custom {
		return nil, "", ErrSkillReadOnly
	}
	data, err := BuildZipSkillPackage(v.Name, v.Files)
	if err != nil {
		return nil, "", err
	}
	return data, v.Name + ".zip", nil
}

// ErrSkillReadOnly: built-in skills cannot be edited/exported by users.
var ErrSkillReadOnly = errors.New("built-in skill is read-only")

// checkSkillFrontmatterSingleLine rejects YAML block scalars for name/description
// (the runtime parser only reads single-line `key: value`).
func checkSkillFrontmatterSingleLine(files []SkillFileRecord) error {
	for _, f := range files {
		if filepath.ToSlash(strings.TrimSpace(f.Path)) != "SKILL.md" {
			continue
		}
		text := strings.TrimSpace(f.Content)
		if !strings.HasPrefix(text, "---") {
			return fmt.Errorf("SKILL.md must start with --- frontmatter (name / description)")
		}
		parts := strings.SplitN(strings.TrimPrefix(text, "---"), "\n---", 2)
		if len(parts) < 2 {
			return fmt.Errorf("SKILL.md frontmatter is not closed with ---")
		}
		for _, line := range strings.Split(parts[0], "\n") {
			k, v, ok := strings.Cut(strings.TrimSpace(line), ":")
			if !ok {
				continue
			}
			k = strings.TrimSpace(k)
			if k != "name" && k != "description" {
				continue
			}
			switch strings.TrimSpace(v) {
			case "", ">", ">-", ">+", "|", "|-", "|+":
				return fmt.Errorf("SKILL.md frontmatter %s must be a non-empty single line", k)
			}
		}
	}
	return nil
}
