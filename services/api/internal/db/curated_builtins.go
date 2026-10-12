package db

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// CuratedBuiltin is one skills.sh skill that is default-on for new bots.
type CuratedBuiltin struct {
	Slug          string `json:"slug"`
	Package       string `json:"package"`
	Skill         string `json:"skill"`
	Repo          string `json:"repo"`
	Path          string `json:"path"`
	Ref           string `json:"ref"`
	PreserveLocal bool   `json:"preserve_local"`
}

type curatedManifest struct {
	Version    int              `json:"version"`
	Skills     []CuratedBuiltin `json:"skills"`
	NotDefault []string         `json:"not_default"`
}

var (
	curatedOnce  sync.Once
	curatedSlugs []string
	curatedSet   map[string]bool
)

func curatedManifestPath() string {
	return filepath.Join(SkillsRoot(), "curated-builtins.json")
}

func loadCuratedManifest() {
	curatedOnce.Do(func() {
		curatedSet = map[string]bool{}
		data, err := os.ReadFile(curatedManifestPath())
		if err != nil {
			return
		}
		var m curatedManifest
		if err := json.Unmarshal(data, &m); err != nil {
			return
		}
		for _, s := range m.Skills {
			slug := strings.TrimSpace(s.Slug)
			if slug == "" {
				continue
			}
			curatedSlugs = append(curatedSlugs, slug)
			curatedSet[slug] = true
		}
	})
}

// CuratedBuiltinSlugs returns the ordered curated skills.sh default-on list.
// Empty when skills/curated-builtins.json is missing or invalid.
func CuratedBuiltinSlugs() []string {
	loadCuratedManifest()
	out := make([]string, len(curatedSlugs))
	copy(out, curatedSlugs)
	return out
}

// IsCuratedBuiltin reports whether name is in the curated skills.sh batch.
func IsCuratedBuiltin(name string) bool {
	loadCuratedManifest()
	return curatedSet[strings.TrimSpace(name)]
}

// ResetCuratedBuiltinCacheForTest clears the sync.Once cache (tests only).
func ResetCuratedBuiltinCacheForTest() {
	curatedOnce = sync.Once{}
	curatedSlugs = nil
	curatedSet = nil
}
