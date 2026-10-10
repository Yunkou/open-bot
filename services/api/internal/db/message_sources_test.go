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

func TestFilterMessageSourcesCited(t *testing.T) {
	sources := []MessageSource{
		{URL: "https://docs.example.com/guide", Title: "Docs"},
		{URL: "https://blog.example.com/post", Title: "Blog"},
		{URL: "https://noise.example.com/x"},
	}
	answer := "See [Docs](https://docs.example.com/guide) and https://blog.example.com/post"
	got := FilterMessageSourcesCited(sources, answer, nil)
	if len(got) != 2 {
		t.Fatalf("len=%d want 2: %+v", len(got), got)
	}
	if got[0].URL != "https://docs.example.com/guide" || got[1].URL != "https://blog.example.com/post" {
		t.Fatalf("got=%+v", got)
	}
	if FilterMessageSourcesCited(sources, "no links here", nil) != nil {
		t.Fatalf("expected nil when answer cites nothing")
	}
	// structured citations without inline URLs
	got2 := FilterMessageSourcesCited(sources, "summary only", []MessageSource{{URL: "https://noise.example.com/x"}})
	if len(got2) != 1 || got2[0].URL != "https://noise.example.com/x" {
		t.Fatalf("structured=%+v", got2)
	}
}
