package db

import "testing"

func TestAvatarWhitelist(t *testing.T) {
	for _, s := range []string{"cloud", "bean", "drop", "soft-hex", "petal", "puff"} {
		if !IsAllowedAvatarShape(s) {
			t.Fatalf("shape %q should be allowed", s)
		}
	}
	for _, s := range []string{"circle", "rounded", "squircle", "hex", "diamond", "soft-square", "star"} {
		if IsAllowedAvatarShape(s) {
			t.Fatalf("legacy/unknown shape %q must not be in whitelist", s)
		}
	}
	want := map[string]string{
		"circle": "cloud", "rounded": "puff", "squircle": "bean",
		"hex": "soft-hex", "diamond": "drop", "soft-square": "petal",
	}
	for old, neu := range want {
		if MapLegacyAvatarShape(old) != neu {
			t.Fatalf("legacy %s → %s, got %s", old, neu, MapLegacyAvatarShape(old))
		}
	}
	if CanonicalAvatarShape("rounded") != "puff" || CanonicalAvatarShape("cloud") != "cloud" {
		t.Fatal("CanonicalAvatarShape mismatch")
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
