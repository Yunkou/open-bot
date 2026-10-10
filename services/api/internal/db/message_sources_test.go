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
	soft := FilterMessageSourcesCited(sources, "I used docs.example.com without a raw link.", nil)
	if len(soft) != 1 || soft[0].URL != "https://docs.example.com/guide" {
		t.Fatalf("soft host=%+v", soft)
	}
	// Paraphrase with no cites: still show curated retrieval sources.
	fb := FilterMessageSourcesCited(sources, "I summarized without links.", nil)
	if len(fb) == 0 {
		t.Fatalf("expected paraphrase fallback sources")
	}
	noisy := []MessageSource{
		{URL: "https://docs.example.com/guide", Title: "Docs"},
		{URL: "https://api.example.com/v1/x"},
		{URL: "https://news.example.com/rss.xml"},
	}
	fb2 := FilterMessageSourcesCited(noisy, "paraphrased answer", nil)
	if len(fb2) != 1 || fb2[0].URL != "https://docs.example.com/guide" {
		t.Fatalf("fallback should drop api/rss: %+v", fb2)
	}
	if FilterMessageSourcesCited(sources, "", nil) != nil {
		t.Fatalf("expected nil when answer empty")
	}
	// structured citations without inline URLs
	got2 := FilterMessageSourcesCited(sources, "summary only", []MessageSource{{URL: "https://noise.example.com/x"}})
	if len(got2) != 1 || got2[0].URL != "https://noise.example.com/x" {
		t.Fatalf("structured=%+v", got2)
	}
}
