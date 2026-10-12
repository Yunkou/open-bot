package db

import "testing"

func TestParseChannelSettingsJSONMigratesBool(t *testing.T) {
	s := parseChannelSettingsJSON(`{"auto_pick":true,"auto_pick_k":2,"show_reply_candidates":true}`)
	if s.AutoPick != AutoPickHeuristic {
		t.Fatalf("true → heuristic, got %q", s.AutoPick)
	}
	if s.AutoPickK != 2 || !s.ShowReplyCandidates {
		t.Fatalf("other fields: %#v", s)
	}
	s = parseChannelSettingsJSON(`{"auto_pick":false}`)
	if s.AutoPick != AutoPickOff {
		t.Fatalf("false → off, got %q", s.AutoPick)
	}
	s = parseChannelSettingsJSON(`{"auto_pick":"decision"}`)
	if s.AutoPick != AutoPickDecision {
		t.Fatalf("string decision, got %q", s.AutoPick)
	}
	s = parseChannelSettingsJSON(`{}`)
	if s.AutoPick != "" {
		t.Fatalf("omit stays empty (inherit), got %q", s.AutoPick)
	}
	s = parseChannelSettingsJSON(`{"auto_pick":"heuristic","auto_pick_k":1}`)
	if s.JSON() == "{}" || s.AutoPick != AutoPickHeuristic {
		t.Fatalf("round-trip marshal: %s", s.JSON())
	}
}

func TestEffectiveAutoPickMode(t *testing.T) {
	s := ChannelSettings{}
	if s.EffectiveAutoPickMode("") != AutoPickHeuristic {
		t.Fatal("default heuristic")
	}
	// Channel settings.auto_pick must NOT override org feature flags.
	s.AutoPick = AutoPickOff
	if s.EffectiveAutoPickMode(AutoPickDecision) != AutoPickDecision {
		t.Fatal("org wins; channel override ignored")
	}
	s.AutoPick = AutoPickDecision
	if s.EffectiveAutoPickMode(AutoPickOff) != AutoPickOff {
		t.Fatal("org off wins even if channel says decision")
	}
}

func TestParseAutoPickMode(t *testing.T) {
	got, err := ParseAutoPickMode("decision")
	if err != nil || got != AutoPickDecision {
		t.Fatal(got, err)
	}
	got, err = ParseAutoPickMode(true)
	if err != nil || got != AutoPickHeuristic {
		t.Fatal(got, err)
	}
	got, err = ParseAutoPickMode(false)
	if err != nil || got != AutoPickOff {
		t.Fatal(got, err)
	}
	if _, err := ParseAutoPickMode("weird"); err == nil {
		t.Fatal("expected error")
	}
}
