package db

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCuratedBuiltinSlugsFromRepo(t *testing.T) {
	ResetCuratedBuiltinCacheForTest()
	root := SkillsRoot()
	manifest := filepath.Join(root, "curated-builtins.json")
	if _, err := os.Stat(manifest); err != nil {
		t.Skip("curated-builtins.json not present:", err)
	}
	slugs := CuratedBuiltinSlugs()
	if len(slugs) < 10 {
		t.Fatalf("expected curated batch, got %v", slugs)
	}
	want := map[string]bool{
		"find-skills": true, "grill-me": true, "tdd": true, "diagnosing-bugs": true,
		"vercel-react-best-practices": true, "frontend-design": true,
	}
	for name := range want {
		if !IsCuratedBuiltin(name) {
			t.Fatalf("missing curated %s in %v", name, slugs)
		}
	}
	if IsCuratedBuiltin("agent-browser") || IsCuratedBuiltin("grilling") {
		t.Fatal("excluded leaderboard skills must not be curated")
	}
}
