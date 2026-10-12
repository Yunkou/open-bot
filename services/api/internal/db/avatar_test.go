package db

import "testing"

func TestAvatarWhitelist(t *testing.T) {
	for _, s := range []string{"circle", "square", "hexagon", "rectangle"} {
		if !IsAllowedAvatarShape(s) {
			t.Fatalf("shape %q should be allowed", s)
		}
	}
	for _, s := range []string{
		"cloud", "bean", "drop", "soft-hex", "petal", "puff",
		"rounded", "soft", "soft-square", "squircle", "diamond", "hex", "star",
	} {
		if IsAllowedAvatarShape(s) {
			t.Fatalf("legacy/unknown shape %q must not be in whitelist", s)
		}
	}
	want := map[string]string{
		"circle": "circle", // already library-four; Canonical only
		"rounded": "square", "soft": "square", "soft-square": "square",
		"squircle": "square", "diamond": "square",
		"hex": "hexagon",
		"cloud": "circle", "bean": "square", "drop": "square",
		"soft-hex": "hexagon", "petal": "square", "puff": "square",
	}
	for old, neu := range want {
		got := CanonicalAvatarShape(old)
		if got != neu {
			t.Fatalf("canonical %s → %s, got %s", old, neu, got)
		}
		if old == "circle" {
			continue // not in legacy map; whitelist passthrough
		}
		if MapLegacyAvatarShape(old) != neu {
			t.Fatalf("legacy %s → %s, got %s", old, neu, MapLegacyAvatarShape(old))
		}
	}
	if CanonicalAvatarShape("rounded") != "square" || CanonicalAvatarShape("hexagon") != "hexagon" {
		t.Fatal("CanonicalAvatarShape mismatch")
	}
	if CanonicalAvatarShape("star") != "" {
		t.Fatal("unknown shape must not canonicalize")
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
