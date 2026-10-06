package db

import "testing"

func TestCheckSkillFrontmatterSingleLine(t *testing.T) {
	ok := []SkillFileRecord{{Path: "SKILL.md", Content: "---\nname: a\ndescription: one line\n---\n"}}
	if err := checkSkillFrontmatterSingleLine(ok); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	for _, bad := range []string{
		"---\nname: a\ndescription: >-\n  x\n---\n",
		"---\nname: a\ndescription: |\n  x\n---\n",
		"no frontmatter",
		"---\nname: a\ndescription: x\n",
	} {
		if err := checkSkillFrontmatterSingleLine([]SkillFileRecord{{Path: "SKILL.md", Content: bad}}); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}

func TestKeepFileAllowedInPackage(t *testing.T) {
	files := []SkillFileRecord{
		{Path: "SKILL.md", Content: "---\nname: a\ndescription: d\n---\n"},
		{Path: "scripts/.keep", Content: ""},
		{Path: "refs/.hidden", Content: "x"},
	}
	out, err := NormalizeSkillPackagePaths(files)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, f := range out {
		got[f.Path] = true
	}
	if !got["scripts/.keep"] || got["refs/.hidden"] {
		t.Fatalf("unexpected paths: %v", got)
	}
}
