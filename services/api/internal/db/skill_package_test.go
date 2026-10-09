package db

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestValidateSkillPackage_ok(t *testing.T) {
	files := []SkillFileRecord{
		{Path: "my-helper/SKILL.md", Content: "---\nname: my-helper\ndescription: Helps with tea\n---\n\n# Hi\n"},
		{Path: "my-helper/references/a.md", Content: "details"},
	}
	name, desc, out, err := ValidateSkillPackage(files, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if name != "my-helper" || desc == "" {
		t.Fatalf("got name=%q desc=%q", name, desc)
	}
	if len(out) != 2 {
		t.Fatalf("files=%d", len(out))
	}
	found := false
	for _, f := range out {
		if f.Path == "SKILL.md" {
			found = true
		}
		if f.Path == "my-helper/SKILL.md" {
			t.Fatal("prefix not stripped")
		}
	}
	if !found {
		t.Fatal("missing SKILL.md")
	}
}

func TestValidateSkillPackage_missingSkillMD(t *testing.T) {
	_, _, _, err := ValidateSkillPackage([]SkillFileRecord{
		{Path: "references/a.md", Content: "x"},
	}, "my-helper", "desc")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateSkillPackage_pathTraversal(t *testing.T) {
	_, _, _, err := ValidateSkillPackage([]SkillFileRecord{
		{Path: "SKILL.md", Content: "---\nname: x\ndescription: y\n---\n"},
		{Path: "../evil.md", Content: "no"},
	}, "", "")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseZipSkillPackage(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("tea-timer/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("---\nname: tea-timer\ndescription: Timer\n---\n\nbody\n"))
	w2, err := zw.Create("tea-timer/scripts/note.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w2.Write([]byte("echo hi"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := ParseZipSkillPackage(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	name, _, out, err := ValidateSkillPackage(raw, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if name != "tea-timer" || len(out) != 2 {
		t.Fatalf("name=%q files=%d", name, len(out))
	}

	zipped, err := BuildZipSkillPackage(name, out)
	if err != nil {
		t.Fatal(err)
	}
	again, err := ParseZipSkillPackage(zipped)
	if err != nil {
		t.Fatal(err)
	}
	name2, _, out2, err := ValidateSkillPackage(again, "", "")
	if err != nil || name2 != name || len(out2) != 2 {
		t.Fatalf("roundtrip name=%q files=%d err=%v", name2, len(out2), err)
	}
}
