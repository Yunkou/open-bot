package db

import (
	"strings"
	"testing"
)

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

func TestInjectCitationMarkers(t *testing.T) {
	sources := []MessageSource{
		{URL: "https://www.uino.com/", Title: "UINO"},
		{URL: "https://www.thingjs.com/"},
	}
	answer := "官网 uino.com 与 ThingJS 站已核对。"
	got := InjectCitationMarkers(answer, sources)
	if got == answer {
		t.Fatalf("expected markers, got unchanged %q", got)
	}
	if !strings.Contains(got, "[1]") || !strings.Contains(got, "[2]") {
		t.Fatalf("missing markers: %q", got)
	}
	// Idempotent
	again := InjectCitationMarkers(got, sources)
	if again != got {
		t.Fatalf("not idempotent: %q vs %q", again, got)
	}
}

func TestIsNoisySearchHosts(t *testing.T) {
	if !isNoisyCitationURL("https://cn.bing.com/search?q=x") {
		t.Fatal("cn.bing search should be noisy")
	}
	if !isNoisyCitationURL("https://r.bing.com/rb/x.css") {
		t.Fatal("r.bing.com should be noisy")
	}
	if isNoisyCitationURL("https://www.uino.com/") {
		t.Fatal("uino should not be noisy")
	}
}

func TestInjectCitationMarkersParentHost(t *testing.T) {
	sources := []MessageSource{
		{URL: "https://dt.uino.com/docs"},
	}
	answer := "详见 uino.com 文档说明。"
	got := InjectCitationMarkers(answer, sources)
	if !strings.Contains(got, "[1]") {
		t.Fatalf("expected [1] after parent host mention, got %q", got)
	}
}
