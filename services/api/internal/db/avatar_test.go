package db

import "testing"

func TestAvatarWhitelist(t *testing.T) {
	for _, s := range []string{"circle", "rounded", "squircle", "hex", "diamond", "soft-square"} {
		if !IsAllowedAvatarShape(s) {
			t.Fatalf("shape %q should be allowed", s)
		}
	}
	if IsAllowedAvatarShape("star") {
		t.Fatal("star must be rejected")
	}
	if len(AllowedAvatarColors) != 12 {
		t.Fatalf("palette size=%d", len(AllowedAvatarColors))
	}
	if NormalizeAvatarColor("#E85D4C") != "#e85d4c" {
		t.Fatal("color should normalize to lowercase palette entry")
	}
	if NormalizeAvatarColor("#123456") != "" {
		t.Fatal("off-palette color must be rejected")
	}
	s, c := AssignAvatarFromID("agent-x")
	if !IsAllowedAvatarShape(s) || !IsAllowedAvatarColor(c) {
		t.Fatalf("hash assign out of whitelist: %s %s", s, c)
	}
	s2, c2 := AssignAvatarAvoiding("agent-x", map[string]struct{}{s + "|" + c: {}})
	if s2+"|"+c2 == s+"|"+c {
		t.Fatal("AssignAvatarAvoiding returned a used pair")
	}
}
