package db

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var skillNameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

type UserSkill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	Custom      bool   `json:"custom"`
}

// SkillsRoot resolves the global skills directory (repo skills/).
func SkillsRoot() string {
	if v := strings.TrimSpace(os.Getenv("SKILLS_DIR")); v != "" {
		return v
	}
	if root := strings.TrimSpace(os.Getenv("OPEN_BOT_ROOT")); root != "" {
		return filepath.Join(root, "skills")
	}
	here, err := os.Getwd()
	if err == nil {
		dir := here
		for i := 0; i < 6; i++ {
			candidate := filepath.Join(dir, "skills")
			if st, err := os.Stat(candidate); err == nil && st.IsDir() {
				abs, _ := filepath.Abs(candidate)
				return abs
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return filepath.Clean(filepath.Join("..", "..", "..", "skills"))
}

func UserSkillsDir(userID string) string {
	safe := sanitizePathSegment(userID)
	return filepath.Join(SkillsRoot(), "users", safe)
}

func SanitizeUserSegment(s string) string { return sanitizePathSegment(s) }

func sanitizePathSegment(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		return "_unknown"
	}
	return out
}

func ValidSkillName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 64 {
		return false
	}
	return skillNameRe.MatchString(name)
}

type GlobalSkill struct {
	Name        string
	Description string
	Custom      bool
	Path        string
}

func listSkillsFromDir(root string, custom bool) ([]GlobalSkill, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []GlobalSkill
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// skip users/ bucket under global root
		if !custom && e.Name() == "users" {
			continue
		}
		skillMD := filepath.Join(root, e.Name(), "SKILL.md")
		data, err := os.ReadFile(skillMD)
		if err != nil {
			continue
		}
		name, desc := parseSkillFrontmatter(string(data), e.Name())
		if name == "" || desc == "" {
			continue
		}
		out = append(out, GlobalSkill{Name: name, Description: desc, Custom: custom, Path: skillMD})
	}
	return out, nil
}

func ListGlobalSkillsFromDisk() ([]GlobalSkill, error) {
	return listSkillsFromDir(SkillsRoot(), false)
}

func ListUserCustomSkillsFromDisk(userID string) ([]GlobalSkill, error) {
	return listSkillsFromDir(UserSkillsDir(userID), true)
}

func parseSkillFrontmatter(text, fallbackName string) (name, desc string) {
	name = fallbackName
	if !strings.HasPrefix(strings.TrimSpace(text), "---") {
		return name, ""
	}
	rest := strings.TrimSpace(text)
	rest = strings.TrimPrefix(rest, "---")
	parts := strings.SplitN(rest, "\n---", 2)
	if len(parts) < 1 {
		return name, ""
	}
	for _, line := range strings.Split(parts[0], "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, ":") {
			continue
		}
		k, v, _ := strings.Cut(line, ":")
		k = strings.TrimSpace(k)
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		switch k {
		case "name":
			if v != "" {
				name = v
			}
		case "description":
			desc = v
		}
	}
	return name, desc
}

// BuildSkillMarkdown ensures Agent Skills frontmatter (name + description).
func BuildSkillMarkdown(name, description, body string) string {
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	body = strings.TrimSpace(body)
	// strip existing frontmatter from body if present
	if strings.HasPrefix(body, "---") {
		rest := strings.TrimPrefix(body, "---")
		parts := strings.SplitN(rest, "\n---", 2)
		if len(parts) == 2 {
			body = strings.TrimSpace(parts[1])
		}
	}
	if description == "" {
		description = "User-uploaded skill: " + name
	}
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("name: ")
	b.WriteString(name)
	b.WriteString("\n")
	b.WriteString("description: ")
	b.WriteString(description)
	b.WriteString("\n")
	b.WriteString("---\n\n")
	if body != "" {
		b.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			b.WriteString("\n")
		}
	} else {
		b.WriteString("# ")
		b.WriteString(name)
		b.WriteString("\n\n")
		b.WriteString(description)
		b.WriteString("\n")
	}
	return b.String()
}

func (d *DB) ListUserSkills(userID string) ([]UserSkill, error) {
	global, err := ListGlobalSkillsFromDisk()
	if err != nil {
		return nil, err
	}
	custom, err := ListUserCustomSkillsFromDisk(userID)
	if err != nil {
		return nil, err
	}
	// custom overrides global same name
	merged := map[string]GlobalSkill{}
	order := make([]string, 0, len(global)+len(custom))
	for _, g := range global {
		merged[g.Name] = g
		order = append(order, g.Name)
	}
	for _, c := range custom {
		if _, ok := merged[c.Name]; !ok {
			order = append(order, c.Name)
		}
		merged[c.Name] = c
	}
	all := make([]GlobalSkill, 0, len(order))
	for _, n := range order {
		all = append(all, merged[n])
	}
	if err := d.SeedUserSkills(userID, all); err != nil {
		return nil, err
	}
	rows, err := d.SQL.Query(
		`SELECT skill_name, enabled FROM user_skills WHERE user_id = $1`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	enabledMap := map[string]bool{}
	for rows.Next() {
		var n string
		var en bool
		if err := rows.Scan(&n, &en); err != nil {
			return nil, err
		}
		enabledMap[n] = en
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]UserSkill, 0, len(all))
	for _, g := range all {
		en, ok := enabledMap[g.Name]
		if !ok {
			en = true
		}
		out = append(out, UserSkill{
			Name:        g.Name,
			Description: g.Description,
			Enabled:     en,
			Custom:      g.Custom,
		})
	}
	return out, nil
}

