package db

import "testing"

func TestNormalizeMessageSources(t *testing.T) {
	got := NormalizeMessageSources([]MessageSource{
		{URL: "https://a.example/x", Title: "A"},
		{URL: "https://a.example/x/", Title: ""},
		{URL: "ftp://nope"},
		{URL: "not-a-url"},
		{URL: "https://b.example/y"},
	})
	if len(got) != 2 {
		t.Fatalf("len=%d want 2: %+v", len(got), got)
	}
	if got[0].URL != "https://a.example/x" || got[0].Title != "A" {
		t.Fatalf("first=%+v", got[0])
	}
	raw := EncodeMessageSources(got)
	back := DecodeMessageSources(raw)
	if len(back) != 2 {
		t.Fatalf("roundtrip len=%d raw=%s", len(back), raw)
	}
}

func TestParseMessageSourcesAny(t *testing.T) {
	got := ParseMessageSourcesAny([]any{
		map[string]any{"url": "https://docs.example/guide", "title": "Docs"},
		map[string]any{"url": "https://docs.example/guide"},
	})
	if len(got) != 1 || got[0].Title != "Docs" {
		t.Fatalf("got=%+v", got)
	}
}