func (d *DB) SeedUserSkills(userID string, global []GlobalSkill) error {
	for _, g := range global {
		_, err := d.SQL.Exec(
			`INSERT INTO user_skills (user_id, skill_name, enabled)
			 VALUES ($1, $2, TRUE)
			 ON CONFLICT (user_id, skill_name) DO NOTHING`,
			userID, g.Name,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) findSkillMeta(userID, name string) (GlobalSkill, bool, error) {
	custom, err := ListUserCustomSkillsFromDisk(userID)
	if err != nil {
		return GlobalSkill{}, false, err
	}
	for _, c := range custom {
		if c.Name == name {
			return c, true, nil
		}
	}
	global, err := ListGlobalSkillsFromDisk()
	if err != nil {
		return GlobalSkill{}, false, err
	}
	for _, g := range global {
		if g.Name == name {
			return g, true, nil
		}
	}
	return GlobalSkill{}, false, nil
}

func (d *DB) SetUserSkillEnabled(userID, name string, enabled bool) (*UserSkill, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("skill name required")
	}
	meta, found, err := d.findSkillMeta(userID, name)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrNotFound
	}
	_, err = d.SQL.Exec(
		`INSERT INTO user_skills (user_id, skill_name, enabled)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (user_id, skill_name) DO UPDATE SET enabled = EXCLUDED.enabled`,
		userID, name, enabled,
	)
	if err != nil {
		return nil, err
	}
	return &UserSkill{Name: name, Description: meta.Description, Enabled: enabled, Custom: meta.Custom}, nil
}

func (d *DB) ListEnabledSkillNames(userID string) ([]string, error) {
	list, err := d.ListUserSkills(userID)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, s := range list {
		if s.Enabled {
			out = append(out, s.Name)
		}
	}
	return out, nil
}

func (d *DB) UploadUserSkill(userID, name, description, bodyMarkdown string) (*UserSkill, error) {
	name = strings.TrimSpace(strings.ToLower(name))
	if !ValidSkillName(name) {
		return nil, fmt.Errorf("invalid skill name: use lowercase letters, digits, hyphens (e.g. my-skill)")
	}
	// disallow overwriting global skill names
	global, err := ListGlobalSkillsFromDisk()
	if err != nil {
		return nil, err
	}
	for _, g := range global {
		if g.Name == name {
			return nil, fmt.Errorf("skill name %q conflicts with a global skill", name)
		}
	}
	md := BuildSkillMarkdown(name, description, bodyMarkdown)
	_, desc := parseSkillFrontmatter(md, name)
	dir := filepath.Join(UserSkillsDir(userID), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte(md), 0o644); err != nil {
		return nil, err
	}
	_, err = d.SQL.Exec(
		`INSERT INTO user_skill_files (user_id, skill_name, description, body_markdown, updated_at)
		 VALUES ($1, $2, $3, $4, NOW())
		 ON CONFLICT (user_id, skill_name) DO UPDATE
		   SET description = EXCLUDED.description,
		       body_markdown = EXCLUDED.body_markdown,
		       updated_at = NOW()`,
		userID, name, desc, md,
	)
	if err != nil {
		return nil, err
	}
	_, err = d.SQL.Exec(
		`INSERT INTO user_skills (user_id, skill_name, enabled)
		 VALUES ($1, $2, TRUE)
		 ON CONFLICT (user_id, skill_name) DO UPDATE SET enabled = TRUE`,
		userID, name,
	)
	if err != nil {
		return nil, err
	}
	return &UserSkill{Name: name, Description: desc, Enabled: true, Custom: true}, nil
}

func (d *DB) DeleteUserSkill(userID, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("skill name required")
	}
	meta, found, err := d.findSkillMeta(userID, name)
	if err != nil {
		return err
	}
	if !found || !meta.Custom {
		return ErrNotFound
	}
	dir := filepath.Join(UserSkillsDir(userID), name)
	_ = os.RemoveAll(dir)
	_, _ = d.SQL.Exec(`DELETE FROM user_skill_files WHERE user_id = $1 AND skill_name = $2`, userID, name)
	_, err = d.SQL.Exec(`DELETE FROM user_skills WHERE user_id = $1 AND skill_name = $2`, userID, name)
	return err
}

// EnsureUserSkillFilesTable is a no-op helper for older code paths.
func (d *DB) EnsureUserSkillFilesTable() error {
	_, err := d.SQL.Exec(`
CREATE TABLE IF NOT EXISTS user_skill_files (
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  skill_name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  body_markdown TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  PRIMARY KEY (user_id, skill_name)
)`)
	return err
}

